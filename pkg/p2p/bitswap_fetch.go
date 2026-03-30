package p2p

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	mh "github.com/multiformats/go-multihash"
)

var ErrFetchBackoffActive = errors.New("content fetch temporarily backed off after recent failure")

// FindProviders queries the Kademlia DHT for peers that have the given content.
// It returns up to maxProviders peer IDs as hex strings.
func (ce *ContentExchange) FindProviders(ctx context.Context, cidHex string) ([]string, error) {
	c, err := hexToCid(cidHex)
	if err != nil {
		return nil, fmt.Errorf("converting CID for provider lookup: %w", err)
	}

	queryCtx, cancel := context.WithTimeout(ctx, findProvidersTimeout)
	defer cancel()

	provCh := ce.host.dht.FindProvidersAsync(queryCtx, c, maxProviders)

	var providers []string
	for pi := range provCh {
		if pi.ID == ce.host.ID() {
			continue
		}
		providers = append(providers, pi.ID.String())

		if len(pi.Addrs) > 0 {
			ce.host.host.Peerstore().AddAddrs(pi.ID, pi.Addrs, time.Hour)
		}
	}

	return providers, nil
}

// FetchContent attempts to retrieve content from the network for the given CID.
// It finds providers via the DHT, then tries each provider sequentially by
// opening a libp2p stream and requesting the content. The received content is
// validated against the CID's multihash before being returned.
func (ce *ContentExchange) FetchContent(ctx context.Context, cidHex string, cas ContentFetcher) ([]byte, error) {
	// First, try local storage.
	if cas != nil {
		data, err := cas(cidHex)
		if err == nil && data != nil {
			return data, nil
		}
	}

	cidBytes, err := hex.DecodeString(cidHex)
	if err != nil {
		return nil, fmt.Errorf("decoding CID hex: %w", err)
	}

	return ce.fetchShared(ctx, cidHex, func(fetchCtx context.Context) ([]byte, error) {
		if remote := ce.fetchRemote; remote != nil {
			return remote(fetchCtx, cidHex, cidBytes)
		}
		providers, err := ce.FindProviders(fetchCtx, cidHex)
		if err != nil {
			return nil, fmt.Errorf("finding providers: %w", err)
		}
		if len(providers) == 0 {
			return nil, fmt.Errorf("no providers found for CID %s", cidHex)
		}
		return ce.tryProviders(fetchCtx, providers, cidHex, cidBytes)
	})
}

// FetchContentToTempFile retrieves remote content into a private temp file
// without materializing the full response in memory. The caller is
// responsible for removing the returned file when done.
func (ce *ContentExchange) FetchContentToTempFile(ctx context.Context, cidHex string) (*FetchedContentFile, error) {
	cidBytes, err := hex.DecodeString(cidHex)
	if err != nil {
		return nil, fmt.Errorf("decoding CID hex: %w", err)
	}
	if err := ce.checkFetchBackoff(cidHex); err != nil {
		return nil, err
	}

	fetched, fetchErr := ce.runRemoteFetchToTempFile(ctx, func(fetchCtx context.Context) (*FetchedContentFile, error) {
		if remote := ce.fetchRemoteToFile; remote != nil {
			return remote(fetchCtx, cidHex, cidBytes)
		}
		providers, err := ce.FindProviders(fetchCtx, cidHex)
		if err != nil {
			return nil, fmt.Errorf("finding providers: %w", err)
		}
		if len(providers) == 0 {
			return nil, fmt.Errorf("no providers found for CID %s", cidHex)
		}
		return ce.tryProvidersToTempFile(fetchCtx, providers, cidHex, cidBytes)
	})
	ce.recordFetchResult(cidHex, fetchErr)
	return fetched, fetchErr
}

