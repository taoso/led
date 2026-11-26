package led

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/joho/godotenv"
	"github.com/stripe/stripe-go/v87"
	"github.com/stripe/stripe-go/v87/webhook"
)

const (
	donationLimit   = 20       // 捐助榜一次返回的条数
	maxDonorLen     = 60       // 捐助人名字的最大字符数
	maxMessageLen   = 500      // 留言的最大字符数（Stripe metadata 上限也是 500）
	defaultCurrency = "usd"    // 默认币种为美元
	anonymousDonor  = "低调的大善人" // 捐助人没填名字时对外显示的名字
	defaultMinCents = 100      // $1
	defaultMaxCents = 100000   // $1000
)

// zeroDecimal lists currencies whose unit_amount has no minor unit.
var zeroDecimal = map[string]bool{
	"bif": true, "clp": true, "djf": true, "gnf": true, "jpy": true,
	"kmf": true, "krw": true, "mga": true, "pyg": true, "rwf": true,
	"ugx": true, "vnd": true, "vuv": true, "xaf": true, "xof": true,
	"xpf": true,
}

var currencySymbol = map[string]string{
	"usd": "$", "eur": "€", "gbp": "£", "cny": "¥", "jpy": "¥",
	"hkd": "HK$", "cad": "C$", "aud": "A$", "sgd": "S$", "nzd": "NZ$",
	"krw": "₩", "twd": "NT$", "chf": "CHF ", "sek": "kr ",
}

// donateInput is the body of POST /+/stripe-donate.
type donateInput struct {
	Amount  float64 `json:"amount"` // 主币种单位，例如 20 表示 20 美元
	Donor   string  `json:"donor"`
	Message string  `json:"message"`
}

// donateOutput is what the donation page shows.
type donateOutput struct {
	Donor    string `json:"donor"`
	Amount   string `json:"amount"` // 已按币种格式化，例如 "$20.00"
	Currency string `json:"currency"`
	Message  string `json:"message"`
	Created  string `json:"created"` // 2006-01-02
}

// SetStripe enables donations. dbPath holds the sqlite file.
func (p *LED) SetStripe(secretKey, webhookSecret, dbPath string) error {
	repo, err := NewDonationRepo(dbPath)
	if err != nil {
		return err
	}
	p.stripe = stripe.NewClient(secretKey)
	p.whsec = webhookSecret
	p.donations = repo
	return nil
}

// siteEnv reads the per-site settings file.
func siteEnv(f *FileHandler) (map[string]string, error) {
	return godotenv.Read(filepath.Join(f.Root, "env"))
}

