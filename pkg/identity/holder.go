package identity

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/xleaks-org/xleaks/pkg/storage"
)

// DefaultDisplayName is the fallback name for users who haven't set a profile.
const DefaultDisplayName = "Anonymous"

// IdentityInfo contains display information for an identity.
type IdentityInfo struct {
	PubkeyHex   string `json:"pubkey"`
	Address     string `json:"address"`
	DisplayName string `json:"displayName"`
	IsActive    bool   `json:"isActive"`
	CreatedAt   int64  `json:"createdAt"`
}

// Holder provides thread-safe access to the active identity key pair.
// All services share the same Holder so that identity changes (create, unlock, switch)
// are immediately visible everywhere.
//
// Storage layout (multi-identity):
//
//	~/.xleaks/identity/
//	├── active              # Text file with pubkey hex of active identity
//	├── keys/
//	│   ├── <pubkey-hex>.key  # Each identity's encrypted key
type Holder struct {
	mu      sync.RWMutex
	kp      *KeyPair
	dataDir string
	db      *storage.DB
}

// NewHolder creates a new identity Holder.
func NewHolder(dataDir string) *Holder {
	return &Holder{
		dataDir: dataDir,
	}
}

// SetDB sets the database connection for identity management.
func (h *Holder) SetDB(db *storage.DB) {
	h.db = db
}

// Get returns the current active key pair. Returns nil if no identity is active.
func (h *Holder) Get() *KeyPair {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.kp
}

// Set updates the active key pair.
func (h *Holder) Set(kp *KeyPair) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.kp = kp
}

// IsUnlocked returns true if an identity is currently active.
func (h *Holder) IsUnlocked() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.kp != nil
}

// HasIdentity checks if any encrypted key file exists on disk.
// Supports both legacy (primary.key) and new (keys/*.key) layouts.
func (h *Holder) HasIdentity() bool {
	// Check new layout first.
	keysDir := filepath.Join(h.dataDir, "identity", "keys")
	entries, err := os.ReadDir(keysDir)
	if err == nil && len(entries) > 0 {
		return true
	}

	// Fall back to legacy layout.
	keyPath := filepath.Join(h.dataDir, "identity", "primary.key")
	_, err = os.Stat(keyPath)
	return err == nil
}

// Lock clears the in-memory key pair.
func (h *Holder) Lock() {
	h.mu.Lock()
	h.kp = nil
	h.mu.Unlock()
}

// Unlock decrypts the active identity's stored key with the given passphrase.
func (h *Holder) Unlock(passphrase string) (*KeyPair, error) {
	_ = h.migrateIfNeeded()

	pubkeyHex, err := h.resolveActivePubkey()
	if err != nil {
		return nil, err
	}

	keyPath := h.keyFilePath(pubkeyHex)
	enc, err := LoadEncryptedKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("load key: %w", err)
	}

	privKey, err := DecryptPrivateKey(enc, passphrase)
	if err != nil {
		return nil, fmt.Errorf("decrypt key: %w", err)
	}

	kp := KeyPairFromPrivateKey(privKey)
	h.fixMigratedKeyName(kp, pubkeyHex, keyPath)
	h.ensureIdentityRegistered(kp)

	h.mu.Lock()
	h.kp = kp
	h.mu.Unlock()

	return kp, nil
}

// resolveActivePubkey determines which pubkey hex to use when unlocking.
func (h *Holder) resolveActivePubkey() (string, error) {
	pubkeyHex, err := h.readActivePubkeyHex()
	if err == nil {
		return pubkeyHex, nil
	}

	// Fall back: check if there's a single key in keys/.
	entries, readErr := os.ReadDir(h.keysDir())
	if readErr == nil && len(entries) == 1 {
		name := entries[0].Name()
		return name[:len(name)-len(".key")], nil
	}
	return "", fmt.Errorf("no active identity set: %w", err)
}

// fixMigratedKeyName renames the key file if it was migrated with a placeholder name.
func (h *Holder) fixMigratedKeyName(kp *KeyPair, pubkeyHex, keyPath string) {
	actualPubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())
	if pubkeyHex != actualPubkeyHex {
		newPath := h.keyFilePath(actualPubkeyHex)
		_ = os.Rename(keyPath, newPath)
		_ = h.writeActivePubkeyHex(actualPubkeyHex)
	}
}

// ensureIdentityRegistered makes sure the identity exists in the DB.
func (h *Holder) ensureIdentityRegistered(kp *KeyPair) {
	if h.db == nil {
		return
	}
	exists, checkErr := h.db.IdentityExists(kp.PublicKeyBytes())
	if checkErr == nil && !exists {
		now := time.Now().UnixMilli()
		_ = h.db.InsertIdentity(kp.PublicKeyBytes(), DefaultDisplayName, true, now)
	}
	_ = h.db.SetActiveIdentity(kp.PublicKeyBytes())
}
