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
	"github.com/xleaks-org/xleaks/pkg/p2p"
	"github.com/xleaks-org/xleaks/pkg/storage"
	"github.com/xleaks-org/xleaks/pkg/web"
)

// setupWebHandler initialises the Go-template web UI and wires its callbacks.
func setupWebHandler(
	db *storage.DB,
	idHolder *identity.Holder,
	svc *ServiceBundle,
	cfg *config.Config,
	p2pHost *p2p.Host,
	dataDir string,
) chi.Router {
	webHandler, err := web.NewHandler(db, idHolder, svc.Timeline)
	if err != nil {
		log.Printf("Warning: web UI failed to initialize: %v", err)
		return nil
	}

	webHandler.SetCreatePost(func(ctx context.Context, text string) (string, error) {
		post, err := svc.Posts.CreatePost(ctx, text, nil, nil)
		if err != nil {
			return "", err
		}
		return hex.EncodeToString(post.Id), nil
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
		Notifs:         svc.Notifs,
		Feed:           svc.Feed,
		Timeline:       svc.Timeline,
		P2PHost:        p2pHost,
		Config:         cfg,
		ConfigPath:     cfgPath,
		IndexerClient:  svc.Indexer,
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