// StripeDonate creates a Checkout Session and returns its hosted URL.
func (p *LED) StripeDonate(w http.ResponseWriter, req *http.Request) {
	f := p.siteOf(req)

	envs, err := siteEnv(f)
	if err != nil || envs["stripe_donate"] != "1" {
		http.Error(w, "donation is closed", http.StatusForbidden)
		return
	}

	in, err := parseDonate(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cur := strings.ToLower(strings.TrimSpace(envs["stripe_currency"]))
	if cur == "" {
		cur = defaultCurrency
	}

	// 前端只传主币种单位，换算与取值区间都在服务端定，避免被篡改
	amount := int64(math.Round(in.Amount))
	if !zeroDecimal[cur] {
		amount = int64(math.Round(in.Amount * 100))
	}

	min, max := envInt(envs, "stripe_min", defaultMinCents), envInt(envs, "stripe_max", defaultMaxCents)
	if amount < min || amount > max {
		http.Error(w, "amount should be between "+
			formatAmount(min, cur)+" and "+formatAmount(max, cur), http.StatusBadRequest)
		return
	}

	name := envs["site_title"]
	if name == "" {
		name = f.Name
	}

	meta := map[string]string{
		"site":    f.Name,
		"donor":   truncate(in.Donor, maxDonorLen),
		"message": truncate(in.Message, maxMessageLen),
	}
	if in.Donor == "" {
		meta["anonymous"] = "1"
	}

	params := &stripe.CheckoutSessionCreateParams{
		Mode:       stripe.String(stripe.CheckoutSessionModePayment),
		SubmitType: stripe.String("donate"),
		// 账户默认开启了 Managed Payments（Stripe 代收付/计税），
		// 它要求每个 line item 带 product tax code，动态金额的 price_data 不方便给，
		// 这里显式关掉，避免报 invalid_request_error。
		ManagedPayments: &stripe.CheckoutSessionCreateManagedPaymentsParams{
			Enabled: new(false),
		},
		SuccessURL:        stripe.String("https://" + f.Name + "/donate.htm?session_id={CHECKOUT_SESSION_ID}"),
		CancelURL:         stripe.String("https://" + f.Name + "/donate.htm"),
		ClientReferenceID: stripe.String(f.Name),
		Metadata:          meta,
		LineItems: []*stripe.CheckoutSessionCreateLineItemParams{{
			Quantity: stripe.Int64(1),
			PriceData: &stripe.CheckoutSessionCreateLineItemPriceDataParams{
				Currency:   stripe.String(cur),
				UnitAmount: new(amount),
				ProductData: &stripe.CheckoutSessionCreateLineItemPriceDataProductDataParams{
					Name: stripe.String("捐助 " + name),
				},
			},
		}},
	}

	sess, err := p.stripe.V1CheckoutSessions.Create(req.Context(), params)
	if err != nil {
		log.Println("stripe checkout session:", err)
		http.Error(w, "stripe is unavailable", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]string{"url": sess.URL})
}

// StripeWebhook applies the payment notifications sent by Stripe.
func (p *LED) StripeWebhook(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
	if err != nil {
		http.Error(w, "cannot read body", http.StatusBadRequest)
		return
	}

	// 签名仍是硬校验；只放宽 API 版本比对，否则 Dashboard 里
	// endpoint 的版本低一档就会导致所有通知被 400 丢弃。
	ev, err := webhook.ConstructEventWithOptions(body,
		req.Header.Get("Stripe-Signature"), p.whsec,
		webhook.ConstructEventOptions{IgnoreAPIVersionMismatch: true})
	if err != nil {
		log.Println("stripe webhook signature:", err)
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}

	switch err := p.applyEvent(&ev, body); {
	case errors.Is(err, ErrDuplicate):
		// 已经记过账，回 200 让 Stripe 停止重试
		w.WriteHeader(http.StatusOK)
	case err != nil:
		log.Println("stripe webhook", ev.Type, err)
		http.Error(w, "cannot store event", http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusOK)
	}
}

func (p *LED) applyEvent(ev *stripe.Event, body []byte) error {
	switch ev.Type {
	case "checkout.session.completed", "checkout.session.async_payment_succeeded":
		var s stripe.CheckoutSession
		if err := json.Unmarshal(ev.Data.Raw, &s); err != nil {
			return err
		}
		if s.PaymentStatus != stripe.CheckoutSessionPaymentStatusPaid {
			// 异步支付方式此时还未到账，等 async_payment_succeeded
			return nil
		}
		se, d := p.donationOf(ev, &s, body)
		return p.donations.Record(se, d)
	case "charge.refunded":
		var c stripe.Charge
		if err := json.Unmarshal(ev.Data.Raw, &c); err != nil {
			return err
		}
		if c.PaymentIntent != nil {
			return p.donations.SetStatus(c.PaymentIntent.ID, "refunded")
		}
	}
	return nil
}

func (p *LED) donationOf(ev *stripe.Event, s *stripe.CheckoutSession, body []byte) (stripeEvent, Donation) {
	d := Donation{
		SessionID: s.ID,
		Site:      p.host(s.Metadata["site"]),
		Amount:    int(s.AmountTotal),
		Currency:  string(s.Currency),
		Donor:     truncate(s.Metadata["donor"], maxDonorLen),
		Message:   truncate(s.Metadata["message"], maxMessageLen),
		Anonymous: s.Metadata["anonymous"] == "1",
		Status:    "paid",
		Shown:     true,
		Created:   time.Unix(s.Created, 0).UTC(),
	}
	if s.CustomerDetails != nil {
		d.Email = s.CustomerDetails.Email
	}
	if s.PaymentIntent != nil {
		d.PaymentIntent = s.PaymentIntent.ID
	}

	e := stripeEvent{
		ID:      ev.ID,
		Type:    string(ev.Type),
		Created: time.Unix(ev.Created, 0).UTC(),
		Payload: body,
	}

	return e, d
}

// StripeDonations lists the recent donations of the site in the request host.
func (p *LED) StripeDonations(w http.ResponseWriter, req *http.Request) {
	if p.donations == nil {
		http.Error(w, "stripe is not configured", http.StatusServiceUnavailable)
		return
	}

	ds, err := p.donations.List(p.host(req.Host), donationLimit)
	if err != nil {
		log.Println("list donations:", err)
		http.Error(w, "cannot list donations", http.StatusInternalServerError)
		return
	}

	out := make([]donateOutput, 0, len(ds))
	for _, d := range ds {
		donor := d.Donor
		if d.Anonymous {
			donor = ""
		}
		// 没填名字的捐助统一显示成匿名，前端不必再猜
		if strings.TrimSpace(donor) == "" {
			donor = anonymousDonor
		}
		out = append(out, donateOutput{
			Donor:    donor,
			Amount:   formatAmount(int64(d.Amount), d.Currency),
			Currency: d.Currency,
			Message:  d.Message,
			Created:  d.Created.Format("2006-01-02"),
		})
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	json.NewEncoder(w).Encode(out)
}

// parseDonate decodes the JSON body of POST /+/stripe-donate.
func parseDonate(req *http.Request) (donateInput, error) {
	var in donateInput

	body := io.LimitReader(req.Body, 1<<16)
	defer req.Body.Close()

	if err := json.NewDecoder(body).Decode(&in); err != nil {
		return in, errors.New("bad request body")
	}
	return in, nil
}

func envInt(envs map[string]string, key string, def int64) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(envs[key]), 10, 64)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// truncate limits s to n characters, never splitting a rune.
func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// formatAmount renders minor units, honouring zero-decimal currencies.
func formatAmount(minor int64, cur string) string {
	scale := int64(100)
	if zeroDecimal[cur] {
		scale = 1
	}

	s := strconv.FormatInt(minor/scale, 10)
	if scale == 100 {
		s += "." + string([]byte{'0' + byte(minor%scale/10), '0' + byte(minor%10)})
	}

	if sym, ok := currencySymbol[cur]; ok {
		return sym + s
	}
	return s + " " + strings.ToUpper(cur)
}
