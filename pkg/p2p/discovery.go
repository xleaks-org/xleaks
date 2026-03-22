package p2p

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	// mdnsServiceTag is the mDNS service tag used for LAN peer discovery.
	mdnsServiceTag = "_xleaks-discovery._udp"

	// indexerDHTKey is the well-known DHT key under which indexer nodes
	// advertise themselves.
	indexerDHTKey = "/xleaks/indexers"
)

// Bootstrap connects to the provided bootstrap peers and bootstraps the DHT
// routing table. If bootstrapPeers is empty, the peers from the host's
// configuration are used.
func (h *Host) Bootstrap(ctx context.Context, bootstrapPeers []string) error {
	if len(bootstrapPeers) == 0 {
		bootstrapPeers = h.cfg.BootstrapPeers
	}

	// Parse and connect to each bootstrap peer concurrently.
	var wg sync.WaitGroup
	errCh := make(chan error, len(bootstrapPeers))

	for _, addrStr := range bootstrapPeers {
		maddr, err := ma.NewMultiaddr(addrStr)
		if err != nil {
			return fmt.Errorf("parsing bootstrap peer address %q: %w", addrStr, err)
		}

		info, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			return fmt.Errorf("extracting peer info from %q: %w", addrStr, err)
		}

		wg.Add(1)
		go func(pi peer.AddrInfo) {
			defer wg.Done()
			if err := h.host.Connect(ctx, pi); err != nil {
				errCh <- fmt.Errorf("connecting to bootstrap peer %s: %w", pi.ID, err)
			}
		}(*info)
	}

	wg.Wait()
	close(errCh)

	// Collect errors but don't fail if at least some peers are unreachable;
	// the DHT can still bootstrap with partial connectivity.
	var connectErrors []error
	for err := range errCh {
		connectErrors = append(connectErrors, err)
	}

	if len(connectErrors) == len(bootstrapPeers) && len(bootstrapPeers) > 0 {
		return fmt.Errorf("failed to connect to any bootstrap peer; last error: %w",
			connectErrors[len(connectErrors)-1])
	}

	// Bootstrap the DHT routing table.
	if err := h.dht.Bootstrap(ctx); err != nil {
		return fmt.Errorf("bootstrapping DHT: %w", err)
	}

	return nil
}

// SetupMDNS enables mDNS discovery for finding peers on the local network.
// Discovered peers are automatically connected to.
func (h *Host) SetupMDNS(ctx context.Context) error {
	notifee := &mdnsNotifee{
		host: h,
		ctx:  ctx,
	}

	svc := mdns.NewMdnsService(h.host, mdnsServiceTag, notifee)
	if err := svc.Start(); err != nil {
		return fmt.Errorf("starting mDNS service: %w", err)
	}

	return nil
}

// mdnsNotifee implements the mdns.Notifee interface and connects to
// discovered peers.
type mdnsNotifee struct {
	host *Host
	ctx  context.Context
}

// HandlePeerFound is called when a new peer is discovered via mDNS.
func (n *mdnsNotifee) HandlePeerFound(info peer.AddrInfo) {
	// Don't connect to ourselves.
	if info.ID == n.host.ID() {
		return
	}
	// Best-effort connection; ignore errors for LAN discovery.
	_ = n.host.host.Connect(n.ctx, info)
}

// AdvertiseAsIndexer publishes this node's peer info under the well-known
// DHT key so that other nodes can discover indexer nodes.
func (h *Host) AdvertiseAsIndexer(ctx context.Context) error {
	info := peer.AddrInfo{
		ID:    h.host.ID(),
		Addrs: h.host.Addrs(),
	}

	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshaling peer info: %w", err)
	}

	if err := h.dht.PutValue(ctx, indexerDHTKey, data); err != nil {
		return fmt.Errorf("advertising as indexer via DHT: %w", err)
	}

	return nil
}

// FindIndexers looks up indexer nodes by querying the DHT for the well-known
// indexer key. It uses GetClosestPeers as a rendezvous point and then attempts
// to fetch the stored value.
func (h *Host) FindIndexers(ctx context.Context) ([]peer.AddrInfo, error) {
	val, err := h.dht.GetValue(ctx, indexerDHTKey)
	if err != nil {
		return nil, fmt.Errorf("querying DHT for indexers: %w", err)
	}

	var info peer.AddrInfo
	if err := json.Unmarshal(val, &info); err != nil {
		return nil, fmt.Errorf("unmarshaling indexer info: %w", err)
	}

	return []peer.AddrInfo{info}, nil
}
