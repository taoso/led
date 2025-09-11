package led

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/json"
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

type Zone struct {
	Domain string
	Email  string
	Owner  string
	Desc   string
	Date   string
}

func (p *Proxy) zone(w http.ResponseWriter, req *http.Request) {
	a := req.FormValue("a")
	switch a {
	case "webdav":
		p.zoneWebDAV(w, req)
		return
	case "link":
		p.zoneLink(w, req)
		return
	case "whois":
		p.zoneWhois(w, req)
		return
	case "apply":
		p.zoneApply(w, req)
		return
	}

	n := req.URL.Query().Get("n")
	t := req.URL.Query().Get("t")
	s := req.URL.Query().Get("s")

	s1, err := base64.RawURLEncoding.DecodeString(s)
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

	switch a {
	case "v1":
		p.zoneApplyVerifyEmail(w, req)
		return
	case "v2":
		p.zoneApplyAuth(w, req)
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

	z, err := p.ZoneRepo.Get(domain)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if z.Email != email {
		http.Error(w, "invalid argument", http.StatusBadRequest)
		return
	}

	h := hmac.New(sha256.New, p.signKey)

	n := time.Now()
	t := n.Unix()
	ts := strconv.Itoa(int(t))

	h.Write([]byte(domain + "@" + ts))
	s := h.Sum(nil)
	sign := base64.RawURLEncoding.EncodeToString(s)

	auth := fmt.Sprintf("?n=%s&t=%s&s=%s", domain, ts, sign)

	var link string

	if url := req.FormValue("url"); url != "" {
		link = url + auth
	} else {
		link = "https://" + req.Host + req.URL.Path + auth
	}

	content := "Hi, " + z.Owner + "\n\n" +
		"Your ZZ.AC Zone Editor Link is:\n" +
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

func (p *Proxy) zoneWebDAV(w http.ResponseWriter, req *http.Request) {
	email := req.FormValue("e")
	domain := req.FormValue("d")

	z, err := p.ZoneRepo.Get(domain)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if z.Email != email {
		http.Error(w, "invalid argument", http.StatusBadRequest)
		return
	}

	if z.WebKey == "" {
		http.Error(w, "Premium Feature! Apply by Email!", http.StatusBadRequest)
		return
	}

	root := p.Root + "/" + domain + ".zz.ac"
	if err := os.Mkdir(root, 0755); err != nil && !os.IsExist(err) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	key := make([]byte, 32)
	rand.Read(key)
	z.WebKey = base64.RawURLEncoding.EncodeToString(key)

	if err := p.ZoneRepo.Update(&z); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	content := "Hi, " + z.Owner + "\n\n" +
		"Your ZZ.AC WebDAV Details are:\n" +
		"\n" +
		"- URL: " + "https://" + domain + ".zz.ac\n" +
		"- Username: " + email + "\n" +
		"- Password: " + z.WebKey + "\n"

	m := enmime.Builder().
		From("zz.ac", os.Getenv("SMTP_USER")).
		To("", email).
		Subject("ZZ.AC WebDAV Details").
		Text([]byte(content))

	ss := TLSSender{
		Username: os.Getenv("SMTP_USER"),
		Password: os.Getenv("SMTP_PASS"),
		Hostaddr: os.Getenv("SMTP_HOST"),
	}

	if err := m.Send(ss); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
	}
}

func (p *Proxy) zoneGet(w http.ResponseWriter, req *http.Request) {
	name := req.FormValue("n")

	db := p.zPath + "/zz.ac/" + name + ".zone"

	b, err := os.ReadFile(db)
	if err != nil && !os.IsNotExist(err) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if req.URL.Query().Get("api") != "" {
		w.Write(b)
		return
	}

	tmpl, err := template.ParseFS(htmls, "*.html")
	if err != nil {
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

func (p *Proxy) zoneWhois(w http.ResponseWriter, req *http.Request) {
	name := req.FormValue("n")

	z, err := p.ZoneRepo.Get(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("content-type", "application/json")

	z.Email = z.Name + "@zz.ac"
	z.WebKey = ""

	json.NewEncoder(w).Encode(z)
}

func (p *Proxy) zonePut(w http.ResponseWriter, req *http.Request) {
	name := req.FormValue("n")
	zone := req.FormValue("zone")
	token := req.FormValue("desec-token")

	d := struct {
		Zone   string
		Domain string
		Error  error
		Msg    string
	}{
		Zone:   zone,
		Domain: name + ".zz.ac",
	}

	if token != "" {
		zone, d.Error = deSecZone(name+".zz.ac", token)
	} else {
		d.Error = parseZone(name+".zz.ac.", zone)
	}

	if d.Error == nil {
		db := p.zPath + "/zz.ac/" + name + ".zone"
		d.Error = os.WriteFile(db, []byte(zone), os.FileMode(0644))
		if d.Error == nil {
			d.Msg = "Zone updated."
		}
	}

	if req.URL.Query().Get("api") != "" {
		var err string
		if d.Error != nil {
			err = d.Error.Error()
		}
		json.NewEncoder(w).Encode(struct {
			Err string `json:"err"`
		}{err})
		return
	}

	tmpl, err := template.ParseFS(htmls, "*.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "zone.html", d)
}

func (p *Proxy) zoneApply(w http.ResponseWriter, req *http.Request) {
	var d struct {
		Domain  string `json:"domain"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Meaning string `json:"meaning"`
		Plan    string `json:"plan"`
	}

	if err := json.NewDecoder(req.Body).Decode(&d); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	d.Domain = strings.ToLower(strings.TrimSpace(strings.ToValidUTF8(d.Domain, "")))
	d.Email = strings.ToLower(strings.TrimSpace(strings.ToValidUTF8(d.Email, "")))
	d.Name = strings.TrimSpace(strings.ToValidUTF8(d.Name, ""))
	d.Meaning = strings.TrimSpace(strings.ToValidUTF8(d.Meaning, ""))
	d.Plan = strings.TrimSpace(strings.ToValidUTF8(d.Plan, ""))

	z, err := p.ZoneRepo.Get(d.Domain)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if z.Name != "" {
		http.Error(w, "domain is unaviable", http.StatusBadRequest)
		return
	}

	if len(d.Domain) < 4 {
		http.Error(w, "domain is invalid", http.StatusBadRequest)
		return
	}

	if len(d.Name) < 3 {
		http.Error(w, "name is invalid", http.StatusBadRequest)
		return
	}

	if len(d.Meaning) < 10 {
		http.Error(w, "meaning is invalid", http.StatusBadRequest)
		return
	}

	if len(d.Plan) < 10 {
		http.Error(w, "plan is invalid", http.StatusBadRequest)
		return
	}

	k := d.Domain

	path := p.zPath + "/tmp/" + k + ".json"

	if _, err = os.Stat(path); err == nil {
		http.Error(w, "domain is unaviable", http.StatusBadRequest)
		return
	}

	ds, _ := json.MarshalIndent(d, "", "  ")
	if err := os.WriteFile(path, ds, 0666); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h := hmac.New(sha256.New, p.signKey)

	n := time.Now()
	t := n.Unix()
	ts := strconv.Itoa(int(t))

	h.Write([]byte(k + "@" + ts))
	s := h.Sum(nil)
	sign := base64.RawURLEncoding.EncodeToString(s)

	auth := fmt.Sprintf("?n=%s&t=%s&s=%s&a=v1", k, ts, sign)

	link := "https://" + req.Host + req.URL.Path + auth

	content := "Hi, " + d.Name + "\n\n" +
		"You are applying the domain of " + d.Domain + ".zz.ac\n\n" +
		"Please click the following link to very your ZZ.AC apply Email.\n" +
		"\n" +
		link + "\n" +
		"\n" +
		"This link will expire after one hour.\n" +
		"\n" +
		"If you have not applyed the zz.ac domain name, please ignore the Email.\n" +
		"\n" +
		"\n" +
		"zz.nic"

	m := enmime.Builder().
		From("", os.Getenv("SMTP_USER")).
		To(d.Name, d.Email).
		Subject("Verify your ZZ.AC Email").
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

func (p *Proxy) zoneApplyVerifyEmail(w http.ResponseWriter, req *http.Request) {
	k := req.URL.Query().Get("n")
	path := p.zPath + "/tmp/" + k + ".json"

	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		http.Error(w, "link expired or verifed", http.StatusBadRequest)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var d struct {
		Domain  string `json:"domain"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Meaning string `json:"meaning"`
		Plan    string `json:"plan"`
	}

	if err := json.Unmarshal(b, &d); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	z, err := p.ZoneRepo.Get(d.Domain)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if z.Name != "" {
		http.Error(w, "domain is unaviable", http.StatusBadRequest)
		return
	}

	h := hmac.New(sha256.New, p.signKey)

	n := time.Now().AddDate(0, 0, 7)
	t := n.Unix()
	ts := strconv.Itoa(int(t))

	h.Write([]byte(k + "@" + ts))
	s := h.Sum(nil)
	sign := base64.RawURLEncoding.EncodeToString(s)

	auth := fmt.Sprintf("?n=%s&t=%s&s=%s&a=v2", k, ts, sign)

	link := "https://" + req.Host + req.URL.Path + auth

	zs, err := p.ZoneRepo.ListByEmail(d.Email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	zs2, err := p.ZoneRepo.GetAll(d.Domain)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	zs = append(zs, zs2...)

	content := string(b) + "\n\n"

	if len(zs) > 0 {
		b, err := json.MarshalIndent(zs, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		content += string(b) + "\n\n"
	}

	content += link + "\n\n" +
		"zz.nic"

	m := enmime.Builder().
		From("", os.Getenv("SMTP_USER")).
		To("", "nic@zz.ac").
		ReplyTo(d.Name, d.Email).
		Subject("New ZZ.AC application.").
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

	os.Rename(path, path+"_")

	w.Write([]byte("Hi, " + d.Name + "\n\n" +
		"You Email has been verified.\n\n" +
		"We will review your application as soon as possible.\n" +
		"Further updates will be sent to your Email.\n\n" +
		"zz.nic"))
}

func (p *Proxy) zoneApplyAuth(w http.ResponseWriter, req *http.Request) {
	k := req.URL.Query().Get("n")
	path := p.zPath + "/tmp/" + k + ".json_"

	b, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var d struct {
		Domain  string `json:"domain"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Meaning string `json:"meaning"`
		Plan    string `json:"plan"`
	}

	if err := json.Unmarshal(b, &d); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	z, err := p.ZoneRepo.Get(d.Domain)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if z.Name != "" {
		http.Error(w, "domain is unaviable", http.StatusBadRequest)
		return
	}

	z.Name = d.Domain
	z.Email = d.Email
	z.Owner = d.Name
	z.Descr = d.Meaning
	z.Time = time.Now().Truncate(time.Second)

	if err := p.ZoneRepo.New(&z); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	os.Remove(path)

	err = os.WriteFile(p.zPath+"/zz.ac/"+d.Domain+".zone", []byte(""), 0644)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	db, err := os.OpenFile(p.zPath+"/db.zz.ac", os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	_, err = db.WriteString("$INCLUDE zz.ac/" + d.Domain + ".zone\t\t" + d.Domain + "\n")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	content := "Hi " + z.Owner + "\n\n" +
		"You domain " + z.Name + ".zz.ac has been created.\n\n" +
		"The DNS records can be managed via https://nic.zz.ac/#zone\n\n" +
		"You need to publish your website on https://" + z.Name + ".zz.ac within 10 days.\n" +
		"Othewise, this domain will be reclaimed.\n\n" +
		"More discussion can be found in Telegram Group https://t.me/zz_nic\n\n" +
		"zz.nic"

	m := enmime.Builder().
		From("", os.Getenv("SMTP_USER")).
		To(z.Owner, z.Email).
		Subject(z.Name + ".zz.ac domain created").
		Text([]byte(content))

	ss := TLSSender{
		Username: os.Getenv("SMTP_USER"),
		Password: os.Getenv("SMTP_PASS"),
		Hostaddr: os.Getenv("SMTP_HOST"),
	}

	if err := m.Send(ss); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
	}

	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	e.Encode(z)
}

func parseZone(origin, zone string) error {
	f := strings.NewReader(zone)
	zp := dns.NewZoneParser(f, origin, "")

	zp.SetDefaultTTL(60)

	rrs := map[string]bool{}
	cnames := map[string]bool{}
	for r, ok := zp.Next(); ok; r, ok = zp.Next() {
		h := r.Header()
		if h.Ttl < 60 {
			return fmt.Errorf("TTL must be longer than %ds.", 60)
		}
		if strings.Contains(h.Name, "*") {
			return fmt.Errorf("Wildcard name %s are not supported.", h.Name)
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
		if h.Rrtype == dns.TypeRRSIG || h.Rrtype == dns.TypeDS || h.Rrtype == dns.TypeDNSKEY {
			return fmt.Errorf("DNSSEC records will be generated automatically.")
		}

		if h.Rrtype == dns.TypeCNAME {
			if rrs[h.Name] {
				return fmt.Errorf("CNAME cannot co-exit with other RR.")
			}
			cnames[h.Name] = true
		} else if cnames[h.Name] {
			return fmt.Errorf("CNAME cannot co-exit with other RR.")
		}

		rrs[h.Name] = true
	}
	return zp.Err()
}
