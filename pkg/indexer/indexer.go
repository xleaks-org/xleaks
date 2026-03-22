package indexer

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"path/filepath"

	pb "github.com/xleaks/xleaks/proto/gen"

	"github.com/xleaks/xleaks/pkg/storage"
)

// Indexer represents a node running in indexer mode. It subscribes broadly
// to the network, builds a full-text search index, and computes trending content.
type Indexer struct {
	db       *storage.DB
	search   *SearchIndex
	trending *TrendingEngine
	stats    *StatsCollector
	// Subscribe is called to subscribe to a publisher's content.
	Subscribe func(pubkeyHex string) error

	cancel context.CancelFunc
}

// NewIndexer creates a new Indexer with its search, trending, and stats subsystems.
func NewIndexer(db *storage.DB, dataDir string) (*Indexer, error) {
	searchPath := filepath.Join(dataDir, "search.bleve")
	search, err := NewSearchIndex(searchPath)
	if err != nil {
		return nil, fmt.Errorf("create search index: %w", err)
	}

	return &Indexer{
		db:       db,
		search:   search,
		trending: NewTrendingEngine(db),
		stats:    NewStatsCollector(db),
	}, nil
}

// Start begins the indexer's background processing.
func (idx *Indexer) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	idx.cancel = cancel

	// Run in background; currently a placeholder for future periodic tasks
	// (e.g., re-indexing stale data, refreshing trending caches).
	go func() {
		<-ctx.Done()
		log.Println("indexer: stopped")
	}()

	log.Println("indexer: started")
	return nil
}

// Stop shuts down the indexer and closes the search index.
func (idx *Indexer) Stop() error {
	if idx.cancel != nil {
		idx.cancel()
	}
	return idx.search.Close()
}

// IndexPost indexes a protobuf Post into the full-text search index.
func (idx *Indexer) IndexPost(post *pb.Post) error {
	id := hex.EncodeToString(post.GetId())
	author := hex.EncodeToString(post.GetAuthor())
	content := post.GetContent()
	tags := post.GetTags()
	timestamp := int64(post.GetTimestamp())

	if err := idx.search.IndexPost(id, author, content, tags, timestamp); err != nil {
		return fmt.Errorf("index post %s: %w", id, err)
	}
	return nil
}

// IndexProfile indexes a protobuf Profile into the full-text search index.
func (idx *Indexer) IndexProfile(profile *pb.Profile) error {
	pubkeyHex := hex.EncodeToString(profile.GetAuthor())
	displayName := profile.GetDisplayName()
	bio := profile.GetBio()

	if err := idx.search.IndexProfile(pubkeyHex, displayName, bio); err != nil {
		return fmt.Errorf("index profile %s: %w", pubkeyHex, err)
	}
	return nil
}
