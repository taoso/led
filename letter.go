package led

import (
	"bytes"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/emersion/go-imap/client"
	"github.com/jhillyerd/enmime"
	"github.com/joho/godotenv"
)

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
