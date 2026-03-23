package p2p

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
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
