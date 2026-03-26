package main

import (
	"context"
	"log/slog"

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
		slog.Warn("P2P host failed to start", "error", err)
		slog.Info("running in offline mode, local data is still accessible")
		return nil, nil
	}

	slog.Info("P2P host started", "peer_id", host.ID())
	for _, addr := range host.Addrs() {
		slog.Info("P2P listening", "addr", addr, "peer_id", host.ID())
	}

	if err := host.InitPubSub(ctx); err != nil {
		slog.Warn("GossipSub init failed", "error", err)
	}

	go func() {
		if err := host.Bootstrap(ctx, cfg.Network.BootstrapPeers); err != nil {
			fallbackPeers := discoverBootstrapFallbackPeers(ctx, cfg)
			if len(fallbackPeers) > 0 {
				slog.Info("bootstrap retry with remote discovery", "peers", len(fallbackPeers))
				if retryErr := host.Bootstrap(ctx, dedupeStrings(append(cfg.Network.BootstrapPeers, fallbackPeers...))); retryErr == nil {
					return
				} else {
					slog.Warn("DHT bootstrap retry failed", "error", retryErr)
				}
			}
			slog.Warn("DHT bootstrap failed", "error", err)
		}
	}()

	if cfg.Network.EnableMDNS {
		if err := host.SetupMDNS(ctx); err != nil {
			slog.Warn("mDNS setup failed", "error", err)
		}
	}

	if cfg.Network.EnableRelay && len(cfg.Network.RelayAddresses) > 0 {
		go host.ConnectToRelays(ctx, cfg.Network.RelayAddresses)
	}

	return host, nil
}
