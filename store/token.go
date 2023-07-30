package store

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-kiss/sqlx"
	"github.com/taoso/led/ecdsa"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

var (
	ClientErr = errors.New("Client error")
	ServerErr = errors.New("Server error")
)

type LogType int

const (
	LogTypeBuy    = LogType(0)
	LogTypeCost   = LogType(1)
	LogTypeRefund = LogType(2)
	LogTypeInvite = LogType(3)
)

type TokenRepo struct {
	db *sqlx.DB
}

type Session struct {
	ID      int       `db:"id"`      // 会话编号
	UserID  int       `db:"user_id"` // 用户编号
	Pubkey  string    `db:"pubkey"`  // 用户签名公钥 P-256 ECDSA，压缩，base64
	Agent   string    `db:"agent"`   // 用户 User-Agent
	Address string    `db:"address"` // 登录时的 IP:Port
	Created time.Time `db:"created"` // 创建时间
}

func (s *Session) GetPubkey() (ecdsa.PublicKey, error) {
	return ecdsa.GetPubkey(s.Pubkey)
}

func (_ *Session) KeyName() string   { return "id" }
func (_ *Session) TableName() string { return "sessions" }
func (s *Session) Schema() string {
	return `CREATE TABLE ` + s.TableName() + `(
	` + s.KeyName() + ` INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER,
	pubkey TEXT,
	agent TEXT,
	address TEXT,
	created DATETIME
); 
	CREATE INDEX s_user_id ON ` + s.TableName() + `(user_id);
	CREATE UNIQUE INDEX s_pubkey ON ` + s.TableName() + `(pubkey);`
}

type TokenWallet struct {
	ID     int    `db:"id"`     // 钱包编号
	Tokens int    `db:"tokens"` // Token 余额
	Pubkey string `db:"pubkey"` // 用户签名公钥 P-256 ECDSA，压缩，base64

	Username string `db:"username"`          // 登录名字
	Password []byte `db:"password" json:"-"` // 登录密码
	Extra    KV     `db:"extra" json:"-"`    // 扩展信息，如支付宝ID等
	FromID   int    `db:"from_id"`           // 邀请我的人

	InviteTokens int `db:"invite_tokens"` // 我邀请的奖励
	InviteUsers  int `db:"invite_users"`  // 我邀请的人数

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
	username TEXT default '',
	password BLOB default '',
	from_id INTEGER default 0,
	invite_tokens INTEGER default 0,
	invite_users INTEGER default 0,
    	created DATETIME,
    	updated DATETIME
); 
	CREATE INDEX w_from_id ON ` + w.TableName() + `(from_id);
	CREATE UNIQUE INDEX pubkey ON ` + w.TableName() + `(pubkey);`
}

func (w *TokenWallet) GetPubkey() (ecdsa.PublicKey, error) {
	return ecdsa.GetPubkey(w.Pubkey)
}

func (w *TokenWallet) SetPassword(value string) (err error) {
	w.Password, err = bcrypt.GenerateFromPassword([]byte(value), 16)
	return
}

func (w *TokenWallet) CheckPassword(value string) bool {
	return bcrypt.CompareHashAndPassword(w.Password, []byte(value)) == nil
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
	PayNo    string  `db:"pay_no"`    // 支付业务订单号
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
        pay_no TEXT NOT NULL,
        created TIMESTAMP NOT NULL
); 
	CREATE INDEX user_order ON ` + l.TableName() + `(user_id, id);
	CREATE INDEX pay_no ON ` + l.TableName() + `(pay_no) where pay_no != "";`
}

