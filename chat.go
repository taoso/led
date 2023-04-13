package led

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/taoso/led/ecdsa"
	"github.com/taoso/led/pay"
	"github.com/taoso/led/store"
)

func (p *Proxy) chat(w http.ResponseWriter, req *http.Request, f *FileHandler) {
	user, pass, ok := req.BasicAuth()
	if !ok || !p.auth(user, pass) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	defer req.Body.Close()

	var msg struct {
		Messages []map[string]string `json:"messages"`
		Model    string              `json:"model"`
		Stream   bool                `json:"stream"`
	}

	if err := json.NewDecoder(req.Body).Decode(&msg); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	var u struct {
		Usage struct {
			ReplyTokens  int `json:"completion_tokens"`
			PromptTokens int `json:"prompt_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}

	defer func() {
		if msg.Stream {
			b, _ := json.Marshal(u)
			b = append([]byte("data: "), b...)
			b = append(b, []byte("\n\ndata: [DONE]\n\n")...)
			w.Write(b)
		}
		fmt.Printf("%+v\n", u.Usage)
	}()

	u.Usage.PromptTokens = p.BPE.CountMessage(msg.Messages)
	u.Usage.TotalTokens = u.Usage.PromptTokens + u.Usage.ReplyTokens

	b, err := json.Marshal(msg)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	url := "https://api.openai.com" + req.URL.Path[len("/+/chat"):]
	r, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	r.Header.Set("Authorization", "Bearer "+os.Getenv("CHAT_TOKEN"))
	r.Header.Set("Content-Type", req.Header.Get("Content-Type"))

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	defer resp.Body.Close()

	linkKey := user + resp.Header.Get("X-Request-Id")
	p.chatLinks.Store(linkKey, resp.Body)

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("X-Request-Id", resp.Header.Get("X-Request-Id"))
	fmt.Println(resp.Header.Get("X-Request-Id"))
	w.WriteHeader(resp.StatusCode)

	if resp.StatusCode != http.StatusOK || !msg.Stream {
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusOK {
			if err := json.Unmarshal(b, &u); err != nil {
				log.Println("unmarshal data error: ", err)
			}
		}
		w.Write(b)
		return
	}

	s := bufio.NewScanner(resp.Body)
	for s.Scan() {
		l := s.Text()
		if !strings.HasPrefix(l, "data: {") {
			continue
		}

		var data struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason,omitempty"`
				Index        int     `json:"index,omitempty"`
			} `json:"choices"`
		}
		err := json.Unmarshal([]byte(l[len("data: "):]), &data)
		if err != nil {
			log.Println("unmarshal data error: ", err)
			return
		}

		for _, c := range data.Choices {
			u.Usage.ReplyTokens += p.BPE.Count(c.Delta.Content)
			u.Usage.TotalTokens = u.Usage.PromptTokens + u.Usage.ReplyTokens
		}

		b, err := json.Marshal(data)
		if err != nil {
			log.Println("marshal data error: ", err)
			return
		}

		buf := make([]byte, len(b)+len("data: ")+len("\n\n"))
		copy(buf, []byte("data: "))
		copy(buf[len("data: "):], b)
		copy(buf[len(b)+len("data: "):], []byte("\n\n"))

		if _, err := w.Write(buf); err != nil {
			log.Println("write data error: ", err)
			return
		}
	}

	if err := s.Err(); err != nil {
		log.Println("scan err", err)
	}
}

func (p *Proxy) chatCancel(w http.ResponseWriter, req *http.Request, f *FileHandler) {
	user, pass, ok := req.BasicAuth()
	if !ok || !p.auth(user, pass) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	linkKey := user + req.FormValue("id")
	v, ok := p.chatLinks.Load(linkKey)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("link not found"))
		return
	}
	v.(io.Closer).Close()
	p.chatLinks.Delete(linkKey)
}

type alipayArgs struct {
	UserID   int       `json:"user_id"`
	TokenNum int       `json:"token_num"`
	CentNum  int       `json:"cent_num"`
	Sign     string    `json:"sign"`
	Pubkey   string    `json:"pubkey"`
	Created  time.Time `json:"created"`
}

func (p *Proxy) buyTokens(w http.ResponseWriter, req *http.Request, f *FileHandler) {
	args := alipayArgs{}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	defer req.Body.Close()
	if err := json.Unmarshal(body, &args); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	if args.CentNum < 100 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("cent_num must > 100"))
		return
	}

	if args.CentNum != args.CentNum/100*100 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("cent_num must be a multiple of 100"))
		return
	}

	if args.TokenNum != args.CentNum/25*1000 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid token_num"))
		return
	}

	if args.Created.Sub(time.Now()).Abs() > 5*time.Minute {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("client time is inaccurate"))
		return
	}

	log := store.TokenLog{
		Type:     store.LogTypeBuy,
		TokenNum: args.TokenNum,
		ExtraNum: args.CentNum,
		Created:  args.Created,
	}

	var pk ecdsa.PublicKey
	if args.UserID != 0 {
		u, err := p.TokenRepo.GetWallet(args.UserID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		} else if u.ID == 0 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("invalid user_id"))
			return
		}
		pk, err = ecdsa.GetPubkey(u.Pubkey)
	} else {
		pk, err = ecdsa.ParsePubkey(args.Pubkey)
	}
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid pubkey"))
		return
	}

	ok, err := ecdsa.VerifyES256(log.SignData(), args.Sign, pk)
	if err != nil || !ok {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid signature"))
		return
	}

	order := pay.Order{
		TradeNo:   genTradeNo(pk),
		Amount:    strconv.Itoa(args.CentNum / 100),
		Subject:   strconv.Itoa(args.TokenNum) + " tokens",
		Extra:     url.QueryEscape(string(body)),
		NotifyURL: "https://" + f.Name + "/+/buy-tokens-notify",
	}
	qr, err := p.Alipay.CreateQR(order)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"qr":"` + qr + `","ttl":900}`))
}

// genTradeNo 为当前用户生成订单号
//
// 当前毫秒时间戳再追加一部分用户公钥信息，几乎不可能冲突。
func genTradeNo(pubkey ecdsa.PublicKey) string {
	ts := time.Now().UTC().Format("20060102150405.000")
	ts = strings.Replace(ts, ".", "", 1)

	var i uint32
	p := unsafe.Pointer(&i)
	copy((*[4]byte)(p)[:], pubkey.X.Bytes()[:4])

	return ts + strconv.Itoa(int(i))
}

func (p *Proxy) buyTokensNotify(w http.ResponseWriter, req *http.Request, f *FileHandler) {
	trade, err := p.Alipay.GetNotification(req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	args := alipayArgs{}
	params, err := url.QueryUnescape(trade.PassbackParams)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}
	err = json.Unmarshal([]byte(params), &args)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	pk, err := ecdsa.ParsePubkey(args.Pubkey)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	l, err := p.TokenRepo.FindLog(trade.OutTradeNo)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	} else if l.ID != 0 {
		w.Write([]byte("success"))
		return
	}

	log := store.TokenLog{
		UserID:   args.UserID,
		Type:     store.LogTypeBuy,
		TokenNum: args.TokenNum,
		ExtraNum: args.CentNum,
		PayNo:    trade.OutTradeNo,
		Extra: map[string]string{
			"trade_no":  trade.TradeNo,
			"_buyer_id": trade.BuyerId,
			"_pubkey":   ecdsa.Compress(pk),
		},
		Sign:    args.Sign,
		Created: args.Created,
	}

	if _, err := p.TokenRepo.UpdateWallet(&log); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	w.Write([]byte("success"))
}
