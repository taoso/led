package main

import (
	"bufio"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/taoso/led"
	"github.com/taoso/led/pay"
)

var root, sites, http1 string

func init() {
	flag.StringVar(&root, "root", "", "static server root")
	flag.StringVar(&sites, "sites", "", "static server sites")
	flag.StringVar(&http1, "http1", "", "listen address for http1")

	log.SetOutput(os.Stderr)
}

func listen() (net.Listener, error) {
	if http1 == "" {
		return nil, errors.New("-http1 is required")
	}
	return net.Listen("tcp", http1)
}

func main() {
	flag.Parse()

	lnH1, err := listen()
	if err != nil {
		panic(err)
	}

	proxy := &led.Proxy{
		Root: root,
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

	s := &http.Server{
		Handler:     proxy,
		IdleTimeout: 30 * time.Second,
	}

	log.Println("listening on", lnH1.Addr())
	log.Fatal(s.Serve(lnH1))
}

func load(proxy *led.Proxy) error {
	if id := os.Getenv("ALIPAY_APP_ID"); id != "" {
		proxy.Alipay = pay.New(
			id,
			os.Getenv("ALIPAY_PRIVATE_KEY"),
			os.Getenv("ALIPAY_PUBLIC_KEY"),
		)
	}

	d, err := loadfile(sites)
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
		if strings.HasPrefix(l, "#") {
			continue
		}
		i := strings.Index(l, ":")
		if i == -1 {
			i = len(l)
		}

		host := l[:i]
		kv[host] = l[i+1:]
	}
	return kv, nil
}
