package p2p

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	mh "github.com/multiformats/go-multihash"
)

const (
	// contentProtocol is the libp2p protocol ID for content exchange.
	contentProtocol = protocol.ID("/xleaks/content/1.0.0")

	// maxContentSize is the maximum content size we will read from a stream (16 MiB).
	maxContentSize = 16 << 20

	// findProvidersTimeout is the default timeout for DHT provider queries.
	findProvidersTimeout = 10 * time.Second

	// maxProviders is the maximum number of providers to collect from a DHT query.
	maxProviders = 20

	// streamDeadline is the read/write deadline applied to content streams.
	streamDeadline = 30 * time.Second
)

// ContentExchange provides an interface for exchanging content-addressed data
// between peers. It uses the Kademlia DHT for provider discovery and direct
// libp2p streams for content transfer.
type ContentExchange struct {
	mu sync.RWMutex
	// host is the P2P host used for network communication.
	host *Host
	// provided maps CID hex -> bool (content we can serve).
	provided map[string]bool
	// fetcher is called when we need to retrieve content from local storage.
	fetcher ContentFetcher
	// server is called to handle incoming content requests.
	server ContentServer
}

// ContentFetcher is called to retrieve content from local storage.
type ContentFetcher func(cidHex string) ([]byte, error)

// ContentServer is called to handle incoming content requests.
// It returns the content and a boolean indicating whether the content was found.
type ContentServer func(cidHex string) ([]byte, bool)

// NewContentExchange creates a new ContentExchange instance attached to the
// given host.
func NewContentExchange(h *Host) *ContentExchange {
	return &ContentExchange{
		host:     h,
		provided: make(map[string]bool),
	}
}

// hexToCid converts a hex-encoded multihash string to a cid.Cid suitable for
// DHT operations. The XLeaks CIDs are raw multihashes (SHA2-256), so we wrap
// them as CIDv1 with the raw codec (0x55).
func hexToCid(cidHex string) (cid.Cid, error) {
	b, err := hex.DecodeString(cidHex)
	if err != nil {
		return cid.Undef, fmt.Errorf("decoding hex CID: %w", err)
	}

	// Validate that the bytes form a valid multihash.
	if _, err := mh.Decode(b); err != nil {
		return cid.Undef, fmt.Errorf("invalid multihash: %w", err)
	}

	// Build a CIDv1 with raw codec (0x55) wrapping the multihash.
	return cid.NewCidV1(cid.Raw, mh.Multihash(b)), nil
}

// Provide announces that this node has content for the given CID by
// advertising it via the Kademlia DHT.
func (ce *ContentExchange) Provide(ctx context.Context, cidHex string) error {
	ce.mu.Lock()
	ce.provided[cidHex] = true
	ce.mu.Unlock()

	c, err := hexToCid(cidHex)
	if err != nil {
		return fmt.Errorf("converting CID for DHT provide: %w", err)
	}

	if err := ce.host.dht.Provide(ctx, c, true); err != nil {
		return fmt.Errorf("announcing CID to DHT: %w", err)
	}

	return nil
}

// Unprovide removes the announcement for a CID so this node no longer
// advertises the content. Note: this only removes the local record; DHT
// provider records will expire naturally.
func (ce *ContentExchange) Unprovide(cidHex string) {
	ce.mu.Lock()
	delete(ce.provided, cidHex)
	ce.mu.Unlock()
}

// HasContent checks if this node has content for a CID.
func (ce *ContentExchange) HasContent(cidHex string) bool {
	ce.mu.RLock()
	defer ce.mu.RUnlock()
	return ce.provided[cidHex]
}

// ProvidedCIDs returns all CID hex strings that this node provides.
func (ce *ContentExchange) ProvidedCIDs() []string {
	ce.mu.RLock()
	defer ce.mu.RUnlock()
	cids := make([]string, 0, len(ce.provided))
	for c := range ce.provided {
		cids = append(cids, c)
	}
	return cids
}

// SetContentFetcher sets the function used to retrieve local content.
func (ce *ContentExchange) SetContentFetcher(f ContentFetcher) {
	ce.mu.Lock()
	ce.fetcher = f
	ce.mu.Unlock()
}

// SetContentServer sets the function used to serve content to remote peers.
func (ce *ContentExchange) SetContentServer(s ContentServer) {
	ce.mu.Lock()
	ce.server = s
	ce.mu.Unlock()
}

// FetchLocal attempts to retrieve content from local storage using the
// configured fetcher. Returns nil, nil if no fetcher is configured.
func (ce *ContentExchange) FetchLocal(cidHex string) ([]byte, error) {
	ce.mu.RLock()
	f := ce.fetcher
	ce.mu.RUnlock()

	if f == nil {
		return nil, nil
	}
	return f(cidHex)
}

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
		// Skip ourselves.
		if pi.ID == ce.host.ID() {
			continue
		}
		providers = append(providers, pi.ID.String())

		// Also add the peer's addresses to our peerstore so we can
		// connect to them later.
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

	// Find providers via DHT.
	providers, err := ce.FindProviders(ctx, cidHex)
	if err != nil {
		return nil, fmt.Errorf("finding providers: %w", err)
	}

	if len(providers) == 0 {
		return nil, fmt.Errorf("no providers found for CID %s", cidHex)
	}

	// Decode the CID hex to raw bytes for hash validation.
	cidBytes, err := hex.DecodeString(cidHex)
	if err != nil {
		return nil, fmt.Errorf("decoding CID hex: %w", err)
	}

	// Try each provider.
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
			log.Printf("content exchange: failed to fetch %s from %s: %v", cidHex, pid, err)
			continue
		}

		return data, nil
	}

	return nil, fmt.Errorf("all providers failed for CID %s: %w", cidHex, lastErr)
}

