package ssltun

import "testing"

func TestIP(t *testing.T) {
	ip1 := nextIP()
	if ip1.String() != "10.86.0.1" {
		t.Error("invalid ip", ip1)
	}

	ip2 := nextIP()
	if ip2.String() != "10.86.0.2" {
		t.Error("invalid ip", ip2)
	}

	maxIP = 0
	ip3 := nextIP()
	if ip3.String() != "10.86.0.3" {
		t.Error("invalid ip", ip3)
	}

	maxIP = 0
	releaseIP(ip2)
	ip4 := nextIP()
	if ip4.String() != "10.86.0.2" {
		t.Error("invalid ip", ip4)
	}
}
