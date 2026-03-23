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
	subscribers map[string]bool // pubkey hex -> subscribed

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

// LoadSubscriptions loads existing subscriptions from the database into memory.
func (m *Manager) LoadSubscriptions() error {
	subs, err := m.db.GetSubscriptions()
	if err != nil {
		return fmt.Errorf("load subscriptions: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, sub := range subs {
		hexKey := hex.EncodeToString(sub.Pubkey)
		m.subscribers[hexKey] = true
	}

	return nil
}

// Follow subscribes to a publisher's content.
func (m *Manager) Follow(ctx context.Context, pubkey []byte, timestamp int64) error {
	hexKey := hex.EncodeToString(pubkey)

	m.mu.Lock()
	if m.subscribers[hexKey] {
		m.mu.Unlock()
		return nil // Already following
	}
	m.subscribers[hexKey] = true
	m.mu.Unlock()

	if err := m.db.AddSubscription(pubkey, timestamp); err != nil {
		m.mu.Lock()
		delete(m.subscribers, hexKey)
		m.mu.Unlock()
		return fmt.Errorf("add subscription: %w", err)
	}

	if m.OnSubscribe != nil {
		if err := m.OnSubscribe(ctx, hexKey); err != nil {
			return fmt.Errorf("subscribe to topic: %w", err)
		}
	}

	return nil
}

// Unfollow removes a subscription to a publisher.
func (m *Manager) Unfollow(pubkey []byte) error {
	hexKey := hex.EncodeToString(pubkey)

	m.mu.Lock()
	if !m.subscribers[hexKey] {
		m.mu.Unlock()
		return nil // Not following
	}
	delete(m.subscribers, hexKey)
	m.mu.Unlock()

	if err := m.db.RemoveSubscription(pubkey); err != nil {
		m.mu.Lock()
		m.subscribers[hexKey] = true
		m.mu.Unlock()
		return fmt.Errorf("remove subscription: %w", err)
	}

	if m.OnUnsubscribe != nil {
		if err := m.OnUnsubscribe(hexKey); err != nil {
			return fmt.Errorf("unsubscribe from topic: %w", err)
		}
	}

	return nil
}

// IsFollowing checks if a publisher is currently followed.
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
