package ecdsa

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"math/big"

	lru "github.com/hashicorp/golang-lru/v2"
)

type PublicKey = ecdsa.PublicKey

var keyCache *lru.Cache[string, ecdsa.PublicKey]

func init() {
	var err error
	keyCache, err = lru.New[string, ecdsa.PublicKey](1024)
	if err != nil {
		panic(err)
	}
}

// GetPubkey 从压缩格式公钥获取非压缩格式
//
// 使用 LRU 缓存结果，避免反复计算
func GetPubkey(compressed string) (pk ecdsa.PublicKey, err error) {
	if pk, ok := keyCache.Get(compressed); ok {
		return pk, nil
	}

	key, err := base64.StdEncoding.DecodeString(compressed)
	if err != nil {
		return
	}
	x, y := elliptic.UnmarshalCompressed(elliptic.P256(), key)
	pk = ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     x,
		Y:     y,
	}
	keyCache.Add(compressed, pk)
	return pk, nil
}

// ParsePubkey 解析非压缩格式公钥
func ParsePubkey(pubkey string) (pk ecdsa.PublicKey, err error) {
	key, err := base64.StdEncoding.DecodeString(pubkey)
	if err != nil {
		return
	}

	x, y := elliptic.Unmarshal(elliptic.P256(), key)
	pk = ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     x,
		Y:     y,
	}
	return
}

func Compress(k ecdsa.PublicKey) string {
	b := elliptic.MarshalCompressed(k.Curve, k.X, k.Y)
	return base64.StdEncoding.EncodeToString(b)
}

// VerifyES256 校验 ES256 签名
func VerifyES256(data, sign string, pubkey ecdsa.PublicKey) (ok bool, err error) {
	hash := sha256.Sum256([]byte(data))

	sig, err := base64.StdEncoding.DecodeString(sign)
	if err != nil {
		return
	}

	r := big.Int{}
	s := big.Int{}
	r.SetBytes(sig[:32])
	s.SetBytes(sig[32:])

	return ecdsa.Verify(&pubkey, hash[:], &r, &s), nil
}
