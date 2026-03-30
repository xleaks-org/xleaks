package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

const defaultWSTicketTTL = time.Minute

// WSTicketManager issues short-lived, one-time tickets for authenticating
// browser WebSocket upgrades without exposing the shared API token in URLs.
type WSTicketManager struct {
	mu      sync.Mutex
	tickets map[string]time.Time
	ttl     time.Duration
	now     func() time.Time
}

// NewWSTicketManager creates a new WebSocket ticket manager.
func NewWSTicketManager(ttl time.Duration) *WSTicketManager {
	if ttl <= 0 {
		ttl = defaultWSTicketTTL
	}
	return &WSTicketManager{
		tickets: make(map[string]time.Time),
		ttl:     ttl,
		now:     time.Now,
	}
}

// Issue generates a new one-time WebSocket ticket.
func (m *WSTicketManager) Issue() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	ticket := hex.EncodeToString(b)
	now := m.now()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupExpiredLocked(now)
	m.tickets[ticket] = now.Add(m.ttl)
	return ticket, nil
}

// ValidateAndConsume reports whether the ticket is currently valid and marks it
// as used so it cannot be replayed.
func (m *WSTicketManager) ValidateAndConsume(ticket string) bool {
	if m == nil || ticket == "" {
		return false
	}

	now := m.now()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupExpiredLocked(now)

	expiresAt, ok := m.tickets[ticket]
	if !ok || !now.Before(expiresAt) {
		delete(m.tickets, ticket)
		return false
	}

	delete(m.tickets, ticket)
	return true
}

// IssueHandler returns a new WebSocket ticket as JSON.
func (m *WSTicketManager) IssueHandler(w http.ResponseWriter, _ *http.Request) {
	ticket, err := m.Issue()
	if err != nil {
		http.Error(w, "failed to issue websocket ticket", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ticket":             ticket,
		"expires_in_seconds": int(m.ttl / time.Second),
	})
}

func (m *WSTicketManager) cleanupExpiredLocked(now time.Time) {
	for ticket, expiresAt := range m.tickets {
		if !now.Before(expiresAt) {
			delete(m.tickets, ticket)
		}
	}
}
