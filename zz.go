package led

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jhillyerd/enmime"
	"github.com/miekg/dns"
)

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
	case "vps":
		p.zoneOpenVPS(w, req)
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

	// TODO
	// ok, err := p.zidExist(domain)
	// if err != nil {
	// 	http.Error(w, err.Error(), http.StatusInternalServerError)
	// 	return
	// }
	//
	// if ok {
	// 	w.Write([]byte("https://zone.nic.zz.ac"))
	// 	return
	// }
	//
	// id, err := p.zidNew(z.Name, z.Owner, z.Email)
	// if err != nil {
	// 	http.Error(w, err.Error(), http.StatusInternalServerError)
	// 	return
	// }
	//
	// token, err := p.zidToken(id)
	// if err != nil {
	// 	w.Write([]byte("https://id.zz.ac/lc/" + token))
	// 	return
	// }

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

	w.Write(b)
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

	var err string
	if d.Error != nil {
		err = d.Error.Error()
	}
	json.NewEncoder(w).Encode(struct {
		Err string `json:"err"`
	}{err})
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

	// TODO
	// 生成 zz.ID
	// 用户名、邮箱、显示名
	// 生成激活链接

	content := "Hi " + z.Owner + "\n\n" +
		"你的域名 " + z.Name + ".zz.ac 已经注册成功。\n" +
		"You domain " + z.Name + ".zz.ac has been created.\n\n" +
		"管理 DNS 记录请移步 https://nic.zz.ac/#zone\n" +
		"The DNS records can be managed via https://nic.zz.ac/#zone\n\n" +
		"你需要在 10 天内为 https://" + z.Name + ".zz.ac 发布网站内容。\n" +
		"否则域名将被回收。\n\n" +
		"You need to publish your website on https://" + z.Name + ".zz.ac within 10 days.\n" +
		"Othewise, this domain will be reclaimed.\n\n" +
		"请务必加入社区电报群 https://t.me/zz_nic\n" +
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

