package feed

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/xleaks/xleaks/pkg/content"
	"github.com/xleaks/xleaks/pkg/storage"
)

// Replicator handles fetching and pinning content from followed publishers.
// It coordinates with the P2P layer to discover and download historical content.
type Replicator struct {
	db  *storage.DB
	cas *content.ContentStore
	// OnFetchContent is called to request content from the network (set by P2P layer).
	OnFetchContent func(ctx context.Context, cidHex string) ([]byte, error)
}

// NewReplicator creates a new Replicator.
func NewReplicator(db *storage.DB, cas *content.ContentStore) *Replicator {
	return &Replicator{
		db:  db,
		cas: cas,
	}
}

// PinContent marks content from a followed publisher as pinned (never evict).
func (r *Replicator) PinContent(cid []byte) error {
	if err := r.db.TrackContentAccess(cid, true); err != nil {
		return fmt.Errorf("pin content: %w", err)
	}
	return nil
}

// UnpinContent removes the pin from content (eligible for eviction).
func (r *Replicator) UnpinContent(cid []byte) error {
	if err := r.db.TrackContentAccess(cid, false); err != nil {
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
		data, err := r.OnFetchContent(ctx, cidHex)
		if err != nil {
			// Log but continue with other content.
			continue
		}

		// Validate the fetched data matches the CID.
		if !content.ValidateCID(post.CID, data) {
			continue
		}

		// Store the fetched content.
		if err := r.cas.Put(post.CID, data); err != nil {
			continue
		}

		// Track access for eviction purposes.
		_ = r.db.TrackContentAccess(post.CID, false)
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
		// Remove from content-addressed store.
		_ = r.cas.Delete(cid)

		// Remove from tracking table.
		_ = r.db.DeleteContentAccess(cid)
	}

	return nil
}

// StartStorageManager launches a background goroutine that periodically checks
// the CAS data directory size and evicts stale content when usage exceeds
// maxBytes. Eviction continues until usage drops below 90% of maxBytes.
// The goroutine exits when the context is cancelled.
func (r *Replicator) StartStorageManager(ctx context.Context, maxBytes int64, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.checkAndEvict(maxBytes)
			}
		}
	}()
}

// checkAndEvict calculates the current CAS directory size and evicts LRU
// content in batches until usage drops below 90% of maxBytes.
func (r *Replicator) checkAndEvict(maxBytes int64) {
	dataDir := r.cas.BasePath()
	currentSize, err := dirSize(dataDir)
	if err != nil {
		return
	}

	if currentSize <= maxBytes {
		return
	}

	// Evict until we're under 90% of max.
	target := int64(float64(maxBytes) * 0.9)
	for currentSize > target {
		if err := r.EvictStaleContent(maxBytes); err != nil {
			break
		}

		// Recalculate size after eviction.
		newSize, err := dirSize(dataDir)
		if err != nil {
			break
		}

		// If no progress was made (nothing to evict), stop.
		if newSize >= currentSize {
			break
		}
		currentSize = newSize
	}
}

// dirSize recursively walks a directory and returns the total size in bytes
// of all files it contains.
func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		size += info.Size()
		return nil
	})
	return size, err
}