// SignData 返回需要签名的数据
func (l *TokenLog) SignData() string {
	ss := []string{
		// see js Date.toISOString
		l.Created.UTC().Format("2006-01-02T15:04:05.000Z"),
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

func (r *TokenRepo) Init() error {
	_, err := r.db.Exec((*TokenWallet).Schema(nil))
	if err != nil {
		panic(err)
		return err
	}
	_, err = r.db.Exec((*TokenLog).Schema(nil))
	if err != nil {
		panic(err)
		return err
	}
	_, err = r.db.Exec((*Session).Schema(nil))
	if err != nil {
		panic(err)
	}
	return err
}

func (r *TokenRepo) FindWallet(pubkey string) (w TokenWallet, err error) {
	err = r.db.Get(&w, "select * from "+w.TableName()+" where pubkey = ?", pubkey)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}

func (r *TokenRepo) FindWalletByName(username string) (w TokenWallet, err error) {
	err = r.db.Get(&w, "select * from "+w.TableName()+" where username = ?", username)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}

func (r *TokenRepo) GetSession(id int) (s Session, err error) {
	err = r.db.Get(&s, "select * from "+s.TableName()+" where id = ?", id)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}

func (r *TokenRepo) FindWalletBySession(id int) (w TokenWallet, err error) {
	var s Session
	err = r.db.Get(&s, "select * from "+s.TableName()+" where id = ?", id)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return
	}
	w, err = r.GetWallet(s.UserID)
	if err != nil {
		return
	}
	w.Pubkey = s.Pubkey
	return
}

func (r *TokenRepo) GetWallet(id int) (w TokenWallet, err error) {
	err = r.db.Get(&w, "select * from "+w.TableName()+" where id = ?", id)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}

func (r *TokenRepo) SaveWallet(w TokenWallet) error {
	w.Updated = time.Now()
	_, err := r.db.Update(&w)
	return err
}

func (r *TokenRepo) AddSession(s *Session) error {
	s.Created = time.Now()
	x, err := r.db.Insert(s)
	if err != nil {
		return err
	}
	id, err := x.LastInsertId()
	if err != nil {
		return err
	}
	s.ID = int(id)
	return nil
}

func (r *TokenRepo) ListSession(uid int) (s []Session, err error) {
	err = r.db.Select(&s, "select * from "+(&Session{}).TableName()+" where user_id = ?", uid)
	return
}

func (r *TokenRepo) DelSession(id, uid int) (err error) {
	_, err = r.db.Exec("delete from "+(&Session{}).TableName()+" where id = ? and user_id = ?", id, uid)
	return
}

func (r *TokenRepo) newWallet(tx *sqlx.Tx, w *TokenWallet) error {
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
	if log.UserID == 0 && log.Extra["_pubkey"] != "" { // 老用户在新设备登录场景
		err = tx.Get(&w, "select * from "+w.TableName()+" where pubkey = ?", log.Extra["_pubkey"])
		if !errors.Is(err, sql.ErrNoRows) && err != nil {
			err = fmt.Errorf("%v %w", err, ServerErr)
			return
		}
		if w.ID != 0 {
			log.UserID = w.ID
		}
	}
	if log.UserID == 0 { // 新用户场景
		w = TokenWallet{
			Tokens: log.TokenNum,
			Pubkey: log.Extra["_pubkey"],
			Extra: KV{
				"alipay": log.Extra["_buyer_id"],
			},
			Created: now,
			Updated: now,
		}
		if f := log.Extra["_from_id"]; f != "" {
			if i, err := strconv.Atoi(f); err == nil {
				w.FromID = i
			}
		}
		if err = r.newWallet(tx, &w); err != nil {
			return
		}
		log.UserID = w.ID
	} else {
		if w.ID == 0 { // 前面使用公钥查没查到，继续使用用户ID查找
			err = tx.Get(&w, "select * from "+w.TableName()+" where id = ?", log.UserID)
			if err != nil {
				err = fmt.Errorf("%v %w", err, ServerErr)
				return
			}
			if w.ID == 0 {
				err = fmt.Errorf("wallet not found %w", ClientErr)
				return
			}
		}
		if log.Type == LogTypeBuy {
			w.Tokens += log.TokenNum
		} else {
			if w.Tokens <= 0 {
				err = fmt.Errorf("there is not enough tokens %w", ClientErr)
				return
			}
			w.Tokens -= log.TokenNum
		}
		w.Updated = now
		if _, err = tx.Update(&w); err != nil {
			err = fmt.Errorf("%v %w", err, ServerErr)
			return
		}
	}
	log.AfterNum = w.Tokens

	for k := range log.Extra {
		if strings.HasPrefix(k, "_") {
			delete(log.Extra, k)
		}
	}

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

	if log.Type == LogTypeBuy && w.FromID > 0 {
		var fw TokenWallet
		fw, err = r.GetWallet(w.FromID)
		if err != nil {
			err = fmt.Errorf("%v %w", err, ServerErr)
			return
		}
		if fw.ID != 0 {
			t := log.TokenNum / 10
			fw.Tokens += t
			fw.InviteUsers += 1
			fw.InviteTokens += t
			if _, err = tx.Update(&fw); err != nil {
				err = fmt.Errorf("%v %w", err, ServerErr)
				return
			}
			fo := TokenLog{
				UserID:   fw.ID,
				Type:     LogTypeInvite,
				TokenNum: t,
				AfterNum: fw.Tokens,
				Extra: map[string]string{
					"from_id":  strconv.Itoa(w.ID),
					"from_oid": strconv.FormatInt(id, 10),
				},
				Created: log.Created,
			}
			if _, err = tx.Insert(&fo); err != nil {
				err = fmt.Errorf("%v %w", err, ServerErr)
				return
			}
		}
	}

	if err = tx.Commit(); err == nil {
		log.ID = int(id)
		return
	}
	err = fmt.Errorf("%v %w", err, ServerErr)
	return
}

func (r *TokenRepo) ScanLogs(userID, last, num int) (logs []TokenLog, err error) {
	q := "select * from " + (&TokenLog{}).TableName() + " where " +
		"user_id = ? and id < ? order by id desc limit ?"
	err = r.db.Select(&logs, q, userID, last, num)
	return
}

func (r *TokenRepo) FindLog(payNo string) (log TokenLog, err error) {
	q := "select * from " + (&TokenLog{}).TableName() + " where pay_no = ?"
	err = r.db.Get(&log, q, payNo)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}
