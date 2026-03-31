package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"github.com/xleaks-org/xleaks/pkg/config"
	"github.com/xleaks-org/xleaks/pkg/indexer"
	"github.com/xleaks-org/xleaks/pkg/logging"
	"github.com/xleaks-org/xleaks/pkg/p2p"
	"github.com/xleaks-org/xleaks/pkg/storage"
)

const maxIndexerHeaderBytes = 64 << 10

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
	indexerServer := newIndexerHTTPServer(cfg.Indexer.PublicAPIAddress, idxAPI.Handler())
	go func() {
		slog.Info("indexer API listening", "addr", logging.RedactAddr(indexerServer.Addr))
		if err := indexerServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("indexer API error", "error", err)
		}
	}()
	go shutdownIndexerHTTPServer(ctx, indexerServer)

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

	// Start periodic trending digest broadcast to /xleaks/global.
	go startTrendingDigestBroadcast(ctx, idx, p2pHost)

	slog.Info("indexer mode enabled")
	return idx
}

func newIndexerHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    maxIndexerHeaderBytes,
	}
}

func shutdownIndexerHTTPServer(ctx context.Context, srv *http.Server) {
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil && err != context.Canceled && err != http.ErrServerClosed {
		slog.Warn("indexer API shutdown failed", "error", err)
	}
}

// startTrendingDigestBroadcast periodically fetches trending tags and posts
// from the indexer and publishes a JSON digest to the /xleaks/global topic.
func startTrendingDigestBroadcast(ctx context.Context, idx *indexer.Indexer, host *p2p.Host) {
	if idx == nil || host == nil {
		return
	}

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			publishTrendingDigest(ctx, idx, host)
		}
	}
}

// publishTrendingDigest fetches trending data and publishes it to the global topic.
func publishTrendingDigest(ctx context.Context, idx *indexer.Indexer, host *p2p.Host) {
	trending := idx.Trending()
	if trending == nil {
		return
	}

	tags, err := trending.GetTrendingTags("24h", 10)
	if err != nil {
		slog.Warn("trending digest: failed to get tags", "error", err)
		tags = nil
	}

	posts, err := trending.GetTrendingPosts("24h", 10)
	if err != nil {
		slog.Warn("trending digest: failed to get posts", "error", err)
		posts = nil
	}

	digest := map[string]interface{}{
		"type":      "trending_digest",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"tags":      tags,
		"posts":     posts,
	}

	data, err := json.Marshal(digest)
	if err != nil {
		slog.Warn("trending digest: failed to marshal", "error", err)
		return
	}

	if err := host.Publish(ctx, p2p.GlobalTopic(), data); err != nil {
		slog.Warn("trending digest: failed to publish", "error", err)
		return
	}

	slog.Debug("trending digest published", "tags", len(tags), "posts", len(posts))
}
