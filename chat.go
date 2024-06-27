package led

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/taoso/led/ecdsa"
	"github.com/taoso/led/pay"
	"github.com/taoso/led/store"
)

const utcTime = "2006-01-02T15:04:05.000Z"

func (p *Proxy) getWallet(wid int, req *http.Request) (w store.TokenWallet, err error) {
	w, err = p.TokenRepo.GetWallet(wid)
	if err != nil || w.ID == 0 {
		return
	}
	c, err := req.Cookie("sid")
	if err != nil {
		err = nil
		return
	}
	i, err := strconv.Atoi(c.Value)
	if err != nil {
		err = nil
		return
	}
	s, err := p.TokenRepo.GetSession(i)
	if err != nil {
		return
	}
	w.Pubkey = s.Pubkey
	return
}

type chatmsg struct {
	Messages  []map[string]string `json:"messages"`
	Model     string              `json:"model"`
	Stream    bool                `json:"stream"`
	User      string              `json:"user"`
	MaxTokens int                 `json:"max_tokens"`
}
type _msg struct {
	chatmsg
	Sign    string    `json:"_sign,omitempty"`
	UserID  int       `json:"_user_id,omitempty"`
	Created time.Time `json:"_created,omitempty"`
}

func (p *Proxy) chat(w http.ResponseWriter, req *http.Request, f *FileHandler) {
	defer req.Body.Close()

	var msg _msg

	if err := json.NewDecoder(req.Body).Decode(&msg); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	var err error
	var hash [32]byte
	var wallet store.TokenWallet

	if auth := req.Header.Get("Authorization"); auth != "" {
		key := auth[len("Bearer "):]
		wallet, err = p.TokenRepo.FindWallet(key)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		} else if wallet.ID == 0 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	} else {
		if msg.Created.Sub(time.Now()).Abs() > 30*time.Second {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("invalid created"))
			return
		}

		wallet, err = p.getWallet(msg.UserID, req)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		} else if wallet.ID == 0 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// 会话过期
		if wallet.Pubkey == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		pk, err := wallet.GetPubkey()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}

		var buf bytes.Buffer
		for _, m := range msg.Messages {
			buf.WriteString(m["role"])
			buf.WriteString(m["content"])
		}
		buf.WriteString(strconv.Itoa(msg.UserID))
		buf.WriteString(msg.Created.UTC().Format("2006-01-02T15:04:05.000Z"))
		if msg.Model != "gpt-3.5-turbo" {
			buf.WriteString(msg.Model)
		}

		var ok bool
		ok, hash, err = ecdsa.VerifyES256(buf.String(), msg.Sign, pk)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("invalid signature"))
			return
		}
	}

	if strings.HasPrefix(msg.Model, "dall") {
		p.image(w, req, f, msg, hash)
		return
	}

	msg.Stream = true
	var tokenRate int
	var maxTokens int
	switch msg.Model {
	case "3.5-8k", "", "gpt-3.5-turbo", "3.5-4k", "3.5-16k", "3.5-turbo":
		tokenRate = 2
		msg.Model = "gpt-3.5-turbo"
		maxTokens = 4 * 1024
	case "4.0-8k", "4.0-128k", "4.0-turbo":
		tokenRate = 6
		msg.Model = "gpt-4-turbo"
		maxTokens = 4 * 1024
	case "gpt-4o":
		tokenRate = 4
		msg.Model = "gpt-4o"
		maxTokens = 4 * 1024
	default:
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid model"))
		return
	}

	if t := int(float64(wallet.Tokens) / float64(tokenRate)); t < maxTokens {
		maxTokens = t
	}

	var u struct {
		Usage struct {
			ReplyTokens  int `json:"completion_tokens"`
			PromptTokens int `json:"prompt_tokens"`
			TotalTokens  int `json:"total_tokens"`
			RemainTokens int `json:"remain_tokens"`
			TokenRate    int `json:"token_rate"`
		} `json:"usage"`
	}

	var chatID string

	defer func() {
		if msg.Stream && u.Usage.ReplyTokens > 0 {
			u.Usage.TokenRate = tokenRate
			u.Usage.TotalTokens = u.Usage.PromptTokens + u.Usage.ReplyTokens

			tl := store.TokenLog{
				UserID:   msg.UserID,
				Type:     store.LogTypeCost,
				TokenNum: u.Usage.TotalTokens * tokenRate,
				Extra: map[string]string{
					"chatid":        chatID,
					"model":         msg.Model,
					"token_rate":    strconv.Itoa(tokenRate),
					"sha256":        hex.EncodeToString(hash[:]),
					"prompt_tokens": strconv.Itoa(u.Usage.PromptTokens),
				},
				Created: msg.Created,
				Sign:    msg.Sign,
			}
			uw, err := p.TokenRepo.UpdateWallet(&tl)
			if err != nil {
				log.Printf("save token log %+v err %v", tl, err)
			} else {
				u.Usage.RemainTokens = uw.Tokens
			}
			b, _ := json.Marshal(u)
			b = append([]byte("data: "), b...)
			b = append(b, []byte("\n\ndata: [DONE]\n\n")...)
			w.Write(b)
		}
	}()

	u.Usage.PromptTokens = p.bpe(msg.Model).CountMessage(msg.Messages)

	if maxTokens <= u.Usage.PromptTokens {
		w.WriteHeader(http.StatusPaymentRequired)
		w.Write([]byte(strconv.Itoa(tokenRate * u.Usage.PromptTokens)))
		return
	}

	msg.chatmsg.MaxTokens = maxTokens - u.Usage.PromptTokens

	msg.User = strconv.Itoa(msg.UserID)

	b, err := json.Marshal(msg.chatmsg)
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

	chatID = resp.Header.Get("X-Request-Id")

	linkKey := msg.User + chatID
	p.chatLinks.Store(linkKey, resp.Body)

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("X-Request-Id", chatID)
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
			u.Usage.ReplyTokens += p.bpe(msg.Model).Count(c.Delta.Content)
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
		w.(http.Flusher).Flush()
	}

	if err := s.Err(); err != nil {
		log.Println("scan err", err)
	}
}

