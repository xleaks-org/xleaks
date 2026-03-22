package middleware

import (
	"net/http"
	"sync"
	"time"
)

type rateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     int
	window   time.Duration
}

type visitor struct {
	count    int
	resetAt  time.Time
}

// RateLimit returns middleware that limits requests per IP per time window.
func RateLimit(maxRequests int, window time.Duration) func(http.Handler) http.Handler {
	rl := &rateLimiter{
		visitors: make(map[string]*visitor),
		rate:     maxRequests,
		window:   window,
	}

	// Cleanup stale entries periodically.
	go func() {
		for {
			time.Sleep(window)
			rl.cleanup()
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !rl.allow(r.RemoteAddr) {
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	v, exists := rl.visitors[key]
	if !exists || now.After(v.resetAt) {
		rl.visitors[key] = &visitor{count: 1, resetAt: now.Add(rl.window)}
		return true
	}

	if v.count >= rl.rate {
		return false
	}

	v.count++
	return true
}

func (rl *rateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for key, v := range rl.visitors {
		if now.After(v.resetAt) {
			delete(rl.visitors, key)
		}
	}
}
