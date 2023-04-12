package store

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-kiss/sqlx"
	_ "modernc.org/sqlite"
)

var (
	ClientErr = errors.New("Client error")
	ServerErr = errors.New("Server error")
)

type LogType int

const (
	LogTypeBuy LogType = iota
	LogTypeCost
	LogTypeRefund
)

type TokenRepo struct {
	db *sqlx.DB
}

type TokenWallet struct {
	ID     int    `db:"id"`     // 钱包编号
	Tokens int    `db:"tokens"` // Token 余额
	Pubkey string `db:"pubkey"` // 用户签名公钥 P-256 ECDSA
	Extra  KV     `db:"extra"`  // 扩展信息，如支付宝ID等

	Created time.Time `db:"created"` // 创建时间
	Updated time.Time `db:"updated"` // 更新时间
}

func (w *TokenWallet) KeyName() string   { return "id" }
func (w *TokenWallet) TableName() string { return "token_wallets" }
func (w *TokenWallet) Schema() string {
	return `CREATE TABLE ` + w.TableName() + `(
	` + w.KeyName() + ` INTEGER PRIMARY KEY AUTOINCREMENT,
    	tokens INTEGER,
    	pubkey TEXT,
    	extra TEXT,
    	created DATETIME,
    	updated DATETIME
);`
}

type KV map[string]string

func (kv KV) Scan(value any) error {
	return json.Unmarshal([]byte(value.(string)), &kv)
}

func (kv KV) Value() (driver.Value, error) {
	if len(kv) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(kv)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// TokenLog 用户 Token 数量变化流水记录
type TokenLog struct {
	ID       int     `db:"id"`        // 日志编号
	UserID   int     `db:"user_id"`   // 用户标识
	Type     LogType `db:"type"`      // 流水日志类型
	TokenNum int     `db:"token_num"` // 需要更新的 Token 数量
	AfterNum int     `db:"after_num"` // 更新后的 Token 数量
	ExtraNum int     `db:"extra_num"` // 购买和退款时为法币金额，消耗时为请求 Token 数量
	Extra    KV      `db:"extra"`     // 扩展信息，如 ChatGPT 请求ID、请求内容摘要、支付宝ID等
	Sign     string  `db:"sign"`      // 用户签名，字段为 Created，充值退款还要签 AfterNum

	Created time.Time `db:"created"` // 创建时间，由客户端提供，不能跟服务器时间差距太大
}

func (l *TokenLog) KeyName() string   { return "id" }
func (l *TokenLog) TableName() string { return "token_logs" }
func (l *TokenLog) Schema() string {
	return `CREATE TABLE ` + l.TableName() + `(
	` + l.KeyName() + ` INTEGER PRIMARY KEY AUTOINCREMENT,
        user_id INTEGER NOT NULL,
        type INTEGER NOT NULL,
        token_num INTEGER NOT NULL,
        after_num INTEGER NOT NULL,
        extra_num INTEGER NOT NULL,
        extra TEXT NOT NULL,
        sign TEXT NOT NULL,
        created TIMESTAMP NOT NULL
); CREATE INDEX user_order ON ` + l.TableName() + `(user_id, id);`
}

// SignData 返回需要签名的数据
func (l *TokenLog) SignData() string {
	ss := []string{
		strconv.Itoa(int(l.Created.Unix())),
		strconv.Itoa(int(l.Type)),
	}
	switch l.Type {
	case LogTypeBuy, LogTypeRefund:
		ss = append(ss, strconv.Itoa(l.TokenNum))
		ss = append(ss, strconv.Itoa(l.ExtraNum))
	case LogTypeCost:
		ss = append(ss, l.Extra["sha256"])
	}
	return strings.Join(ss, ":")
}

func NewTokenRepo(path string) *TokenRepo {
	db, err := sqlx.Connect("sqlite", "file://"+path)
	if err != nil {
		panic(err)
	}
	return &TokenRepo{db: db}
}

func (r TokenRepo) Init() error {
	s1 := (*TokenWallet).Schema(nil)
	_, err := r.db.Exec(s1)
	if err != nil {
		return err
	}
	s2 := (*TokenLog).Schema(nil)
	_, err = r.db.Exec(s2)
	return err
}

func (r TokenRepo) GetWallet(id int) (w TokenWallet, err error) {
	err = r.db.Get(&w, "select * from "+w.TableName()+" where id = ?", id)
	return
}

func (r TokenRepo) newWallet(tx *sqlx.Tx, w *TokenWallet) error {
	res, err := tx.Insert(w)
	if err != nil {
		err = fmt.Errorf("%v %w", err, ServerErr)
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		err = fmt.Errorf("%v %w", err, ServerErr)
		return err
	}
	w.ID = int(id)
	return nil
}

func (r *TokenRepo) UpdateWallet(log *TokenLog) (w TokenWallet, err error) {
	tx, err := r.db.Beginx()
	if err != nil {
		err = fmt.Errorf("%v %w", err, ServerErr)
		return
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	now := time.Now()
	if log.UserID == 0 {
		w = TokenWallet{
			Tokens: log.TokenNum,
			Pubkey: log.Extra["pubkey"],
			Extra: KV{
				"alipay": log.Extra["alipay_buyer_id"],
			},
			Created: now,
			Updated: now,
		}
		delete(log.Extra, "pubkey")
		delete(log.Extra, "alipay_buyer_id")
		if err = r.newWallet(tx, &w); err != nil {
			return
		}
		log.UserID = w.ID
	} else {
		err = tx.Get(&w, "select * from "+w.TableName()+" where id = ?", log.UserID)
		if err != nil {
			err = fmt.Errorf("%v %w", err, ServerErr)
			return
		}
		if w.ID == 0 {
			err = fmt.Errorf("wallet not found %w", ClientErr)
			return
		}
		if w.Tokens <= 0 {
			err = fmt.Errorf("there is not enough tokens %w", ClientErr)
			return
		}
		if log.Type == LogTypeBuy {
			w.Tokens += log.TokenNum
		} else {
			w.Tokens -= log.TokenNum
		}
		w.Updated = now
		if _, err = tx.Update(&w); err != nil {
			err = fmt.Errorf("%v %w", err, ServerErr)
			return
		}
	}
	log.AfterNum = w.Tokens
	res, err := tx.Insert(log)
	if err != nil {
		err = fmt.Errorf("%v %w", err, ServerErr)
		return
	}
	id, err := res.LastInsertId()
	if err != nil {
		err = fmt.Errorf("%v %w", err, ServerErr)
		return
	}
	if err = tx.Commit(); err == nil {
		log.ID = int(id)
		return
	}
	err = fmt.Errorf("%v %w", err, ServerErr)
	return
}

func (r TokenRepo) ScanLogs(userID, last, num int) (logs []TokenLog, err error) {
	q := "select * from " + (&TokenLog{}).TableName() + " where " +
		"user_id = ? and id < ? order by id desc limit ?"
	err = r.db.Select(&logs, q, userID, last, num)
	return
}
