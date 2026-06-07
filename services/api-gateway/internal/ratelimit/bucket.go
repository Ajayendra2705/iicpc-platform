package ratelimit

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type tokenBucket struct {
	mu     sync.Mutex
	tokens float64
	last   time.Time
	rps    float64
	burst  float64
}

func newBucket(rps, burst float64) *tokenBucket {
	return &tokenBucket{tokens: burst, last: time.Now(), rps: rps, burst: burst}
}

func (b *tokenBucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	b.tokens += now.Sub(b.last).Seconds() * b.rps
	if b.tokens > b.burst {
		b.tokens = b.burst
	}
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

type clientEntry struct {
	bkt  *tokenBucket
	seen time.Time
}

type IPLimiter struct {
	mu          sync.Mutex
	clients     map[string]*clientEntry
	rps         float64
	burst       float64
	trustedHops int
}

// New builds an IP rate limiter. trustedHops is the number of trusted reverse
// proxies in front of the gateway: 0 keys on the transport RemoteAddr (correct
// when hit directly); N keys on the Nth X-Forwarded-For entry from the right —
// the address the closest trusted proxy observed, which a client cannot spoof.
func New(rps, burst float64, trustedHops int) *IPLimiter {
	il := &IPLimiter{
		clients:     make(map[string]*clientEntry),
		rps:         rps,
		burst:       burst,
		trustedHops: trustedHops,
	}
	go il.evict()
	return il
}

// clientIP resolves the rate-limit key for a request given the trusted-proxy
// count. See New for the trust model. Falls back to RemoteAddr when the
// X-Forwarded-For header is missing or shorter than trustedHops.
func clientIP(r *http.Request, trustedHops int) string {
	host := func(addr string) string {
		if h, _, err := net.SplitHostPort(addr); err == nil {
			return h
		}
		return addr
	}
	if trustedHops <= 0 {
		return host(r.RemoteAddr)
	}
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return host(r.RemoteAddr)
	}
	parts := strings.Split(xff, ",")
	idx := len(parts) - trustedHops
	if idx < 0 || idx >= len(parts) {
		return host(r.RemoteAddr)
	}
	return strings.TrimSpace(parts[idx])
}

func (il *IPLimiter) evict() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		il.mu.Lock()
		for ip, e := range il.clients {
			if time.Since(e.seen) > 5*time.Minute {
				delete(il.clients, ip)
			}
		}
		il.mu.Unlock()
	}
}

func (il *IPLimiter) Allow(ip string) bool {
	il.mu.Lock()
	e, ok := il.clients[ip]
	if !ok {
		e = &clientEntry{bkt: newBucket(il.rps, il.burst)}
		il.clients[ip] = e
	}
	e.seen = time.Now()
	il.mu.Unlock()
	return e.bkt.allow()
}

func (il *IPLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r, il.trustedHops)
		if !il.Allow(ip) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
