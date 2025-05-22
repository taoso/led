package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/go-kiss/sqlx"
	_ "modernc.org/sqlite"
)

type Zone struct {
	ID    int       `db:"id"`
	Name  string    `db:"name"`
	Email string    `db:"email"`
	Owner string    `db:"owner"`
	Descr string    `db:"descr"`
	Time  time.Time `db:"time"`
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
        time DATETIME
);
        CREATE INDEX IF NOT EXISTS name ON ` + t.TableName() + `(name);`
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
	err = r.db.Get(
		&z,
		"select * from "+(*Zone).TableName(nil)+" where name = ?",
		name,
	)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
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
