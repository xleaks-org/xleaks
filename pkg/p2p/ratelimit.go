package p2p

import (
	"sync"
	"time"
)

const (
	// maxPostsPerMinute is the maximum number of post messages allowed per
	// author per minute.
	maxPostsPerMinute = 10

	// maxReactionsPerMinute is the maximum number of reaction messages allowed
	// per author per minute.
	maxReactionsPerMinute = 100

	// rateLimitWindow is the time window for rate limiting.
	rateLimitWindow = time.Minute
)

// pubsubRateLimiter tracks per-author message rates and enforces limits.
type pubsubRateLimiter struct {
	mu     sync.Mutex
	counts map[string]*authorCounts // author pubkey hex -> counts
}

// authorCounts tracks message counts for a single author within a time window.
type authorCounts struct {
	posts     int
	reactions int
	resetAt   time.Time
}

// newPubsubRateLimiter creates a new rate limiter for pubsub messages.
// It starts a background goroutine that periodically cleans up expired entries.
func newPubsubRateLimiter() *pubsubRateLimiter {
	rl := &pubsubRateLimiter{
		counts: make(map[string]*authorCounts),
	}
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			rl.Cleanup()
		}
	}()
	return rl
}

// Allow checks whether a message of the given type from the given author
// should be allowed. It returns true if the message is within rate limits,
// false if it should be dropped. The msgType should be "post" or "reaction".
func (rl *pubsubRateLimiter) Allow(authorHex string, msgType string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	ac, exists := rl.counts[authorHex]
	if !exists {
		ac = &authorCounts{
			resetAt: now.Add(rateLimitWindow),
		}
		rl.counts[authorHex] = ac
	}

	// Reset counts if the window has expired.
	if now.After(ac.resetAt) {
		ac.posts = 0
		ac.reactions = 0
		ac.resetAt = now.Add(rateLimitWindow)
	}

	switch msgType {
	case "post":
		if ac.posts >= maxPostsPerMinute {
			return false
		}
		ac.posts++
	case "reaction":
		if ac.reactions >= maxReactionsPerMinute {
			return false
		}
		ac.reactions++
	default:
		// Unknown message types are allowed (not rate-limited).
	}

	return true
}

// Reset clears all rate limit state. This is primarily useful for testing.
func (rl *pubsubRateLimiter) Reset() {
	rl.mu.Lock()
	rl.counts = make(map[string]*authorCounts)
	rl.mu.Unlock()
}

// Cleanup removes expired entries to prevent unbounded memory growth.
func (rl *pubsubRateLimiter) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for author, ac := range rl.counts {
		if now.After(ac.resetAt) {
			delete(rl.counts, author)
		}
	}
}
