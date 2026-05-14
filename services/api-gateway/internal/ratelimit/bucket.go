package ratelimit

import (
	"net"
	"net/http"
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
	mu      sync.Mutex
	clients map[string]*clientEntry
	rps     float64
	burst   float64
}

func New(rps, burst float64) *IPLimiter {
	il := &IPLimiter{
		clients: make(map[string]*clientEntry),
		rps:     rps,
		burst:   burst,
	}
	go il.evict()
	return il
}

func (il *IPLimiter) evict() {
	for range time.Tick(time.Minute) {
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
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if !il.Allow(ip) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
