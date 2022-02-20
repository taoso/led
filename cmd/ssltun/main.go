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
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
	"github.com/lucas-clemente/quic-go/http3"
	"github.com/lvht/ssltun"
	"golang.org/x/crypto/acme/autocert"
)

var root, sites, users string
var imap bool

var flags struct {
	http1, http2, http3 string
}

func init() {
	flag.StringVar(&root, "root", "", "static server root")
	flag.StringVar(&sites, "sites", "", "static server sites")
	flag.StringVar(&users, "users", "", "proxy server users")
	flag.BoolVar(&imap, "imap", false, "start mail forwarding")
	flag.StringVar(&flags.http1, "http1", "", "listen address for http1")
	flag.StringVar(&flags.http2, "http2", "", "listen address for http2")
	flag.StringVar(&flags.http3, "http3", "", "listen address for http3")
}

func watchload(path string, fn func(map[string]string)) {
	w, ch := watch(path)
	defer w.Close()
	defer close(ch)
load:
	f, err := os.Open(path)
	if err != nil {
		panic(err)
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

	fn(kv)

	for range ch {
		goto load
	}
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
	if flags.http1 == "" && flags.http2 == "" {
		flag.Usage()
		return
	}

	proxy := &ssltun.Proxy{}

	if imap {
		proxy.Mail = startImap()
	}

	var names atomic.Value
	go watchload(users, proxy.SetUsers)
	go watchload(sites, func(s map[string]string) {
		names.Store(s)
		proxy.SetSites(root, s)
	})

	lnH1, lnH2, lnH3, err := listen()
	if err != nil {
		panic(err)
	}

	var tlsCfg *tls.Config
	if lnH2 != nil || lnH3 != nil {
		dir := os.Getenv("HOME") + "/.autocert"
		acm := autocert.Manager{
			Cache:  autocert.DirCache(dir),
			Prompt: autocert.AcceptTOS,
			HostPolicy: func(ctx context.Context, host string) error {
				sites := names.Load().(map[string]string)
				if _, ok := sites[host]; !ok {
					return errors.New(host + " not found")
				}
				return nil
			},
		}

		tlsCfg = acm.TLSConfig()
		tlsCfg.NextProtos = []string{"acme-tls/1", "http/1.1", "h3", "h3-29"}
	}

	if lnH3 != nil {
		h3 := http3.Server{Server: &http.Server{Handler: proxy}}
		h3.TLSConfig = tlsCfg
		go h3.Serve(lnH3.(net.PacketConn))
	}

	if lnH2 == nil {
		http.Serve(lnH1, proxy)
		return
	}

	// http -> https
	h301 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		url := "https://" + r.Host + r.RequestURI
		http.Redirect(w, r, url, http.StatusMovedPermanently)
	})
	go http.Serve(lnH1, h301)

	// https
	lnTLS := tls.NewListener(lnH2, tlsCfg)
	http.Serve(lnTLS, proxy)
}

func watch(path string) (watcher *fsnotify.Watcher, ch chan interface{}) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}

	err = watcher.Add(path)
	if err != nil {
		log.Fatal(err)
	}
	ch = make(chan interface{}, 1)

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					ch <- "read"
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				fmt.Println(err)
			}
		}
	}()
	return
}
