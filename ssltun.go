package ssltun

import (
	"encoding/base64"
	"encoding/binary"
	"io"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/songgao/water"
)

// Proxy http proxy handler
type Proxy struct {
	// DomainName proxy server domain name
	DomainName string
	// Auth is function to check if username and password is match.
	Auth func(username, password string) bool

	FileHandler http.Handler
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Host == p.DomainName {
		if p.FileHandler != nil {
			p.FileHandler.ServeHTTP(w, req)
			return
		}

		// send default slogan
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("Across the Great Wall we can reach every corner in the world.\n"))
		return
	}

	auth := req.Header.Get("Proxy-Authorization")
	username, password, _ := parseBasicAuth(auth)
	if !p.Auth(username, password) {
		w.Header().Set("Proxy-Authenticate", `Basic realm="`+p.DomainName+`"`)
		w.WriteHeader(http.StatusProxyAuthRequired)
		return
	}

	if req.Method == http.MethodConnect {
		if req.ProtoMajor == 1 && req.RequestURI == "*" {
			proxyVPN(w, req)
			return
		}
		proxyHTTPS(w, req)
	} else {
		proxyHTTP(w, req)
	}
}

func proxyHTTPS(w http.ResponseWriter, req *http.Request) {
	address := req.RequestURI
	upConn, err := net.DialTimeout("tcp", address, 500*time.Millisecond)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer upConn.Close()

	var downConn io.ReadWriter
	if req.ProtoMajor == 2 {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		downConn = flushWriter{w: w, r: req.Body}
	} else {
		downConn, _, err = w.(http.Hijacker).Hijack()
		downConn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))
	}

	go func() {
		io.Copy(upConn, downConn)
	}()

	io.Copy(downConn, upConn)
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

func proxyVPN(w http.ResponseWriter, req *http.Request) (err error) {
	c, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		log.Println("hijack faild", err)
		return
	}
	defer c.Close()

	tun, err := water.New(water.Config{DeviceType: water.TUN})
	if err != nil {
		log.Println("create tun faild", err)
		return
	}
	defer tun.Close()

	hostIP := nextIP()
	clientIP := nextIP()
	defer releaseIP(hostIP, clientIP)

	log.Printf("host %s -> %s", hostIP, clientIP)

	args := []string{"link", "set", tun.Name(), "up"}
	if err = exec.Command("/usr/sbin/ip", args...).Run(); err != nil {
		log.Println("link set up", err)
		return
	}

	args = []string{"addr", "add", hostIP.String(), "peer", clientIP.String(), "dev", tun.Name()}
	if err = exec.Command("/usr/sbin/ip", args...).Run(); err != nil {
		log.Println("addr add faild", err)
		return
	}

	if _, err = c.Write(append(clientIP.To4(), hostIP.To4()...)); err != nil {
		return
	}

	go func() {
		defer tun.Close()
		buf := make([]byte, 10240)
		for {
			if _, err := io.ReadFull(c, buf[:4]); err != nil {
				log.Println("read ip length error", err)
				return
			}
			l := int(binary.BigEndian.Uint16(buf[2:4]))

			if _, err = io.ReadFull(c, buf[4:l]); err != nil {
				log.Println("read ip body error", err)
				return
			}

			if _, err := tun.Write(buf[:l]); err != nil {
				log.Println("send ip packet error", err)
				return
			}
		}
	}()

	io.CopyBuffer(c, tun, make([]byte, 10240))
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
