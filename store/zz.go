package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/go-kiss/sqlx"
	_ "modernc.org/sqlite"
)

type Status int

const (
	StatusOK Status = iota
	StatusDeleted
)

type Zone struct {
	ID     int       `db:"id"`
	Name   string    `db:"name"`
	Email  string    `db:"email"`
	Owner  string    `db:"owner"`
	Descr  string    `db:"descr"`
	Time   time.Time `db:"time"`
	WebKey string    `db:"webkey"`
	Status Status    `db:"status"`
}

func (_ *Zone) KeyName() string   { return "id" }
func (_ *Zone) TableName() string { return "zones" }
func (t *Zone) Schema() string {
	return "CREATE TABLE IF NOT EXISTS " + t.TableName() + `(
        ` + t.KeyName() + ` INTEGER PRIMARY KEY AUTOINCREMENT,
        name TEXT,
        email TEXT,
        owner TEXT,
        descr TEXT,
        time DATETIME,
        webkey TEXT,
        status integer
);
        CREATE INDEX IF NOT EXISTS name ON ` + t.TableName() + `(name);
        CREATE INDEX IF NOT EXISTS email ON ` + t.TableName() + `(email);`
}

type ZoneRepo struct {
	db *sqlx.DB
}

func (r ZoneRepo) init() {
	s := (*Zone).Schema(nil)
	if _, err := r.db.Exec(s); err != nil {
		panic(err)
	}
}

func (r ZoneRepo) New(z *Zone) error {
	x, err := r.db.Insert(z)
	if err != nil {
		return err
	}

	id, err := x.LastInsertId()
	if err != nil {
		return err
	}

	z.ID = int(id)
	return nil
}

func (r ZoneRepo) Update(z *Zone) error {
	_, err := r.db.Update(z)
	return err
}

func (r ZoneRepo) Get(name string) (z Zone, err error) {
	name = strings.ToLower(name)
	err = r.db.Get(
		&z,
		"select * from "+(*Zone).TableName(nil)+" where name = ? and status = ?",
		name,
		StatusOK,
	)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}

func (r ZoneRepo) GetAll(name string) (zs []Zone, err error) {
	name = strings.ToLower(name)
	err = r.db.Select(
		&zs,
		"select * from "+(*Zone).TableName(nil)+" where name = ? order by id desc",
		name,
	)
	return
}

func (r ZoneRepo) ListByEmail(email string) (zs []Zone, err error) {
	email = strings.ToLower(email)
	err = r.db.Select(
		&zs,
		"select * from "+(*Zone).TableName(nil)+" where email = ? order by id desc",
		email,
	)
	return
}

func NewZoneRepo(path string) ZoneRepo {
	db, err := sqlx.Connect("sqlite", path)
	if err != nil {
		panic(err)
	}
	db.SetMaxOpenConns(1)

	r := ZoneRepo{db: db}
	r.init()

	return r
}
