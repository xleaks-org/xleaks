package p2p

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"os"
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

func TestContentExchangeFetchContentToTempFileBacksOffAfterFailure(t *testing.T) {
	t.Parallel()

	ce := NewContentExchange(nil)
	cid, err := content.ComputeCID([]byte("remote-media"))
	if err != nil {
		t.Fatalf("ComputeCID: %v", err)
	}
	cidHex := hex.EncodeToString(cid)

	var calls atomic.Int32
	ce.fetchRemoteToFile = func(ctx context.Context, gotCID string, gotCIDBytes []byte) (*FetchedContentFile, error) {
		calls.Add(1)
		return nil, errors.New("boom")
	}

	if _, err := ce.FetchContentToTempFile(context.Background(), cidHex); err == nil {
		t.Fatal("expected initial temp-file fetch error")
	}
	if _, err := ce.FetchContentToTempFile(context.Background(), cidHex); !errors.Is(err, ErrFetchBackoffActive) {
		t.Fatalf("second FetchContentToTempFile error = %v, want ErrFetchBackoffActive", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("remote temp-file fetch calls = %d, want 1", got)
	}
}

func TestReadContentResponseToFileWritesValidatedPayload(t *testing.T) {
	t.Parallel()

	payload := []byte("validated remote payload")
	cid, err := content.ComputeCID(payload)
	if err != nil {
		t.Fatalf("ComputeCID: %v", err)
	}

	var resp bytes.Buffer
	if err := writeContentResponse(&resp, bytes.NewReader(payload), int64(len(payload))); err != nil {
		t.Fatalf("writeContentResponse() error = %v", err)
	}

	temp, err := os.CreateTemp(t.TempDir(), "xleaks-fetch-test-*")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	defer temp.Close()

	size, err := readContentResponseToFile(bytes.NewReader(resp.Bytes()), temp, cid)
	if err != nil {
		t.Fatalf("readContentResponseToFile() error = %v", err)
	}
	if size != int64(len(payload)) {
		t.Fatalf("size = %d, want %d", size, len(payload))
	}
	if _, err := temp.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Seek() error = %v", err)
	}
	got, err := io.ReadAll(temp)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload = %q, want %q", got, payload)
	}
}

func TestReadContentResponseToFileRejectsHashMismatch(t *testing.T) {
	t.Parallel()

	payload := []byte("validated remote payload")
	cid, err := content.ComputeCID([]byte("different payload"))
	if err != nil {
		t.Fatalf("ComputeCID: %v", err)
	}

	var resp bytes.Buffer
	if err := writeContentResponse(&resp, bytes.NewReader(payload), int64(len(payload))); err != nil {
		t.Fatalf("writeContentResponse() error = %v", err)
	}

	temp, err := os.CreateTemp(t.TempDir(), "xleaks-fetch-test-*")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	defer temp.Close()

	if _, err := readContentResponseToFile(bytes.NewReader(resp.Bytes()), temp, cid); err == nil {
		t.Fatal("expected hash mismatch to be rejected")
	}
}
