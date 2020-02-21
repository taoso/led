package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/lvht/ssltun"
	"golang.org/x/crypto/acme/autocert"
)

var name, key, root string
var h2 bool

func init() {
	flag.StringVar(&name, "name", "", "server domain name")
	flag.StringVar(&key, "key", "", "server auth key")
	flag.StringVar(&root, "root", "", "static server root")
	flag.BoolVar(&h2, "h2", false, "enable http/2 protocol")
}

func main() {
	flag.Parse()

	dir := os.Getenv("HOME") + "/.autocert"
	acm := autocert.Manager{
		Cache:      autocert.DirCache(dir),
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(name),
	}
	tlsCfg := acm.TLSConfig()

	if !h2 {
		tlsCfg.NextProtos = []string{"http/1.1", "acme-tls/1"}
	}

	ln, err := tls.Listen("tcp", ":443", tlsCfg)
	if err != nil {
		log.Fatal(err)
	}

	proxy := &ssltun.Proxy{DomainName: name}
	proxy.Auth = func(u, p string) bool { return u == key }
	if root != "" {
		proxy.FileHandler = http.FileServer(http.Dir(root))
	}

	if err = http.Serve(ln, proxy); err != nil {
		log.Fatal(err)
	}
}
