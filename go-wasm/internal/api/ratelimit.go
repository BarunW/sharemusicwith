package api

import (
	"math"
	"net"
	"net/http"
	"sync"
	"time"
)

// tokenBucket is a single client's refilling allowance.
type tokenBucket struct {
	tokens   float64
	last     time.Time
	lastSeen time.Time
}

// rateLimiter is an in-memory per-IP token bucket. It throttles the unauthenticated,
// abuse-prone endpoints (event beacons + public reads) so a loop can't mint events
// or hammer the DB.
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
	rps     float64 // refill rate (tokens/sec)
	burst   float64 // bucket capacity
	trusted []*net.IPNet
	now     func() time.Time // injectable for tests
}

func newRateLimiter(rps float64, burst int, trusted []*net.IPNet) *rateLimiter {
	if rps <= 0 {
		rps = 5
	}
	if burst <= 0 {
		burst = 10
	}
	rl := &rateLimiter{
		buckets: make(map[string]*tokenBucket),
		rps:     rps,
		burst:   float64(burst),
		trusted: trusted,
		now:     time.Now,
	}
	go rl.janitor(5*time.Minute, 10*time.Minute)
	return rl
}

// allow consumes a token for key, refilling first. Returns false when empty.
func (rl *rateLimiter) allow(key string) bool {
	now := rl.now()
	rl.mu.Lock()
	defer rl.mu.Unlock()
	b := rl.buckets[key]
	if b == nil {
		b = &tokenBucket{tokens: rl.burst, last: now}
		rl.buckets[key] = b
	}
	b.tokens = math.Min(rl.burst, b.tokens+now.Sub(b.last).Seconds()*rl.rps)
	b.last = now
	b.lastSeen = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := "unknown"
		if ip := clientIP(r, rl.trusted); ip != nil {
			key = ip.String()
		}
		if !rl.allow(key) {
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusTooManyRequests, "rate_limited", "")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// cleanup drops buckets unused for longer than ttl, bounding memory.
func (rl *rateLimiter) cleanup(ttl time.Duration) {
	cutoff := rl.now().Add(-ttl)
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for k, b := range rl.buckets {
		if b.lastSeen.Before(cutoff) {
			delete(rl.buckets, k)
		}
	}
}

func (rl *rateLimiter) janitor(every, ttl time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for range t.C {
		rl.cleanup(ttl)
	}
}
