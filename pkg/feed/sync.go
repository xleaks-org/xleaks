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

// Syncer handles historical content sync when a user follows a new publisher.
type Syncer struct {
	db         *storage.DB
	replicator *Replicator
	// OnDiscoverContent is called to discover CIDs from a publisher via DHT.
	OnDiscoverContent func(ctx context.Context, authorPubkeyHex string) ([]string, error)
}

// NewSyncer creates a new Syncer.
func NewSyncer(db *storage.DB, replicator *Replicator) *Syncer {
	return &Syncer{
		db:         db,
		replicator: replicator,
	}
}

// SyncPublisher performs a full historical sync for a newly followed publisher.
// It discovers their content via DHT and fetches everything.
func (s *Syncer) SyncPublisher(ctx context.Context, pubkey []byte) error {
	if s.OnDiscoverContent == nil {
		return fmt.Errorf("OnDiscoverContent callback not set")
	}
	if s.replicator.OnFetchContent == nil {
		return fmt.Errorf("OnFetchContent callback not set on replicator")
	}

	pubkeyHex := hex.EncodeToString(pubkey)

	// Discover content CIDs for this publisher via DHT.
	cidHexList, err := s.OnDiscoverContent(ctx, pubkeyHex)
	if err != nil {
		return fmt.Errorf("discover content for %s: %w", pubkeyHex, err)
	}

	// Fetch and store each discovered CID.
	for _, cidHex := range cidHexList {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		cidBytes, err := content.HexToCID(cidHex)
		if err != nil {
			log.Printf("sync: invalid CID hex %s: %v", cidHex, err)
			continue
		}

		// Skip content we already have.
		if s.replicator.cas.Has(cidBytes) {
			continue
		}

		data, err := s.replicator.OnFetchContent(ctx, cidHex)
		if err != nil {
			log.Printf("sync: failed to fetch content %s: %v", cidHex, err)
			continue
		}

		// Validate the fetched data matches the CID.
		if !content.ValidateCID(cidBytes, data) {
			log.Printf("sync: CID validation failed for %s", cidHex)
			continue
		}

		// Store the fetched content.
		if err := s.replicator.cas.Put(cidBytes, data); err != nil {
			log.Printf("sync: failed to store content %s: %v", cidHex, err)
			continue
		}

		// Track access.
		if err := s.db.TrackContentAccess(cidBytes, false); err != nil {
			log.Printf("sync: failed to track content access for %s: %v", cidHex, err)
		}
	}

	// Mark sync as complete.
	if err := s.MarkSyncComplete(pubkey); err != nil {
		return err
	}

	return nil
}

// MarkSyncComplete marks a subscription's historical sync as done.
func (s *Syncer) MarkSyncComplete(pubkey []byte) error {
	if err := s.db.MarkSyncCompleted(pubkey); err != nil {
		return fmt.Errorf("mark sync complete: %w", err)
	}
	return nil
}

// GetPendingSyncs returns publishers that still need historical sync.
func (s *Syncer) GetPendingSyncs() ([][]byte, error) {
	subs, err := s.db.GetPendingSyncs()
	if err != nil {
		return nil, fmt.Errorf("get pending syncs: %w", err)
	}

	pubkeys := make([][]byte, len(subs))
	for i, sub := range subs {
		pubkeys[i] = sub.Pubkey
	}
	return pubkeys, nil
}

// StartBackgroundSync starts a goroutine that periodically checks for pending syncs.
// It uses exponential backoff: starts at 30s, doubles on error (max 10min),
// resets to 30s on success.
func (s *Syncer) StartBackgroundSync(ctx context.Context) {
	const (
		baseInterval = 30 * time.Second
		maxInterval  = 10 * time.Minute
	)

	go func() {
		backoff := baseInterval

		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}

			pubkeys, err := s.GetPendingSyncs()
			if err != nil {
				log.Printf("background sync: failed to get pending syncs: %v", err)
				backoff = nextBackoff(backoff, maxInterval)
				continue
			}

			hadError := false
			for _, pubkey := range pubkeys {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if err := s.SyncPublisher(ctx, pubkey); err != nil {
					log.Printf("background sync: failed to sync publisher %s: %v",
						hex.EncodeToString(pubkey)[:16], err)
					hadError = true
				}
			}

			if hadError {
				backoff = nextBackoff(backoff, maxInterval)
			} else {
				backoff = baseInterval
			}
		}
	}()
}

// nextBackoff doubles the current backoff interval, capping at maxInterval.
func nextBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}
