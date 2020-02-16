package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net/http"

	"github.com/lvht/ssltun"
	"github.com/mholt/certmagic"
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

	tlsCfg, err := certmagic.TLS([]string{name})
	if err != nil {
		log.Fatal(err)
	}
	if !h2 {
		tlsCfg.NextProtos = []string{"http/1.1", "acme-tls/1"}
	}

	ln, err := tls.Listen("tcp", ":443", tlsCfg)
	if err != nil {
		log.Fatal(err)
	}

	proxy := &ssltun.Proxy{Name: name, Key: key}
	if root != "" {
		proxy.FileHandler = http.FileServer(http.Dir(root))
	}

	if err = http.Serve(ln, proxy); err != nil {
		log.Fatal(err)
	}
}
