package main

import (
	"crypto/tls"
	"encoding/base64"
	"flag"
	"io"
	"log"
	"net"
	"os/exec"

	"github.com/songgao/water"
)

var name, key string

func init() {
	flag.StringVar(&name, "name", "", "server domain name")
	flag.StringVar(&key, "key", "", "server auth key")
}

func main() {
	flag.Parse()
	if name == "" || key == "" {
		flag.Usage()
		return
	}

	c, err := net.Dial("tcp", name+":443")
	if err != nil {
		log.Panic(err)
	}
	defer c.Close()

	c = tls.Client(c, &tls.Config{
		ServerName: name,
		MinVersion: tls.VersionTLS13,
		NextProtos: []string{"http/1.1"},
	})

	auth := base64.StdEncoding.EncodeToString([]byte(key + ":"))
	req := "CONNECT * HTTP/1.1\r\nProxy-Authorization: Basic " + auth + "\r\n\r\n"
	if _, err = c.Write([]byte(req)); err != nil {
		return
	}

	buf := make([]byte, 8)
	if _, err := io.ReadFull(c, buf); err != nil {
		log.Panic(err)
	}

	clientIP := net.IP(buf[:4]).String()
	hostIP := net.IP(buf[4:]).String()

	log.Printf("client %s -> %s", clientIP, hostIP)

	tun, err := water.New(water.Config{DeviceType: water.TUN})
	if err != nil {
		log.Panic(err)
	}
	defer tun.Close()

	cmd := exec.Command("/sbin/ifconfig", tun.Name(), clientIP, hostIP, "up")
	if err := cmd.Run(); err != nil {
		log.Panic(err)
		return
	}

	go func() {
		io.Copy(c, tun)
	}()

	io.Copy(tun, c)
	return
}
