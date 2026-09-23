package auth

import (
	"net"
	"net/http/httptest"
	"testing"
)

func TestUntrustedForwardedHeadersIgnored(t *testing.T) {
	r := httptest.NewRequest("GET", "http://example/", nil)
	r.RemoteAddr = "10.0.0.2:1234"
	r.Header.Set("X-Forwarded-For", "127.0.0.1")
	r = WithRequestMeta(r, nil)
	if Meta(r).IP != "10.0.0.2" {
		t.Fatalf("trusted spoofed IP: %s", Meta(r).IP)
	}
	_, network, _ := net.ParseCIDR("10.0.0.0/8")
	r = httptest.NewRequest("GET", "http://example/", nil)
	r.RemoteAddr = "10.0.0.2:1234"
	r.Header.Set("X-Forwarded-For", "127.0.0.1")
	r = WithRequestMeta(r, []*net.IPNet{network})
	if Meta(r).IP != "127.0.0.1" {
		t.Fatalf("trusted proxy IP not used: %s", Meta(r).IP)
	}
}
