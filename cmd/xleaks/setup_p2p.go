package main

import (
	"context"
	"log"

	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/xleaks-org/xleaks/pkg/config"
	"github.com/xleaks-org/xleaks/pkg/p2p"
)

// setupP2P creates and configures the libp2p host, initialises GossipSub, DHT
// bootstrapping, and mDNS discovery. Returns nil (not an error) when the host
// fails to start so the node can run in offline mode.
func setupP2P(ctx context.Context, cfg *config.Config) (*p2p.Host, error) {
	p2pCfg := &p2p.Config{
		ListenAddresses:    cfg.Network.ListenAddresses,
		EnableRelay:        cfg.Network.EnableRelay,
		EnableMDNS:         cfg.Network.EnableMDNS,
		EnableHolePunching: cfg.Network.EnableHolePunching,
		MaxPeers:           cfg.Network.MaxPeers,
		BandwidthLimitMbps: cfg.Network.BandwidthLimitMbps,
	}

	// Generate an ephemeral libp2p identity. The real user identity is
	// separate from the P2P transport identity.
	p2pPrivKey, _, err := libp2pcrypto.GenerateEd25519Key(nil)
	if err != nil {
		return nil, err
	}

	host, err := p2p.NewHost(ctx, p2pPrivKey, p2pCfg)
	if err != nil {
		log.Printf("WARNING: P2P host failed to start: %v", err)
		log.Println("Running in offline mode — local data is still accessible.")
		return nil, nil
	}

	log.Printf("P2P host started. Peer ID: %s", host.ID())
	for _, addr := range host.Addrs() {
		log.Printf("  Listening on: %s/p2p/%s", addr, host.ID())
	}

	if err := host.InitPubSub(ctx); err != nil {
		log.Printf("Warning: GossipSub init failed: %v", err)
	}

	go func() {
		if err := host.Bootstrap(ctx, cfg.Network.BootstrapPeers); err != nil {
			log.Printf("Warning: DHT bootstrap failed: %v", err)
		}
	}()

	if cfg.Network.EnableMDNS {
		if err := host.SetupMDNS(ctx); err != nil {
			log.Printf("Warning: mDNS setup failed: %v", err)
		}
	}

	if cfg.Network.EnableRelay && len(cfg.Network.RelayAddresses) > 0 {
		go host.ConnectToRelays(ctx, cfg.Network.RelayAddresses)
	}

	return host, nil
}
