package led

import (
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/smartwalle/alipay/v3"
	"github.com/stripe/stripe-go/v87"
	"golang.org/x/net/idna"
)

// LED serves static sites published under the root dir
type LED struct {
	Root string

	mux    http.Handler
	once   sync.Once
	sites  sync.Map // host -> *FileHandler
	alipay *alipay.Client

	stripe    *stripe.Client
	whsec     string // stripe webhook signing secret
	donations *DonationRepo
}

// site returns the handler serving that host, creating it on first use.
// Hosts without their own directory fall back to root/default; only real
// sites are cached, so a newly created directory takes effect right away.
func (p *LED) site(host string) *FileHandler {
	if v, ok := p.sites.Load(host); ok {
		return v.(*FileHandler)
	}
	f := NewHandler(p.Root, host)
	if f.own {
		p.sites.Store(host, f)
	}
	return f
}

func (p *LED) host(host string) string {
	if i := strings.Index(host, ":"); i > 0 {
		host = host[:i]
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
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
	if u, err := url.Parse(origin); err == nil && u.Host != "" {
		if p.site(p.host(u.Host)).own {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
	}

	p.once.Do(func() { p.mux = p.routes() })
	p.mux.ServeHTTP(w, req)
}

func (p *LED) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /+/alipay-order-create", func(w http.ResponseWriter, req *http.Request) {
		p.AlipayOrderCreate(w, req, p.siteOf(req))
	})
	mux.HandleFunc("/+/alipay-order-notify", func(w http.ResponseWriter, req *http.Request) {
		p.AlipayOrderNotify(w, req, p.siteOf(req))
	})
	mux.HandleFunc("POST /+/mail", func(w http.ResponseWriter, req *http.Request) {
		p.Comment(p.host(req.Host), w, req)
	})
	mux.HandleFunc("POST /+/push", func(w http.ResponseWriter, req *http.Request) {
		p.siteOf(req).webPush(w, req)
	})
	mux.HandleFunc("POST /+/stripe-donate", p.StripeDonate)
	mux.HandleFunc("POST /+/stripe-webhook", p.StripeWebhook)
	mux.HandleFunc("GET /+/stripe-donations", p.StripeDonations)
	mux.HandleFunc("/", p.static)

	return mux
}

func (p *LED) siteOf(req *http.Request) *FileHandler {
	return p.site(p.host(req.Host))
}

func (p *LED) static(w http.ResponseWriter, req *http.Request) {
	f := p.siteOf(req)

	if strings.HasSuffix(req.URL.Path, "/index.htm") {
		localRedirect(w, req, "./")
		return
	}

	if f.Rewritten(w, req) {
		return
	}

	f.fs.ServeHTTP(w, req)
}
