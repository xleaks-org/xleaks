package p2p

// Config holds P2P network configuration options.
type Config struct {
	// ListenAddresses are the multiaddr strings on which the node listens.
	ListenAddresses []string

	// BootstrapPeers are multiaddr strings of initial peers to connect to.
	BootstrapPeers []string

	// EnableRelay enables the relay client, allowing the node to connect
	// through relay nodes when direct connections are not possible.
	EnableRelay bool

	// EnableMDNS enables multicast DNS discovery for finding peers on the
	// local network.
	EnableMDNS bool

	// EnableHolePunching enables NAT hole punching so that peers behind NATs
	// can establish direct connections.
	EnableHolePunching bool

	// MaxPeers is the upper bound on the number of connections maintained by
	// the connection manager.
	MaxPeers int

	// BandwidthLimitMbps is the bandwidth limit in megabits per second.
	// A value of 0 means no limit.
	BandwidthLimitMbps int
}

// DefaultConfig returns a Config populated with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		ListenAddresses: []string{
			"/ip4/0.0.0.0/tcp/0",
			"/ip6/::/tcp/0",
			"/ip4/0.0.0.0/udp/0/quic-v1",
			"/ip6/::/udp/0/quic-v1",
		},
		BootstrapPeers:     nil,
		EnableRelay:        true,
		EnableMDNS:         true,
		EnableHolePunching: true,
		MaxPeers:           100,
		BandwidthLimitMbps: 0,
	}
}
