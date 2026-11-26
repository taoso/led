package led

import (
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/taoso/led/pay"
	"golang.org/x/net/idna"
)

// Proxy serves static sites published under the root dir
type Proxy struct {
	sites map[string]*FileHandler

	Alipay *pay.Alipay

	Root string
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

		if f.Rewritten(w, req) {
			return
		}

		f.fs.ServeHTTP(w, req)
		return
	}
}
