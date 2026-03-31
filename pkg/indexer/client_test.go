package indexer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestNewIndexerHTTPClientSetsTransportTimeouts(t *testing.T) {
	t.Parallel()

	client := newIndexerHTTPClient()
	if client.Timeout != defaultIndexerClientTimeout {
		t.Fatalf("Timeout = %s, want %s", client.Timeout, defaultIndexerClientTimeout)
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport type = %T, want *http.Transport", client.Transport)
	}
	if transport.DialContext == nil {
		t.Fatal("expected DialContext to be configured")
	}
	if transport.TLSHandshakeTimeout != indexerClientTLSHandshakeTimeout {
		t.Fatalf("TLSHandshakeTimeout = %s, want %s", transport.TLSHandshakeTimeout, indexerClientTLSHandshakeTimeout)
	}
	if transport.ResponseHeaderTimeout != indexerClientResponseHeaderTimeout {
		t.Fatalf("ResponseHeaderTimeout = %s, want %s", transport.ResponseHeaderTimeout, indexerClientResponseHeaderTimeout)
	}
	if transport.MaxResponseHeaderBytes != 32<<10 {
		t.Fatalf("MaxResponseHeaderBytes = %d, want %d", transport.MaxResponseHeaderBytes, 32<<10)
	}
}

func TestIndexerClientRejectsOversizedIndexerResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("x", maxIndexerClientResponseBytes+1)))
	}))
	defer server.Close()

	client := NewIndexerClient(context.Background())
	defer client.Close()
	client.SetIndexers([]string{server.URL})

	_, err := client.GetExplorePublishers(5)
	if err == nil {
		t.Fatal("expected oversized response to be rejected")
	}
	if !strings.Contains(err.Error(), "response too large") {
		t.Fatalf("error = %v, want oversized-response error", err)
	}
}
