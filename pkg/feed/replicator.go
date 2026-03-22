package feed

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/storage"
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
			log.Printf("replicator: failed to fetch content %s: %v", cidHex, err)
			continue
		}

		// Validate the fetched data matches the CID.
		if !content.ValidateCID(post.CID, data) {
			log.Printf("replicator: CID validation failed for %s", cidHex)
			continue
		}

		// Store the fetched content.
		if err := r.cas.Put(post.CID, data); err != nil {
			log.Printf("replicator: failed to store content %s: %v", cidHex, err)
			continue
		}

		// Track access for eviction purposes.
		if err := r.db.TrackContentAccess(post.CID, false); err != nil {
			log.Printf("replicator: failed to track content access for %s: %v", cidHex, err)
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
			log.Printf("replicator: eviction: failed to delete content %s from CAS: %v", cidHex, err)
		}

		// Remove from tracking table.
		if err := r.db.DeleteContentAccess(cid); err != nil {
			log.Printf("replicator: eviction: failed to delete content access tracking for %s: %v", cidHex, err)
		}
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
// content in batches until usage drops below 85% of maxBytes.
func (r *Replicator) checkAndEvict(maxBytes int64) {
	dataDir := r.cas.BasePath()
	currentSize, err := content.DirSize(dataDir)
	if err != nil {
		log.Printf("replicator: checkAndEvict: failed to compute DirSize for %s: %v", dataDir, err)
		return
	}

	if currentSize <= maxBytes {
		return
	}

	log.Printf("replicator: checkAndEvict: starting eviction — current=%d bytes, max=%d bytes", currentSize, maxBytes)

	// Evict until we're under 85% of max.
	targetBytes := int64(float64(maxBytes) * 0.85)
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
			log.Printf("replicator: checkAndEvict: failed to recompute DirSize after eviction: %v", err)
			break
		}

		// If no progress was made (nothing left to evict), stop.
		if newSize >= currentSize {
			log.Printf("replicator: checkAndEvict: no progress made, stopping eviction")
			break
		}
		currentSize = newSize
	}

	log.Printf("replicator: checkAndEvict: eviction complete — current=%d bytes", currentSize)
}

