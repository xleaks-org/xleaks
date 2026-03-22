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

// keysDir returns the path to the keys directory.
func (h *Holder) keysDir() string {
	return filepath.Join(h.dataDir, "identity", "keys")
}

// activeFilePath returns the path to the active identity file.
func (h *Holder) activeFilePath() string {
	return filepath.Join(h.dataDir, "identity", "active")
}

// keyFilePath returns the path to a specific identity's key file.
func (h *Holder) keyFilePath(pubkeyHex string) string {
	return filepath.Join(h.keysDir(), pubkeyHex+".key")
}

// legacyKeyPath returns the path to the old primary.key file.
func (h *Holder) legacyKeyPath() string {
	return filepath.Join(h.dataDir, "identity", "primary.key")
}

// migrateIfNeeded checks for legacy primary.key and migrates it to the new
// keys/ directory layout. If the key can't be identified (no active file, no DB),
// it moves it based on the filename but the user will need to unlock to register
// it in the DB.
func (h *Holder) migrateIfNeeded() error {
	legacyPath := h.legacyKeyPath()
	if _, err := os.Stat(legacyPath); os.IsNotExist(err) {
		return nil // Nothing to migrate.
	}

	// Load the legacy key to determine its pubkey. We can't decrypt it without
	// a passphrase, so we just move the file. If there's an active identity in
	// memory, use its pubkey hex. Otherwise, read the active file.
	var pubkeyHex string

	h.mu.RLock()
	if h.kp != nil {
		pubkeyHex = hex.EncodeToString(h.kp.PublicKeyBytes())
	}
	h.mu.RUnlock()

	if pubkeyHex == "" {
		// Try reading the active file.
		data, err := os.ReadFile(h.activeFilePath())
		if err == nil && len(data) > 0 {
			pubkeyHex = string(data)
		}
	}

	if pubkeyHex == "" {
		// Can't determine pubkey; use a placeholder name. The user will need to
		// unlock to properly register this identity.
		pubkeyHex = "legacy"
	}

	// Ensure keys directory exists.
	if err := os.MkdirAll(h.keysDir(), 0o755); err != nil {
		return fmt.Errorf("create keys directory: %w", err)
	}

	newPath := h.keyFilePath(pubkeyHex)
	// Only move if the destination doesn't already exist.
	if _, err := os.Stat(newPath); os.IsNotExist(err) {
		data, err := os.ReadFile(legacyPath)
		if err != nil {
			return fmt.Errorf("read legacy key: %w", err)
		}
		if err := os.WriteFile(newPath, data, 0o600); err != nil {
			return fmt.Errorf("write migrated key: %w", err)
		}
	}

	// Remove legacy file after successful migration.
	_ = os.Remove(legacyPath)

	// Write active file if it doesn't exist and we have a pubkey.
	if pubkeyHex != "legacy" {
		if _, err := os.Stat(h.activeFilePath()); os.IsNotExist(err) {
			_ = os.WriteFile(h.activeFilePath(), []byte(pubkeyHex), 0o644)
		}
	}

	return nil
}

// readActivePubkeyHex reads the active identity's pubkey hex from the active file.
func (h *Holder) readActivePubkeyHex() (string, error) {
	data, err := os.ReadFile(h.activeFilePath())
	if err != nil {
		return "", fmt.Errorf("read active file: %w", err)
	}
	pubkeyHex := string(data)
	if pubkeyHex == "" {
		return "", fmt.Errorf("active file is empty")
	}
	return pubkeyHex, nil
}

// writeActivePubkeyHex writes the active identity's pubkey hex to the active file.
func (h *Holder) writeActivePubkeyHex(pubkeyHex string) error {
	identityDir := filepath.Join(h.dataDir, "identity")
	if err := os.MkdirAll(identityDir, 0o755); err != nil {
		return fmt.Errorf("create identity directory: %w", err)
	}
	return os.WriteFile(h.activeFilePath(), []byte(pubkeyHex), 0o644)
}

