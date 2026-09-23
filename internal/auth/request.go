package auth

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type requestContextKey string

const metaKey requestContextKey = "request-meta"

type RequestMeta struct {
	IP, Protocol       string
	Insecure, Loopback bool
}

func WithRequestMeta(r *http.Request, trusted []*net.IPNet) *http.Request {
	meta := RequestMeta{Protocol: "http"}
	if r.TLS != nil {
		meta.Protocol = "https"
	}
	peer := clientIP(r.RemoteAddr)
	if isTrusted(peer, trusted) {
		if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
			meta.IP = clientIP(forwarded)
		}
		if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto == "http" || proto == "https" {
			meta.Protocol = proto
		}
	}
	if meta.IP == "" {
		meta.IP = peer
	}
	ip := net.ParseIP(meta.IP)
	meta.Loopback = ip != nil && ip.IsLoopback()
	meta.Insecure = meta.Protocol != "https"
	return r.WithContext(context.WithValue(r.Context(), metaKey, meta))
}

func Meta(r *http.Request) RequestMeta {
	if v, ok := r.Context().Value(metaKey).(RequestMeta); ok {
		return v
	}
	return RequestMeta{IP: clientIP(r.RemoteAddr), Protocol: "http", Insecure: true}
}
func clientIP(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return host
	}
	return strings.TrimSpace(strings.Split(address, ",")[0])
}
func isTrusted(ip string, networks []*net.IPNet) bool {
	parsed := net.ParseIP(ip)
	for _, network := range networks {
		if parsed != nil && network.Contains(parsed) {
			return true
		}
	}
	return false
}
func ParseTrustedProxies(values []string) ([]*net.IPNet, error) {
	result := make([]*net.IPNet, 0, len(values))
	for _, value := range values {
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			return nil, err
		}
		result = append(result, network)
	}
	return result, nil
}
