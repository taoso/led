package led

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBytesCounter(t *testing.T) {
	b := bytes.NewBuffer(nil)
	i := 0
	bc := bytesCounter{
		w: b,
		d: 100 * time.Millisecond,
		f: func(n int) {
			i += n
		},
	}

	go bc.Start()

	for _, s := range []string{"a", "bc", "def"} {
		bc.Write([]byte(s))
	}

	time.Sleep(1 * time.Second)

	bc.Done()

	assert.Equal(t, 6, i)
	assert.Equal(t, "abcdef", b.String())
}
