package store

import (
	"errors"
	"time"

	"github.com/go-kiss/sqlx"
	"modernc.org/sqlite"
)

type Ticket struct {
	ID         int    `db:"id" json:"id"`
	Token      string `db:"token" json:"-"`
	Bytes      int    `db:"bytes" json:"bytes"`
	TotalBytes int    `db:"total_bytes" json:"total_bytes"`
	PayOrder   string `db:"pay_order" json:"pay_order"`
	BuyOrder   string `db:"buy_order" json:"buy_order"`

	Created time.Time `db:"created" json:"created"`
	Updated time.Time `db:"updated" json:"updated"`
	Expires time.Time `db:"expires" json:"expires"`
}

func (_ *Ticket) KeyName() string   { return "id" }
func (_ *Ticket) TableName() string { return "tickets" }
func (t *Ticket) Schema() string {
	return "CREATE TABLE IF NOT EXISTS " + t.TableName() + `(
	` + t.KeyName() + ` INTEGER PRIMARY KEY AUTOINCREMENT,
	token TEXT,
	bytes INTEGER,
	total_bytes INTEGER,
	pay_order TEXT,
	buy_order TEXT,
	created DATETIME,
	updated DATETIME,
	expires DATETIME
);
	CREATE INDEX IF NOT EXISTS t_token_expires ON ` + t.TableName() + `(token, expires);
	CREATE UNIQUE INDEX IF NOT EXISTS t_pay_order ON ` + t.TableName() + `(pay_order);`
}

type TicketRepo interface {
	// New create and save one Ticket
	New(token string, bytes, days int, trade string, order string) error
	// Cost decreases  bytes of one Ticket
	Cost(token string, bytes int) error
	// List fetches all current Tickets with bytes available.
	List(token string, limit int) ([]Ticket, error)
}

func NewTicketRepo(path string) TicketRepo {
	db, err := sqlx.Connect("sqlite", path)
	if err != nil {
		panic(err)
	}
	db.SetMaxOpenConns(1)
	r := sqliteTicketReop{db: db}
	r.Init()
	return r
}

type FreeTicketRepo struct{}

func (r FreeTicketRepo) New(token string, bytes, days int, trade, order string) error {
	return nil
}

func (r FreeTicketRepo) Cost(token string, bytes int) error {
	return nil
}

func (r FreeTicketRepo) List(token string, limit int) ([]Ticket, error) {
	return []Ticket{{Bytes: 100}}, nil
}

type sqliteTicketReop struct {
	db *sqlx.DB
}

func (r sqliteTicketReop) Init() {
	if _, err := r.db.Exec((*Ticket).Schema(nil)); err != nil {
		panic(err)
	}
}

func (r sqliteTicketReop) New(token string, bytes, days int, trade, order string) error {
	now := time.Now()
	begin := time.Now()

	ts, err := r.List(token, 1)
	if err != nil {
		return err
	}

	if len(ts) == 1 {
		begin = ts[0].Expires
	}

	t := Ticket{
		Token:      token,
		Bytes:      bytes,
		TotalBytes: bytes,
		PayOrder:   order,
		BuyOrder:   trade,
		Created:    now,
		Updated:    now,
		Expires:    begin.AddDate(0, 0, days),
	}

	_, err = r.db.Insert(&t)

	se := &sqlite.Error{}
	// constraint failed: UNIQUE constraint failed
	if errors.As(err, &se) && se.Code() == 2067 {
		return nil
	}

	return err
}

func (r sqliteTicketReop) Cost(token string, bytes int) error {
	now := time.Now()

	sql := "update " + (*Ticket).TableName(nil) +
		" set bytes = bytes - ?, updated = ? where id in (select id from " + (*Ticket).TableName(nil) +
		" where token = ? and expires > ? order by id asc limit 1) and bytes >= ?"

	_r, err := r.db.Exec(sql, bytes, now, token, now, bytes)
	if err != nil {
		return err
	}
	n, err := _r.RowsAffected()
	if err != nil {
		return err
	}
	if n == 1 {
		return nil
	}

	return r.costSlow(token, bytes)
}

func (r sqliteTicketReop) costSlow(token string, bytes int) error {
	sql := "select * from " + (*Ticket).TableName(nil) +
		" where token = ? and bytes > 0 and expires > ?" +
		" order by id asc"
	var ts []Ticket
	if err := r.db.Select(&ts, sql, token, time.Now()); err != nil {
		return err
	}

	var i int
	var t Ticket
	for i, t = range ts {
		if t.Bytes >= bytes {
			ts[i].Bytes -= bytes
			bytes = 0
			break
		} else {
			bytes -= t.Bytes
			ts[i].Bytes = 0
		}
	}

	if bytes > 0 {
		ts[i].Bytes -= bytes
	}

	if i == 0 {
		t := ts[i]
		t.Updated = time.Now()
		_, err := r.db.Update(&t)
		return err
	}

	tx, err := r.db.Beginx()
	if err != nil {
		return err
	}

	for ; i >= 0; i-- {
		t := ts[i]
		t.Updated = time.Now()
		_, err := tx.Update(&t)
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (r sqliteTicketReop) List(token string, limit int) (tickets []Ticket, err error) {
	sql := "select * from " + (*Ticket).TableName(nil) +
		" where token = ? order by id desc limit ?"
	err = r.db.Select(&tickets, sql, token, limit)
	return
}
