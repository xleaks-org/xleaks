package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/xleaks/xleaks/pkg/api"
	"github.com/xleaks/xleaks/pkg/config"
	"github.com/xleaks/xleaks/pkg/content"
	"github.com/xleaks/xleaks/pkg/feed"
	"github.com/xleaks/xleaks/pkg/identity"
	"github.com/xleaks/xleaks/pkg/indexer"
	"github.com/xleaks/xleaks/pkg/p2p"
	"github.com/xleaks/xleaks/pkg/social"
	"github.com/xleaks/xleaks/pkg/storage"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load configuration.
	cfg, err := config.Load("~/.xleaks/config.toml")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	dataDir := cfg.DataDir()

	// Ensure data directories exist.
	for _, dir := range []string{
		dataDir,
		filepath.Join(dataDir, "identity"),
		filepath.Join(dataDir, "identity", "identities"),
		filepath.Join(dataDir, "data", "objects"),
		filepath.Join(dataDir, "data", "media"),
		filepath.Join(dataDir, "logs"),
		filepath.Join(dataDir, "cache", "thumbnails"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Initialize SQLite database.
	dbPath := filepath.Join(dataDir, "data", "index.db")
	db, err := storage.NewDB(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		return fmt.Errorf("failed to run database migrations: %w", err)
	}

	// Initialize content-addressed store.
	casPath := filepath.Join(dataDir, "data", "objects")
	cas, err := content.NewContentStore(casPath)
	if err != nil {
		return fmt.Errorf("failed to create content store: %w", err)
	}

	// Media chunk store (separate from objects).
	mediaPath := filepath.Join(dataDir, "data", "media")
	mediaCAS, err := content.NewContentStore(mediaPath)
	if err != nil {
		return fmt.Errorf("failed to create media store: %w", err)
	}
	_ = mediaCAS // Used for media chunk storage

	// Initialize identity holder with DB access for multi-identity support.
	idHolder := identity.NewHolder(dataDir)
	idHolder.SetDB(db)

	// Try to load identity. If none exists, the UI will handle onboarding.
	var kp *identity.KeyPair
	if idHolder.HasIdentity() {
		log.Println("Identity found. Unlock via API to activate.")
	} else {
		log.Println("No identity found. The UI will guide you through onboarding.")
	}

	// Create a placeholder identity for services that require one at init.
	// The real identity will be set when the user unlocks or creates one via the API.
	if kp == nil {
		kp = &identity.KeyPair{
			PrivateKey: ed25519.PrivateKey(make([]byte, ed25519.PrivateKeySize)),
			PublicKey:  ed25519.PublicKey(make([]byte, ed25519.PublicKeySize)),
		}
	}

	// Create context for the node lifecycle.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize P2P host.
	p2pCfg := &p2p.Config{
		ListenAddresses:    cfg.Network.ListenAddresses,
		EnableRelay:        cfg.Network.EnableRelay,
		EnableMDNS:         cfg.Network.EnableMDNS,
		EnableHolePunching: cfg.Network.EnableHolePunching,
		MaxPeers:           cfg.Network.MaxPeers,
		BandwidthLimitMbps: cfg.Network.BandwidthLimitMbps,
	}

	// Generate a libp2p identity from the ed25519 key.
	// If no identity is unlocked yet, generate an ephemeral one for P2P.
	var p2pPrivKey libp2pcrypto.PrivKey
	if kp.PrivateKey.Seed() != nil && len(kp.PrivateKey.Seed()) == 32 {
		p2pPrivKey, _, err = libp2pcrypto.GenerateEd25519Key(nil)
		if err != nil {
			return fmt.Errorf("failed to generate P2P key: %w", err)
		}
	} else {
		p2pPrivKey, _, err = libp2pcrypto.GenerateEd25519Key(nil)
		if err != nil {
			return fmt.Errorf("failed to generate P2P key: %w", err)
		}
	}

	p2pHost, err := p2p.NewHost(ctx, p2pPrivKey, p2pCfg)
	if err != nil {
		return fmt.Errorf("failed to create P2P host: %w", err)
	}
	defer p2pHost.Close()

	log.Printf("P2P host started. Peer ID: %s", p2pHost.ID())
	for _, addr := range p2pHost.Addrs() {
		log.Printf("  Listening on: %s/p2p/%s", addr, p2pHost.ID())
	}

	// Initialize GossipSub.
	if err := p2pHost.InitPubSub(ctx); err != nil {
		return fmt.Errorf("failed to initialize GossipSub: %w", err)
	}

	// Bootstrap DHT with known peers.
	go func() {
		if err := p2pHost.Bootstrap(ctx, cfg.Network.BootstrapPeers); err != nil {
			log.Printf("Warning: DHT bootstrap failed: %v", err)
		}
	}()

	// Set up mDNS for local discovery.
	if cfg.Network.EnableMDNS {
		if err := p2pHost.SetupMDNS(ctx); err != nil {
			log.Printf("Warning: mDNS setup failed: %v", err)
		}
	}

	// Subscribe to own DM topic.
	ownPubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())
	if err := p2pHost.Subscribe(p2p.DMTopic(ownPubkeyHex), func(ctx context.Context, _ p2p.PeerID, data []byte) {
		log.Printf("Received DM from peer")
	}); err != nil {
		log.Printf("Warning: failed to subscribe to DM topic: %v", err)
	}

	// Subscribe to global topic.
	if err := p2pHost.Subscribe(p2p.GlobalTopic(), func(ctx context.Context, _ p2p.PeerID, data []byte) {
		log.Printf("Received global announcement")
	}); err != nil {
		log.Printf("Warning: failed to subscribe to global topic: %v", err)
	}

	// Subscribe to profiles topic.
	if err := p2pHost.Subscribe(p2p.ProfilesTopic(), func(ctx context.Context, _ p2p.PeerID, data []byte) {
		log.Printf("Received profile update")
	}); err != nil {
		log.Printf("Warning: failed to subscribe to profiles topic: %v", err)
	}

	// Initialize social services.
	postService := social.NewPostService(db, cas, kp)
	reactionService := social.NewReactionService(db, kp)
	profileService := social.NewProfileService(db, kp)
	dmService := social.NewDMService(db, kp)
	notifService := social.NewNotificationService(db)

	// Initialize feed.
	feedManager := feed.NewManager(db)
	if err := feedManager.LoadSubscriptions(); err != nil {
		log.Printf("Warning: failed to load subscriptions: %v", err)
	}

	// Wire feed subscriptions to P2P topic subscriptions.
	feedManager.OnSubscribe = func(ctx context.Context, pubkeyHex string) error {
		return p2pHost.Subscribe(p2p.PostsTopic(pubkeyHex), func(ctx context.Context, _ p2p.PeerID, data []byte) {
			log.Printf("Received post from followed publisher %s", pubkeyHex[:16])
		})
	}
	feedManager.OnUnsubscribe = func(pubkeyHex string) error {
		return p2pHost.Unsubscribe(p2p.PostsTopic(pubkeyHex))
	}

	// Subscribe to all currently followed publishers.
	for _, pubkeyHex := range feedManager.FollowedPubkeys() {
		topicName := p2p.PostsTopic(pubkeyHex)
		if err := p2pHost.Subscribe(topicName, func(ctx context.Context, _ p2p.PeerID, data []byte) {
			log.Printf("Received post from followed publisher")
		}); err != nil {
			log.Printf("Warning: failed to subscribe to %s: %v", topicName, err)
		}
	}

	timeline := feed.NewTimeline(db, kp.PublicKeyBytes())

	// Initialize indexer client for search/trending queries.
	idxClient := indexer.NewIndexerClient()

	// Initialize content replication with storage eviction.
	replicator := feed.NewReplicator(db, cas)
	maxStorageBytes := int64(cfg.Node.MaxStorageGB) * 1024 * 1024 * 1024
	replicator.StartStorageManager(ctx, maxStorageBytes, 5*time.Minute)

	// Set up content exchange (serve stored content to peers).
	ce := p2pHost.ContentExchange()
	if ce != nil {
		ce.SetContentFetcher(func(cidHex string) ([]byte, error) {
			cidBytes, err := content.HexToCID(cidHex)
			if err != nil {
				return nil, err
			}
			return cas.Get(cidBytes)
		})
	}

	// Determine config path.
	cfgPath := filepath.Join(dataDir, "config.toml")

	// Create API server.
	deps := &api.HandlerDeps{
		DB:             db,
		CAS:            cas,
		KeyPair:        kp,
		IdentityHolder: idHolder,
		Posts:          postService,
		Reactions:      reactionService,
		Profiles:       profileService,
		DMs:            dmService,
		Notifs:         notifService,
		Feed:           feedManager,
		Timeline:       timeline,
		P2PHost:        p2pHost,
		Config:         cfg,
		ConfigPath:     cfgPath,
		IndexerClient:  idxClient,
	}

	server := api.NewServer(cfg.API.ListenAddress, deps)

	// Handle shutdown signals.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("Shutting down gracefully...")
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
		if err := p2pHost.Close(); err != nil {
			log.Printf("P2P host shutdown error: %v", err)
		}
	}()

	log.Printf("XLeaks node starting on %s", cfg.API.ListenAddress)
	log.Printf("Connected peers: %d", p2pHost.PeerCount())

	if err := server.Start(); err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}
