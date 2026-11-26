package led

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stripe/stripe-go/v87"
	"github.com/stripe/stripe-go/v87/webhook"
)

const testSecret = "whsec_test"

// signEvent builds the raw body and the Stripe-Signature header of an event.
func signEvent(t *testing.T, body string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()

	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload:   []byte(body),
		Secret:    testSecret,
		Timestamp: time.Now(),
	})

	req := httptest.NewRequest("POST", "http://led.example.com/+/stripe-webhook", strings.NewReader(body))
	req.Header.Set("Stripe-Signature", signed.Header)

	return httptest.NewRecorder(), req
}

func newTestLED(t *testing.T) *LED {
	t.Helper()

	root := t.TempDir()
	site := filepath.Join(root, "a.zz.ac")
	if err := os.MkdirAll(site, 0o755); err != nil {
		t.Fatal(err)
	}
	env := "stripe_donate=1\nstripe_max=5000\nsite_title=乐乎\n"
	if err := os.WriteFile(filepath.Join(site, "env"), []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}

	return &LED{
		Root:      root,
		stripe:    stripe.NewClient("sk_test_fake"),
		whsec:     testSecret,
		donations: newTestRepo(t),
	}
}

func sessionEvent(id, payStatus string) string {
	return `{"id":"` + id + `","object":"event","type":"checkout.session.completed",
	"created":1700000000,"data":{"object":{
	"id":"cs_test_1","object":"checkout.session","amount_total":2000,"currency":"usd",
	"created":1700000000,"payment_status":"` + payStatus + `",
	"metadata":{"site":"a.zz.ac","donor":"张三","message":"写得好"},
	"customer_details":{"email":"reader@example.com"},
	"payment_intent":{"id":"pi_1"}}}}`
}

func TestWebhookRecordsDonation(t *testing.T) {
	p := newTestLED(t)

	w, req := signEvent(t, sessionEvent("evt_1", "paid"))
	p.StripeWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}

	ds, err := p.donations.List("a.zz.ac", 20)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(ds) != 1 {
		t.Fatalf("got %d donations, want 1", len(ds))
	}
	d := ds[0]
	if d.Donor != "张三" || d.Message != "写得好" || d.Amount != 2000 {
		t.Fatalf("unexpected donation: %+v", d)
	}
	if d.Email != "reader@example.com" {
		t.Fatalf("email = %q", d.Email)
	}
	if d.PaymentIntent != "pi_1" || d.SessionID != "cs_test_1" {
		t.Fatalf("ids not stored: %+v", d)
	}

	// 重投同一事件：仍然 200，但不重复记账
	w, req = signEvent(t, sessionEvent("evt_1", "paid"))
	p.StripeWebhook(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("replay status = %d, want 200", w.Code)
	}

	ds, err = p.donations.List("a.zz.ac", 20)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(ds) != 1 {
		t.Fatalf("replay stored twice: %d rows", len(ds))
	}
}

func TestWebhookIgnoresUnpaid(t *testing.T) {
	p := newTestLED(t)

	// 异步支付方式下先收到 completed 但尚未到账
	w, req := signEvent(t, sessionEvent("evt_1", "unpaid"))
	p.StripeWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	ds, err := p.donations.List("a.zz.ac", 20)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(ds) != 0 {
		t.Fatalf("unpaid session stored: %+v", ds)
	}
}

