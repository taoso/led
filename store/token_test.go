package store

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTokenRepo(t *testing.T) {
	f, err := os.CreateTemp("", "led-*.db")
	assert.Nil(t, err)
	f.Close()
	defer os.Remove(f.Name())

	repo := NewTokenRepo(f.Name())
	err = repo.Init()
	assert.Nil(t, err)

	l1 := TokenLog{
		Type:     LogTypeBuy,
		TokenNum: 1000,
		ExtraNum: 20,
		Extra: KV{
			"_pubkey":   "my-pubkey",
			"_buyer_id": "123",
		},
		Sign:    "my sign",
		Created: time.Now(),
	}

	w, err := repo.UpdateWallet(&l1)
	assert.Nil(t, err)
	assert.Equal(t, 1, l1.ID)
	assert.Equal(t, 1000, l1.AfterNum)
	assert.Equal(t, 1, w.ID)
	assert.Equal(t, 1000, w.Tokens)
	assert.Equal(t, "my-pubkey", w.Pubkey)
	assert.Equal(t, "123", w.Extra["alipay"])
	assert.Equal(t, l1.Created.Unix(), w.Created.Unix())

	l2 := TokenLog{
		Type:     LogTypeCost,
		UserID:   1,
		TokenNum: 100,
		ExtraNum: 30,
		Extra: KV{
			"sha256": "hehe",
		},
		Sign:    "my sign2",
		Created: time.Now(),
	}

	w, err = repo.UpdateWallet(&l2)
	assert.Nil(t, err)
	assert.Equal(t, 2, l2.ID)
	assert.Equal(t, 900, l2.AfterNum)
	assert.Equal(t, 1, w.ID)
	assert.Equal(t, 900, w.Tokens)

	l3 := TokenLog{
		Type:     LogTypeRefund,
		UserID:   1,
		TokenNum: 500,
		ExtraNum: 10,
		Extra:    KV{},
		Sign:     "my sign3",
		Created:  time.Now(),
	}

	w, err = repo.UpdateWallet(&l3)
	assert.Nil(t, err)
	assert.Equal(t, 3, l3.ID)
	assert.Equal(t, 400, l3.AfterNum)
	assert.Equal(t, 1, w.ID)
	assert.Equal(t, 400, w.Tokens)

	// 未指定用户ID则尝试使用公钥确定用户身份
	l4 := TokenLog{
		Type:     LogTypeBuy,
		UserID:   0,
		TokenNum: 1000,
		ExtraNum: 20,
		Extra:    KV{"_pubkey": "my-pubkey"},
		Sign:     "my sign3",
		Created:  time.Now(),
	}

	w, err = repo.UpdateWallet(&l4)
	assert.Nil(t, err)
	assert.Equal(t, 4, l4.ID)
	assert.Equal(t, 1400, l4.AfterNum)
	assert.Equal(t, 1, w.ID)
	assert.Equal(t, 1400, w.Tokens)

	logs, err := repo.ScanLogs(1, math.MaxInt, 2)
	assert.Nil(t, err)
	assert.Equal(t, 4, logs[0].ID)
	assert.Equal(t, 3, logs[1].ID)

	logs, err = repo.ScanLogs(1, logs[1].ID, 2)
	assert.Nil(t, err)
	assert.Equal(t, 2, logs[0].ID)
	assert.Equal(t, 1, logs[1].ID)

	w, err = repo.GetWallet(1)
	assert.Nil(t, err)
	assert.Equal(t, 1400, w.Tokens)
}

func TestSignData(t *testing.T) {
	now := time.Now()
	nows := now.UTC().Format("2006-01-02T15:04:05.000Z")

	l1 := TokenLog{
		Type:     LogTypeBuy,
		TokenNum: 1000,
		ExtraNum: 20,
		Extra: KV{
			"alipay": "123",
			"pubkey": "my-pubkey",
		},
		Created: now,
	}
	assert.Equal(t, nows+":0:1000:20", l1.SignData())

	l2 := TokenLog{
		Type:     LogTypeCost,
		TokenNum: 1000,
		ExtraNum: 30,
		Extra: KV{
			"sha256": "hex",
		},
		Created: now,
	}
	assert.Equal(t, nows+":1:hex", l2.SignData())

	l3 := TokenLog{
		Type:     LogTypeRefund,
		TokenNum: 1000,
		ExtraNum: 20,
		Extra:    KV{},
		Created:  now,
	}
	assert.Equal(t, nows+":2:1000:20", l3.SignData())
}

func TestSession(t *testing.T) {
	f, err := os.CreateTemp("", "led-*.db")
	assert.Nil(t, err)
	f.Close()
	defer os.Remove(f.Name())

	repo := NewTokenRepo(f.Name())
	err = repo.Init()
	assert.Nil(t, err)

	l1 := TokenLog{
		Type:     LogTypeBuy,
		TokenNum: 1000,
		ExtraNum: 20,
		Extra: KV{
			"_pubkey":   "foo",
			"_buyer_id": "123",
		},
		Sign:    "my sign",
		Created: time.Now(),
	}

	w, err := repo.UpdateWallet(&l1)
	assert.Nil(t, err)

	w.Username = "bob"
	w.SetPassword("alice")

	err = repo.SaveWallet(w)
	assert.Nil(t, err)

	w, err = repo.GetWallet(w.ID)
	assert.Nil(t, err)
	assert.Equal(t, "bob", w.Username)
	assert.True(t, w.CheckPassword("alice"))

	s1 := Session{
		UserID:  w.ID,
		Pubkey:  "bar",
		Agent:   "curl",
		Address: "1.1.1.1:1",
	}
	err = repo.AddSession(&s1)
	assert.Nil(t, err)

	w2, err := repo.FindWalletBySession(s1.ID)
	assert.Nil(t, err)
	assert.Equal(t, w.ID, w2.ID)
	assert.Equal(t, s1.Pubkey, w2.Pubkey)
}
