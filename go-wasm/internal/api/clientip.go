package api

import (
	"net"
	"net/http"
	"strings"
)

// clientIP returns the real client IP, honoring X-Forwarded-For only for hops we
// trust. We walk XFF right-to-left and return the first address that is NOT a
// trusted proxy (that's the closest hop the trusted chain actually observed). If
// XFF is absent or fully trusted, we fall back to the transport peer
// (r.RemoteAddr). Never blindly trust XFF from an untrusted source.
func clientIP(r *http.Request, trusted []*net.IPNet) net.IP {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			ip := net.ParseIP(strings.TrimSpace(parts[i]))
			if ip == nil {
				continue
			}
			if !ipInAny(ip, trusted) {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return net.ParseIP(strings.TrimSpace(host))
}

func ipInAny(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n != nil && n.Contains(ip) {
			return true
		}
	}
	return false
}
