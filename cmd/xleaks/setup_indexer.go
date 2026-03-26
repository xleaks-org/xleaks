package main

import (
	"context"
	"encoding/hex"
	"log/slog"
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
		slog.Warn("indexer initialization failed", "error", err)
		return nil
	}

	if err := idx.Start(ctx); err != nil {
		slog.Warn("indexer start failed", "error", err)
		return nil
	}

	// Mount indexer API on public port.
	idxAPI := indexer.NewIndexerAPI(idx.Search(), idx.Trending(), idx.Stats())
	go func() {
		addr := cfg.Indexer.PublicAPIAddress
		slog.Info("indexer API listening", "addr", addr)
		if err := http.ListenAndServe(addr, idxAPI.Handler()); err != nil {
			slog.Error("indexer API error", "error", err)
		}
	}()

	// Advertise as indexer on DHT.
	if p2pHost != nil {
		go func() {
			if err := p2pHost.AdvertiseAsIndexer(ctx, cfg.Indexer.PublicAPIAddress); err != nil {
				slog.Warn("indexer advertisement failed", "error", err)
			}
		}()
	}

	// Reindex existing posts so the Bleve index catches up with DB content.
	go func() {
		posts, err := db.GetAllPosts(0, 10000)
		if err != nil {
			slog.Warn("failed to load posts for reindexing", "error", err)
			return
		}
		indexed := 0
		for _, p := range posts {
			if err := idx.Search().IndexPost(
				hex.EncodeToString(p.CID),
				hex.EncodeToString(p.Author),
				p.Content,
				nil, // tags not stored in PostRow
				p.Timestamp,
			); err == nil {
				indexed++
			}
		}
		slog.Info("indexer reindexed existing posts", "count", indexed)
	}()

	go func() {
		profiles, err := db.GetAllProfiles()
		if err != nil {
			slog.Warn("failed to load profiles for reindexing", "error", err)
			return
		}
		indexed := 0
		for _, profile := range profiles {
			if err := idx.Search().IndexProfile(
				hex.EncodeToString(profile.Pubkey),
				profile.DisplayName,
				profile.Bio,
			); err == nil {
				indexed++
			}
		}
		slog.Info("indexer reindexed existing profiles", "count", indexed)
	}()

	slog.Info("indexer mode enabled")
	return idx
}
