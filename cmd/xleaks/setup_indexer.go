package main

import (
	"context"
	"log"
	"net/http"
	"path/filepath"

	"github.com/xleaks-org/xleaks/pkg/config"
	"github.com/xleaks-org/xleaks/pkg/indexer"
	"github.com/xleaks-org/xleaks/pkg/p2p"
	"github.com/xleaks-org/xleaks/pkg/storage"
)

// setupIndexer initialises the indexer subsystem when the node is running in
// indexer mode. It starts the indexer, mounts its public API on a separate port,
// and advertises the node as an indexer on the DHT.
// Returns nil if the node is not in indexer mode or if initialisation fails.
func setupIndexer(ctx context.Context, db *storage.DB, dataDir string, cfg *config.Config, p2pHost *p2p.Host) *indexer.Indexer {
	if !cfg.IsIndexer() {
		return nil
	}

	idx, err := indexer.NewIndexer(db, filepath.Join(dataDir, "indexer"))
	if err != nil {
		log.Printf("Warning: indexer initialization failed: %v", err)
		return nil
	}

	if err := idx.Start(ctx); err != nil {
		log.Printf("Warning: indexer start failed: %v", err)
		return nil
	}

	// Mount indexer API on public port.
	idxAPI := indexer.NewIndexerAPI(idx.Search(), idx.Trending(), idx.Stats())
	go func() {
		addr := cfg.Indexer.PublicAPIAddress
		log.Printf("Indexer API listening on %s", addr)
		if err := http.ListenAndServe(addr, idxAPI.Handler()); err != nil {
			log.Printf("Indexer API error: %v", err)
		}
	}()

	// Advertise as indexer on DHT.
	if p2pHost != nil {
		go p2pHost.AdvertiseAsIndexer(ctx)
	}

	log.Println("Indexer mode enabled")
	return idx
}
