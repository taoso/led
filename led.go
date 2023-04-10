package led

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/emersion/go-imap/client"
	"github.com/gorilla/handlers"
	"github.com/jhillyerd/enmime"
	"github.com/joho/godotenv"
	"github.com/taoso/led/tiktoken"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/net/idna"
)

type FileHandler struct {
	Root string
	fs   http.Handler
}

func NewHandler(root string, name string) *FileHandler {
	path := filepath.Join(root, name)

	h := http.FileServer(leDir{http.Dir(path)})
	h = handlers.CombinedLoggingHandler(os.Stdout, h)
	h = handlers.CompressHandler(h)

	return &FileHandler{
		Root: path,
		fs:   h,
	}
}

func (h *FileHandler) Rewritten(w http.ResponseWriter, req *http.Request) bool {
	b, err := os.ReadFile(path.Join(h.Root, "rewrite.txt"))

	if os.IsNotExist(err) {
		return false
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return false
	}

	lines := strings.TrimSpace(string(b))
	for _, line := range strings.Split(lines, "\n") {
		parts := strings.Split(line, " -> ")

		if len(parts) < 2 {
			continue
		}

		oldURL := parts[0]
		newURL := parts[1]

		if req.URL.Path == oldURL {
			http.Redirect(w, req, newURL, http.StatusMovedPermanently)
			return true
		}
	}

	return false
}

// Proxy http proxy handler
type Proxy struct {
	sites atomic.Value
	users atomic.Value

	BPE *tiktoken.BPE

	chatLinks sync.Map
}