func (p *Proxy) image(w http.ResponseWriter, req *http.Request, f *FileHandler, msg _msg, hash [32]byte) {
	prompt := msg.Messages[len(msg.Messages)-1]["content"]
	model := msg.Model
	quality := "standard"
	var size string
	var token int
	switch model {
	case "dall-e-3-1024x1024":
		model = "dall-e-3"
		size = "1024x1024"
		token = 10000
	case "dall-e-3-1024x1792":
		model = "dall-e-3"
		size = "1024x1792"
		token = 20000
	case "dall-e-3-1792x1024":
		model = "dall-e-3"
		size = "1792x1024"
		token = 20000
	case "dall-e-3-1024x1024-hd":
		model = "dall-e-3"
		size = "1024x1024"
		quality = "hd"
		token = 20000
	case "dall-e-3-1024x1792-hd":
		model = "dall-e-3"
		size = "1024x1792"
		quality = "hd"
		token = 30000
	case "dall-e-3-1792x1024-hd":
		model = "dall-e-3"
		size = "1792x1024"
		quality = "hd"
		token = 30000
	case "dall-e-2-256x256":
		model = "dall-e-2"
		size = "256x256"
		token = 4000
	case "dall-e-2-512x512":
		model = "dall-e-2"
		size = "512x512"
		token = 4000
	case "dall-e-2-1024x1024":
		model = "dall-e-2"
		size = "1024x1024"
		token = 4000
	}

	b, err := json.Marshal(map[string]any{
		"model":   model,
		"prompt":  prompt,
		"n":       1,
		"size":    size,
		"quality": quality,
		"user":    strconv.Itoa(msg.UserID),
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	url := "https://api.openai.com/v1/images/generations"
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

	chatID := resp.Header.Get("X-Request-Id")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Request-Id", chatID)
	w.WriteHeader(resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		io.Copy(w, resp.Body)
		return
	}

	var d struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"![%s](%s)\"}}]}\n\n", prompt, d.Data[0].URL)

	var u struct {
		Usage struct {
			ReplyTokens  int `json:"completion_tokens"`
			PromptTokens int `json:"prompt_tokens"`
			TotalTokens  int `json:"total_tokens"`
			RemainTokens int `json:"remain_tokens"`
			TokenRate    int `json:"token_rate"`
		} `json:"usage"`
	}

	u.Usage.TokenRate = 1
	u.Usage.TotalTokens = token

	tl := store.TokenLog{
		UserID:   msg.UserID,
		Type:     store.LogTypeCost,
		TokenNum: u.Usage.TotalTokens,
		Extra: map[string]string{
			"chatid":  chatID,
			"model":   msg.Model,
			"sha256":  hex.EncodeToString(hash[:]),
			"size":    size,
			"quality": quality,
		},
		Created: msg.Created,
		Sign:    msg.Sign,
	}
	uw, err := p.TokenRepo.UpdateWallet(&tl)
	if err != nil {
		log.Printf("save token log %+v err %v", tl, err)
	} else {
		u.Usage.RemainTokens = uw.Tokens
	}
	b, _ = json.Marshal(u)
	b = append([]byte("data: "), b...)
	b = append(b, []byte("\n\ndata: [DONE]\n\n")...)
	w.Write(b)
}

func (p *Proxy) chatCancel(w http.ResponseWriter, req *http.Request, f *FileHandler) {
	defer req.Body.Close()
	var args struct {
		ChatID  string    `json:"chat_id"`
		UserID  int       `json:"user_id"`
		Created time.Time `json:"created"`
		Sign    string    `json:"sign"`
	}

	if err := json.NewDecoder(req.Body).Decode(&args); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	wallet, err := p.getWallet(args.UserID, req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	} else if wallet.ID == 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("user_id not found"))
		return
	}

	pk, err := wallet.GetPubkey()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
	}

	var buf bytes.Buffer
	buf.WriteString(args.ChatID)
	buf.WriteString(args.Created.UTC().Format("2006-01-02T15:04:05.000Z"))

	ok, _, err := ecdsa.VerifyES256(buf.String(), args.Sign, pk)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
	}
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid signature"))
		return
	}

	linkKey := strconv.Itoa(args.UserID) + args.ChatID
	v, ok := p.chatLinks.Load(linkKey)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("link not found"))
		return
	}

	v.(io.Closer).Close()
	p.chatLinks.Delete(linkKey)
}

