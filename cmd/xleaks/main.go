package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/xleaks-org/xleaks/pkg/api"
	"github.com/xleaks-org/xleaks/pkg/config"
	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/feed"
	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/p2p"
	"github.com/xleaks-org/xleaks/pkg/storage"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load("~/.xleaks/config.toml")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	db, cas, err := setupDatabase(cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	dataDir := cfg.DataDir()
	idHolder, kp := setupIdentity(dataDir, db)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p2pHost, err := setupP2P(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to set up P2P: %w", err)
	}
	if p2pHost != nil {
		defer p2pHost.Close()
	}

	svc := setupServices(db, cas, kp, idHolder)

	wireP2PSubscriptions(ctx, p2pHost, kp, db, cas, svc)
	wireContentExchange(p2pHost, cas)
	wireIndexerDiscovery(ctx, cfg, p2pHost, svc)

	replicator := feed.NewReplicator(db, cas)
	maxStorage := int64(cfg.Node.MaxStorageGB) * 1024 * 1024 * 1024
	replicator.StartStorageManager(ctx, maxStorage, 5*time.Minute)

	cfgPath := filepath.Join(dataDir, "config.toml")
	webRoutes := setupWebHandler(db, idHolder, svc, cfg, p2pHost, dataDir)
	deps := buildAPIDeps(db, cas, kp, idHolder, svc, p2pHost, cfg, cfgPath, webRoutes)

	server := api.NewServer(cfg.API.ListenAddress, deps)
	return runServer(ctx, cancel, server, p2pHost, cfg)
}

// wireP2PSubscriptions sets up GossipSub topic subscriptions and feed hooks.
// It creates a MessageProcessor that handles incoming messages.
func wireP2PSubscriptions(
	ctx context.Context,
	host *p2p.Host,
	kp *identity.KeyPair,
	db *storage.DB,
	cas *content.ContentStore,
	svc *ServiceBundle,
) {
	if host == nil {
		return
	}

	mp := p2p.NewMessageProcessor(db, cas, svc.Notifs)
	handler := func(ctx context.Context, _ p2p.PeerID, data []byte) {
		if err := mp.HandleMessage(ctx, data); err != nil {
			log.Printf("P2P message error: %v", err)
		}
	}

	ownPubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())
	host.Subscribe(p2p.DMTopic(ownPubkeyHex), handler)
	host.Subscribe(p2p.GlobalTopic(), handler)
	host.Subscribe(p2p.ProfilesTopic(), handler)

	if err := svc.Feed.LoadSubscriptions(); err != nil {
		log.Printf("Warning: failed to load subscriptions: %v", err)
	}

	svc.Feed.OnSubscribe = func(_ context.Context, pubkeyHex string) error {
		return host.Subscribe(p2p.PostsTopic(pubkeyHex), handler)
	}
	svc.Feed.OnUnsubscribe = func(pubkeyHex string) error {
		return host.Unsubscribe(p2p.PostsTopic(pubkeyHex))
	}

	for _, pk := range svc.Feed.FollowedPubkeys() {
		host.Subscribe(p2p.PostsTopic(pk), handler)
	}
}

// wireContentExchange configures the content exchange fetcher on the P2P host.
func wireContentExchange(host *p2p.Host, cas *content.ContentStore) {
	if host == nil {
		return
	}
	ce := host.ContentExchange()
	if ce == nil {
		return
	}
	ce.SetContentFetcher(func(cidHex string) ([]byte, error) {
		cidBytes, err := content.HexToCID(cidHex)
		if err != nil {
			return nil, err
		}
		return cas.Get(cidBytes)
	})
}

// wireIndexerDiscovery adds known indexers and starts periodic DHT discovery.
func wireIndexerDiscovery(
	ctx context.Context,
	cfg *config.Config,
	host *p2p.Host,
	svc *ServiceBundle,
) {
	for _, url := range cfg.Indexer.KnownIndexers {
		svc.Indexer.AddIndexer(url)
	}
	if host == nil {
		return
	}
	go discoverIndexersPeriodically(ctx, host, svc)
}

// discoverIndexersPeriodically queries the DHT for indexers every 5 minutes.
func discoverIndexersPeriodically(ctx context.Context, host *p2p.Host, svc *ServiceBundle) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			indexers, err := host.FindIndexers(ctx)
			if err != nil {
				log.Printf("DHT indexer discovery failed: %v", err)
				continue
			}
			for _, info := range indexers {
				for _, addr := range info.Addrs {
					svc.Indexer.AddIndexer(fmt.Sprintf("http://%s", addr.String()))
				}
			}
			log.Printf("DHT indexer discovery: found %d indexer(s)", len(indexers))
		}
	}
}
