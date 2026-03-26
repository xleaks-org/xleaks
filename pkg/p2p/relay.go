package p2p

import (
	"context"
	"fmt"
	"log/slog"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// RelayOptions returns libp2p options for relay client support.
// When enabled, the node can reach peers behind NATs via relay circuits.
func RelayOptions(enabled bool) []libp2p.Option {
	if !enabled {
		return nil
	}
	return []libp2p.Option{
		libp2p.EnableRelay(),
	}
}

// ConnectToRelays connects to known relay nodes for NAT traversal.
// It parses each multiaddr string and attempts to connect to the relay peer.
// Connection failures are logged but do not stop the process.
func (h *Host) ConnectToRelays(ctx context.Context, relayAddrs []string) {
	for _, addr := range relayAddrs {
		maddr, err := ma.NewMultiaddr(addr)
		if err != nil {
			slog.Warn("invalid relay address", "addr", addr, "error", err)
			continue
		}

		info, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			slog.Warn("invalid relay address", "addr", addr, "error", err)
			continue
		}

		if err := h.host.Connect(ctx, *info); err != nil {
			slog.Warn("failed to connect to relay", "addr", addr, "error", err)
		} else {
			slog.Info("connected to relay", "peer_id", info.ID)
		}
	}
}

// RelayAddrsFor returns relay-routed multiaddrs for the given peer through
// the specified relay peer. This is useful for constructing circuit addresses
// when direct connectivity is not available.
func RelayAddrsFor(relayID, targetID peer.ID) ([]ma.Multiaddr, error) {
	circuitAddr, err := ma.NewMultiaddr(
		fmt.Sprintf("/p2p/%s/p2p-circuit/p2p/%s", relayID, targetID),
	)
	if err != nil {
		return nil, fmt.Errorf("building circuit relay address: %w", err)
	}
	return []ma.Multiaddr{circuitAddr}, nil
}
