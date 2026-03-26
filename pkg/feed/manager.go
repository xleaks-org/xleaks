package feed

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/xleaks-org/xleaks/pkg/storage"
)

// Manager handles subscription management (follow/unfollow logic) and
// coordinates topic subscriptions with the P2P layer.
type Manager struct {
	db          *storage.DB
	mu          sync.RWMutex
	subscribers map[string]bool // pubkey hex -> subscribed for the active identity

	// Callbacks for P2P integration.
	OnSubscribe   func(ctx context.Context, pubkeyHex string) error
	OnUnsubscribe func(pubkeyHex string) error
}

// NewManager creates a new feed Manager.
func NewManager(db *storage.DB) *Manager {
	return &Manager{
		db:          db,
		subscribers: make(map[string]bool),
	}
}

// ReloadSubscriptions reloads the current owner's subscriptions from the database
// and reconciles runtime topic subscriptions.
func (m *Manager) ReloadSubscriptions(ctx context.Context, ownerPubkey []byte) error {
	subs, err := m.db.GetSubscriptions(ownerPubkey)
	if err != nil {
		return fmt.Errorf("load subscriptions: %w", err)
	}

	next := make(map[string]bool, len(subs))
	for _, sub := range subs {
		next[hex.EncodeToString(sub.Pubkey)] = true
	}

	m.mu.Lock()
	current := make(map[string]bool, len(m.subscribers))
	for k, v := range m.subscribers {
		current[k] = v
	}
	m.subscribers = next
	m.mu.Unlock()

	for pubkeyHex := range current {
		if next[pubkeyHex] {
			continue
		}
		if m.OnUnsubscribe != nil {
			if err := m.OnUnsubscribe(pubkeyHex); err != nil {
				return fmt.Errorf("unsubscribe from topic: %w", err)
			}
		}
	}

	for pubkeyHex := range next {
		if current[pubkeyHex] {
			continue
		}
		if m.OnSubscribe != nil {
			if err := m.OnSubscribe(ctx, pubkeyHex); err != nil {
				return fmt.Errorf("subscribe to topic: %w", err)
			}
		}
	}

	return nil
}

// Follow subscribes the given owner to a publisher's content.
func (m *Manager) Follow(ctx context.Context, ownerPubkey, pubkey []byte, timestamp int64) error {
	hexKey := hex.EncodeToString(pubkey)

	m.mu.RLock()
	alreadyFollowing := m.subscribers[hexKey]
	m.mu.RUnlock()
	if alreadyFollowing {
		return nil
	}

	if err := m.db.AddSubscription(ownerPubkey, pubkey, timestamp); err != nil {
		return fmt.Errorf("add subscription: %w", err)
	}

	m.mu.Lock()
	m.subscribers[hexKey] = true
	m.mu.Unlock()

	if m.OnSubscribe != nil {
		if err := m.OnSubscribe(ctx, hexKey); err != nil {
			return fmt.Errorf("subscribe to topic: %w", err)
		}
	}

	return nil
}

// Unfollow removes a subscription to a publisher for the given owner.
func (m *Manager) Unfollow(ownerPubkey, pubkey []byte) error {
	hexKey := hex.EncodeToString(pubkey)

	m.mu.RLock()
	isFollowing := m.subscribers[hexKey]
	m.mu.RUnlock()
	if !isFollowing {
		return nil
	}

	if err := m.db.RemoveSubscription(ownerPubkey, pubkey); err != nil {
		return fmt.Errorf("remove subscription: %w", err)
	}

	m.mu.Lock()
	delete(m.subscribers, hexKey)
	m.mu.Unlock()

	if m.OnUnsubscribe != nil {
		if err := m.OnUnsubscribe(hexKey); err != nil {
			return fmt.Errorf("unsubscribe from topic: %w", err)
		}
	}

	return nil
}

// IsFollowing checks if a publisher is currently followed by the active runtime identity.
func (m *Manager) IsFollowing(pubkey []byte) bool {
	hexKey := hex.EncodeToString(pubkey)
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.subscribers[hexKey]
}

// FollowedPubkeys returns all currently followed publisher pubkey hex strings.
func (m *Manager) FollowedPubkeys() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0, len(m.subscribers))
	for k := range m.subscribers {
		keys = append(keys, k)
	}
	return keys
}
