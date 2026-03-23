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
	"github.com/xleaks-org/xleaks/pkg/api"
	"github.com/xleaks-org/xleaks/pkg/config"
	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/feed"
	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/indexer"
	"github.com/xleaks-org/xleaks/pkg/p2p"
	"github.com/xleaks-org/xleaks/pkg/social"
	"github.com/xleaks-org/xleaks/pkg/storage"
	"github.com/xleaks-org/xleaks/pkg/web"
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
		log.Printf("WARNING: P2P host failed to start: %v", err)
		log.Println("Running in offline mode — local data is still accessible.")
		p2pHost = nil
	}
	if p2pHost != nil {
		defer p2pHost.Close()
		log.Printf("P2P host started. Peer ID: %s", p2pHost.ID())
		for _, addr := range p2pHost.Addrs() {
			log.Printf("  Listening on: %s/p2p/%s", addr, p2pHost.ID())
		}
	}

	// Initialize P2P networking (only if host started successfully).
	if p2pHost != nil {
		if err := p2pHost.InitPubSub(ctx); err != nil {
			log.Printf("Warning: GossipSub init failed: %v", err)
		}

		go func() {
			if err := p2pHost.Bootstrap(ctx, cfg.Network.BootstrapPeers); err != nil {
				log.Printf("Warning: DHT bootstrap failed: %v", err)
			}
		}()

		if cfg.Network.EnableMDNS {
			if err := p2pHost.SetupMDNS(ctx); err != nil {
				log.Printf("Warning: mDNS setup failed: %v", err)
			}
		}

		ownPubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())
		p2pHost.Subscribe(p2p.DMTopic(ownPubkeyHex), func(ctx context.Context, _ p2p.PeerID, data []byte) {
			log.Printf("Received DM from peer")
		})
		p2pHost.Subscribe(p2p.GlobalTopic(), func(ctx context.Context, _ p2p.PeerID, data []byte) {
			log.Printf("Received global announcement")
		})
		p2pHost.Subscribe(p2p.ProfilesTopic(), func(ctx context.Context, _ p2p.PeerID, data []byte) {
			log.Printf("Received profile update")
		})
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

	// Wire feed subscriptions to P2P topic subscriptions (only if P2P is active).
	if p2pHost != nil {
		feedManager.OnSubscribe = func(ctx context.Context, pubkeyHex string) error {
			return p2pHost.Subscribe(p2p.PostsTopic(pubkeyHex), func(ctx context.Context, _ p2p.PeerID, data []byte) {
				log.Printf("Received post from followed publisher %s", pubkeyHex[:16])
			})
		}
		feedManager.OnUnsubscribe = func(pubkeyHex string) error {
			return p2pHost.Unsubscribe(p2p.PostsTopic(pubkeyHex))
		}
		for _, pubkeyHex := range feedManager.FollowedPubkeys() {
			topicName := p2p.PostsTopic(pubkeyHex)
			p2pHost.Subscribe(topicName, func(ctx context.Context, _ p2p.PeerID, data []byte) {
				log.Printf("Received post from followed publisher")
			})
		}
	}

	timeline := feed.NewTimeline(db, kp.PublicKeyBytes())

	// Initialize indexer client for search/trending queries.
	idxClient := indexer.NewIndexerClient()
	for _, url := range cfg.Indexer.KnownIndexers {
		idxClient.AddIndexer(url)
	}

	// Discover indexers via DHT every 5 minutes (only if P2P is active).
	if p2pHost != nil {
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					indexers, err := p2pHost.FindIndexers(ctx)
					if err != nil {
						log.Printf("DHT indexer discovery failed: %v", err)
						continue
					}
					for _, info := range indexers {
						for _, addr := range info.Addrs {
							idxClient.AddIndexer(fmt.Sprintf("http://%s", addr.String()))
						}
					}
					log.Printf("DHT indexer discovery: found %d indexer(s)", len(indexers))
				}
			}
		}()
	}

	// Initialize content replication with storage eviction.
	replicator := feed.NewReplicator(db, cas)
	maxStorageBytes := int64(cfg.Node.MaxStorageGB) * 1024 * 1024 * 1024
	replicator.StartStorageManager(ctx, maxStorageBytes, 5*time.Minute)

	// Set up content exchange (serve stored content to peers).
	var ce *p2p.ContentExchange
	if p2pHost != nil {
		ce = p2pHost.ContentExchange()
	}
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

	// Initialize web UI handler (Go templates + htmx).
	webHandler, err := web.NewHandler(db, idHolder, timeline)
	if err != nil {
		log.Printf("Warning: web UI failed to initialize: %v", err)
	}
	if webHandler != nil {
		webHandler.SetCreatePost(func(ctx context.Context, content string) (string, error) {
			post, err := postService.CreatePost(ctx, content, nil, nil)
			if err != nil {
				return "", err
			}
			return hex.EncodeToString(post.Id), nil
		})

		// Wire node status callback so the web UI can read status directly
		// instead of making an HTTP round-trip to the API.
		nodeStartTime := time.Now()
		webHandler.SetNodeStatus(func() (peers int, uptimeSecs float64, storageUsed, storageLimit int64, subscriptions int) {
			uptimeSecs = time.Since(nodeStartTime).Seconds()
			storageLimit = int64(cfg.Node.MaxStorageGB) * 1024 * 1024 * 1024

			if p2pHost != nil {
				peers = p2pHost.PeerCount()
			}

			if s, err := content.DirSize(filepath.Join(dataDir, "data")); err == nil {
				storageUsed = s
			}

			if count, err := db.CountSubscriptions(); err == nil {
				subscriptions = count
			}

			return
		})
	}

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
	if webHandler != nil {
		deps.WebHandler = webHandler.Routes()
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
		if p2pHost != nil {
			if err := p2pHost.Close(); err != nil {
				log.Printf("P2P host shutdown error: %v", err)
			}
		}
	}()

	log.Printf("XLeaks node starting on %s", cfg.API.ListenAddress)
	if p2pHost != nil {
		log.Printf("Connected peers: %d", p2pHost.PeerCount())
	} else {
		log.Println("Running in offline mode (no P2P)")
	}

	if err := server.Start(); err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}
