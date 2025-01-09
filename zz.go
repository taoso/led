package led

import (
	"crypto/hmac"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/jhillyerd/enmime"
	"github.com/miekg/dns"
)

//go:embed *.html
var htmls embed.FS

func (p *Proxy) zone(w http.ResponseWriter, req *http.Request) {
	if a := req.FormValue("a"); a == "link" {
		p.zoneLink(w, req)
		return
	}

	n := req.URL.Query().Get("n")
	t := req.URL.Query().Get("t")
	s := req.URL.Query().Get("s")

	s1, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h := hmac.New(sha256.New, p.signKey)
	h.Write([]byte(n + "@" + t))
	s2 := h.Sum(nil)

	if !hmac.Equal(s1, s2) {
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}

	i, err := strconv.Atoi(t)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tt := time.Unix(int64(i), 0)
	if time.Now().Sub(tt) > 1*time.Hour {
		http.Error(w, "The link has expired", http.StatusBadRequest)
		return
	}

	if req.Method == http.MethodGet {
		p.zoneGet(w, req)
	} else {
		p.zonePut(w, req)
	}
}

func (p *Proxy) zoneLink(w http.ResponseWriter, req *http.Request) {
	email := req.FormValue("e")
	domain := req.FormValue("d")

	if p.zones[domain] != email {
		http.Error(w, "invalid argument", http.StatusBadRequest)
		return
	}

	h := hmac.New(sha256.New, p.signKey)

	n := time.Now()
	t := n.Unix()
	ts := strconv.Itoa(int(t))

	h.Write([]byte(domain + "@" + ts))
	s := h.Sum(nil)
	sign := base64.URLEncoding.EncodeToString(s)

	link := fmt.Sprintf(
		"https://%s%s?n=%s&t=%s&s=%s",
		req.Host, req.URL.Path,
		domain, ts, sign,
	)

	content := "Your ZZ.AC Zone Editor Link is:\n" +
		"\n" +
		link + "\n" +
		"\n" +
		"This link will expire after one hour."

	m := enmime.Builder().
		From("zz.ac", os.Getenv("SMTP_USER")).
		To("", email).
		Subject("ZZ.AC Zone Editor Link").
		Text([]byte(content))

	ss := TLSSender{
		Username: os.Getenv("SMTP_USER"),
		Password: os.Getenv("SMTP_PASS"),
		Hostaddr: os.Getenv("SMTP_HOST"),
	}

	if err := m.Send(ss); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
	}
}

func (p *Proxy) zoneGet(w http.ResponseWriter, req *http.Request) {
	tmpl, err := template.ParseFS(htmls, "*.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	name := req.FormValue("n")

	db := p.zPath + "/" + name + ".zz.ac.zone"

	b, err := os.ReadFile(db)
	if err != nil && !os.IsNotExist(err) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "zone.html", struct {
		Zone   string
		Domain string
		Error  error
		Msg    string
	}{
		Zone:   string(b),
		Domain: name + ".zz.ac",
	})
}

func (p *Proxy) zonePut(w http.ResponseWriter, req *http.Request) {
	tmpl, err := template.ParseFS(htmls, "*.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	name := req.FormValue("n")
	zone := req.FormValue("zone")

	d := struct {
		Zone   string
		Domain string
		Error  error
		Msg    string
	}{
		Zone:   zone,
		Domain: name + ".zz.ac",
	}

	d.Error = parseZone(name+".zz.ac.", zone)
	if d.Error == nil {
		db := p.zPath + "/" + name + ".zz.ac.zone"
		d.Error = os.WriteFile(db, []byte(zone), os.FileMode(0644))
		if d.Error == nil {
			d.Msg = "Zone updated."
		}
	}

	tmpl.ExecuteTemplate(w, "zone.html", d)
}

func parseZone(origin, zone string) error {
	f := strings.NewReader(zone)
	zp := dns.NewZoneParser(f, origin, "")

	for r, ok := zp.Next(); ok; r, ok = zp.Next() {
		h := r.Header()
		if h.Ttl < 60 {
			return fmt.Errorf("TTL must be longer than %ds.", 60)
		}
		if !strings.HasSuffix(h.Name, origin) {
			return fmt.Errorf("Subdomain %s does not belongs to %s.", h.Name, origin)
		}
		if h.Class != dns.ClassINET {
			return fmt.Errorf("Class of %s is not supprted now.", dns.Class(h.Class).String())
		}
		if h.Rrtype == dns.TypeNS {
			return fmt.Errorf("NS record is not supported now.")
		}
		if h.Rrtype == dns.TypeSOA {
			return fmt.Errorf("You must not change SOA record.")
		}
	}
	return zp.Err()
}
