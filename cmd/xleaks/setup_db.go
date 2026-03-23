package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/xleaks-org/xleaks/pkg/config"
	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/storage"
)

// setupDatabase initialises the data directories, SQLite database, and
// content-addressed store.
func setupDatabase(cfg *config.Config) (*storage.DB, *content.ContentStore, error) {
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
			return nil, nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Initialize SQLite database.
	dbPath := filepath.Join(dataDir, "data", "index.db")
	db, err := storage.NewDB(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Migrate(); err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("failed to run database migrations: %w", err)
	}

	// Initialize content-addressed store.
	casPath := filepath.Join(dataDir, "data", "objects")
	cas, err := content.NewContentStore(casPath)
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("failed to create content store: %w", err)
	}

	return db, cas, nil
}
