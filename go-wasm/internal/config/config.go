// Package config loads server configuration from the environment.
package config

import (
	"log"
	"net"
	"os"
	"strconv"
	"strings"
)

// Config holds runtime settings.
type Config struct {
	DatabaseURL string // required; e.g. postgres://user:pass@host:5432/db?sslmode=disable
	Addr        string // listen address
	StaticDir   string // directory containing index.html + assets

	// Metrics / ranking hardening.
	MetricsSalt    string       // salt for visitor/IP hashes; if empty, ranking ingestion is disabled
	TrustedProxies []*net.IPNet // proxies whose X-Forwarded-For we trust (e.g. nginx)
	RateLimitRPS   float64      // token-bucket refill rate per client IP
	RateLimitBurst int          // token-bucket capacity per client IP

	// Stricter bucket for unauthenticated writes (playlist create/update),
	// which hit the DB and — for create — require no auth at all.
	RateLimitWriteRPS   float64
	RateLimitWriteBurst int

	// YouTube Data API v3 key for the playlist track-listing endpoint;
	// if empty, /api/youtube/... responds 503. The daily budget caps upstream
	// fetches per UTC day (cache misses only) so hostile traffic cannot drain
	// the API quota; <= 0 disables the cap.
	YouTubeAPIKey      string
	YouTubeDailyBudget int
}

// Load reads configuration from the environment with sane defaults.
func Load() Config {
	return Config{
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		Addr:           getenv("ADDR", "0.0.0.0:8081"),
		StaticDir:      getenv("STATIC_DIR", "."),
		MetricsSalt:    os.Getenv("METRICS_SALT"),
		TrustedProxies: parseCIDRs(getenv("TRUSTED_PROXIES", "127.0.0.1/32,::1/128")),
		RateLimitRPS:   getenvFloat("RATE_LIMIT_RPS", 5),
		RateLimitBurst: getenvInt("RATE_LIMIT_BURST", 10),

		RateLimitWriteRPS:   getenvFloat("RATE_LIMIT_WRITE_RPS", 1),
		RateLimitWriteBurst: getenvInt("RATE_LIMIT_WRITE_BURST", 5),

		YouTubeAPIKey:      os.Getenv("YOUTUBE_API_KEY"),
		YouTubeDailyBudget: getenvInt("YOUTUBE_DAILY_BUDGET", 3000),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
		log.Printf("config: %s=%q is not a number; using %v", key, v, def)
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		log.Printf("config: %s=%q is not an integer; using %d", key, v, def)
	}
	return def
}

// parseCIDRs accepts a comma-separated list of CIDRs or bare IPs (a bare IP is
// treated as a single-host network) and returns the parsed networks.
func parseCIDRs(s string) []*net.IPNet {
	var nets []*net.IPNet
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, n, err := net.ParseCIDR(part); err == nil {
			nets = append(nets, n)
			continue
		}
		if ip := net.ParseIP(part); ip != nil {
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		log.Printf("config: TRUSTED_PROXIES entry %q is not a valid CIDR or IP; skipping", part)
	}
	return nets
}
