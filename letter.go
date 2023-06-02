package led

import (
	"net"
	"net/http"
	"net/smtp"
	"os"

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

	addr := os.Getenv("SMTP_HOST")
	host, _, err := net.SplitHostPort(addr)
	auth := smtp.PlainAuth("", os.Getenv("SMTP_USER"), os.Getenv("SMTP_PASS"), host)

	if err = m.Send(enmime.NewSMTP(addr, auth)); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
	}
}
