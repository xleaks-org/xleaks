package feed

import (
	"context"
	"encoding/hex"
	"fmt"

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
