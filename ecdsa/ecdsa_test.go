package ecdsa

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVerifyES256(t *testing.T) {
	data := "hello"
	sign := "SnBIMSMsFZRiMhFQG65465iXDp3I4k6KDdbzo8w0jYinZlvXKhXG4CN5UzCanNP2MKGE9e1Hy6LLYf+inpCpDQ=="
	pubkey := "BKjQmlNjXfWLeprdKDpmdHNFQZz4mdQktEfXo0FsSj+r1gegK/6OPh/L4oKcfxl8P6tPa5EvTK3tibnOjlk2Vxs="

	pk, err := ParsePubkey(pubkey)
	assert.Nil(t, err)

	ok, err := VerifyES256(data, sign, pk)
	assert.Nil(t, err)
	assert.True(t, ok)
}

func TestCompress(t *testing.T) {
	pubkey := "BKjQmlNjXfWLeprdKDpmdHNFQZz4mdQktEfXo0FsSj+r1gegK/6OPh/L4oKcfxl8P6tPa5EvTK3tibnOjlk2Vxs="
	pk, err := ParsePubkey(pubkey)
	assert.Nil(t, err)

	assert.Equal(t, "A6jQmlNjXfWLeprdKDpmdHNFQZz4mdQktEfXo0FsSj+r", Compress(pk))
}

func TestGetPubkey(t *testing.T) {
	compkey := "A6jQmlNjXfWLeprdKDpmdHNFQZz4mdQktEfXo0FsSj+r"
	pubkey := "BKjQmlNjXfWLeprdKDpmdHNFQZz4mdQktEfXo0FsSj+r1gegK/6OPh/L4oKcfxl8P6tPa5EvTK3tibnOjlk2Vxs="

	pk1, err := GetPubkey(compkey)
	assert.Nil(t, err)

	pk2, err := ParsePubkey(pubkey)
	assert.Nil(t, err)

	assert.Equal(t, 0, pk1.X.Cmp(pk2.X))
	assert.Equal(t, 0, pk1.Y.Cmp(pk2.Y))
}