func (p *Proxy) checkName(w http.ResponseWriter, req *http.Request, f *FileHandler) {
	var args struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(req.Body).Decode(&args); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	u, err := p.TokenRepo.FindWalletByName(args.Name)
	if err != nil {
		return
	}
	if u.ID != 0 {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte("用户名已存在"))
		return
	}

	w.Write([]byte("ok"))
}

var usernameRE = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

func (p *Proxy) setAuth(w http.ResponseWriter, req *http.Request, f *FileHandler) {
	var args struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(req.Body).Decode(&args); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	if !usernameRE.MatchString(args.Username) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid username"))
		return
	}
	if args.Password == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid password"))
		return
	}

	uid, err := strconv.Atoi(req.Header.Get("cg-uid"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("用户不存在"))
		return
	}
	wallet, err := p.TokenRepo.GetWallet(uid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	if u, err := p.TokenRepo.FindWalletByName(args.Username); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	} else if u.ID != 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("用户名已存在"))
		return
	}

	wallet.Username = args.Username
	wallet.SetPassword(args.Password)

	if err = p.TokenRepo.SaveWallet(wallet); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
	}
	w.Write([]byte("ok"))
}

func (p *Proxy) login(w http.ResponseWriter, req *http.Request, f *FileHandler) {
	var args struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(req.Body).Decode(&args); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	u, err := p.TokenRepo.FindWalletByName(args.Username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	// 同时检查用户不存在的情形
	if !u.CheckPassword(args.Password) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	s := store.Session{
		UserID:  u.ID,
		Pubkey:  req.Header.Get("cg-pubk"),
		Agent:   req.UserAgent(),
		Address: req.RemoteAddr,
		Created: time.Now(),
	}

	if err = p.TokenRepo.AddSession(&s); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{" +
		"\"sid\":" + strconv.Itoa(s.ID) + "," +
		"\"uid\":" + strconv.Itoa(s.UserID) +
		"}"))
}

func (p *Proxy) listSession(w http.ResponseWriter, req *http.Request, f *FileHandler) {
	uid, _ := strconv.Atoi(req.Header.Get("cg-uid"))

	ss, err := p.TokenRepo.ListSession(uid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	if err := json.NewEncoder(w).Encode(ss); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
	}
}

func (p *Proxy) delSession(w http.ResponseWriter, req *http.Request, f *FileHandler) {
	var args struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(req.Body).Decode(&args); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}
	uid, _ := strconv.Atoi(req.Header.Get("cg-uid"))

	if err := p.TokenRepo.DelSession(args.ID, uid); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write([]byte("ok"))
}

type alipayArgs struct {
	UserID   int       `json:"user_id"`
	TokenNum int       `json:"token_num"`
	CentNum  int       `json:"cent_num"`
	Sign     string    `json:"sign"`
	Pubkey   string    `json:"pubkey"`
	Created  time.Time `json:"created"`
	FromID   string    `json:"from_id"`
}

const tokenPrice = 5