// saveKeyAndRegister encrypts a key pair, saves it to keys/<pubkey>.key,
// registers it in the DB, and sets it as active if it's the first identity.
func (h *Holder) saveKeyAndRegister(kp *KeyPair, passphrase string) error {
	enc, err := EncryptPrivateKey(kp.PrivateKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to encrypt key: %w", err)
	}

	pubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())

	// Ensure keys directory exists.
	if err := os.MkdirAll(h.keysDir(), 0o755); err != nil {
		return fmt.Errorf("failed to create keys directory: %w", err)
	}

	keyPath := h.keyFilePath(pubkeyHex)
	if err := SaveEncryptedKey(enc, keyPath); err != nil {
		return fmt.Errorf("failed to save key: %w", err)
	}

	// Determine if this should be the active identity.
	isFirst := true
	if h.db != nil {
		count, err := h.db.CountIdentities()
		if err == nil && count > 0 {
			isFirst = false
		}
	}

	// Register in DB.
	if h.db != nil {
		now := time.Now().UnixMilli()
		if err := h.db.InsertIdentity(kp.PublicKeyBytes(), "Anonymous", isFirst, now); err != nil {
			// Non-fatal: key is saved on disk.
			_ = err
		}
		if isFirst {
			_ = h.db.SetActiveIdentity(kp.PublicKeyBytes())
		}
	}

	// Set as active identity (always for first, or if explicitly the active one).
	if isFirst {
		if err := h.writeActivePubkeyHex(pubkeyHex); err != nil {
			return fmt.Errorf("failed to write active file: %w", err)
		}
	}

	return nil
}

// CreateAndSave generates a new identity, encrypts it, and saves to disk.
// Returns the key pair and mnemonic seed phrase.
func (h *Holder) CreateAndSave(passphrase string) (*KeyPair, string, error) {
	_ = h.migrateIfNeeded()

	mnemonic, err := GenerateMnemonic()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate mnemonic: %w", err)
	}

	seed, err := MnemonicToSeed(mnemonic, "")
	if err != nil {
		return nil, "", fmt.Errorf("failed to derive seed: %w", err)
	}

	kp, err := KeyPairFromSeed(seed)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate key pair: %w", err)
	}

	if err := h.saveKeyAndRegister(kp, passphrase); err != nil {
		return nil, "", err
	}

	// Set as the in-memory active identity.
	pubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())
	_ = h.writeActivePubkeyHex(pubkeyHex)
	if h.db != nil {
		_ = h.db.SetActiveIdentity(kp.PublicKeyBytes())
	}

	h.mu.Lock()
	h.kp = kp
	h.mu.Unlock()

	return kp, mnemonic, nil
}

// ImportAndSave imports an identity from a mnemonic, encrypts it, and saves to disk.
func (h *Holder) ImportAndSave(mnemonic, passphrase string) (*KeyPair, error) {
	_ = h.migrateIfNeeded()

	if !ValidateMnemonic(mnemonic) {
		return nil, fmt.Errorf("invalid mnemonic")
	}

	seed, err := MnemonicToSeed(mnemonic, "")
	if err != nil {
		return nil, fmt.Errorf("failed to derive seed: %w", err)
	}

	kp, err := KeyPairFromSeed(seed)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	if err := h.saveKeyAndRegister(kp, passphrase); err != nil {
		return nil, err
	}

	// Set as the in-memory active identity.
	pubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())
	_ = h.writeActivePubkeyHex(pubkeyHex)
	if h.db != nil {
		_ = h.db.SetActiveIdentity(kp.PublicKeyBytes())
	}

	h.mu.Lock()
	h.kp = kp
	h.mu.Unlock()

	return kp, nil
}

