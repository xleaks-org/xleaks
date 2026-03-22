package p2p

import (
	libp2p "github.com/libp2p/go-libp2p"
)

// HolePunchOptions returns libp2p options for NAT hole punching (DCUtR).
// When enabled, the node can coordinate direct connections through relay
// nodes using the Direct Connection Upgrade through Relay protocol.
func HolePunchOptions(enabled bool) []libp2p.Option {
	if !enabled {
		return nil
	}
	return []libp2p.Option{
		libp2p.EnableHolePunching(),
	}
}

// NATTraversalOptions returns the combined set of libp2p options for full
// NAT traversal support, including relay client and hole punching.
func NATTraversalOptions(enableRelay, enableHolePunch bool) []libp2p.Option {
	var opts []libp2p.Option
	opts = append(opts, RelayOptions(enableRelay)...)
	opts = append(opts, HolePunchOptions(enableHolePunch)...)

	// Always enable UPnP/NAT-PMP port mapping when NAT traversal is desired.
	if enableRelay || enableHolePunch {
		opts = append(opts, libp2p.NATPortMap())
	}

	return opts
}
