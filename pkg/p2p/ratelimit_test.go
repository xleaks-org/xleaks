package p2p

import (
	"testing"
	"time"
)

func TestPostRateLimit(t *testing.T) {
	rl := newPubsubRateLimiter()
	author := "author1"

	// First maxPostsPerMinute posts should be allowed.
	for i := 0; i < maxPostsPerMinute; i++ {
		if !rl.Allow(author, "post") {
			t.Fatalf("post %d should have been allowed (limit is %d)", i+1, maxPostsPerMinute)
		}
	}

	// The 11th post should be rejected.
	if rl.Allow(author, "post") {
		t.Fatal("post beyond the limit should have been rejected")
	}
}

func TestReactionRateLimit(t *testing.T) {
	rl := newPubsubRateLimiter()
	author := "author1"

	// First maxReactionsPerMinute reactions should be allowed.
	for i := 0; i < maxReactionsPerMinute; i++ {
		if !rl.Allow(author, "reaction") {
			t.Fatalf("reaction %d should have been allowed (limit is %d)", i+1, maxReactionsPerMinute)
		}
	}

	// The 101st reaction should be rejected.
	if rl.Allow(author, "reaction") {
		t.Fatal("reaction beyond the limit should have been rejected")
	}
}

func TestRateLimitReset(t *testing.T) {
	rl := newPubsubRateLimiter()
	author := "author1"

	// Exhaust the post limit.
	for i := 0; i < maxPostsPerMinute; i++ {
		rl.Allow(author, "post")
	}

	if rl.Allow(author, "post") {
		t.Fatal("post should be rejected after exhausting limit")
	}

	// Manually expire the window by manipulating the reset time.
	rl.mu.Lock()
	ac := rl.counts[author]
	ac.resetAt = time.Now().Add(-time.Second)
	rl.mu.Unlock()

	// After the window expires, the author should be allowed again.
	if !rl.Allow(author, "post") {
		t.Fatal("post should be allowed after window reset")
	}
}

func TestIndependentAuthors(t *testing.T) {
	rl := newPubsubRateLimiter()
	author1 := "author1"
	author2 := "author2"

	// Exhaust author1's post limit.
	for i := 0; i < maxPostsPerMinute; i++ {
		rl.Allow(author1, "post")
	}

	if rl.Allow(author1, "post") {
		t.Fatal("author1 should be rate limited")
	}

	// author2 should still be allowed.
	if !rl.Allow(author2, "post") {
		t.Fatal("author2 should not be affected by author1's rate limit")
	}
}

func TestUnknownMessageTypeAllowed(t *testing.T) {
	rl := newPubsubRateLimiter()
	author := "author1"

	// Unknown message types should always be allowed.
	for i := 0; i < 200; i++ {
		if !rl.Allow(author, "unknown") {
			t.Fatalf("unknown message type should always be allowed, failed on iteration %d", i+1)
		}
	}
}

func TestCleanupRemovesExpiredEntries(t *testing.T) {
	rl := newPubsubRateLimiter()
	author := "author1"

	rl.Allow(author, "post")

	// Set the reset time to the past.
	rl.mu.Lock()
	rl.counts[author].resetAt = time.Now().Add(-time.Second)
	rl.mu.Unlock()

	rl.Cleanup()

	rl.mu.Lock()
	_, exists := rl.counts[author]
	rl.mu.Unlock()

	if exists {
		t.Fatal("expired entry should have been cleaned up")
	}
}

func TestResetClearsAll(t *testing.T) {
	rl := newPubsubRateLimiter()

	rl.Allow("a", "post")
	rl.Allow("b", "reaction")

	rl.Reset()

	rl.mu.Lock()
	count := len(rl.counts)
	rl.mu.Unlock()

	if count != 0 {
		t.Fatalf("expected 0 entries after Reset, got %d", count)
	}
}