func (p *Proxy) buyTokens(w http.ResponseWriter, req *http.Request, f *FileHandler) {
	args := alipayArgs{}

	defer req.Body.Close()
	if err := json.NewDecoder(req.Body).Decode(&args); err != nil {
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

	if args.TokenNum != args.CentNum/tokenPrice*1000 {
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

	var err error
	var pk ecdsa.PublicKey
	if args.UserID != 0 {
		u, err := p.getWallet(args.UserID, req)
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

	ok, _, err := ecdsa.VerifyES256(log.SignData(), args.Sign, pk)
	if err != nil || !ok {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid signature"))
		return
	}

	if f, err := req.Cookie("from"); err == nil {
		args.FromID = f.Value
	}

	body, _ := json.Marshal(args)
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
	w.Write([]byte(`{"qr":"` + qr + `","trade_no":"` + order.TradeNo + `","ttl":900}`))
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

	if l, err := p.TokenRepo.FindLog(trade.OutTradeNo); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
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
			"_from_id":  args.FromID,
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

func (p *Proxy) buyTokensLog(w http.ResponseWriter, req *http.Request, f *FileHandler) {
	defer req.Body.Close()
	args := struct {
		TradeNo string    `json:"trade_no"`
		Sign    string    `json:"sign"`
		Pubkey  string    `json:"pubkey"`
		Created time.Time `json:"created"`
	}{}
	if err := json.NewDecoder(req.Body).Decode(&args); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	if args.Created.Sub(time.Now()).Abs() > 30*time.Second {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("client time is inaccurate"))
		return
	}

	pk, err := ecdsa.ParsePubkey(args.Pubkey)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid pubkey"))
		return
	}

	var buf bytes.Buffer
	buf.WriteString(args.TradeNo)
	buf.WriteString(args.Created.UTC().Format("2006-01-02T15:04:05.000Z"))

	ok, _, err := ecdsa.VerifyES256(buf.String(), args.Sign, pk)
	if err != nil || !ok {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid signature"))
		return
	}

	log, err := p.TokenRepo.FindLog(args.TradeNo)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	b, err := json.Marshal(log)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write(b)
}

func (p *Proxy) buyTokensLogs(w http.ResponseWriter, req *http.Request, f *FileHandler) {
	defer req.Body.Close()
	args := struct {
		UserID  int       `json:"user_id"`
		LastID  int       `json:"last_id"`
		Sign    string    `json:"sign"`
		Created time.Time `json:"created"`
	}{}

	if err := json.NewDecoder(req.Body).Decode(&args); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	if args.Created.Sub(time.Now()).Abs() > 30*time.Second {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("client time is inaccurate"))
		return
	}

	u, err := p.getWallet(args.UserID, req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("client time is inaccurate"))
		return
	} else if u.ID == 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("user not found"))
		return
	}

	pk, err := u.GetPubkey()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("invalid pubkey"))
		return
	}

	var buf bytes.Buffer
	buf.WriteString(strconv.Itoa(args.UserID))
	buf.WriteString(strconv.Itoa(args.LastID))
	buf.WriteString(args.Created.UTC().Format("2006-01-02T15:04:05.000Z"))

	ok, _, err := ecdsa.VerifyES256(buf.String(), args.Sign, pk)
	if err != nil || !ok {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid signature"))
		return
	}

	if args.LastID <= 0 {
		args.LastID = math.MaxInt
	}

	logs, err := p.TokenRepo.ScanLogs(u.ID, args.LastID, 10)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	b, err := json.Marshal(logs)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write(b)
}

func (p *Proxy) buyTokensWallet(w http.ResponseWriter, req *http.Request, f *FileHandler) {
	defer req.Body.Close()
	args := struct {
		Sign    string    `json:"sign"`
		Pubkey  string    `json:"pubkey"`
		Created time.Time `json:"created"`
	}{}
	if err := json.NewDecoder(req.Body).Decode(&args); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	if args.Created.Sub(time.Now()).Abs() > 30*time.Second {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("client time is inaccurate"))
		return
	}

	pk, err := ecdsa.ParsePubkey(args.Pubkey)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid pubkey"))
		return
	}

	var buf bytes.Buffer
	buf.WriteString(args.Created.UTC().Format("2006-01-02T15:04:05.000Z"))

	ok, _, err := ecdsa.VerifyES256(buf.String(), args.Sign, pk)
	if err != nil || !ok {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid signature"))
		return
	}

	var u store.TokenWallet
	if c, err := req.Cookie("sid"); err == nil {
		i, err := strconv.Atoi(c.Value)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
		s, err := p.TokenRepo.GetSession(i)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		if s.Pubkey != ecdsa.Compress(pk) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		u, err = p.TokenRepo.GetWallet(s.UserID)
	} else {
		u, err = p.TokenRepo.FindWallet(ecdsa.Compress(pk))
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	b, err := json.Marshal(u)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Header().Set("X-Token-Price", strconv.Itoa(tokenPrice))
	w.Write(b)
}
