package indexer

import (
	"context"
	"testing"
	"time"
)

func TestIndexerClientEvictsOldestCacheEntryWhenAtCapacity(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	client := NewIndexerClient(context.Background())
	defer client.Close()
	client.now = func() time.Time { return now }
	client.maxCacheEntries = 2

	client.putInCache("first", "a")
	now = now.Add(time.Second)
	client.putInCache("second", "b")
	now = now.Add(time.Second)
	client.putInCache("third", "c")

	if got := client.getFromCache("first"); got != nil {
		t.Fatalf("oldest cache entry = %v, want nil", got)
	}
	if got := client.getFromCache("second"); got != "b" {
		t.Fatalf("second cache entry = %v, want %q", got, "b")
	}
	if got := client.getFromCache("third"); got != "c" {
		t.Fatalf("third cache entry = %v, want %q", got, "c")
	}
}

func TestIndexerClientEvictsExpiredCacheEntryOnAccess(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	client := NewIndexerClient(context.Background())
	defer client.Close()
	client.now = func() time.Time { return now }
	client.cacheTTL = time.Minute

	client.putInCache("search", "value")
	now = now.Add(2 * time.Minute)

	if got := client.getFromCache("search"); got != nil {
		t.Fatalf("expired cache entry = %v, want nil", got)
	}
	if got := len(client.cache); got != 0 {
		t.Fatalf("cache size = %d, want 0", got)
	}
}

func TestIndexerClientCloseIsIdempotent(t *testing.T) {
	t.Parallel()

	client := NewIndexerClient(context.Background())
	client.Close()
	client.Close()
}
