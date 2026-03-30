package p2p

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xleaks-org/xleaks/pkg/content"
)

func TestContentExchangeFetchContentDeduplicatesConcurrentRequests(t *testing.T) {
	t.Parallel()

	ce := NewContentExchange(nil)
	cid, err := content.ComputeCID([]byte("remote-media"))
	if err != nil {
		t.Fatalf("ComputeCID: %v", err)
	}
	cidHex := hex.EncodeToString(cid)
	want := []byte("fetched bytes")

	release := make(chan struct{})
	started := make(chan struct{}, 1)
	var calls atomic.Int32
	ce.fetchRemote = func(ctx context.Context, gotCID string, gotCIDBytes []byte) ([]byte, error) {
		if calls.Add(1) == 1 {
			started <- struct{}{}
		}
		if gotCID != cidHex {
			t.Fatalf("cidHex = %q, want %q", gotCID, cidHex)
		}
		if !bytes.Equal(gotCIDBytes, cid) {
			t.Fatalf("cidBytes = %x, want %x", gotCIDBytes, cid)
		}
		select {
		case <-release:
			return want, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	var wg sync.WaitGroup
	results := make([][]byte, 2)
	errs := make([]error, 2)
	wg.Add(1)
	go func() {
		defer wg.Done()
		results[0], errs[0] = ce.FetchContent(context.Background(), cidHex, nil)
	}()
	<-started
	wg.Add(1)
	go func() {
		defer wg.Done()
		results[1], errs[1] = ce.FetchContent(context.Background(), cidHex, nil)
	}()
	time.Sleep(25 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("remote fetch calls = %d, want 1", got)
	}
	for i := range errs {
		if errs[i] != nil {
			t.Fatalf("FetchContent #%d error = %v", i, errs[i])
		}
		if !bytes.Equal(results[i], want) {
			t.Fatalf("FetchContent #%d data = %q, want %q", i, results[i], want)
		}
	}
}

func TestContentExchangeFetchContentBacksOffAfterFailure(t *testing.T) {
	t.Parallel()

	ce := NewContentExchange(nil)
	cid, err := content.ComputeCID([]byte("remote-media"))
	if err != nil {
		t.Fatalf("ComputeCID: %v", err)
	}
	cidHex := hex.EncodeToString(cid)

	var calls atomic.Int32
	ce.fetchRemote = func(ctx context.Context, gotCID string, gotCIDBytes []byte) ([]byte, error) {
		calls.Add(1)
		return nil, errors.New("boom")
	}

	if _, err := ce.FetchContent(context.Background(), cidHex, nil); err == nil {
		t.Fatal("expected initial fetch error")
	}
	if _, err := ce.FetchContent(context.Background(), cidHex, nil); !errors.Is(err, ErrFetchBackoffActive) {
		t.Fatalf("second FetchContent error = %v, want ErrFetchBackoffActive", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("remote fetch calls = %d, want 1", got)
	}
}
