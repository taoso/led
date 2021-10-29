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

func init() {
	flag.StringVar(&root, "root", "", "static server root")
	flag.StringVar(&sites, "sites", "", "static server sites")
	flag.StringVar(&users, "users", "", "proxy server users")
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

func listen() (ln80, ln443 net.Listener, lnUDP net.PacketConn, err error) {
	if os.Getenv("LISTEN_PID") == strconv.Itoa(os.Getpid()) {
		f1 := os.NewFile(3, "http port from systemd")
		ln80, err = net.FileListener(f1)
		if err != nil {
			return
		}
		f2 := os.NewFile(4, "https port from systemd")
		ln443, err = net.FileListener(f2)
		if err != nil {
			return
		}
		f3 := os.NewFile(5, "quic port from systemd")
		lnUDP, err = net.FilePacketConn(f3)
	} else {
		ln80, err = net.Listen("tcp", ":80")
		if err != nil {
			return
		}
		ln443, err = net.Listen("tcp", ":443")
		if err != nil {
			return
		}
		lnUDP, err = net.ListenPacket("udp", ":443")
	}
	return
}

func main() {
	flag.Parse()
	if users == "" {
		flag.Usage()
		return
	}

	proxy := &ssltun.Proxy{}

	var names atomic.Value
	go watchload(users, proxy.SetUsers)
	go watchload(sites, func(s map[string]string) {
		names.Store(s)
		proxy.SetSites(root, s)
	})

	dir := os.Getenv("HOME") + "/.autocert"
	acm := autocert.Manager{
		Cache:  autocert.DirCache(dir),
		Prompt: autocert.AcceptTOS,
		HostPolicy: func(ctx context.Context, host string) error {
			sites := names.Load().(map[string]string)
			_, ok := sites[host]
			if ok {
				return nil
			} else {
				return errors.New(host + " not found")
			}
		},
	}

	tlsCfg := acm.TLSConfig()
	tlsCfg.NextProtos = []string{"acme-tls/1", "http/1.1", "h3", "h3-29"}

	ln80, ln443, lnUDP, err := listen()
	if err != nil {
		panic(err)
	}

	// http3
	h3 := http3.Server{Server: &http.Server{Handler: proxy}}
	h3.TLSConfig = tlsCfg
	go h3.Serve(lnUDP.(net.PacketConn))

	// http -> https
	h301 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		url := "https://" + r.Host + r.RequestURI
		http.Redirect(w, r, url, http.StatusMovedPermanently)
	})
	go http.Serve(ln80, h301)

	// https
	lnTLS := tls.NewListener(ln443, tlsCfg)
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