func (ce *ContentExchange) fetchShared(ctx context.Context, cidHex string, fetch func(context.Context) ([]byte, error)) ([]byte, error) {
	ce.fetchStateMu.Lock()
	if err := ce.checkFetchBackoffLocked(cidHex, ce.currentTime()); err != nil {
		ce.fetchStateMu.Unlock()
		return nil, err
	}
	if call, ok := ce.fetchInFlight[cidHex]; ok {
		ce.fetchStateMu.Unlock()
		select {
		case <-call.done:
			return call.data, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	call := &contentFetchCall{done: make(chan struct{})}
	ce.fetchInFlight[cidHex] = call
	ce.fetchStateMu.Unlock()

	call.data, call.err = ce.runRemoteFetch(ctx, fetch)

	ce.fetchStateMu.Lock()
	delete(ce.fetchInFlight, cidHex)
	ce.recordFetchResultLocked(cidHex, call.err)
	close(call.done)
	ce.fetchStateMu.Unlock()

	return call.data, call.err
}

func (ce *ContentExchange) runRemoteFetch(ctx context.Context, fetch func(context.Context) ([]byte, error)) ([]byte, error) {
	if ce.fetchSemaphore == nil {
		return ce.runRemoteFetchWithTimeout(ctx, fetch)
	}

	select {
	case ce.fetchSemaphore <- struct{}{}:
		defer func() { <-ce.fetchSemaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return ce.runRemoteFetchWithTimeout(ctx, fetch)
}

func (ce *ContentExchange) runRemoteFetchToTempFile(ctx context.Context, fetch func(context.Context) (*FetchedContentFile, error)) (*FetchedContentFile, error) {
	if ce.fetchSemaphore == nil {
		return ce.runRemoteFetchToTempFileWithTimeout(ctx, fetch)
	}

	select {
	case ce.fetchSemaphore <- struct{}{}:
		defer func() { <-ce.fetchSemaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return ce.runRemoteFetchToTempFileWithTimeout(ctx, fetch)
}

func (ce *ContentExchange) runRemoteFetchWithTimeout(ctx context.Context, fetch func(context.Context) ([]byte, error)) ([]byte, error) {
	fetchCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		fetchCtx, cancel = context.WithTimeout(ctx, contentFetchTimeout)
		defer cancel()
	}
	return fetch(fetchCtx)
}

func (ce *ContentExchange) runRemoteFetchToTempFileWithTimeout(ctx context.Context, fetch func(context.Context) (*FetchedContentFile, error)) (*FetchedContentFile, error) {
	fetchCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		fetchCtx, cancel = context.WithTimeout(ctx, contentFetchTimeout)
		defer cancel()
	}
	return fetch(fetchCtx)
}

// tryProviders attempts to fetch content from each provider in order.
func (ce *ContentExchange) tryProviders(
	ctx context.Context,
	providers []string,
	cidHex string,
	cidBytes []byte,
) ([]byte, error) {
	var lastErr error
	for _, providerID := range providers {
		pid, err := peer.Decode(providerID)
		if err != nil {
			lastErr = fmt.Errorf("decoding provider peer ID %q: %w", providerID, err)
			continue
		}

		data, err := ce.fetchFromPeer(ctx, pid, cidBytes)
		if err != nil {
			lastErr = fmt.Errorf("fetching from peer %s: %w", pid, err)
			slog.Debug("content exchange: failed to fetch from peer", "cid", cidHex, "peer", pid, "error", err)
			continue
		}

		return data, nil
	}

	return nil, fmt.Errorf("all providers failed for CID %s: %w", cidHex, lastErr)
}

func (ce *ContentExchange) tryProvidersToTempFile(
	ctx context.Context,
	providers []string,
	cidHex string,
	cidBytes []byte,
) (*FetchedContentFile, error) {
	var lastErr error
	for _, providerID := range providers {
		pid, err := peer.Decode(providerID)
		if err != nil {
			lastErr = fmt.Errorf("decoding provider peer ID %q: %w", providerID, err)
			continue
		}

		fetched, err := ce.fetchFromPeerToTempFile(ctx, pid, cidBytes)
		if err != nil {
			lastErr = fmt.Errorf("fetching from peer %s: %w", pid, err)
			slog.Debug("content exchange: failed to fetch temp file from peer", "cid", cidHex, "peer", pid, "error", err)
			continue
		}

		return fetched, nil
	}

	return nil, fmt.Errorf("all providers failed for CID %s: %w", cidHex, lastErr)
}

// FetchLocal retrieves content from local storage. Returns nil, nil if no fetcher is configured.
func (ce *ContentExchange) FetchLocal(cidHex string) ([]byte, error) {
	ce.mu.RLock()
	f := ce.fetcher
	ce.mu.RUnlock()
	if f == nil {
		return nil, nil
	}
	return f(cidHex)
}

// fetchFromPeer opens a libp2p stream to the given peer and requests content
// by CID. It validates the response hash before returning.
func (ce *ContentExchange) fetchFromPeer(ctx context.Context, pid peer.ID, cidBytes []byte) ([]byte, error) {
	stream, err := ce.host.host.NewStream(ctx, pid, contentProtocol)
	if err != nil {
		return nil, fmt.Errorf("opening stream: %w", err)
	}
	defer stream.Close()

	if err := stream.SetDeadline(time.Now().Add(streamDeadline)); err != nil {
		return nil, fmt.Errorf("setting stream deadline: %w", err)
	}

	if err := writeCIDRequest(stream, cidBytes); err != nil {
		return nil, err
	}

	if err := stream.CloseWrite(); err != nil {
		return nil, fmt.Errorf("closing write side: %w", err)
	}

	data, err := readContentResponse(stream)
	if err != nil {
		return nil, err
	}

	return data, validateContentHash(data, cidBytes)
}

func (ce *ContentExchange) fetchFromPeerToTempFile(ctx context.Context, pid peer.ID, cidBytes []byte) (*FetchedContentFile, error) {
	stream, err := ce.host.host.NewStream(ctx, pid, contentProtocol)
	if err != nil {
		return nil, fmt.Errorf("opening stream: %w", err)
	}
	defer stream.Close()

	if err := stream.SetDeadline(time.Now().Add(streamDeadline)); err != nil {
		return nil, fmt.Errorf("setting stream deadline: %w", err)
	}

	if err := writeCIDRequest(stream, cidBytes); err != nil {
		return nil, err
	}

	if err := stream.CloseWrite(); err != nil {
		return nil, fmt.Errorf("closing write side: %w", err)
	}

	temp, err := os.CreateTemp("", "xleaks-fetch-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp fetch file: %w", err)
	}
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		os.Remove(temp.Name())
		return nil, fmt.Errorf("setting temp fetch file permissions: %w", err)
	}

	cleanup := true
	defer func() {
		if cleanup {
			temp.Close()
			os.Remove(temp.Name())
		}
	}()

	size, err := readContentResponseToFile(stream, temp, cidBytes)
	if err != nil {
		return nil, err
	}
	if err := temp.Sync(); err != nil {
		return nil, fmt.Errorf("syncing temp fetch file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return nil, fmt.Errorf("closing temp fetch file: %w", err)
	}

	cleanup = false
	return &FetchedContentFile{
		Path: temp.Name(),
		Size: size,
	}, nil
}

// writeCIDRequest writes the CID length (4 bytes big-endian) and CID bytes.
func writeCIDRequest(w io.Writer, cidBytes []byte) error {
	cidLen := uint32(len(cidBytes))
	header := []byte{
		byte(cidLen >> 24),
		byte(cidLen >> 16),
		byte(cidLen >> 8),
		byte(cidLen),
	}
	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("writing CID length: %w", err)
	}
	if _, err := w.Write(cidBytes); err != nil {
		return fmt.Errorf("writing CID: %w", err)
	}
	return nil
}

// readContentResponse reads a length-prefixed content response from a stream.
func readContentResponse(r io.Reader) ([]byte, error) {
	respLen, err := readContentResponseLength(r)
	if err != nil {
		return nil, err
	}

	data := make([]byte, respLen)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("reading content: %w", err)
	}
	return data, nil
}

func readContentResponseToFile(r io.Reader, dst *os.File, cidBytes []byte) (int64, error) {
	respLen, err := readContentResponseLength(r)
	if err != nil {
		return 0, err
	}

	decoded, err := mh.Decode(cidBytes)
	if err != nil {
		return 0, fmt.Errorf("decoding multihash for validation: %w", err)
	}

	counter := &countingWriter{}
	tee := io.TeeReader(io.LimitReader(r, int64(respLen)), io.MultiWriter(dst, counter))
	computed, err := mh.SumStream(tee, decoded.Code, decoded.Length)
	if err != nil {
		return 0, fmt.Errorf("computing hash for validation: %w", err)
	}
	if counter.n != int64(respLen) {
		return 0, fmt.Errorf("short content read: expected %d bytes, got %d", respLen, counter.n)
	}
	if !bytes.Equal([]byte(computed), cidBytes) {
		return 0, fmt.Errorf("content hash mismatch: expected %x, got %x", cidBytes, []byte(computed))
	}
	return int64(respLen), nil
}

// validateContentHash verifies the received data matches the CID multihash.
func validateContentHash(data, cidBytes []byte) error {
	decoded, err := mh.Decode(cidBytes)
	if err != nil {
		return fmt.Errorf("decoding multihash for validation: %w", err)
	}

	computed, err := mh.Sum(data, decoded.Code, decoded.Length)
	if err != nil {
		return fmt.Errorf("computing hash for validation: %w", err)
	}

	if !bytes.Equal([]byte(computed), cidBytes) {
		return fmt.Errorf("content hash mismatch: expected %x, got %x", cidBytes, []byte(computed))
	}
	return nil
}

func readContentResponseLength(r io.Reader) (uint32, error) {
	var respHeader [4]byte
	if _, err := io.ReadFull(r, respHeader[:]); err != nil {
		return 0, fmt.Errorf("reading response length: %w", err)
	}
	respLen := uint32(respHeader[0])<<24 | uint32(respHeader[1])<<16 |
		uint32(respHeader[2])<<8 | uint32(respHeader[3])

	if respLen == 0 {
		return 0, fmt.Errorf("provider does not have the content")
	}
	if respLen > maxContentSize {
		return 0, fmt.Errorf("response too large: %d bytes", respLen)
	}
	return respLen, nil
}

func (ce *ContentExchange) currentTime() time.Time {
	if ce.now == nil {
		ce.now = time.Now
	}
	return ce.now()
}

func (ce *ContentExchange) checkFetchBackoff(cidHex string) error {
	ce.fetchStateMu.Lock()
	defer ce.fetchStateMu.Unlock()
	return ce.checkFetchBackoffLocked(cidHex, ce.currentTime())
}

func (ce *ContentExchange) checkFetchBackoffLocked(cidHex string, now time.Time) error {
	if until, ok := ce.fetchFailures[cidHex]; ok {
		if now.Before(until) {
			remaining := until.Sub(now).Round(time.Second)
			return fmt.Errorf("%w for CID %s (%s remaining)", ErrFetchBackoffActive, cidHex, remaining)
		}
		delete(ce.fetchFailures, cidHex)
	}
	return nil
}

func (ce *ContentExchange) recordFetchResult(cidHex string, err error) {
	ce.fetchStateMu.Lock()
	defer ce.fetchStateMu.Unlock()
	ce.recordFetchResultLocked(cidHex, err)
}

func (ce *ContentExchange) recordFetchResultLocked(cidHex string, err error) {
	if err != nil {
		ce.fetchFailures[cidHex] = ce.currentTime().Add(contentFetchFailureBackoff)
	} else {
		delete(ce.fetchFailures, cidHex)
	}
}

type countingWriter struct {
	n int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.n += int64(len(p))
	return len(p), nil
}
