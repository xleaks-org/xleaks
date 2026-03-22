package p2p

import (
	"sync"
	"time"
)

const (
	// defaultSeenCacheMaxAge is the default duration after which seen entries
	// are eligible for cleanup.
	defaultSeenCacheMaxAge = 10 * time.Minute

	// seenCacheCleanupInterval is how often the background cleanup runs.
	seenCacheCleanupInterval = 5 * time.Minute
)

// seenCache tracks CIDs that have already been processed to provide replay
// protection. Messages with CIDs already in the cache are dropped to prevent
// duplicate processing.
type seenCache struct {
	mu     sync.RWMutex
	seen   map[string]time.Time // CID hex -> when first seen
	maxAge time.Duration

	stopCleanup chan struct{}
}

// newSeenCache creates a new seen cache with the given maximum age for entries.
// It starts a background goroutine that periodically cleans up expired entries.
func newSeenCache(maxAge time.Duration) *seenCache {
	sc := &seenCache{
		seen:        make(map[string]time.Time),
		maxAge:      maxAge,
		stopCleanup: make(chan struct{}),
	}

	go sc.cleanupLoop()

	return sc
}

// HasSeen returns true if the given CID has been seen before and is still
// within the max age window.
func (sc *seenCache) HasSeen(cidHex string) bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	_, ok := sc.seen[cidHex]
	return ok
}

// MarkSeen records a CID as seen. If the CID was already seen, this is a
// no-op (the original timestamp is preserved).
func (sc *seenCache) MarkSeen(cidHex string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if _, exists := sc.seen[cidHex]; !exists {
		sc.seen[cidHex] = time.Now()
	}
}

// CheckAndMark atomically checks if a CID has been seen and marks it if not.
// Returns true if the CID was already seen (i.e., this is a replay).
func (sc *seenCache) CheckAndMark(cidHex string) bool {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if _, exists := sc.seen[cidHex]; exists {
		return true
	}
	sc.seen[cidHex] = time.Now()
	return false
}

// Size returns the number of entries in the cache.
func (sc *seenCache) Size() int {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return len(sc.seen)
}

// cleanup removes entries that are older than maxAge.
func (sc *seenCache) cleanup() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	cutoff := time.Now().Add(-sc.maxAge)
	for cid, seenAt := range sc.seen {
		if seenAt.Before(cutoff) {
			delete(sc.seen, cid)
		}
	}
}

// cleanupLoop runs periodic cleanup in the background.
func (sc *seenCache) cleanupLoop() {
	ticker := time.NewTicker(seenCacheCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sc.cleanup()
		case <-sc.stopCleanup:
			return
		}
	}
}

// Stop halts the background cleanup goroutine. Call this when the cache is
// no longer needed to avoid goroutine leaks.
func (sc *seenCache) Stop() {
	close(sc.stopCleanup)
}
