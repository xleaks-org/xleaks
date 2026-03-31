package feed

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/xleaks-org/xleaks/pkg/content"
	xlog "github.com/xleaks-org/xleaks/pkg/logging"
	"github.com/xleaks-org/xleaks/pkg/storage"
)

// evictionTarget is the percentage of maxBytes to evict down to during storage cleanup.
const evictionTarget = 0.85

// defaultEvictionInterval is the default interval between storage eviction checks.
const defaultEvictionInterval = 5 * time.Minute

// Replicator handles fetching and pinning content from followed publishers.
// It coordinates with the P2P layer to discover and download historical content.
type Replicator struct {
	db  *storage.DB
	cas *content.ContentStore
	// OnFetchContent is called to request content from the network (set by P2P layer).
	OnFetchContent func(ctx context.Context, cidHex string) ([]byte, error)
	storageLimitMu sync.RWMutex
	storageLimit   int64
}

// NewReplicator creates a new Replicator.
func NewReplicator(db *storage.DB, cas *content.ContentStore) *Replicator {
	return &Replicator{
		db:  db,
		cas: cas,
	}
}

// SetStorageLimit updates the active storage limit used by the background
// eviction loop.
func (r *Replicator) SetStorageLimit(maxBytes int64) {
	if maxBytes < 0 {
		maxBytes = 0
	}
	r.storageLimitMu.Lock()
	r.storageLimit = maxBytes
	r.storageLimitMu.Unlock()
}

func (r *Replicator) currentStorageLimit() int64 {
	r.storageLimitMu.RLock()
	defer r.storageLimitMu.RUnlock()
	return r.storageLimit
}

// fetchAndStore fetches content by CID hex, validates it, stores it in the CAS,
// and tracks the access. This is the shared logic between replicator and syncer.
func fetchAndStore(ctx context.Context, cidHex string, fetcher func(context.Context, string) ([]byte, error), cas *content.ContentStore, db *storage.DB) error {
	data, err := fetcher(ctx, cidHex)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", cidHex, err)
	}
	cidBytes, err := content.HexToCID(cidHex)
	if err != nil {
		return fmt.Errorf("parse CID %s: %w", cidHex, err)
	}
	if !content.ValidateCID(cidBytes, data) {
		return fmt.Errorf("CID mismatch for %s", cidHex)
	}
	if err := cas.Put(cidBytes, data); err != nil {
		return fmt.Errorf("store %s: %w", cidHex, err)
	}
	if err := db.TrackContentAccess(cidBytes, false); err != nil {
		return fmt.Errorf("track %s: %w", cidHex, err)
	}
	return nil
}

// PinContent marks content from a followed publisher as pinned (never evict).
func (r *Replicator) PinContent(cid []byte) error {
	if err := r.db.SetContentPinned(cid, true); err != nil {
		return fmt.Errorf("pin content: %w", err)
	}
	return nil
}

// UnpinContent removes the pin from content (eligible for eviction).
func (r *Replicator) UnpinContent(cid []byte) error {
	if err := r.db.SetContentPinned(cid, false); err != nil {
		return fmt.Errorf("unpin content: %w", err)
	}
	return nil
}

// FetchMissingContent checks for content we should have but don't, and fetches it.
// It looks at all posts by the given author in the DB and tries to fetch any
// whose CID is not present in the content-addressed store.
func (r *Replicator) FetchMissingContent(ctx context.Context, authorPubkey []byte) error {
	if r.OnFetchContent == nil {
		return fmt.Errorf("OnFetchContent callback not set")
	}

	// Retrieve all posts by this author.
	posts, err := r.db.GetPostsByAuthor(authorPubkey, 0, 1000)
	if err != nil {
		return fmt.Errorf("get posts for replication: %w", err)
	}

	for _, post := range posts {
		// Skip content we already have in the CAS.
		if r.cas.Has(post.CID) {
			continue
		}

		cidHex := hex.EncodeToString(post.CID)
		if err := fetchAndStore(ctx, cidHex, r.OnFetchContent, r.cas, r.db); err != nil {
			slog.Warn("replicator fetch failed", "cid", cidHex, "error", err)
			continue
		}
	}

	return nil
}

// EvictStaleContent removes LRU non-pinned content when storage exceeds limit.
// It fetches the least recently used content and deletes it from both the
// content store and the access tracking table until the eviction batch is
// complete.
func (r *Replicator) EvictStaleContent(maxBytes int64) error {
	// Estimate how many items to evict: fetch a batch and delete them.
	// A simple approach: get 100 LRU items per pass.
	const batchSize = 100

	cids, err := r.db.GetLRUContent(batchSize)
	if err != nil {
		return fmt.Errorf("get LRU content for eviction: %w", err)
	}

	for _, cid := range cids {
		cidHex := hex.EncodeToString(cid)

		// Remove from content-addressed store.
		if err := r.cas.Delete(cid); err != nil {
			slog.Warn("replicator eviction: failed to delete content from CAS", "cid", cidHex, "error", err)
		}

		// Remove from tracking table.
		if err := r.db.DeleteContentAccess(cid); err != nil {
			slog.Warn("replicator eviction: failed to delete content access tracking", "cid", cidHex, "error", err)
		}
	}

	return nil
}

// StartStorageManager launches a background goroutine that periodically checks
// the CAS data directory size and evicts stale content when usage exceeds
// maxBytes. Eviction continues until usage drops below 90% of maxBytes.
// The goroutine exits when the context is cancelled.
func (r *Replicator) StartStorageManager(ctx context.Context, maxBytes int64, interval time.Duration) {
	r.SetStorageLimit(maxBytes)
	if interval <= 0 {
		interval = defaultEvictionInterval
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.checkAndEvict(r.currentStorageLimit())
			}
		}
	}()
}

// checkAndEvict calculates the current CAS directory size and evicts LRU
// content in batches until usage drops below 85% of maxBytes.
func (r *Replicator) checkAndEvict(maxBytes int64) {
	dataDir := r.cas.BasePath()
	currentSize, err := content.DirSize(dataDir)
	if err != nil {
		slog.Warn("replicator: failed to compute directory size", "dir", xlog.RedactPath(dataDir), "error", err)
		return
	}

	if currentSize <= maxBytes {
		return
	}

	slog.Info("replicator: starting eviction", "current_bytes", currentSize, "max_bytes", maxBytes)

	// Evict until we're under the eviction target percentage of max.
	targetBytes := int64(float64(maxBytes) * evictionTarget)
	for currentSize > targetBytes {
		items, err := r.db.GetLRUContent(100)
		if err != nil || len(items) == 0 {
			break
		}
		for _, cid := range items {
			_ = r.cas.Delete(cid)
			_ = r.db.DeleteContentAccess(cid)
		}

		// Recalculate size after eviction.
		newSize, err := content.DirSize(dataDir)
		if err != nil {
			slog.Warn("replicator: failed to recompute directory size after eviction", "error", err)
			break
		}

		// If no progress was made (nothing left to evict), stop.
		if newSize >= currentSize {
			slog.Warn("replicator: eviction made no progress, stopping")
			break
		}
		currentSize = newSize
	}

	slog.Info("replicator: eviction complete", "current_bytes", currentSize)
}