// Unlock decrypts the active identity's stored key with the given passphrase.
func (h *Holder) Unlock(passphrase string) (*KeyPair, error) {
	_ = h.migrateIfNeeded()

	// Determine which key file to load.
	pubkeyHex, err := h.readActivePubkeyHex()
	if err != nil {
		// Fall back: check if there's a legacy key or a single key in keys/.
		entries, readErr := os.ReadDir(h.keysDir())
		if readErr == nil && len(entries) == 1 {
			name := entries[0].Name()
			pubkeyHex = name[:len(name)-len(".key")]
		} else {
			return nil, fmt.Errorf("no active identity set: %w", err)
		}
	}

	keyPath := h.keyFilePath(pubkeyHex)
	enc, err := LoadEncryptedKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load key: %w", err)
	}

	privKey, err := DecryptPrivateKey(enc, passphrase)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt key: %w", err)
	}

	kp := KeyPairFromPrivateKey(privKey)

	// If the key was migrated from legacy with placeholder name, fix it now.
	actualPubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())
	if pubkeyHex != actualPubkeyHex {
		// Rename the key file to use the actual pubkey.
		newPath := h.keyFilePath(actualPubkeyHex)
		_ = os.Rename(keyPath, newPath)
		_ = h.writeActivePubkeyHex(actualPubkeyHex)
	}

	// Ensure the identity is registered in the DB.
	if h.db != nil {
		exists, checkErr := h.db.IdentityExists(kp.PublicKeyBytes())
		if checkErr == nil && !exists {
			now := time.Now().UnixMilli()
			_ = h.db.InsertIdentity(kp.PublicKeyBytes(), "Anonymous", true, now)
		}
		_ = h.db.SetActiveIdentity(kp.PublicKeyBytes())
	}

	h.mu.Lock()
	h.kp = kp
	h.mu.Unlock()

	return kp, nil
}

// Lock clears the in-memory key pair.
func (h *Holder) Lock() {
	h.mu.Lock()
	h.kp = nil
	h.mu.Unlock()
}

// ListIdentities returns information about all stored identities.
func (h *Holder) ListIdentities() ([]IdentityInfo, error) {
	if h.db == nil {
		// Fall back to just showing the active identity if available.
		h.mu.RLock()
		kp := h.kp
		h.mu.RUnlock()

		if kp == nil {
			return []IdentityInfo{}, nil
		}
		pubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())
		address, _ := PubKeyToAddress(kp.PublicKeyBytes())
		return []IdentityInfo{
			{
				PubkeyHex:   pubkeyHex,
				Address:     address,
				DisplayName: "Anonymous",
				IsActive:    true,
			},
		}, nil
	}

	rows, err := h.db.GetIdentities()
	if err != nil {
		return nil, fmt.Errorf("list identities: %w", err)
	}

	infos := make([]IdentityInfo, 0, len(rows))
	for _, row := range rows {
		pubkeyHex := hex.EncodeToString(row.Pubkey)
		address, _ := PubKeyToAddress(row.Pubkey)
		infos = append(infos, IdentityInfo{
			PubkeyHex:   pubkeyHex,
			Address:     address,
			DisplayName: row.DisplayName,
			IsActive:    row.IsActive,
			CreatedAt:   row.CreatedAt,
		})
	}

	return infos, nil
}

// SwitchIdentity switches to a different identity by pubkey hex, unlocking it
// with the provided passphrase.
func (h *Holder) SwitchIdentity(pubkeyHex, passphrase string) error {
	keyPath := h.keyFilePath(pubkeyHex)
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		return fmt.Errorf("identity key file not found for %s", pubkeyHex)
	}

	enc, err := LoadEncryptedKey(keyPath)
	if err != nil {
		return fmt.Errorf("failed to load key for %s: %w", pubkeyHex, err)
	}

	privKey, err := DecryptPrivateKey(enc, passphrase)
	if err != nil {
		return fmt.Errorf("failed to decrypt key: %w", err)
	}

	kp := KeyPairFromPrivateKey(privKey)

	// Update active file.
	if err := h.writeActivePubkeyHex(pubkeyHex); err != nil {
		return fmt.Errorf("failed to update active file: %w", err)
	}

	// Update DB.
	if h.db != nil {
		pubkeyBytes, decErr := hex.DecodeString(pubkeyHex)
		if decErr == nil {
			_ = h.db.SetActiveIdentity(pubkeyBytes)
		}
	}

	h.mu.Lock()
	h.kp = kp
	h.mu.Unlock()

	return nil
}