func TestWebhookRejectsBadSignature(t *testing.T) {
	p := newTestLED(t)

	req := httptest.NewRequest("POST", "http://led.example.com/+/stripe-webhook",
		strings.NewReader(sessionEvent("evt_1", "paid")))
	req.Header.Set("Stripe-Signature", "t=1,v1=deadbeef")

	w := httptest.NewRecorder()
	p.StripeWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestWebhookRefund(t *testing.T) {
	p := newTestLED(t)

	w, req := signEvent(t, sessionEvent("evt_1", "paid"))
	p.StripeWebhook(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	refund := `{"id":"evt_2","object":"event","type":"charge.refunded","created":1700000001,
	"data":{"object":{"id":"ch_1","object":"charge","payment_intent":{"id":"pi_1"}}}}`
	w, req = signEvent(t, refund)
	p.StripeWebhook(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("refund status = %d", w.Code)
	}

	ds, err := p.donations.List("a.zz.ac", 20)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(ds) != 0 {
		t.Fatalf("refunded donation still listed: %+v", ds)
	}
}

func TestStripeDonateChecksAmount(t *testing.T) {
	p := newTestLED(t)

	body := `{"amount":100,"donor":"张三"}`
	req := httptest.NewRequest("POST", "http://a.zz.ac/+/stripe-donate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	p.StripeDonate(w, req)

	// 100 美元 = 10000 分，超过测试站点的 stripe_max=5000
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "$1.00") {
		t.Fatalf("message should name the range: %s", w.Body.String())
	}
}

func TestStripeDonateClosed(t *testing.T) {
	p := newTestLED(t)

	site := filepath.Join(p.Root, "b.zz.ac")
	if err := os.MkdirAll(site, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(site, "env"), []byte("site_title=b\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "http://b.zz.ac/+/stripe-donate",
		strings.NewReader(`{"amount":10}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	p.StripeDonate(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestStripeDonationsListsPublicFields(t *testing.T) {
	p := newTestLED(t)
	now := time.Now().Truncate(time.Second)

	for i, d := range []Donation{
		testDonation("a.zz.ac", "cs_1", now),
		testDonation("a.zz.ac", "cs_2", now.Add(time.Minute)),
	} {
		if err := p.donations.Record(testEvent("evt_"+string(rune('a'+i)), now), d); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	req := httptest.NewRequest("GET", "http://a.zz.ac/+/stripe-donations", nil)
	w := httptest.NewRecorder()
	p.StripeDonations(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	got := w.Body.String()
	if strings.Contains(got, "email") || strings.Contains(got, "session") {
		t.Fatalf("private field leaked: %s", got)
	}
	if !strings.Contains(got, `"$20.00"`) {
		t.Fatalf("amount not formatted: %s", got)
	}
	if w.Header().Get("Cache-Control") == "" {
		t.Fatal("missing Cache-Control")
	}
}

func TestStripeDonationsAnonymousName(t *testing.T) {
	p := newTestLED(t)
	now := time.Now().Truncate(time.Second)

	// 没填名字
	blank := testDonation("a.zz.ac", "cs_1", now)
	blank.Donor = ""

	// 勾了匿名，但名字还在库里
	flagged := testDonation("a.zz.ac", "cs_2", now.Add(time.Minute))
	flagged.Anonymous = true

	for i, d := range []Donation{blank, flagged} {
		if err := p.donations.Record(testEvent("evt_"+string(rune('a'+i)), now), d); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	req := httptest.NewRequest("GET", "http://a.zz.ac/+/stripe-donations", nil)
	w := httptest.NewRecorder()
	p.StripeDonations(w, req)

	var out []donateOutput
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("got %d donations, want 2", len(out))
	}
	for _, d := range out {
		if d.Donor != anonymousDonor {
			t.Errorf("donor = %q, want %q", d.Donor, anonymousDonor)
		}
	}
}

func TestFormatAmount(t *testing.T) {
	cases := []struct {
		minor int64
		cur   string
		want  string
	}{
		{2000, "usd", "$20.00"},
		{2050, "usd", "$20.50"},
		{1, "usd", "$0.01"},
		{2000, "eur", "€20.00"},
		{2000, "jpy", "¥2000"},
		{2000, "krw", "₩2000"},
		{2000, "xyz", "20.00 XYZ"},
	}

	for _, c := range cases {
		if got := formatAmount(c.minor, c.cur); got != c.want {
			t.Errorf("formatAmount(%d, %q) = %q, want %q", c.minor, c.cur, got, c.want)
		}
	}
}

func TestParseDonate(t *testing.T) {
	req := httptest.NewRequest("POST", "http://a.zz.ac/+/stripe-donate",
		strings.NewReader(`{"amount":20.5,"donor":"a","message":"m"}`))
	req.Header.Set("Content-Type", "application/json")

	in, err := parseDonate(req)
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	if in.Amount != 20.5 || in.Donor != "a" || in.Message != "m" {
		t.Fatalf("input = %+v", in)
	}

	bad := httptest.NewRequest("POST", "http://a.zz.ac/+/stripe-donate",
		strings.NewReader("not json"))
	bad.Header.Set("Content-Type", "application/json")
	if _, err := parseDonate(bad); err == nil {
		t.Fatal("bad body should fail")
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"abc", 5, "abc"},
		{"  abc  ", 5, "abc"},
		{"中文中文", 2, "中文"},
		{"abc", 0, ""},
	}

	for _, c := range cases {
		if got := truncate(c.in, c.n); got != c.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}
