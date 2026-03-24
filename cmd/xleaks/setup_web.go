package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xleaks-org/xleaks/pkg/api"
	"github.com/xleaks-org/xleaks/pkg/config"
	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/indexer"
	"github.com/xleaks-org/xleaks/pkg/p2p"
	"github.com/xleaks-org/xleaks/pkg/social"
	"github.com/xleaks-org/xleaks/pkg/storage"
	"github.com/xleaks-org/xleaks/pkg/web"
)

// setupWebHandler initialises the Go-template web UI and wires its callbacks.
// The idx parameter may be nil when the node is not running in indexer mode.
func setupWebHandler(
	db *storage.DB,
	idHolder *identity.Holder,
	svc *ServiceBundle,
	cfg *config.Config,
	p2pHost *p2p.Host,
	dataDir string,
	idx *indexer.Indexer,
	identitySync func(*identity.KeyPair),
) chi.Router {
	sessionMgr := web.NewSessionManager()
	webHandler, err := web.NewHandler(db, idHolder, svc.Timeline, sessionMgr)
	if err != nil {
		log.Printf("Warning: web UI failed to initialize: %v", err)
		return nil
	}

	webHandler.SetOnIdentityChange(identitySync)

	webHandler.SetCreatePost(func(ctx context.Context, text string, replyTo string) (string, error) {
		var replyToCID []byte
		if replyTo != "" {
			var err error
			replyToCID, err = hex.DecodeString(replyTo)
			if err != nil {
				return "", fmt.Errorf("invalid reply_to hex: %w", err)
			}
		}
		post, err := svc.Posts.CreatePost(ctx, text, nil, replyToCID)
		if err != nil {
			return "", err
		}

		// WU-3: Index locally created posts.
		if idx != nil {
			if err := idx.IndexPost(post); err != nil {
				log.Printf("Warning: failed to index local post: %v", err)
			}
		}

		return hex.EncodeToString(post.Id), nil
	})

	webHandler.SetRepost(func(ctx context.Context, targetCIDHex string) (string, error) {
		targetCID, err := hex.DecodeString(targetCIDHex)
		if err != nil {
			return "", fmt.Errorf("invalid target CID: %w", err)
		}
		post, err := svc.Posts.CreateRepost(ctx, targetCID)
		if err != nil {
			return "", err
		}
		return hex.EncodeToString(post.Id), nil
	})

	webHandler.SetCreateReaction(func(ctx context.Context, kp *identity.KeyPair, targetCID []byte) error {
		reactions := social.NewReactionService(db, kp)
		reactions.SetPublisher(p2pHost)
		_, err := reactions.CreateReaction(ctx, targetCID)
		return err
	})

	webHandler.SetFollow(func(ctx context.Context, kp *identity.KeyPair, targetPubkey []byte) error {
		follows := social.NewFollowService(db, svc.Feed, kp)
		follows.SetPublisher(p2pHost)
		_, err := follows.Follow(ctx, targetPubkey)
		return err
	})

	webHandler.SetUnfollow(func(ctx context.Context, kp *identity.KeyPair, targetPubkey []byte) error {
		follows := social.NewFollowService(db, svc.Feed, kp)
		follows.SetPublisher(p2pHost)
		_, err := follows.Unfollow(ctx, targetPubkey)
		return err
	})

	webHandler.SetUpdateProfile(func(ctx context.Context, kp *identity.KeyPair, displayName, bio, website string, avatarCID, bannerCID []byte) error {
		profiles := social.NewProfileService(db, kp)
		profiles.SetPublisher(p2pHost)

		existing, err := profiles.GetProfile(kp.PublicKeyBytes())
		if err != nil {
			return err
		}
		if existing == nil {
			_, err = profiles.CreateProfile(ctx, displayName, bio, website, avatarCID, bannerCID)
			return err
		}

		_, err = profiles.UpdateProfile(ctx, displayName, bio, website, avatarCID, bannerCID)
		return err
	})

	webHandler.SetSendDM(func(ctx context.Context, kp *identity.KeyPair, recipientPubkey []byte, content string) error {
		dms := social.NewDMService(db, kp)
		dms.SetPublisher(p2pHost)
		_, err := dms.SendDM(ctx, recipientPubkey, content)
		return err
	})

	nodeStartTime := time.Now()
	webHandler.SetNodeStatus(func() (int, float64, int64, int64, int) {
		uptimeSecs := time.Since(nodeStartTime).Seconds()
		storageLimit := int64(cfg.Node.MaxStorageGB) * 1024 * 1024 * 1024

		var peers int
		if p2pHost != nil {
			peers = p2pHost.PeerCount()
		}

		var storageUsed int64
		if s, err := content.DirSize(filepath.Join(dataDir, "data")); err == nil {
			storageUsed = s
		}

		var subscriptions int
		if count, err := db.CountSubscriptions(); err == nil {
			subscriptions = count
		}

		return peers, uptimeSecs, storageUsed, storageLimit, subscriptions
	})

	// WU-6: Wire the indexer client to the web handler for broader search.
	webHandler.SetIndexerClient(svc.Indexer)

	return webHandler.Routes()
}

// buildAPIDeps constructs the HandlerDeps struct for the API server.
func buildAPIDeps(
	db *storage.DB,
	cas *content.ContentStore,
	kp *identity.KeyPair,
	idHolder *identity.Holder,
	svc *ServiceBundle,
	p2pHost *p2p.Host,
	cfg *config.Config,
	cfgPath string,
	webRoutes chi.Router,
	identitySync func(*identity.KeyPair),
) *api.HandlerDeps {
	deps := &api.HandlerDeps{
		DB:             db,
		CAS:            cas,
		KeyPair:        kp,
		IdentityHolder: idHolder,
		Posts:          svc.Posts,
		Reactions:      svc.Reactions,
		Profiles:       svc.Profiles,
		DMs:            svc.DMs,
		Follows:        svc.Follows,
		Notifs:         svc.Notifs,
		Feed:           svc.Feed,
		Timeline:       svc.Timeline,
		P2PHost:        p2pHost,
		Config:         cfg,
		ConfigPath:     cfgPath,
		IndexerClient:  svc.Indexer,
		IdentityChange: identitySync,
		WebHandler:     webRoutes,
	}
	return deps
}

// runServer starts the API server and blocks until a shutdown signal is
// received. It gracefully stops the server and closes the P2P host.
func runServer(
	ctx context.Context,
	cancel context.CancelFunc,
	server *api.Server,
	p2pHost *p2p.Host,
	cfg *config.Config,
) error {
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
