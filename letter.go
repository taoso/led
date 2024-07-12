package led

import (
	"bytes"
	"net/http"
	"os"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/jhillyerd/enmime"
	"github.com/joho/godotenv"
)

func (h *FileHandler) Comment(w http.ResponseWriter, req *http.Request) {
	if err := req.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	envs, err := godotenv.Read(h.Root + "/env")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	f := req.Form

	content := f.Get("content")
	content += "\n\n" + req.Header.Get("referer")
	m := enmime.Builder().
		From(f.Get("name"), os.Getenv("SMTP_USER")).
		ReplyTo(f.Get("name"), f.Get("email")).
		To(envs["author_name"], envs["author_email"]).
		Subject(f.Get("subject")).
		Text([]byte(content))

	s := TLSSender{
		Username: os.Getenv("SMTP_USER"),
		Password: os.Getenv("SMTP_PASS"),
		Hostaddr: os.Getenv("SMTP_HOST"),
	}

	if err = m.Send(s); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
	}
}

type TLSSender struct {
	Username string
	Password string
	Hostaddr string
}

func (s TLSSender) Send(reversePath string, recipients []string, msg []byte) error {
	auth := sasl.NewPlainClient("", s.Username, s.Password)
	return smtp.SendMailTLS(s.Hostaddr, auth, reversePath, recipients, bytes.NewReader(msg))
}
