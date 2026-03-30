package p2p

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
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

func (ce *ContentExchange) fetchShared(ctx context.Context, cidHex string, fetch func(context.Context) ([]byte, error)) ([]byte, error) {
	ce.fetchStateMu.Lock()
	if ce.now == nil {
		ce.now = time.Now
	}
	now := ce.now()
	if until, ok := ce.fetchFailures[cidHex]; ok {
		if now.Before(until) {
			remaining := until.Sub(now).Round(time.Second)
			ce.fetchStateMu.Unlock()
			return nil, fmt.Errorf("%w for CID %s (%s remaining)", ErrFetchBackoffActive, cidHex, remaining)
		}
		delete(ce.fetchFailures, cidHex)
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
	if call.err != nil {
		ce.fetchFailures[cidHex] = ce.now().Add(contentFetchFailureBackoff)
	} else {
		delete(ce.fetchFailures, cidHex)
	}
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

func (ce *ContentExchange) runRemoteFetchWithTimeout(ctx context.Context, fetch func(context.Context) ([]byte, error)) ([]byte, error) {
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
	var respHeader [4]byte
	if _, err := io.ReadFull(r, respHeader[:]); err != nil {
		return nil, fmt.Errorf("reading response length: %w", err)
	}
	respLen := uint32(respHeader[0])<<24 | uint32(respHeader[1])<<16 |
		uint32(respHeader[2])<<8 | uint32(respHeader[3])

	if respLen == 0 {
		return nil, fmt.Errorf("provider does not have the content")
	}
	if respLen > maxContentSize {
		return nil, fmt.Errorf("response too large: %d bytes", respLen)
	}

	data := make([]byte, respLen)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("reading content: %w", err)
	}
	return data, nil
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
