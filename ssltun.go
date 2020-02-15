package ssltun

import (
	"encoding/base64"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// Proxy http proxy handler
type Proxy struct {
	// Name proxy server domain name
	Name string
	// Key http basic authorization username
	Key string
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Host == p.Name {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("Across the Great Wall we can reach every corner in the world.\n"))
		return
	}

	auth := req.Header.Get("Proxy-Authorization")
	if u, _, _ := parseBasicAuth(auth); u != p.Key {
		w.Header().Set("Proxy-Authenticate", `Basic realm="`+p.Name+`"`)
		w.WriteHeader(http.StatusProxyAuthRequired)
		return
	}

	if req.Method == http.MethodGet {
		proxyHTTP(w, req)
		return
	}

	if req.Method != http.MethodConnect {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	proxyHTTPS(w, req)
}

func proxyHTTPS(w http.ResponseWriter, req *http.Request) {
	host := req.RequestURI
	if strings.LastIndex(host, ":") == -1 {
		host += ":443"
	}
	upConn, err := net.DialTimeout("tcp", host, 500*time.Millisecond)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer upConn.Close()

	w.WriteHeader(http.StatusOK)
	w.(http.Flusher).Flush()

	var downConn io.ReadWriter
	if req.ProtoMajor == 2 {
		downConn = flushWriter{w: w, r: req.Body}
	} else {
		downConn, _, err = w.(http.Hijacker).Hijack()
	}

	timeout := 15 * time.Minute
	go func() {
		iocopy(upConn, downConn, timeout)
	}()

	iocopy(downConn, upConn, timeout)
}

func proxyHTTP(w http.ResponseWriter, req *http.Request) {
	var url string
	if strings.HasPrefix(req.RequestURI, "http") {
		url = req.RequestURI
	} else {
		url = "http://" + req.Host + req.RequestURI
	}
	r, err := http.NewRequest(req.Method, url, req.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	h := req.Header.Clone()
	h.Del("Proxy-Authorization")
	h.Del("Te")
	h.Del("TransferEncoding")
	h.Del("Host")
	h.Set("Connection", "close")
	req.Header = h

	c := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// do not follow redirect response
			return http.ErrUseLastResponse
		},
	}

	resp, err := c.Do(r)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(err.Error()))
		return
	}

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
	return
}

type flushWriter struct {
	w io.Writer
	r io.Reader
}

func (fw flushWriter) Write(p []byte) (n int, err error) {
	n, err = fw.w.Write(p)
	fw.w.(http.Flusher).Flush()
	return
}

func (fw flushWriter) Read(p []byte) (n int, err error) {
	return fw.r.Read(p)
}

func (fw flushWriter) Close() error {
	return nil
}

// parseBasicAuth parses an HTTP Basic Authentication string.
// "Basic QWxhZGRpbjpvcGVuIHNlc2FtZQ==" returns ("Aladdin", "open sesame", true).
func parseBasicAuth(auth string) (username, password string, ok bool) {
	const prefix = "Basic "
	// Case insensitive prefix match. See Issue 22736.
	if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return
	}
	c, err := base64.StdEncoding.DecodeString(auth[len(prefix):])
	if err != nil {
		return
	}
	cs := string(c)
	s := strings.IndexByte(cs, ':')
	if s < 0 {
		return
	}
	return cs[:s], cs[s+1:], true
}

func iocopy(dst io.Writer, src io.Reader, timeout time.Duration) {
	size := 32 * 1024
	buf := make([]byte, size)

	timer := time.NewTimer(timeout)
	ch := make(chan bool, 0)

	go func() {
		defer func() { ch <- true }()

		for {
			n, err := src.Read(buf)
			if err != nil {
				log.Println("read", err)
				return
			}

			n, err = dst.Write(buf[:n])
			if err != nil {
				log.Println("write", err)
				return
			}

			if !timer.Reset(timeout) {
				return
			}
		}
	}()

	select {
	case <-ch:
		log.Println("finished")
	case <-timer.C:
		log.Println("timeout")
	}
	timer.Stop()
}
