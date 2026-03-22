package main

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/xleaks/xleaks/pkg/api"
	"github.com/xleaks/xleaks/pkg/config"
	"github.com/xleaks/xleaks/pkg/content"
	"github.com/xleaks/xleaks/pkg/feed"
	"github.com/xleaks/xleaks/pkg/identity"
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

	// Try to load identity. If none exists, the UI will handle onboarding.
	var kp *identity.KeyPair
	keyPath := filepath.Join(dataDir, "identity", "primary.key")
	if _, err := os.Stat(keyPath); err == nil {
		log.Println("Identity found. Unlock via API to activate.")
		// Key exists but needs to be unlocked via the API.
		// For now, create a placeholder nil KeyPair; the unlock endpoint will set it.
	} else {
		log.Println("No identity found. The UI will guide you through onboarding.")
	}

	// Create a temporary identity for services that require one at init.
	// The real identity will be set when the user unlocks or creates one via the API.
	if kp == nil {
		kp = &identity.KeyPair{
			PrivateKey: ed25519.PrivateKey(make([]byte, ed25519.PrivateKeySize)),
			PublicKey:  ed25519.PublicKey(make([]byte, ed25519.PublicKeySize)),
		}
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
	timeline := feed.NewTimeline(db, kp.PublicKeyBytes())

	// Create API server.
	deps := &api.HandlerDeps{
		DB:        db,
		CAS:       cas,
		KeyPair:   kp,
		Posts:     postService,
		Reactions: reactionService,
		Profiles:  profileService,
		DMs:       dmService,
		Notifs:    notifService,
		Feed:      feedManager,
		Timeline:  timeline,
	}

	server := api.NewServer(cfg.API.ListenAddress, deps)

	// Handle shutdown signals.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("Shutting down...")
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
	}()

	_ = ctx // Will be used for P2P host lifecycle

	log.Printf("XLeaks node starting on %s", cfg.API.ListenAddress)

	if err := server.Start(); err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}
