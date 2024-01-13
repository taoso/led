package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/CAFxX/httpcompression"
	"github.com/gorilla/handlers"
	"github.com/quic-go/quic-go/http3"
	"github.com/taoso/led"
	"github.com/taoso/led/pay"
	"github.com/taoso/led/store"
	"github.com/taoso/led/tiktoken"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/idna"
)

var root, sites, users string

var flags struct {
	http1, http2, http3 string
}

func init() {
	flag.StringVar(&root, "root", "", "static server root")
	flag.StringVar(&sites, "sites", "", "static server sites")
	flag.StringVar(&users, "users", "", "proxy server users")
	flag.StringVar(&flags.http1, "http1", "", "listen address for http1")
	flag.StringVar(&flags.http2, "http2", "", "listen address for http2")
	flag.StringVar(&flags.http3, "http3", "", "listen address for http3")
}

func listen() (h1, h2 net.Listener, h3 net.PacketConn, err error) {
	if os.Getenv("LISTEN_PID") == strconv.Itoa(os.Getpid()) {
		if os.Getenv("LISTEN_FDS") != "3" {
			panic("LISTEN_FDS should be 3")
		}
		names := strings.Split(os.Getenv("LISTEN_FDNAMES"), ":")
		for i, name := range names {
			switch name {
			case "http":
				f1 := os.NewFile(uintptr(i+3), "http port")
				h1, err = net.FileListener(f1)
			case "https":
				f2 := os.NewFile(uintptr(i+3), "https port")
				h2, err = net.FileListener(f2)
			case "quic":
				f3 := os.NewFile(uintptr(i+3), "quic port")
				h3, err = net.FilePacketConn(f3)
			}
		}
	} else {
		if flags.http1 != "" {
			h1, err = net.Listen("tcp", flags.http1)
			if err != nil {
				return
			}
		}
		if flags.http2 != "" {
			h2, err = net.Listen("tcp", flags.http2)
			if err != nil {
				return
			}
		}
		if flags.http3 != "" {
			h3, err = net.ListenPacket("udp", flags.http3)
		}
	}
	return
}

func main() {
	flag.Parse()

	lnH1, lnH2, lnH3, err := listen()
	if err != nil {
		panic(err)
	}
	if lnH1 == nil && lnH2 == nil && lnH3 == nil {
		panic("No listen port specified")
	}

	proxy := &led.Proxy{
		DavEvs: make(chan string, 1024),
		Root:   root,
	}

	if err := load(proxy); err != nil {
		panic(err)
	}

	sg := make(chan os.Signal, 3)
	signal.Notify(sg, syscall.SIGHUP)
	go func() {
		for range sg {
			if err := load(proxy); err != nil {
				log.Println("load err:", err)
			}
		}
	}()

	h := handlers.VhostCombinedLoggingHandler(os.Stdout, proxy)

	ch, err := httpcompression.DefaultAdapter(
		httpcompression.MinSize(1024),
		httpcompression.ContentTypes([]string{
			"application/atom+xml",
			"application/javascript",
			"application/json",
			"application/rss+xml",
			"application/xml",
			"image/svg+xml",
			"text/css",
			"text/html",
			"text/javascript",
			"text/mathml",
			"text/plain",
			"text/xml",
		}, false),
	)
	if err != nil {
		panic(err)
	}

	h = ch(h)

	// http only
	if lnH2 == nil && lnH3 == nil {
		http.Serve(lnH1, h)
		return
	}

	// http2 or http3
	acm := autocert.Manager{
		Prompt: autocert.AcceptTOS,
		Cache:  autocert.DirCache(os.Getenv("HOME") + "/.autocert"),
		HostPolicy: func(ctx context.Context, host string) error {
			host, err := idna.ToUnicode(host)
			if err != nil {
				return err
			}
			if !proxy.MySite(host) {
				return errors.New(host + " not found")
			}
			return nil
		},
	}

	tlsCfg := acm.TLSConfig()
	tlsCfg.NextProtos = append(tlsCfg.NextProtos, http3.NextProtoH3)

	if lnH3 != nil {
		p := lnH3.LocalAddr().(*net.UDPAddr).Port
		proxy.AltSvc = fmt.Sprintf(`h3=":%d"`, p)

		h3 := http3.Server{Handler: h, TLSConfig: tlsCfg}
		go h3.Serve(lnH3)
	}

	// http -> https
	h301 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		url := "https://" + r.Host + r.RequestURI
		http.Redirect(w, r, url, http.StatusMovedPermanently)
	})
	go http.Serve(lnH1, h301)

	// https
	lnTLS := tls.NewListener(lnH2, tlsCfg)
	s := http.Server{
		Handler:     h,
		IdleTimeout: 30 * time.Second,
	}

	s.Serve(lnTLS)
}

func load(proxy *led.Proxy) error {
	if tk := os.Getenv("TIKTOKEN_FILE"); tk != "" {
		f, err := os.Open(tk)
		if err != nil {
			return err
		}
		defer f.Close()

		bpe, err := tiktoken.NewCL100K(f)
		if err != nil {
			return err
		}
		proxy.BPE = bpe
	}

	if id := os.Getenv("ALIPAY_APP_ID"); id != "" {
		proxy.Alipay = pay.New(
			id,
			os.Getenv("ALIPAY_PRIVATE_KEY"),
			os.Getenv("ALIPAY_PUBLIC_KEY"),
		)
	}

	if db := os.Getenv("TOKEN_REPO_DB"); db != "" {
		proxy.TokenRepo = store.NewTokenRepo(db)
	}

	d, err := loadfile(users)
	if err != nil {
		return err
	}
	proxy.SetUsers(d)

	d, err = loadfile(sites)
	if err != nil {
		return err
	}
	proxy.SetSites(d)

	return nil
}

func loadfile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	kv := map[string]string{}
	for s.Scan() {
		l := s.Text()
		i := strings.Index(l, ":")
		if i == -1 {
			i = len(l)
		}

		host := l[:i]
		kv[host] = l[i+1:]
	}
	return kv, nil
}
