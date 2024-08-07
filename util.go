package led

import (
	"io"
	"sync/atomic"
	"time"
)

type bytesCounter struct {
	w io.ReadWriter
	f func(n int)
	d time.Duration

	c atomic.Int64
	t *time.Ticker
	s chan int
}

func (bc *bytesCounter) Done() {
	bc.t.Stop()
	close(bc.s)
}

func (bc *bytesCounter) Start() {
	bc.s = make(chan int, 1)
	bc.t = time.NewTicker(bc.d)

	for {
		select {
		case <-bc.t.C:
			if n := bc.c.Swap(0); n > 0 {
				bc.f(int(n))
			}
		case <-bc.s:
			if n := bc.c.Swap(0); n > 0 {
				bc.f(int(n))
			}
			return
		}
	}
}

func (bc *bytesCounter) Write(p []byte) (n int, err error) {
	n, err = bc.w.Write(p)
	bc.c.Add(int64(n))
	return
}

func (bc *bytesCounter) Read(p []byte) (n int, err error) {
	n, err = bc.w.Read(p)
	bc.c.Add(int64(n))
	return
}
