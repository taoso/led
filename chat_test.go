package led

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-kiss/monkey"
	"github.com/smartwalle/alipay/v3"
	"github.com/stretchr/testify/assert"
	"github.com/taoso/led/pay"
	"github.com/taoso/led/store"
)

func TestBuyTokens(t *testing.T) {
	args := alipayArgs{
		TokenNum: 4000,
		CentNum:  100,
		Sign:     "D407CaeVnPXkzyDPTI94tq8L460u5K1GvGgDDm40TCfgxh+hcWZ9cusP4ZvGJEJiS808yULCM19UO1SJnxYmTQ==",
		Pubkey:   "BKjQmlNjXfWLeprdKDpmdHNFQZz4mdQktEfXo0FsSj+r1gegK/6OPh/L4oKcfxl8P6tPa5EvTK3tibnOjlk2Vxs=",
		Created:  time.Date(2023, 4, 12, 23, 23, 23, 0, time.UTC),
	}
	b, err := json.Marshal(args)
	assert.Nil(t, err)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/+/buy-tokens", bytes.NewReader(b))
	p := &Proxy{}

	g1 := monkey.Patch(time.Now, func() time.Time {
		return args.Created.Add(2 * time.Minute)
	})
	defer g1.Unpatch()

	g2 := monkey.Patch((*pay.Alipay).CreateQR, func(_ *pay.Alipay, o pay.Order) (string, error) {
		assert.Equal(t, "1", o.Amount)
		assert.Equal(t, "4000 tokens", o.Subject)
		assert.Equal(t, url.QueryEscape(string(b)), o.Extra)
		assert.Equal(t, "https://lehu.in/+/buy-tokens-notify", o.NotifyURL)
		return "https://example.qr", nil
	})
	defer g2.Unpatch()

	p.buyTokens(w, req, &FileHandler{Name: "lehu.in"})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"qr":"https://example.qr","ttl":900}`, w.Body.String())
}

func TestBuyTokensErr(t *testing.T) {
	for _, c := range []struct {
		args alipayArgs
		code int
		err  string
	}{
		{
			args: alipayArgs{
				CentNum: 30,
			},
			code: http.StatusBadRequest,
			err:  "cent_num must > 100",
		},
		{
			args: alipayArgs{
				CentNum: 110,
			},
			code: http.StatusBadRequest,
			err:  "cent_num must be a multiple of 100",
		},
		{
			args: alipayArgs{
				CentNum:  100,
				TokenNum: 3000,
			},
			code: http.StatusBadRequest,
			err:  "invalid token_num",
		},
		{
			args: alipayArgs{
				CentNum:  100,
				TokenNum: 4000,
				Created:  time.Now().Add(6 * time.Minute),
			},
			code: http.StatusBadRequest,
			err:  "client time is inaccurate",
		},
		{
			args: alipayArgs{
				CentNum:  100,
				TokenNum: 4000,
				Created:  time.Now().Add(1 * time.Minute),
				Sign:     "D407CaeVnPXkzyDPTI94tq8L460u5K1GvGgDDm40TCfgxh+hcWZ9cusP4ZvGJEJiS808yULCM19UO1SJnxYmTQ==",
				Pubkey:   "BKjQmlNjXfWLeprdKDpmdHNFQZz4mdQktEfXo0FsSj+r1gegK/6OPh/L4oKcfxl8P6tPa5EvTK3tibnOjlk2Vxs=",
			},
			code: http.StatusBadRequest,
			err:  "invalid signature",
		},
	} {
		b, err := json.Marshal(c.args)
		assert.Nil(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/+/buy-tokens", bytes.NewReader(b))

		p := &Proxy{}
		p.buyTokens(w, req, &FileHandler{Name: "lehu.in"})

		assert.Equal(t, c.code, w.Code)
		assert.Equal(t, c.err, w.Body.String())
	}
}

func TestBuyTokensNotify(t *testing.T) {
	args := alipayArgs{
		TokenNum: 4000,
		CentNum:  100,
		Sign:     "s1",
		Pubkey:   "p1",
		Created:  time.Date(2023, 4, 12, 23, 23, 23, 0, time.UTC),
	}
	b, err := json.Marshal(args)
	assert.Nil(t, err)

	params := url.Values{}
	params.Set("passback_params", string(b))
	params.Set("buyer_id", "1024")
	params.Set("trade_no", "tn1")
	params.Set("out_trade_no", "otn1")

	w := httptest.NewRecorder()

	req := httptest.NewRequest("POST", "/+/buy-tokens-notify", strings.NewReader(params.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.ParseForm()

	p := &Proxy{}

	g1 := monkey.Patch((*pay.Alipay).GetNotification, func(_ *pay.Alipay, req *http.Request) (n alipay.TradeNotification, err error) {
		n.PassbackParams = req.FormValue("passback_params")
		n.BuyerId = req.FormValue("buyer_id")
		n.TradeNo = req.FormValue("trade_no")
		n.OutTradeNo = req.FormValue("out_trade_no")
		return
	})
	defer g1.Unpatch()

	g2 := monkey.Patch((*store.TokenRepo).UpdateWallet, func(_ *store.TokenRepo, log *store.TokenLog) (w store.TokenWallet, err error) {
		assert.Equal(t, 0, log.UserID)
		assert.Equal(t, store.LogTypeBuy, log.Type)
		assert.Equal(t, args.TokenNum, log.TokenNum)
		assert.Equal(t, args.CentNum, log.ExtraNum)
		assert.Equal(t, args.Sign, log.Sign)
		assert.Equal(t, args.Created, log.Created)
		assert.Equal(t, args.Pubkey, log.Extra["pubkey"])
		assert.Equal(t, params.Get("out_trade_no"), log.Extra["our_trade_no"])
		assert.Equal(t, params.Get("trade_no"), log.Extra["alipay_trade_no"])
		assert.Equal(t, params.Get("buyer_id"), log.Extra["alipay_buyer_id"])
		return
	})
	defer g2.Unpatch()

	p.buyTokensNotify(w, req, &FileHandler{Name: "lehu.in"})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `success`, w.Body.String())
}