// fetchFromPeer opens a libp2p stream to the given peer and requests content
// by CID. It validates the response hash before returning.
func (ce *ContentExchange) fetchFromPeer(ctx context.Context, pid peer.ID, cidBytes []byte) ([]byte, error) {
	stream, err := ce.host.host.NewStream(ctx, pid, contentProtocol)
	if err != nil {
		return nil, fmt.Errorf("opening stream: %w", err)
	}
	defer stream.Close()

	// Set a deadline on the stream.
	if err := stream.SetDeadline(time.Now().Add(streamDeadline)); err != nil {
		return nil, fmt.Errorf("setting stream deadline: %w", err)
	}

	// Send the CID length (4 bytes big-endian) followed by the CID bytes.
	cidLen := uint32(len(cidBytes))
	header := []byte{
		byte(cidLen >> 24),
		byte(cidLen >> 16),
		byte(cidLen >> 8),
		byte(cidLen),
	}
	if _, err := stream.Write(header); err != nil {
		return nil, fmt.Errorf("writing CID length: %w", err)
	}
	if _, err := stream.Write(cidBytes); err != nil {
		return nil, fmt.Errorf("writing CID: %w", err)
	}

	// Close the write side to signal we're done sending.
	if err := stream.CloseWrite(); err != nil {
		return nil, fmt.Errorf("closing write side: %w", err)
	}

	// Read the response: 4-byte length prefix + content.
	var respHeader [4]byte
	if _, err := io.ReadFull(stream, respHeader[:]); err != nil {
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
	if _, err := io.ReadFull(stream, data); err != nil {
		return nil, fmt.Errorf("reading content: %w", err)
	}

	// Validate: compute the multihash of the received data and compare with
	// the requested CID.
	decoded, err := mh.Decode(cidBytes)
	if err != nil {
		return nil, fmt.Errorf("decoding multihash for validation: %w", err)
	}

	computed, err := mh.Sum(data, decoded.Code, decoded.Length)
	if err != nil {
		return nil, fmt.Errorf("computing hash for validation: %w", err)
	}

	if !bytes.Equal([]byte(computed), cidBytes) {
		return nil, fmt.Errorf("content hash mismatch: expected %x, got %x", cidBytes, []byte(computed))
	}

	return data, nil
}

// ServeContent registers a stream handler on the host for the content exchange
// protocol. When a remote peer requests content by CID, the handler looks up
// the content using the provided ContentServer and sends it back.
func (ce *ContentExchange) ServeContent(cas ContentServer) {
	ce.mu.Lock()
	ce.server = cas
	ce.mu.Unlock()

	ce.host.host.SetStreamHandler(contentProtocol, func(stream network.Stream) {
		defer stream.Close()

		// Set a deadline on the stream.
		if err := stream.SetDeadline(time.Now().Add(streamDeadline)); err != nil {
			log.Printf("content exchange: failed to set stream deadline: %v", err)
			return
		}

		// Read the CID length (4 bytes big-endian).
		var header [4]byte
		if _, err := io.ReadFull(stream, header[:]); err != nil {
			log.Printf("content exchange: failed to read CID length: %v", err)
			return
		}
		cidLen := uint32(header[0])<<24 | uint32(header[1])<<16 |
			uint32(header[2])<<8 | uint32(header[3])

		if cidLen == 0 || cidLen > 1024 {
			log.Printf("content exchange: invalid CID length: %d", cidLen)
			return
		}

		cidBytes := make([]byte, cidLen)
		if _, err := io.ReadFull(stream, cidBytes); err != nil {
			log.Printf("content exchange: failed to read CID: %v", err)
			return
		}

		cidHex := hex.EncodeToString(cidBytes)

		// Look up the content.
		ce.mu.RLock()
		srv := ce.server
		ce.mu.RUnlock()

		if srv == nil {
			log.Printf("content exchange: no content server configured")
			ce.writeEmptyResponse(stream)
			return
		}

		data, found := srv(cidHex)
		if !found {
			ce.writeEmptyResponse(stream)
			return
		}

		// Send the response: 4-byte length prefix + content.
		dataLen := uint32(len(data))
		respHeader := []byte{
			byte(dataLen >> 24),
			byte(dataLen >> 16),
			byte(dataLen >> 8),
			byte(dataLen),
		}
		if _, err := stream.Write(respHeader); err != nil {
			log.Printf("content exchange: failed to write response length: %v", err)
			return
		}
		if _, err := stream.Write(data); err != nil {
			log.Printf("content exchange: failed to write response data: %v", err)
			return
		}
	})
}

// writeEmptyResponse sends a zero-length response to indicate "not found".
func (ce *ContentExchange) writeEmptyResponse(stream network.Stream) {
	zeros := []byte{0, 0, 0, 0}
	_, _ = stream.Write(zeros)
}
