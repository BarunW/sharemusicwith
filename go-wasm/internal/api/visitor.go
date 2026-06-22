package api

import (
	"crypto/sha256"
	"net"
	"strings"
)

// visitorHash is a salted digest of (IP, User-Agent) used to dedupe metric events
// to one per visitor per day. Returns nil when we lack a salt or IP, in which
// case the event is stored but never counted toward rankings (fail-closed).
func visitorHash(salt string, ip net.IP, ua string) []byte {
	if salt == "" || ip == nil {
		return nil
	}
	sum := sha256.Sum256([]byte(salt + "|v|" + ip.String() + "|" + ua))
	return sum[:]
}

// ipHashOf is a salted digest of the IP alone, for future per-IP contribution
// caps. Raw IP is never stored.
func ipHashOf(salt string, ip net.IP) []byte {
	if salt == "" || ip == nil {
		return nil
	}
	sum := sha256.Sum256([]byte(salt + "|ip|" + ip.String()))
	return sum[:]
}

// botMarkers are case-insensitive substrings that mark a non-human user agent.
var botMarkers = []string{
	"bot", "crawl", "spider", "slurp", "curl", "wget", "python-requests",
	"httpclient", "go-http-client", "headless", "phantomjs", "scrapy",
	"libwww", "okhttp", "java/", "axios", "node-fetch",
}

// isBotUA reports whether a User-Agent looks automated. Empty UAs count as bots.
func isBotUA(ua string) bool {
	if strings.TrimSpace(ua) == "" {
		return true
	}
	l := strings.ToLower(ua)
	for _, m := range botMarkers {
		if strings.Contains(l, m) {
			return true
		}
	}
	return false
}
