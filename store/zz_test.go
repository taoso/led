package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestZone(t *testing.T) {
	r := NewZoneRepo(":memory:")

	d1 := Zone{
		Name:  "foo.zz.ac",
		Email: "hi@foo.zz",
		Owner: "zz",
		Descr: "foo",
		Time:  time.Now().Truncate(time.Second),
	}

	err := r.New(&d1)
	assert.Nil(t, err)

	d2, err := r.Get("foo.zz.ac")
	assert.Nil(t, err)
	assert.Equal(t, 1, d1.ID)
	assert.Equal(t, d1, d2)

	d1.Owner = "zznic"

	err = r.Update(&d1)
	assert.Nil(t, err)
	d3, err := r.Get("foo.zz.ac")
	assert.Nil(t, err)
	assert.Equal(t, "zznic", d3.Owner)
}