func (p *Proxy) auth(username, password string) bool {
	users := p.users.Load().(map[string]string)
	hash, ok := users[username]
	if !ok {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func (p *Proxy) SetUsers(users map[string]string) {
	p.users.Store(users)
}

func (p *Proxy) SetSites(root string, sites map[string]string) {
	hs := make(map[string]*FileHandler, len(sites))
	for name := range sites {
		hs[name] = NewHandler(root, name)
	}

	p.sites.Store(hs)
}

func (p *Proxy) host(req *http.Request) string {
	host := req.Host
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
	fs := p.sites.Load().(map[string]*FileHandler)
	if f := fs[p.host(req)]; f != nil {
		if strings.HasSuffix(req.RequestURI, "/index.htm") {
			localRedirect(w, req, "./")
			return
		}

		if req.RequestURI == "/+/mail" && req.Method == http.MethodPost {
			f.Comment(w, req)
			return
		}

		if req.RequestURI == "/+/push" && req.Method == http.MethodPost {
			f.webPush(w, req)
			return
		}

		if strings.HasPrefix(req.RequestURI, "/+/chat/cancel") && req.Method == http.MethodPost {
			p.chatCancel(w, req, f)
			return
		}

		if strings.HasPrefix(req.RequestURI, "/+/chat") && req.Method == http.MethodPost {
			p.chat(w, req, f)
			return
		}

		if f.Rewritten(w, req) {
			return
		}

		f.fs.ServeHTTP(w, req)
		return
	}

	auth := req.Header.Get("Proxy-Authorization")
	username, password, _ := parseBasicAuth(auth)
	if !p.auth(username, password) {
		w.Header().Set("Proxy-Authenticate", `Basic realm="word wide web"`)
		w.WriteHeader(http.StatusProxyAuthRequired)
		return
	}

	if req.Method == http.MethodConnect {
		proxyHTTPS(w, req)
	} else {
		proxyHTTP(w, req)
	}
}

func (f *FileHandler) Comment(w http.ResponseWriter, req *http.Request) {
	if err := req.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	envs, err := godotenv.Read(filepath.Join(f.Root, "env"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	if envs["IMAP_PASSWORD"] == "" {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(err.Error()))
		return
	}

	r := imapReply{
		Host:     envs["IMAP_HOST"],
		Account:  envs["IMAP_ACCOUNT"],
		Password: envs["IMAP_PASSWORD"],
	}

	if err := r.Comment(
		req.Form.Get("name"),
		req.Form.Get("email"),
		req.Form.Get("subject"),
		req.Form.Get("content"),
		req.Header.Get("referer"),
	); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
	}
	return
}

func proxyHTTPS(w http.ResponseWriter, req *http.Request) {
	address := req.RequestURI
	upConn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer upConn.Close()

	var downConn io.ReadWriteCloser
	if req.ProtoMajor == 2 {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		downConn = flushWriter{w: w, r: req.Body}
	} else {
		downConn, _, err = w.(http.Hijacker).Hijack()
		downConn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))
	}

	go io.Copy(upConn, downConn)
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

type leDir struct {
	fs http.FileSystem
}

func (d leDir) Open(path string) (f http.File, err error) {
	if strings.HasSuffix(path, "/env") || strings.HasSuffix(path, ".md") {
		return nil, fs.ErrPermission
	}

	if strings.HasSuffix(path, ".html") {
		htm := strings.TrimSuffix(path, "l")
		if f, err = d.fs.Open(htm); err == nil {
			return f, nil
		}
	}

	f, err = d.fs.Open(path)
	if err != nil {
		return nil, err
	}
	s, err := f.Stat()
	if s.IsDir() {
		if a, err := d.fs.Open(path + "/.autoindex"); err == nil {
			a.Close()
			return f, nil
		}
		if _, err := d.fs.Open(path + "/index.htm"); err != nil {
			if _, err := d.fs.Open(path + "/index.html"); err != nil {
				return nil, err
			}
		}
	}

	return f, nil
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

type imapReply struct {
	Host     string
	Account  string
	Password string
}

func (s *imapReply) Comment(name, email, subject, content, path string) error {
	content += "\n\n" + path
	m := enmime.Builder().
		From(name, email).
		To("", s.Account).
		Subject(subject).
		Header("In-Reply-To", strings.Replace(path+s.Account, "/", "", -1)).
		Header("Message-ID", time.Now().Format(time.RFC3339Nano)+email).
		Text([]byte(content))

	return m.Send(s)
}

func (s *imapReply) Send(_ string, _ []string, msg []byte) error {
	c, err := client.DialTLS(s.Host, nil)
	if err != nil {
		return err
	}

	if err := c.Login(s.Account, s.Password); err != nil {
		return err
	}

	b := bytes.NewBuffer(msg)
	return c.Append("INBOX", nil, time.Now(), b)
}

func (f *FileHandler) webPush(w http.ResponseWriter, req *http.Request) {
	d := map[string]string{
		"title": req.FormValue("title"),
		"body":  req.FormValue("body"),
		"data":  req.FormValue("data"),
	}
	m, err := json.Marshal(d)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	var s webpush.Subscription
	subs := req.FormValue("subs")
	if err := json.Unmarshal([]byte(subs), &s); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	envs, err := godotenv.Read(filepath.Join(f.Root, "env"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	o := webpush.Options{
		TTL:             86400,
		Subscriber:      envs["PUSH_SUBSCRIBER"],
		VAPIDPublicKey:  envs["PUSH_VAPID_PUB"],
		VAPIDPrivateKey: envs["PUSH_VAPID_PRI"],
	}

	r, err := webpush.SendNotification(m, &s, &o)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	defer r.Body.Close()

	w.WriteHeader(r.StatusCode)
	io.Copy(w, r.Body)
}

func (p *Proxy) chat(w http.ResponseWriter, req *http.Request, f *FileHandler) {
	user, pass, ok := req.BasicAuth()
	if !ok || !p.auth(user, pass) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	defer req.Body.Close()

	var msg struct {
		Messages []map[string]string `json:"messages"`
		Model    string              `json:"model"`
		Stream   bool                `json:"stream"`
	}

	if err := json.NewDecoder(req.Body).Decode(&msg); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	var u struct {
		Usage struct {
			ReplyTokens  int `json:"completion_tokens"`
			PromptTokens int `json:"prompt_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}

	defer func() {
		if msg.Stream {
			b, _ := json.Marshal(u)
			b = append([]byte("data: "), b...)
			b = append(b, []byte("\n\ndata: [DONE]\n\n")...)
			w.Write(b)
		}
		fmt.Printf("%+v\n", u.Usage)
	}()

	u.Usage.PromptTokens = p.BPE.CountMessage(msg.Messages)
	u.Usage.TotalTokens = u.Usage.PromptTokens + u.Usage.ReplyTokens

	b, err := json.Marshal(msg)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	envs, err := godotenv.Read(filepath.Join(f.Root, "env"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	url := "https://api.openai.com" + req.URL.Path[len("/+/chat"):]
	r, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	r.Header.Set("Authorization", "Bearer "+envs["CHAT_TOKEN"])
	r.Header.Set("Content-Type", req.Header.Get("Content-Type"))

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	defer resp.Body.Close()

	linkKey := user + resp.Header.Get("X-Request-Id")
	p.chatLinks.Store(linkKey, resp.Body)

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("X-Request-Id", resp.Header.Get("X-Request-Id"))
	fmt.Println(resp.Header.Get("X-Request-Id"))
	w.WriteHeader(resp.StatusCode)

	if resp.StatusCode != http.StatusOK || !msg.Stream {
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusOK {
			if err := json.Unmarshal(b, &u); err != nil {
				log.Println("unmarshal data error: ", err)
			}
		}
		w.Write(b)
		return
	}

	s := bufio.NewScanner(resp.Body)
	for s.Scan() {
		l := s.Text()
		if !strings.HasPrefix(l, "data: {") {
			continue
		}

		var data struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason,omitempty"`
				Index        int     `json:"index,omitempty"`
			} `json:"choices"`
		}
		err := json.Unmarshal([]byte(l[len("data: "):]), &data)
		if err != nil {
			log.Println("unmarshal data error: ", err)
			return
		}

		for _, c := range data.Choices {
			u.Usage.ReplyTokens += p.BPE.Count(c.Delta.Content)
			u.Usage.TotalTokens = u.Usage.PromptTokens + u.Usage.ReplyTokens
		}

		b, err := json.Marshal(data)
		if err != nil {
			log.Println("marshal data error: ", err)
			return
		}

		buf := make([]byte, len(b)+len("data: ")+len("\n\n"))
		copy(buf, []byte("data: "))
		copy(buf[len("data: "):], b)
		copy(buf[len(b)+len("data: "):], []byte("\n\n"))

		if _, err := w.Write(buf); err != nil {
			log.Println("write data error: ", err)
			return
		}
	}

	if err := s.Err(); err != nil {
		log.Println("scan err", err)
	}
}

func (p *Proxy) chatCancel(w http.ResponseWriter, req *http.Request, f *FileHandler) {
	user, pass, ok := req.BasicAuth()
	if !ok || !p.auth(user, pass) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	linkKey := user + req.URL.Query().Get("id")
	v, ok := p.chatLinks.Load(linkKey)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("link not found"))
		return
	}
	v.(io.Closer).Close()
	p.chatLinks.Delete(linkKey)
}
