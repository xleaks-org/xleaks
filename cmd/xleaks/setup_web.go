package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
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
	"github.com/xleaks-org/xleaks/pkg/logging"
	"github.com/xleaks-org/xleaks/pkg/p2p"
	"github.com/xleaks-org/xleaks/pkg/storage"
	"github.com/xleaks-org/xleaks/pkg/web"
)

type runtimeCloser interface {
	Close() error
}

// setupWebHandler initialises the Go-template web UI and wires its callbacks.
// The idx parameter may be nil when the node is not running in indexer mode.
func setupWebHandler(
	ctx context.Context,
	db *storage.DB,
	idHolder *identity.Holder,
	svc *ServiceBundle,
	cfg *config.Config,
	p2pHost *p2p.Host,
	dataDir string,
	idx *indexer.Indexer,
	identitySync func(*identity.KeyPair),
	ensureTopic func(string) error,
) chi.Router {
	if cfg == nil || !cfg.API.EnableWebUI {
		slog.Info("web UI disabled")
		return nil
	}

	sessionMgr := web.NewSessionManager()
	webHandler, err := web.NewHandler(db, idHolder, svc.Timeline, sessionMgr)
	if err != nil {
		sessionMgr.Stop()
		slog.Warn("web UI failed to initialize", "error", err)
		return nil
	}
	closeOnContextCancel(ctx, webHandler)

	webHandler.SetOnIdentityChange(identitySync)
	webHandler.SetTopicSubscriber(ensureTopic)
	webHandler.SetWebSocketEnabled(cfg.API.EnableWebSocket)
	webHandler.SetPassphraseMinLength(cfg.PassphraseMinLen())

	webHandler.SetCreatePost(func(ctx context.Context, text string, mediaCIDHexes []string, replyTo string) (string, error) {
		mediaCIDs := make([][]byte, 0, len(mediaCIDHexes))
		for _, mediaCIDHex := range mediaCIDHexes {
			mediaCID, err := hex.DecodeString(mediaCIDHex)
			if err != nil {
				return "", fmt.Errorf("invalid media CID hex: %w", err)
			}
			mediaCIDs = append(mediaCIDs, mediaCID)
		}

		var replyToCID []byte
		if replyTo != "" {
			var err error
			replyToCID, err = hex.DecodeString(replyTo)
			if err != nil {
				return "", fmt.Errorf("invalid reply_to hex: %w", err)
			}
		}
		post, err := svc.Posts.CreatePost(ctx, text, mediaCIDs, replyToCID)
		if err != nil {
			return "", err
		}

		// WU-3: Index locally created posts.
		if idx != nil {
			if err := idx.IndexPost(post); err != nil {
				slog.Warn("failed to index local post", "error", err)
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
		_, err := svc.Reactions.CreateReactionAs(ctx, kp, targetCID)
		return err
	})

	webHandler.SetFollow(func(ctx context.Context, kp *identity.KeyPair, targetPubkey []byte) error {
		_, err := svc.Follows.FollowAs(ctx, kp, targetPubkey)
		return err
	})

	webHandler.SetUnfollow(func(ctx context.Context, kp *identity.KeyPair, targetPubkey []byte) error {
		_, err := svc.Follows.UnfollowAs(ctx, kp, targetPubkey)
		return err
	})

	webHandler.SetUpdateProfile(func(ctx context.Context, kp *identity.KeyPair, displayName, bio, website string, avatarCID, bannerCID []byte) error {
		existing, err := svc.Profiles.GetProfile(kp.PublicKeyBytes())
		if err != nil {
			return err
		}
		if existing == nil {
			_, err = svc.Profiles.CreateProfileAs(ctx, kp, displayName, bio, website, avatarCID, bannerCID)
			return err
		}

		_, err = svc.Profiles.UpdateProfileAs(ctx, kp, displayName, bio, website, avatarCID, bannerCID)
		return err
	})

	webHandler.SetSendDM(func(ctx context.Context, kp *identity.KeyPair, recipientPubkey []byte, content string) error {
		_, err := svc.DMs.SendDMAs(ctx, kp, recipientPubkey, content)
		return err
	})

	nodeStartTime := time.Now()
	webHandler.SetNodeStatus(func() (int, float64, int64, int64, int) {
		uptimeSecs := time.Since(nodeStartTime).Seconds()
		storageLimit := cfg.MaxStorageBytes()

		var peers int
		if p2pHost != nil {
			peers = p2pHost.PeerCount()
		}

		var storageUsed int64
		if s, err := content.DirSize(filepath.Join(dataDir, "data")); err == nil {
			storageUsed = s
		}

		var subscriptions int
		var ownerPubkey []byte
		if kp := idHolder.Get(); kp != nil {
			ownerPubkey = kp.PublicKeyBytes()
		}
		if count, err := db.CountSubscriptions(ownerPubkey); err == nil {
			subscriptions = count
		}

		return peers, uptimeSecs, storageUsed, storageLimit, subscriptions
	})

	// WU-6: Wire the indexer client to the web handler for broader search.
	webHandler.SetIndexerClient(svc.Indexer)

	return webHandler.Routes()
}

func closeOnContextCancel(ctx context.Context, closer runtimeCloser) {
	if ctx == nil || closer == nil {
		return
	}

	go func() {
		<-ctx.Done()
		if err := closer.Close(); err != nil {
			slog.Warn("runtime cleanup failed", "error", err)
		}
	}()
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
	apiTokenConfigured bool,
	identitySync func(*identity.KeyPair),
	ensureTopic func(string) error,
) *api.HandlerDeps {
	deps := &api.HandlerDeps{
		DB:                 db,
		CAS:                cas,
		KeyPair:            kp,
		IdentityHolder:     idHolder,
		Posts:              svc.Posts,
		Reactions:          svc.Reactions,
		Profiles:           svc.Profiles,
		DMs:                svc.DMs,
		Follows:            svc.Follows,
		Notifs:             svc.Notifs,
		Feed:               svc.Feed,
		Timeline:           svc.Timeline,
		P2PHost:            p2pHost,
		Config:             cfg,
		ConfigPath:         cfgPath,
		APITokenConfigured: false,
		IndexerClient:      svc.Indexer,
		IdentityChange:     identitySync,
		EnsureTopic:        ensureTopic,
		WebHandler:         webRoutes,
	}
	deps.APITokenConfigured = apiTokenConfigured
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
		slog.Info("shutting down gracefully")
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("server shutdown error", "error", err)
		}
	}()

	slog.Info("XLeaks node starting", "addr", logging.RedactAddr(cfg.API.ListenAddress))
	if p2pHost != nil {
		slog.Info("P2P connected", "peers", p2pHost.PeerCount())
	} else {
		slog.Info("running in offline mode (no P2P)")
	}

	if err := server.Start(); err != nil {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}
