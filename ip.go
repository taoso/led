package ssltun

import (
	"fmt"
	"net"
	"sync"
)

var maxIP uint16
var usedIPs = map[uint16]bool{}
var m sync.Mutex

func releaseIP(ips ...net.IP) {
	m.Lock()
	defer m.Unlock()

	for _, ip := range ips {
		ip = ip.To4()

		i := (uint16(ip[2]) << 8) ^ uint16(ip[3])
		delete(usedIPs, i)
	}
}

func nextIP() (ip net.IP) {
	m.Lock()
	defer m.Unlock()

	n := 0
	for ; maxIP == 0 || usedIPs[maxIP]; maxIP++ {
		n++
		if n > 0xffff {
			return
		}
	}

	usedIPs[maxIP] = true

	s := fmt.Sprintf("10.86.%d.%d", (maxIP>>8)&0xff, maxIP&0xff)

	ip = net.ParseIP(s)
	return
}