func (p *Proxy) zoneOpenVPS(w http.ResponseWriter, req *http.Request) {
	zone := req.URL.Query().Get("n")
	sub := req.FormValue("sub")

	vps := os.Getenv("ZZ_VPS_HOST")

	domain := sub + "." + zone + ".zz.ac"

	out, _ := exec.Command("ssh", "root@"+vps, domain).CombinedOutput()

	w.Write([]byte(out))
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

func (p *Proxy) zidExist(name string) (ok bool, err error) {
	req, err := http.NewRequest(http.MethodGet, "https://id.zz.ac/api/users?search=", nil)
	if err != nil {
		return
	}
	req.Header.Set("x-api-key", p.ZzIDAppKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var b struct {
		Data []struct {
			Username string `json:"username"`
		} `json:"data"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&b); err != nil {
		return
	}

	for _, u := range b.Data {
		if u.Username == name {
			return true, nil
		}
	}
	return
}

func (p *Proxy) zidNew(domain, username, email string) (id string, err error) {
	d := struct {
		Email         string `json:"email"`
		UserName      string `json:"username"`
		DisplayName   string `json:"displayName"`
		EmailVerified bool   `json:"emailVerified"`
	}{
		Email:         email,
		UserName:      domain,
		DisplayName:   username,
		EmailVerified: true,
	}
	b, err := json.Marshal(d)
	if err != nil {
		return
	}

	req, err := http.NewRequest(http.MethodPost, "https://id.zz.ac/api/users", bytes.NewReader(b))
	if err != nil {
		return
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", p.ZzIDAppKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var u struct {
		ID string `json:"id"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return
	}

	return u.ID, nil
}

func (p *Proxy) zidToken(id string) (token string, err error) {
	d := struct {
		TTL int `json:"ttl"`
	}{
		TTL: 3600,
	}
	b, err := json.Marshal(d)
	if err != nil {
		return
	}

	api := "https://id.zz.ac/api/users/" + id + "/one-time-access-token"
	req, err := http.NewRequest(http.MethodPost, api, bytes.NewReader(b))
	if err != nil {
		return
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", p.ZzIDAppKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var u struct {
		Token string `json:"token"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return
	}

	return u.Token, nil
}

// zzOIDCConfig holds discovered OIDC endpoints and client credentials.
type zzOIDCConfig struct {
	AuthEndpoint     string
	TokenEndpoint    string
	UserinfoEndpoint string
	ClientID         string
	ClientSecret     string
}

// zzJWTClaims is the payload stored in Zone Editor JWTs.
type zzJWTClaims struct {
	Username string `json:"u"`
	Picture  string `json:"p"`
	jwt.RegisteredClaims
}

// InitZzAuth reads env vars and, if OIDC_ISSUER is set, discovers OIDC
// endpoints via the well-known configuration URL.
func (p *Proxy) InitZzAuth() error {
	p.ZzIDAppKey = os.Getenv("ZZ_APP_KEY")
	p.ZzJWTSecret = os.Getenv("ZZ_JWT_SECRET")

	issuer := os.Getenv("ZZ_OIDC_ISSUER")
	if issuer == "" {
		return nil
	}

	resp, err := http.Get(issuer + "/.well-known/openid-configuration")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var meta struct {
		AuthEndpoint     string `json:"authorization_endpoint"`
		TokenEndpoint    string `json:"token_endpoint"`
		UserinfoEndpoint string `json:"userinfo_endpoint"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return err
	}

	p.ZzOIDC = &zzOIDCConfig{
		AuthEndpoint:     meta.AuthEndpoint,
		TokenEndpoint:    meta.TokenEndpoint,
		UserinfoEndpoint: meta.UserinfoEndpoint,
		ClientID:         os.Getenv("ZZ_OIDC_CLIENT_ID"),
		ClientSecret:     os.Getenv("ZZ_OIDC_CLIENT_SECRET"),
	}
	return nil
}

// zzAPI routes /api/* requests for the zz.NIC backend.
func (p *Proxy) zzAPI(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	path := req.URL.Path
	switch {
	case path == "/api/auth/me" && req.Method == http.MethodGet:
		p.zzMe(w, req)
	case path == "/api/auth/logout" && req.Method == http.MethodDelete:
		p.zzLogout(w, req)
	case path == "/api/auth/login" && req.Method == http.MethodGet:
		p.zzLogin(w, req)
	case path == "/api/auth/callback" && req.Method == http.MethodGet:
		p.zzCallback(w, req)
	case strings.HasPrefix(path, "/api/zones/"):
		name, err := url.PathUnescape(strings.TrimPrefix(path, "/api/zones/"))
		if err != nil || name == "" {
			zzError(w, http.StatusBadRequest, "invalid zone name")
			return
		}
		switch req.Method {
		case http.MethodGet:
			p.zzZoneGet(w, req, name)
		case http.MethodPut:
			p.zzZonePut(w, req, name)
		default:
			zzError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
	default:
		zzError(w, http.StatusNotFound, "Not found")
	}
}

// zzMe handles GET /api/auth/me.
func (p *Proxy) zzMe(w http.ResponseWriter, req *http.Request) {
	claims, err := p.zzVerifyAuth(req)
	if err != nil {
		if err.Error() == "no cookie" {
			zzError(w, http.StatusUnauthorized, "Unauthorized")
		} else {
			zzError(w, http.StatusUnauthorized, "Token invalid or expired")
		}
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"origin":   claims.Username + ".zz.ac",
		"username": claims.Username,
		"picture":  claims.Picture,
	})
}

// zzLogout handles DELETE /api/auth/logout.
func (p *Proxy) zzLogout(w http.ResponseWriter, req *http.Request) {
	p.zzSetCookie(w, req, "", 0)
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// zzLogin handles GET /api/auth/login — initiates OIDC authorization code flow.
func (p *Proxy) zzLogin(w http.ResponseWriter, req *http.Request) {
	if p.ZzOIDC == nil {
		zzError(w, http.StatusServiceUnavailable, "Auth not configured")
		return
	}

	now := time.Now()
	state := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)),
	})
	stateStr, err := state.SignedString([]byte(p.ZzJWTSecret))
	if err != nil {
		zzError(w, http.StatusInternalServerError, err.Error())
		return
	}

	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", p.ZzOIDC.ClientID)
	q.Set("scope", "openid profile")
	q.Set("state", stateStr)

	http.Redirect(w, req, p.ZzOIDC.AuthEndpoint+"?"+q.Encode(), http.StatusFound)
}

