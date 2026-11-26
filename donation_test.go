package led

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newTestRepo(t *testing.T) *DonationRepo {
	t.Helper()

	repo, err := NewDonationRepo(filepath.Join(t.TempDir(), "led.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	return repo
}

func testDonation(site, session string, at time.Time) Donation {
	return Donation{
		SessionID: session,
		Site:      site,
		Amount:    2000,
		Currency:  "usd",
		Donor:     "张三",
		Message:   "写得好",
		Status:    "paid",
		Shown:     true,
		Created:   at,
	}
}

func testEvent(id string, at time.Time) stripeEvent {
	return stripeEvent{
		ID:      id,
		Type:    "checkout.session.completed",
		Created: at,
		Payload: []byte(`{"id":"` + id + `"}`),
	}
}

func TestRecordIsIdempotent(t *testing.T) {
	repo := newTestRepo(t)
	now := time.Now().Truncate(time.Second)

	if err := repo.Record(testEvent("evt_1", now), testDonation("a.zz.ac", "cs_1", now)); err != nil {
		t.Fatalf("first record: %v", err)
	}

	if err := repo.Record(testEvent("evt_1", now), testDonation("a.zz.ac", "cs_1", now)); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("replayed event: err = %v, want ErrDuplicate", err)
	}

	// 不同事件、同一 session 也不能重复记账
	if err := repo.Record(testEvent("evt_2", now), testDonation("a.zz.ac", "cs_1", now)); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("replayed session: err = %v, want ErrDuplicate", err)
	}

	ds, err := repo.List("a.zz.ac", 20)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(ds) != 1 {
		t.Fatalf("got %d donations, want 1", len(ds))
	}
	if ds[0].Amount != 2000 || ds[0].Currency != "usd" || ds[0].Donor != "张三" {
		t.Fatalf("unexpected donation: %+v", ds[0])
	}
}

func TestListFilters(t *testing.T) {
	repo := newTestRepo(t)
	base := time.Now().Truncate(time.Second)

	paid := testDonation("a.zz.ac", "cs_1", base)
	hidden := testDonation("a.zz.ac", "cs_2", base)
	hidden.Shown = false
	refunded := testDonation("a.zz.ac", "cs_3", base)
	refunded.Status = "refunded"
	other := testDonation("b.zz.ac", "cs_4", base)
	anon := testDonation("a.zz.ac", "cs_5", base.Add(time.Minute))
	anon.Anonymous = true
	anon.PaymentIntent = "pi_1"

	for i, d := range []Donation{paid, hidden, refunded, other, anon} {
		if err := repo.Record(testEvent("evt_"+string(rune('a'+i)), base), d); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}

	ds, err := repo.List("a.zz.ac", 20)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(ds) != 2 {
		t.Fatalf("got %d donations, want 2 (paid + anonymous)", len(ds))
	}
	// 最新在前，且 anonymous 标志能原样读回
	if ds[0].SessionID != "cs_5" || !ds[0].Anonymous {
		t.Fatalf("newest first or anonymous flag broken: %+v", ds[0])
	}

	ds, err = repo.List("a.zz.ac", 1)
	if err != nil {
		t.Fatalf("list limit: %v", err)
	}
	if len(ds) != 1 {
		t.Fatalf("limit ignored: got %d", len(ds))
	}

	if err := repo.SetStatus("pi_1", "refunded"); err != nil {
		t.Fatalf("set status: %v", err)
	}
	ds, err = repo.List("a.zz.ac", 20)
	if err != nil {
		t.Fatalf("list after refund: %v", err)
	}
	if len(ds) != 1 || ds[0].SessionID != "cs_1" {
		t.Fatalf("refunded donation still visible: %+v", ds)
	}
}
