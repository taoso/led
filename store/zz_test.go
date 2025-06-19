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

func TestGetAll(t *testing.T) {
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

	d2 := Zone{
		Name:   "foo.zz.ac",
		Email:  "hi@foo.zz",
		Owner:  "zz",
		Descr:  "foo",
		Time:   time.Now().Truncate(time.Second),
		Status: StatusDeleted,
	}

	err = r.New(&d2)
	assert.Nil(t, err)

	d3, err := r.Get("foo.zz.ac")
	assert.Nil(t, err)
	assert.Equal(t, d1.ID, d3.ID)
	assert.Equal(t, StatusOK, d3.Status)

	ds, err := r.GetAll("foo.zz.ac")
	assert.Nil(t, err)
	assert.Equal(t, d2.ID, ds[0].ID)
	assert.Equal(t, StatusDeleted, ds[0].Status)
	assert.Equal(t, d1.ID, ds[1].ID)
	assert.Equal(t, StatusOK, ds[1].Status)

	ds, err = r.ListByEmail("hi@foo.zz")
	assert.Nil(t, err)
	assert.Equal(t, d2.ID, ds[0].ID)
	assert.Equal(t, d1.ID, ds[1].ID)
}
