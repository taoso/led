package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/taoso/led"
)

var root, http1 string

func init() {
	flag.StringVar(&root, "root", "", "static server root")
	flag.StringVar(&http1, "http1", "", "listen address for http1")

	log.SetOutput(os.Stderr)
}

func main() {
	flag.Parse()

	lnH1, err := net.Listen("tcp", http1)
	if err != nil {
		panic(err)
	}

	svr := &led.LED{Root: root}

	if id := os.Getenv("ALIPAY_APP_ID"); id != "" {
		err := svr.SetAlipay(
			id,
			os.Getenv("ALIPAY_PRIVATE_KEY"),
			os.Getenv("ALIPAY_PUBLIC_KEY"),
		)
		if err != nil {
			panic(err)
		}
	}

	s := &http.Server{Handler: svr}

	log.Println("listening on", lnH1.Addr())
	log.Fatal(s.Serve(lnH1))
}