// zzCallback handles GET /api/auth/callback — exchanges code, issues JWT cookie.
func (p *Proxy) zzCallback(w http.ResponseWriter, req *http.Request) {
	if p.ZzOIDC == nil {
		zzError(w, http.StatusServiceUnavailable, "Auth not configured")
		return
	}

	code := req.URL.Query().Get("code")
	state := req.URL.Query().Get("state")
	if code == "" || state == "" {
		zzError(w, http.StatusBadRequest, "missing code or state")
		return
	}

	stateToken, err := jwt.Parse(state, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(p.ZzJWTSecret), nil
	})
	if err != nil || !stateToken.Valid {
		zzError(w, http.StatusBadRequest, "invalid state")
		return
	}

	// Exchange code for access token.
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", p.ZzOIDC.ClientID)
	form.Set("client_secret", p.ZzOIDC.ClientSecret)
	resp, err := http.PostForm(p.ZzOIDC.TokenEndpoint, form)
	if err != nil {
		zzError(w, http.StatusInternalServerError, "token exchange failed: "+err.Error())
		return
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil || tokenResp.AccessToken == "" {
		zzError(w, http.StatusInternalServerError, "invalid token response")
		return
	}

	// Fetch userinfo.
	uReq, err := http.NewRequest(http.MethodGet, p.ZzOIDC.UserinfoEndpoint, nil)
	if err != nil {
		zzError(w, http.StatusInternalServerError, err.Error())
		return
	}
	uReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	uResp, err := http.DefaultClient.Do(uReq)
	if err != nil {
		zzError(w, http.StatusInternalServerError, "userinfo request failed: "+err.Error())
		return
	}
	defer uResp.Body.Close()

	var userinfo struct {
		Sub               string `json:"sub"`
		PreferredUsername string `json:"preferred_username"`
		Picture           string `json:"picture"`
	}
	if err := json.NewDecoder(uResp.Body).Decode(&userinfo); err != nil {
		zzError(w, http.StatusInternalServerError, "invalid userinfo response")
		return
	}
	if userinfo.Sub == "" || userinfo.PreferredUsername == "" {
		zzError(w, http.StatusInternalServerError, "missing required claims")
		return
	}

	now := time.Now()
	claims := zzJWTClaims{
		Username: userinfo.PreferredUsername,
		Picture:  userinfo.Picture,
		RegisteredClaims: jwt.RegisteredClaims{
			// Subject:   userinfo.Sub,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(p.ZzJWTSecret))
	if err != nil {
		zzError(w, http.StatusInternalServerError, "failed to sign token: "+err.Error())
		return
	}

	p.zzSetCookie(w, req, signed, 3600)
	http.Redirect(w, req, "/", http.StatusFound)
}

// zeZoneGet handles GET /api/zones/:name.
func (p *Proxy) zzZoneGet(w http.ResponseWriter, req *http.Request, name string) {
	claims, err := p.zzVerifyAuth(req)
	if err != nil {
		if err.Error() == "no cookie" {
			zzError(w, http.StatusUnauthorized, "Unauthorized")
		} else {
			zzError(w, http.StatusUnauthorized, "Token invalid or expired")
		}
		return
	}
	if claims.Username+".zz.ac" != name {
		zzError(w, http.StatusForbidden, "Forbidden")
		return
	}

	b, err := os.ReadFile(p.zzZonePath(name))
	if os.IsNotExist(err) {
		zzError(w, http.StatusNotFound, "Zone not found")
		return
	}
	if err != nil {
		zzError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write(b)
}

// zeZonePut handles PUT /api/zones/:name.
func (p *Proxy) zzZonePut(w http.ResponseWriter, req *http.Request, name string) {
	claims, err := p.zzVerifyAuth(req)
	if err != nil {
		if err.Error() == "no cookie" {
			zzError(w, http.StatusUnauthorized, "Unauthorized")
		} else {
			zzError(w, http.StatusUnauthorized, "Token invalid or expired")
		}
		return
	}
	if claims.Username+".zz.ac" != name {
		zzError(w, http.StatusForbidden, "Forbidden")
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		zzError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if strings.EqualFold(req.Header.Get("X-Mode"), "desec") {
		token := strings.TrimSpace(string(body))
		if token == "" {
			zzError(w, http.StatusBadRequest, "Token required")
			return
		}

		zone, err := deSecZone(name, token)
		if err != nil {
			zzError(w, http.StatusInternalServerError, err.Error())
			return
		}

		if err := os.WriteFile(p.zzZonePath(name), []byte(zone), 0644); err != nil {
			zzError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(zone))
		return
	}

	if err := os.WriteFile(p.zzZonePath(name), body, 0644); err != nil {
		zzError(w, http.StatusInternalServerError, err.Error())
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// zzVerifyAuth reads and validates the zz-auth cookie JWT.
func (p *Proxy) zzVerifyAuth(req *http.Request) (*zzJWTClaims, error) {
	cookie, err := req.Cookie("zz-auth")
	if err != nil {
		return nil, fmt.Errorf("no cookie")
	}
	token, err := jwt.ParseWithClaims(cookie.Value, &zzJWTClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(p.ZzJWTSecret), nil
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	claims, ok := token.Claims.(*zzJWTClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims")
	}
	return claims, nil
}

// zzSetCookie sets or clears the zz-auth cookie.
func (p *Proxy) zzSetCookie(w http.ResponseWriter, req *http.Request, value string, maxAge int) {
	cookie := &http.Cookie{
		Name:     "zz-auth",
		Value:    value,
		HttpOnly: true,
		Path:     "/",
		MaxAge:   maxAge,
		SameSite: http.SameSiteLaxMode,
	}
	if req.TLS != nil {
		cookie.Secure = true
	}
	http.SetCookie(w, cookie)
}

// zzZonePath returns the filesystem path for a zone given the full origin (e.g. "alice.zz.ac").
func (p *Proxy) zzZonePath(name string) string {
	domain := strings.TrimSuffix(name, ".zz.ac")
	return p.zPath + "/zz.ac/" + domain + ".zone"
}

// zzError writes a JSON error response.
func zzError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
