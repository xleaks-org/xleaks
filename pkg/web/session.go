package web

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/xleaks-org/xleaks/pkg/identity"
)

const (
	sessionCookieName = "xleaks_session"
	sessionMaxAge     = 24 * time.Hour
	sessionTokenLen   = 32
)

// UserSession represents an authenticated user session with their key pair.
type UserSession struct {
	Token     string
	KeyPair   *identity.KeyPair
	PubkeyHex string
	CreatedAt time.Time
	LastSeen  time.Time
}

// SessionManager manages in-memory user sessions mapped by token.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*UserSession
	done     chan struct{}
}

// NewSessionManager creates a new SessionManager and starts a background cleanup loop.
func NewSessionManager() *SessionManager {
	sm := &SessionManager{sessions: make(map[string]*UserSession), done: make(chan struct{})}
	go sm.cleanupLoop()
	return sm
}

// Create generates a new session for the given key pair and returns the token.
func (sm *SessionManager) Create(kp *identity.KeyPair) (string, error) {
	b := make([]byte, sessionTokenLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	sm.mu.Lock()
	sm.sessions[token] = &UserSession{
		Token:     token,
		KeyPair:   kp,
		PubkeyHex: hex.EncodeToString(kp.PublicKeyBytes()),
		CreatedAt: time.Now(),
		LastSeen:  time.Now(),
	}
	sm.mu.Unlock()
	return token, nil
}

// Get returns the session for the given token, or nil if not found.
// Updates the LastSeen timestamp on access.
func (sm *SessionManager) Get(token string) *UserSession {
	sm.mu.RLock()
	sess := sm.sessions[token]
	sm.mu.RUnlock()
	if sess != nil {
		sm.mu.Lock()
		sess.LastSeen = time.Now()
		sm.mu.Unlock()
	}
	return sess
}

// GetFromRequest extracts the session from the request's cookie.
func (sm *SessionManager) GetFromRequest(r *http.Request) *UserSession {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil
	}
	return sm.Get(cookie.Value)
}

// Destroy removes a session by token.
func (sm *SessionManager) Destroy(token string) {
	sm.mu.Lock()
	delete(sm.sessions, token)
	sm.mu.Unlock()
}

// SetCookie sets the session cookie on the response.
func (sm *SessionManager) SetCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionMaxAge.Seconds()),
		HttpOnly: true,
		Secure:   requestIsSecure(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearCookie removes the session cookie from the response.
func (sm *SessionManager) ClearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   requestIsSecure(r),
	})
}

// Stop signals the cleanup goroutine to exit.
func (sm *SessionManager) Stop() { close(sm.done) }

// cleanupLoop periodically removes expired sessions.
func (sm *SessionManager) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			sm.mu.Lock()
			now := time.Now()
			for token, sess := range sm.sessions {
				if now.Sub(sess.LastSeen) > sessionMaxAge {
					delete(sm.sessions, token)
				}
			}
			sm.mu.Unlock()
		case <-sm.done:
			return
		}
	}
}
