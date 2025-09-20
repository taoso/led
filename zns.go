package led

import (
	"bytes"
	"encoding/base64"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/miekg/dns"
)

func (p *Proxy) ServeZNS(w http.ResponseWriter, req *http.Request) {
	url := "https://zns.lehu.in" + req.URL.Path

	if req.URL.RawQuery != "" {
		url += "?" + req.URL.RawQuery
	}

	if req.Method == http.MethodGet && !req.URL.Query().Has("dns") {
		w.WriteHeader(http.StatusMovedPermanently)
		w.Header().Set("location", url)
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Body.Close()

	// 供调用备用线路时读取使用🤦‍♂️
	req.Body = io.NopCloser(bytes.NewReader(body))

	q, err := http.NewRequest(req.Method, url, req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	for name, values := range req.Header {
		for _, value := range values {
			q.Header.Add(name, value)
		}
	}

	q.Header.Set("zns-real-addr", req.RemoteAddr)

	c := http.Client{
		Timeout: 1 * time.Second,
	}

	resp, err := c.Do(q)
	if err != nil {
		log.Println("zns query error", err)
		p.fallbackDoh(w, req)
		return
	}
	defer resp.Body.Close()

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (p *Proxy) fallbackDoh(w http.ResponseWriter, req *http.Request) {
	question, err := getDohQuestion(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	addr, err := netip.ParseAddrPort(req.RemoteAddr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if question, err = addSubnet(question, addr.Addr()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp, err := http.Post(p.ZnsUpstream, "application/dns-message", bytes.NewReader(question))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	w.Header().Add("content-type", "application/dns-message")
	io.Copy(w, resp.Body)
}

func getDohQuestion(req *http.Request) (question []byte, err error) {
	if req.Method == http.MethodGet {
		q := req.URL.Query().Get("dns")
		question, err = base64.RawURLEncoding.DecodeString(q)
	} else {
		question, err = io.ReadAll(req.Body)
		req.Body.Close()
	}
	return
}

func addSubnet(q []byte, clientAddr netip.Addr) (question []byte, err error) {
	question = q

	var m dns.Msg
	if err = m.Unpack(q); err != nil {
		return
	}

	e := m.IsEdns0()
	if e == nil {
		return
	}

	for _, o := range e.Option {
		if o.Option() == dns.EDNS0SUBNET {
			a := o.(*dns.EDNS0_SUBNET).Address[:2]
			// ignore empty subnet like 0.0.0.0/0
			if !bytes.HasPrefix(a, []byte{0, 0}) {
				return
			}
			break
		}
	}

	opt := &dns.OPT{Hdr: dns.RR_Header{Name: ".", Rrtype: dns.TypeOPT}}
	ecs := &dns.EDNS0_SUBNET{Code: dns.EDNS0SUBNET}
	var bits int
	if clientAddr.Is4() {
		bits = 24
		ecs.Family = 1
	} else {
		bits = 48
		ecs.Family = 2
	}
	ecs.SourceNetmask = uint8(bits)
	p := netip.PrefixFrom(clientAddr, bits)
	ecs.Address = net.IP(p.Masked().Addr().AsSlice())
	opt.Option = append(opt.Option, ecs)
	m.Extra = []dns.RR{opt}

	question, err = m.Pack()
	return
}
