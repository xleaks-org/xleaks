package p2p

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	libp2phost "github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	ma "github.com/multiformats/go-multiaddr"
)

// Host wraps a libp2p host with additional XLeaks networking capabilities
// including GossipSub, Kademlia DHT, and bandwidth tracking.
type Host struct {
	host      libp2phost.Host
	pubsub    *pubsub.PubSub
	dht       *dht.IpfsDHT
	bwCounter *metrics.BandwidthCounter
	cfg       *Config
	startTime time.Time

	// Topic management
	mu     sync.RWMutex
	topics map[string]*topicHandle
}

// topicHandle holds references to a joined topic and its subscription.
type topicHandle struct {
	topic  *pubsub.Topic
	sub    *pubsub.Subscription
	cancel context.CancelFunc
}

// NewHost creates a new libp2p host configured for the XLeaks network.
//
// It sets up:
//   - Noise transport security
//   - TCP and QUIC transports
//   - Yamux multiplexing
//   - Connection manager with bounds (low: 20, high: MaxPeers, grace: 5 min)
//   - Relay client (if configured)
//   - Hole punching (if configured)
//   - Identity from the provided private key
//   - Kademlia DHT for peer routing
func NewHost(ctx context.Context, privKey crypto.PrivKey, cfg *Config) (*Host, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	bwCounter := metrics.NewBandwidthCounter()

	cm, err := connmgr.NewConnManager(
		20,           // low watermark
		cfg.MaxPeers, // high watermark
		connmgr.WithGracePeriod(5*time.Minute),
	)
	if err != nil {
		return nil, fmt.Errorf("creating connection manager: %w", err)
	}

	opts := []libp2p.Option{
		libp2p.Identity(privKey),
		libp2p.ListenAddrStrings(cfg.ListenAddresses...),
		libp2p.ConnectionManager(cm),
		libp2p.BandwidthReporter(bwCounter),
		libp2p.DefaultTransports,
		libp2p.DefaultMuxers,
		libp2p.DefaultSecurity,
		libp2p.NATPortMap(),
		libp2p.Ping(true),
	}

	if cfg.EnableRelay {
		opts = append(opts, libp2p.EnableRelay())
	} else {
		opts = append(opts, libp2p.DisableRelay())
	}

	if cfg.EnableHolePunching {
		opts = append(opts, libp2p.EnableHolePunching())
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("creating libp2p host: %w", err)
	}

	// Create the Kademlia DHT in auto mode so it adapts to whether the node
	// is reachable from the public internet.
	kadDHT, err := dht.New(ctx, h,
		dht.Mode(dht.ModeAuto),
		dht.ProtocolPrefix("/xleaks"),
	)
	if err != nil {
		h.Close()
		return nil, fmt.Errorf("creating DHT: %w", err)
	}

	return &Host{
		host:      h,
		dht:       kadDHT,
		bwCounter: bwCounter,
		cfg:       cfg,
		startTime: time.Now(),
		topics:    make(map[string]*topicHandle),
	}, nil
}

// Close gracefully shuts down the host and all associated services.
func (h *Host) Close() error {
	h.mu.Lock()
	for name, th := range h.topics {
		if th.cancel != nil {
			th.cancel()
		}
		if th.sub != nil {
			th.sub.Cancel()
		}
		if th.topic != nil {
			th.topic.Close()
		}
		delete(h.topics, name)
	}
	h.mu.Unlock()

	if h.dht != nil {
		if err := h.dht.Close(); err != nil {
			return fmt.Errorf("closing DHT: %w", err)
		}
	}
	return h.host.Close()
}

// ID returns the peer ID of this host.
func (h *Host) ID() peer.ID {
	return h.host.ID()
}

// Addrs returns the listen addresses of this host.
func (h *Host) Addrs() []ma.Multiaddr {
	return h.host.Addrs()
}

// PeerCount returns the number of currently connected peers.
func (h *Host) PeerCount() int {
	return len(h.host.Network().Peers())
}

// LibP2PHost returns the underlying libp2p host. This is useful for
// advanced operations that need direct access to the host interface.
func (h *Host) LibP2PHost() libp2phost.Host {
	return h.host
}
