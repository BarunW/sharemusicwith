package api

import (
	"net"
	"net/http"
	"testing"
	"time"
)

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

func TestClientIP(t *testing.T) {
	trusted := []*net.IPNet{mustCIDR("10.0.0.0/8"), mustCIDR("127.0.0.1/32")}
	tests := []struct {
		name, xff, remote, want string
	}{
		{"no xff uses transport peer", "", "203.0.113.7:5555", "203.0.113.7"},
		{"single client via trusted proxy", "198.51.100.9", "10.1.2.3:80", "198.51.100.9"},
		{"spoofed prepended; real is rightmost untrusted", "1.1.1.1, 198.51.100.9", "10.1.2.3:80", "198.51.100.9"},
		{"client behind two trusted proxies", "198.51.100.9, 10.8.8.8", "10.1.2.3:80", "198.51.100.9"},
		{"fully trusted chain falls back to peer", "10.9.9.9, 10.8.8.8", "10.1.2.3:80", "10.1.2.3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{Header: http.Header{}, RemoteAddr: tt.remote}
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}
			got := clientIP(r, trusted)
			if got == nil || got.String() != tt.want {
				t.Fatalf("clientIP = %v, want %s", got, tt.want)
			}
		})
	}
}

func TestIsBotUA(t *testing.T) {
	bots := []string{"", "curl/8.1", "python-requests/2.31", "Mozilla/5.0 (compatible; Googlebot/2.1)", "Go-http-client/1.1", "okhttp/4"}
	humans := []string{
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124 Safari/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605 Safari/604.1",
	}
	for _, ua := range bots {
		if !isBotUA(ua) {
			t.Errorf("isBotUA(%q) = false, want true", ua)
		}
	}
	for _, ua := range humans {
		if isBotUA(ua) {
			t.Errorf("isBotUA(%q) = true, want false", ua)
		}
	}
}

func TestVisitorHashStableAndSalted(t *testing.T) {
	ip := net.ParseIP("9.9.9.9")
	a := visitorHash("salt", ip, "ua")
	if a == nil || len(a) != 32 {
		t.Fatalf("want 32-byte digest, got %v", a)
	}
	if string(a) != string(visitorHash("salt", ip, "ua")) {
		t.Fatal("hash should be stable for same inputs")
	}
	if string(a) == string(visitorHash("salt2", ip, "ua")) {
		t.Fatal("different salt should change the hash")
	}
	if visitorHash("", ip, "ua") != nil {
		t.Fatal("empty salt should yield nil (fail-closed)")
	}
	if visitorHash("salt", nil, "ua") != nil {
		t.Fatal("nil ip should yield nil")
	}
}

func TestRateLimiterAllow(t *testing.T) {
	rl := newRateLimiter(1, 3, nil) // 1 token/sec, burst 3
	base := time.Now()
	rl.now = func() time.Time { return base }

	allowed := 0
	for i := 0; i < 5; i++ {
		if rl.allow("k") {
			allowed++
		}
	}
	if allowed != 3 {
		t.Fatalf("burst: allowed %d, want 3", allowed)
	}

	rl.now = func() time.Time { return base.Add(2 * time.Second) } // ~2 tokens back
	allowed = 0
	for i := 0; i < 5; i++ {
		if rl.allow("k") {
			allowed++
		}
	}
	if allowed < 1 || allowed > 3 {
		t.Fatalf("refill: allowed %d, want ~2", allowed)
	}

	// distinct keys have independent buckets
	if !rl.allow("other") {
		t.Fatal("a fresh key should be allowed")
	}
}

func TestDiscoverSections(t *testing.T) {
	for _, s := range []string{"trending", "today", "popular", "new"} {
		if !discoverSections[s] {
			t.Errorf("discoverSections missing %q", s)
		}
	}
}
