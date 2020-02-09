package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/lvht/ssltun"
	"github.com/mholt/certmagic"
)

var name, key string

func init() {
	flag.StringVar(&name, "name", "", "server domain name")
	flag.StringVar(&key, "key", "", "server auth key")
}

func main() {
	flag.Parse()

	ln, err := certmagic.Listen([]string{name})
	if err != nil {
		log.Fatal(err)
	}

	proxy := &ssltun.Proxy{Name: name, Key: key}
	if err = http.Serve(ln, proxy); err != nil {
		log.Fatal(err)
	}
}
