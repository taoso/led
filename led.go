package led

import (
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/smartwalle/alipay/v3"
	"golang.org/x/net/idna"
)

// LED serves static sites published under the root dir
type LED struct {
	sites map[string]*FileHandler

	alipay *alipay.Client

	Root string
}

func (p *LED) SetSites(sites map[string]string) {
	hs := make(map[string]*FileHandler, len(sites))
	for name := range sites {
		hs[name] = NewHandler(p.Root, name)
	}

	p.sites = hs
}

func (p *LED) MySite(name string) bool {
	_, ok := p.sites[name]
	return ok
}

func (p *LED) host(host string) string {
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

func (p *LED) ServeHTTP(w http.ResponseWriter, req *http.Request) {
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

func (p *LED) serveLocal(w http.ResponseWriter, req *http.Request) {
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
