package led

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/taoso/led/pay"
)

type alipayOrderArgs struct {
	CentNum int       `json:"cent_num"` // 支付金额，单位是分
	OrderID string    `json:"order_id"` // 业务订单号
	Subject string    `json:"subject"`  // 订单标题
	Extra   string    `json:"extra"`    // 业务扩展参数
	Created time.Time `json:"created"`  // 请求时间

	NotifyURL string `json:"notify_url,omitempty"` // 异步通知地址
}

func (p *Proxy) AlipayOrderCreate(w http.ResponseWriter, req *http.Request, f *FileHandler) {
	defer req.Body.Close()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	app := req.Header.Get("zz-app")
	sign, err := base64.RawURLEncoding.DecodeString(req.Header.Get("zz-sign"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	key, err := base64.RawURLEncoding.DecodeString(os.Getenv("APP_PUBKEY_" + app))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	if !ed25519.Verify(key, body, sign) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid sign"))
		return
	}

	args := alipayOrderArgs{}
	if err := json.Unmarshal(body, &args); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	if args.Created.Sub(time.Now()).Abs() > 5*time.Minute {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("client time is inaccurate"))
		return
	}

	if args.CentNum < 1 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("cent_num must > 0"))
		return
	}

	extras := url.Values{}
	extras.Set("url", args.NotifyURL)
	extras.Set("extra", args.Extra)

	order := pay.Order{
		TradeNo:   args.OrderID,
		Amount:    strconv.FormatFloat(float64(args.CentNum)/100, 'f', 2, 64),
		Subject:   args.Subject,
		Extra:     extras.Encode(),
		NotifyURL: "https://" + f.Name + "/+/alipay-order-notify",
	}
	qr, err := p.Alipay.CreateQR(order)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"qr":  qr,
		"ttl": 900,
	})
}

func (p *Proxy) AlipayOrderNotify(w http.ResponseWriter, req *http.Request, f *FileHandler) {
	trade, err := p.Alipay.GetNotification(req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	extras, err := url.ParseQuery(trade.PassbackParams)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	notifyUrl := extras.Get("url")
	extras.Del("url")
	extras.Set("trade_no", trade.TradeNo)
	extras.Set("trade_status", string(trade.TradeStatus))

	cent, err := strconv.ParseFloat(trade.TotalAmount, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	now := time.Now()

	args := alipayOrderArgs{
		CentNum: int(cent * 100),
		OrderID: trade.OutTradeNo,
		Subject: trade.Subject,
		Extra:   extras.Encode(),
		Created: now,
	}

	body, err := json.Marshal(args)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	app := "zz"

	seed, err := base64.RawURLEncoding.DecodeString(os.Getenv("APP_PRVKEY_" + app))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	key := ed25519.NewKeyFromSeed(seed)

	sign := base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, body))

	r, err := http.NewRequest("POST", notifyUrl, bytes.NewReader(body))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	r.Header.Set("zz-app", app)
	r.Header.Set("zz-sign", sign)
	r.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	defer resp.Body.Close()
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
