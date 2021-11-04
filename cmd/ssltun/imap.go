package main

import (
	"bytes"
	"log"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/emersion/go-imap/client"
	"github.com/jhillyerd/enmime"
)

type imapReply struct {
	Host     string
	Account  string
	Password string

	c *client.Client
}

func (s *imapReply) Dial() (err error) {
	s.c, err = client.DialTLS(s.Host, nil)
	if err != nil {
		return
	}
	return s.c.Login(s.Account, s.Password)
}

func (s *imapReply) Comment(name, email, path, subject, content string) error {
	if email == "" {
		email = "noreply@example.com"
	}
	if name == "" {
		name = "无名氏"
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return nil
	}

	if subject == "" || content == "" {
		return nil
	}

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
	b := bytes.NewBuffer(msg)
	return s.c.Append("INBOX", nil, time.Now(), b)
}

func startImap() chan url.Values {
	ch := make(chan url.Values, 10)
	ic := &imapReply{
		Host:     os.Getenv("IMAP_HOST"),
		Account:  os.Getenv("IMAP_ACCOUNT"),
		Password: os.Getenv("IMAP_PASSWORD"),
	}
	if err := ic.Dial(); err != nil {
		panic(err)
	}

	go func() {
		for c := range ch {
			if err := ic.Comment(
				c.Get("name"),
				c.Get("email"),
				c.Get("path"),
				c.Get("subject"),
				c.Get("content"),
			); err != nil {
				log.Println("comment error", err)
				time.Sleep(3 * time.Second)
				if err := ic.Dial(); err != nil {
					panic(err)
				}
			}
		}
	}()
	return ch
}
