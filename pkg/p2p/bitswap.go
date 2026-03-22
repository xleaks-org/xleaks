package p2p

import (
	"context"
	"sync"
)

// ContentExchange provides an interface for exchanging content-addressed data
// between peers. This is a simplified implementation that uses direct peer
// requests rather than the full Bitswap protocol.
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

// Provide announces that this node has content for the given CID.
// In a production deployment this would announce the CID via the DHT.
func (ce *ContentExchange) Provide(_ context.Context, cidHex string) error {
	ce.mu.Lock()
	ce.provided[cidHex] = true
	ce.mu.Unlock()
	// In production this would call DHT.Provide() to announce to the network.
	return nil
}

// Unprovide removes the announcement for a CID so this node no longer
// advertises the content.
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
	for cid := range ce.provided {
		cids = append(cids, cid)
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

// FindProviders searches the DHT for peers that have the given content.
// In production this would use the Kademlia DHT to find content providers.
func (ce *ContentExchange) FindProviders(_ context.Context, _ string) ([]string, error) {
	// In production: query DHT for providers of this CID.
	return nil, nil
}
