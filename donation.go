package led

import (
	"errors"
	"time"

	"github.com/go-kiss/sqlx"
	"modernc.org/sqlite"
)

// ErrDuplicate reports a Stripe event that has already been applied.
var ErrDuplicate = errors.New("duplicate stripe event")

// Donation is one paid donation, one row per Stripe Checkout Session.
type Donation struct {
	ID            int       `db:"id" json:"-"`
	SessionID     string    `db:"session_id" json:"-"`
	PaymentIntent string    `db:"payment_intent" json:"-"`
	Site          string    `db:"site" json:"-"`
	Amount        int       `db:"amount" json:"amount"`
	Currency      string    `db:"currency" json:"currency"`
	Donor         string    `db:"donor" json:"donor"`
	Message       string    `db:"message" json:"message"`
	Email         string    `db:"email" json:"-"` // 私有，永不返回
	Anonymous     bool      `db:"anonymous" json:"-"`
	Status        string    `db:"status" json:"-"` // paid / refunded
	Shown         bool      `db:"shown" json:"-"`  // 置 0 即可从页面下架
	Created       time.Time `db:"created" json:"created"`
}

func (*Donation) KeyName() string   { return "id" }
func (*Donation) TableName() string { return "donations" }
func (*Donation) Schema() string {
	return `CREATE TABLE IF NOT EXISTS donations(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id TEXT,
	payment_intent TEXT,
	site TEXT NOT NULL,
	amount INTEGER NOT NULL,
	currency TEXT NOT NULL,
	donor TEXT NOT NULL DEFAULT '',
	message TEXT NOT NULL DEFAULT '',
	email TEXT NOT NULL DEFAULT '',
	anonymous INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL DEFAULT 'paid',
	shown INTEGER NOT NULL DEFAULT 1,
	created DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS d_session ON donations(session_id);
CREATE INDEX IF NOT EXISTS d_site_created ON donations(site, created DESC);
CREATE INDEX IF NOT EXISTS d_payment_intent ON donations(payment_intent);`
}

// stripeEvent keeps every webhook delivery, so a replay is detectable.
type stripeEvent struct {
	ID      string    `db:"id"`
	Type    string    `db:"type"`
	Created time.Time `db:"created"`
	Payload []byte    `db:"payload"`
}

func (*stripeEvent) KeyName() string   { return "id" }
func (*stripeEvent) TableName() string { return "stripe_events" }
func (*stripeEvent) Schema() string {
	return `CREATE TABLE IF NOT EXISTS stripe_events(
	id TEXT PRIMARY KEY,
	type TEXT NOT NULL,
	created DATETIME NOT NULL,
	payload BLOB NOT NULL
);`
}

// DonationRepo stores donations in a sqlite file.
type DonationRepo struct {
	db *sqlx.DB
}

func NewDonationRepo(path string) (*DonationRepo, error) {
	db, err := sqlx.Connect("sqlite", path)
	if err != nil {
		return nil, err
	}
	// sqlite 只允许一个写者，单连接即可避免 BUSY 重试
	db.SetMaxOpenConns(1)

	for _, schema := range []string{
		(*stripeEvent)(nil).Schema(),
		(*Donation)(nil).Schema(),
	} {
		if _, err := db.Exec(schema); err != nil {
			db.Close()
			return nil, err
		}
	}

	return &DonationRepo{db: db}, nil
}

func (r *DonationRepo) Close() error { return r.db.Close() }

// isUnique reports a UNIQUE or PRIMARY KEY constraint violation.
// sqlite 把主键冲突和唯一索引冲突报成两个不同的扩展错误码。
func isUnique(err error) bool {
	se := &sqlite.Error{}
	if !errors.As(err, &se) {
		return false
	}
	return se.Code() == 1555 || se.Code() == 2067
}

// Record writes the event and the donation in one transaction.
// A replayed event, or a session already recorded, returns ErrDuplicate.
func (r *DonationRepo) Record(ev stripeEvent, d Donation) error {
	tx, err := r.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Insert(&ev); err != nil {
		if isUnique(err) {
			return ErrDuplicate
		}
		return err
	}

	if _, err := tx.Insert(&d); err != nil {
		if isUnique(err) {
			return ErrDuplicate
		}
		return err
	}

	return tx.Commit()
}

// List returns the most recent visible donations of one site.
func (r *DonationRepo) List(site string, limit int) ([]Donation, error) {
	var ds []Donation
	err := r.db.Select(&ds,
		`select * from donations
		 where site = ? and shown = 1 and status = 'paid'
		 order by created desc, id desc limit ?`, site, limit)
	return ds, err
}

// SetStatus changes the status of the donation paid by that PaymentIntent.
func (r *DonationRepo) SetStatus(paymentIntent, status string) error {
	_, err := r.db.Exec(
		`update donations set status = ? where payment_intent = ?`,
		status, paymentIntent)
	return err
}
