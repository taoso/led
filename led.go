package led

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/taoso/led/pay"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/net/idna"
)

// Proxy serves static sites published under the root dir
type Proxy struct {
	sites map[string]*FileHandler
	users map[string]string

	Alipay *pay.Alipay

	AltSvc string

	DavEvs chan string
	Root   string
}

func (p *Proxy) auth(username, password string) bool {
	hash, ok := p.users[username]
	if !ok {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func (p *Proxy) SetUsers(users map[string]string) {
	p.users = users
}

func (p *Proxy) SetSites(sites map[string]string) {
	hs := make(map[string]*FileHandler, len(sites))
	for name := range sites {
		hs[name] = NewHandler(p.Root, name)
	}

	p.sites = hs
}

func (p *Proxy) MySite(name string) bool {
	_, ok := p.sites[name]
	return ok
}

func (p *Proxy) host(host string) string {
	if i := strings.Index(host, ":"); i > 0 {
		host = host[:i]
	}
	host, err := idna.ToUnicode(host)
	if err != nil {
		log.Println("host idna.ToUnicode error", host, err)
	}
	return host
}

func localRedirect(w http.ResponseWriter, r *http.Request, newPath string) {
	if q := r.URL.RawQuery; q != "" {
		newPath += "?" + q
	}
	w.Header().Set("Location", newPath)
	w.WriteHeader(http.StatusMovedPermanently)
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Alt-Svc", p.AltSvc)
	w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")

	origin := req.Header.Get("Origin")
	if u, err := url.Parse(origin); err == nil {
		if p.MySite(p.host(u.Host)) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
	}

	if ip := req.Header.Get("CF-Connecting-IP"); ip != "" {
		_, port, _ := net.SplitHostPort(req.RemoteAddr)
		req.RemoteAddr = ip + ":" + port
	}
	p.serveLocal(w, req)
}

func (p *Proxy) serveLocal(w http.ResponseWriter, req *http.Request) {
	host := p.host(req.Host)

	if strings.HasPrefix(req.RequestURI, "/+/dav-events") {
		username, password, ok := req.BasicAuth()
		if username != "" {
			req.URL.User = url.User(username)
		}
		if !ok || username != "webdav" || !p.auth(username, password) {
			w.Header().Set("WWW-Authenticate", `Basic realm="WebDAV"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		d, err := time.ParseDuration(req.URL.Query().Get("d"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		t := time.NewTimer(d)
		select {
		case e := <-p.DavEvs:
			evs := []string{e}
			// 合并通知短时间产生的文件变更
			time.Sleep(1 * time.Second)
			for {
				select {
				case e := <-p.DavEvs:
					evs = append(evs, e)
				default:
					goto resp
				}
			}
		resp:
			for _, e := range evs {
				fmt.Println("my", e)
				w.Write([]byte(e + "\n"))
			}
		case <-t.C:
			w.WriteHeader(http.StatusNoContent)
		}
		return
	}

	if f := p.sites[host]; f != nil {
		if strings.HasSuffix(req.RequestURI, "/index.htm") {
			localRedirect(w, req, "./")
			return
		}

		if req.RequestURI == "/+/alipay-order-create" && req.Method == http.MethodPost {
			p.AlipayOrderCreate(w, req, f)
			return
		}

		if req.RequestURI == "/+/alipay-order-notify" {
			p.AlipayOrderNotify(w, req, f)
			return
		}

		if req.RequestURI == "/+/mail" && req.Method == http.MethodPost {
			p.Comment(host, w, req)
			return
		}

		if req.RequestURI == "/+/push" && req.Method == http.MethodPost {
			f.webPush(w, req)
			return
		}

		if strings.HasPrefix(req.URL.Path, "/+/dav") {
			username, password, ok := req.BasicAuth()
			if username != "" {
				req.URL.User = url.User(username)
			}
			if !ok || username != f.Name || !p.auth(username, password) {
				w.Header().Set("WWW-Authenticate", `Basic realm="WebDAV"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			f.dav.ServeHTTP(w, req)
			p.SendDavEvent(req, host)
			return
		}

		if f.Rewritten(w, req) {
			return
		}

		f.fs.ServeHTTP(w, req)
		return
	}
}

func (p *Proxy) SendDavEvent(req *http.Request, host string) {
	if !strings.HasSuffix(req.URL.Path, ".md") {
		return
	}

	var e string
	switch req.Method {
	case "DELETE", "MOVE":
		e = "-" + host + "/" + strings.TrimLeft(req.URL.Path, "/+/dav/")
	case "COPY", "PUT":
		e = "+" + host
	}

	if e == "" {
		return
	}

	select {
	case p.DavEvs <- e:
	default:
	}
}
