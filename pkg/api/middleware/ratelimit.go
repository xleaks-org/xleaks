package middleware

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// visitor tracks request counts for a single IP within a time window.
type visitor struct {
	count   int
	resetAt time.Time
}

// routeLimit defines the rate limit for a specific path prefix.
type routeLimit struct {
	maxRequests int
	window      time.Duration
}

// RouteRateLimiter applies per-route, per-IP rate limits. Each path prefix
// has its own limit configuration, and each IP address is tracked independently.
type RouteRateLimiter struct {
	mu       sync.Mutex
	limits   map[string]*routeLimit            // path prefix -> limit config
	visitors map[string]map[string]*visitor     // path prefix -> IP -> visitor
	global   *routeLimit                        // global fallback limit
	globalV  map[string]*visitor                // IP -> visitor for global limit
	stop     chan struct{}                       // signals the cleanup goroutine to exit
}

// NewRouteRateLimiter creates a new per-route rate limiter.
// Call Stop() when the limiter is no longer needed to release the background goroutine.
func NewRouteRateLimiter() *RouteRateLimiter {
	rl := &RouteRateLimiter{
		limits:   make(map[string]*routeLimit),
		visitors: make(map[string]map[string]*visitor),
		globalV:  make(map[string]*visitor),
		stop:     make(chan struct{}),
	}

	// Start background cleanup.
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				rl.cleanup()
			case <-rl.stop:
				return
			}
		}
	}()

	return rl
}

// Stop shuts down the background cleanup goroutine and releases its resources.
// It is safe to call Stop multiple times.
func (rl *RouteRateLimiter) Stop() {
	select {
	case <-rl.stop:
		// Already stopped.
	default:
		close(rl.stop)
	}
}

// AddLimit sets a rate limit for a specific path prefix. Requests whose
// path starts with pathPrefix will be subject to maxRequests per window.
func (rl *RouteRateLimiter) AddLimit(pathPrefix string, maxRequests int, window time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.limits[pathPrefix] = &routeLimit{
		maxRequests: maxRequests,
		window:      window,
	}
	rl.visitors[pathPrefix] = make(map[string]*visitor)
}

// SetGlobalLimit sets a global per-IP rate limit that applies to all requests
// regardless of route.
func (rl *RouteRateLimiter) SetGlobalLimit(maxRequests int, window time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.global = &routeLimit{
		maxRequests: maxRequests,
		window:      window,
	}
}

// Middleware returns an http.Handler middleware that enforces rate limits.
func (rl *RouteRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)

		rl.mu.Lock()

		// Check global limit first.
		if rl.global != nil {
			if !rl.allowLocked(rl.globalV, ip, rl.global) {
				retryAfter := rl.retryAfterLocked(rl.globalV, ip)
				rl.mu.Unlock()
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
		}

		// Find the most specific matching route limit.
		prefix, limit := rl.matchRoute(r.Method, r.URL.Path)
		if limit != nil {
			visitors := rl.visitors[prefix]
			if !rl.allowLocked(visitors, ip, limit) {
				retryAfter := rl.retryAfterLocked(visitors, ip)
				rl.mu.Unlock()
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
		}

		rl.mu.Unlock()
		next.ServeHTTP(w, r)
	})
}

// matchRoute finds the longest matching path prefix for the given method+path.
// Route keys are stored as "METHOD path" (e.g. "POST /api/posts") or just
// a method name for catch-all method limits (e.g. "GET").
func (rl *RouteRateLimiter) matchRoute(method, path string) (string, *routeLimit) {
	bestKey := ""
	var bestLimit *routeLimit

	for prefix, limit := range rl.limits {
		// Check "METHOD /path" style prefixes.
		if strings.Contains(prefix, " ") {
			parts := strings.SplitN(prefix, " ", 2)
			m, p := parts[0], parts[1]
			if method == m && strings.HasPrefix(path, p) {
				if len(prefix) > len(bestKey) {
					bestKey = prefix
					bestLimit = limit
				}
			}
		} else {
			// Method-only prefix (e.g. "GET") matches all paths for that method.
			if method == prefix {
				if bestLimit == nil || len(prefix) > len(bestKey) {
					bestKey = prefix
					bestLimit = limit
				}
			}
		}
	}

	return bestKey, bestLimit
}

// allowLocked checks and increments the rate counter for the given IP.
// Must be called with rl.mu held.
func (rl *RouteRateLimiter) allowLocked(visitors map[string]*visitor, ip string, limit *routeLimit) bool {
	now := time.Now()

	v, exists := visitors[ip]
	if !exists || now.After(v.resetAt) {
		visitors[ip] = &visitor{count: 1, resetAt: now.Add(limit.window)}
		return true
	}

	if v.count >= limit.maxRequests {
		return false
	}

	v.count++
	return true
}

// retryAfterLocked returns the number of seconds until the rate limit window
// resets for the given IP. Must be called with rl.mu held.
func (rl *RouteRateLimiter) retryAfterLocked(visitors map[string]*visitor, ip string) int {
	v, exists := visitors[ip]
	if !exists {
		return 1
	}
	seconds := int(time.Until(v.resetAt).Seconds()) + 1
	if seconds < 1 {
		return 1
	}
	return seconds
}

// cleanup removes expired visitor entries to prevent unbounded memory growth.
func (rl *RouteRateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	for _, visitors := range rl.visitors {
		for ip, v := range visitors {
			if now.After(v.resetAt) {
				delete(visitors, ip)
			}
		}
	}

	for ip, v := range rl.globalV {
		if now.After(v.resetAt) {
			delete(rl.globalV, ip)
		}
	}
}

// extractIP extracts the client IP from the request, stripping the port.
func extractIP(r *http.Request) string {
	// Check X-Forwarded-For first (for reverse proxy setups).
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}

	// Check X-Real-IP.
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
