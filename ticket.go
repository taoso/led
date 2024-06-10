package led

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/taoso/led/pay"
)

func (h *Proxy) ServeTicket(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("query") != "" {
		req := struct {
			Token string `json:"token"`
		}{}
		defer r.Body.Close()
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ts, err := h.TicketRepo.List(req.Token, 10)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Add("content-type", "application/json")
		json.NewEncoder(w).Encode(ts)
		return
	}

	if r.URL.Query().Get("buy") != "" {
		req := struct {
			Token string `json:"token"`
			Cents int    `json:"cents"`
		}{}
		defer r.Body.Close()
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var bytes int
		var months int
		const GB = 1024 * 1024 * 1024
		switch req.Cents {
		case 200:
			bytes = 2 * GB
			months = 1
		case 400:
			bytes = 8 * GB
			months = 2
		case 800:
			bytes = 32 * GB
			months = 3
		default:
			http.Error(w, "cents must in [200,400,800]", http.StatusBadRequest)
			return
		}

		if req.Token == "" {
			b := make([]byte, 16)
			_, err := rand.Read(b)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			req.Token = base64.RawURLEncoding.EncodeToString(b)
		}

		now := time.Now().Format(time.RFC3339)
		yuan := strconv.FormatFloat(float64(req.Cents)/100, 'f', 2, 64)

		o := pay.Order{
			Subject:   fmt.Sprintf("Traffic: %dGB@%dM", bytes/GB, months),
			TradeNo:   req.Token + "@" + now,
			Amount:    yuan,
			NotifyURL: "https://" + r.Host + r.URL.Path,
			Extra:     fmt.Sprintf(`{"bytes":%d,"months":%d}`, bytes, months),
		}

		qr, err := h.Alipay.CreateQR(o)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Add("content-type", "application/json")
		json.NewEncoder(w).Encode(struct {
			QR    string `json:"qr"`
			Token string `json:"token"`
			Order string `json:"order"`
		}{QR: qr, Token: req.Token, Order: o.TradeNo})
	} else {
		o, err := h.Alipay.GetNotification(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		i := strings.Index(o.OutTradeNo, "@")
		token := o.OutTradeNo[:i]

		var extra struct {
			Bytes  int `json:"bytes"`
			Months int `json:"months"`
		}

		err = json.Unmarshal([]byte(o.PassbackParams), &extra)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		err = h.TicketRepo.New(token, extra.Bytes, extra.Months, o.OutTradeNo, o.TradeNo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Write([]byte("success"))
	}
}
