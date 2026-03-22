package p2p

import (
	"testing"
	"time"
)

func TestCheckAndMarkFirstCall(t *testing.T) {
	sc := newSeenCache(10 * time.Minute)
	defer sc.Stop()

	// First call should return false (not seen yet).
	if sc.CheckAndMark("cid1") {
		t.Fatal("first CheckAndMark should return false (not previously seen)")
	}
}

func TestCheckAndMarkSecondCall(t *testing.T) {
	sc := newSeenCache(10 * time.Minute)
	defer sc.Stop()

	sc.CheckAndMark("cid1")

	// Second call should return true (already seen).
	if !sc.CheckAndMark("cid1") {
		t.Fatal("second CheckAndMark should return true (already seen)")
	}
}

func TestCacheExpiry(t *testing.T) {
	// Use a very short max age so entries expire quickly.
	sc := newSeenCache(50 * time.Millisecond)
	defer sc.Stop()

	sc.CheckAndMark("cid1")

	if !sc.HasSeen("cid1") {
		t.Fatal("cid1 should be seen immediately after marking")
	}

	// Wait for the entry to expire.
	time.Sleep(100 * time.Millisecond)

	// Trigger manual cleanup since the background ticker interval is 5min.
	sc.cleanup()

	if sc.HasSeen("cid1") {
		t.Fatal("cid1 should have been forgotten after max age")
	}
}

func TestStopPreventsLeak(t *testing.T) {
	sc := newSeenCache(10 * time.Minute)

	// Stop should return without blocking, proving the goroutine exits.
	sc.Stop()

	// After Stop, the cache should still be usable for reads/writes
	// (no panic), even though the cleanup goroutine has stopped.
	sc.MarkSeen("cid1")
	if !sc.HasSeen("cid1") {
		t.Fatal("cache should still work after Stop")
	}
}

func TestMarkSeenPreservesTimestamp(t *testing.T) {
	sc := newSeenCache(10 * time.Minute)
	defer sc.Stop()

	sc.MarkSeen("cid1")

	sc.mu.RLock()
	firstTime := sc.seen["cid1"]
	sc.mu.RUnlock()

	// Small delay to ensure time difference.
	time.Sleep(5 * time.Millisecond)

	// Second MarkSeen should be a no-op.
	sc.MarkSeen("cid1")

	sc.mu.RLock()
	secondTime := sc.seen["cid1"]
	sc.mu.RUnlock()

	if !firstTime.Equal(secondTime) {
		t.Fatal("MarkSeen should preserve the original timestamp")
	}
}

func TestSize(t *testing.T) {
	sc := newSeenCache(10 * time.Minute)
	defer sc.Stop()

	if sc.Size() != 0 {
		t.Fatalf("expected size 0, got %d", sc.Size())
	}

	sc.MarkSeen("a")
	sc.MarkSeen("b")
	sc.MarkSeen("c")

	if sc.Size() != 3 {
		t.Fatalf("expected size 3, got %d", sc.Size())
	}

	// Marking the same CID again should not increase size.
	sc.MarkSeen("a")
	if sc.Size() != 3 {
		t.Fatalf("expected size 3 after duplicate mark, got %d", sc.Size())
	}
}
