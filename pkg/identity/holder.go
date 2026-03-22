package identity

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Holder provides thread-safe access to the active identity key pair.
// All services share the same Holder so that identity changes (create, unlock, switch)
// are immediately visible everywhere.
type Holder struct {
	mu      sync.RWMutex
	kp      *KeyPair
	dataDir string
}

// NewHolder creates a new identity Holder.
func NewHolder(dataDir string) *Holder {
	return &Holder{
		dataDir: dataDir,
	}
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

// HasIdentity checks if an encrypted key file exists on disk.
func (h *Holder) HasIdentity() bool {
	keyPath := filepath.Join(h.dataDir, "identity", "primary.key")
	_, err := os.Stat(keyPath)
	return err == nil
}

// CreateAndSave generates a new identity, encrypts it, and saves to disk.
// Returns the key pair and mnemonic seed phrase.
func (h *Holder) CreateAndSave(passphrase string) (*KeyPair, string, error) {
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

	// Encrypt and save to disk.
	enc, err := EncryptPrivateKey(kp.PrivateKey, passphrase)
	if err != nil {
		return nil, "", fmt.Errorf("failed to encrypt key: %w", err)
	}

	keyPath := filepath.Join(h.dataDir, "identity", "primary.key")
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o755); err != nil {
		return nil, "", fmt.Errorf("failed to create identity directory: %w", err)
	}
	if err := SaveEncryptedKey(enc, keyPath); err != nil {
		return nil, "", fmt.Errorf("failed to save key: %w", err)
	}

	h.mu.Lock()
	h.kp = kp
	h.mu.Unlock()

	return kp, mnemonic, nil
}

// ImportAndSave imports an identity from a mnemonic, encrypts it, and saves to disk.
func (h *Holder) ImportAndSave(mnemonic, passphrase string) (*KeyPair, error) {
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

	enc, err := EncryptPrivateKey(kp.PrivateKey, passphrase)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt key: %w", err)
	}

	keyPath := filepath.Join(h.dataDir, "identity", "primary.key")
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create identity directory: %w", err)
	}
	if err := SaveEncryptedKey(enc, keyPath); err != nil {
		return nil, fmt.Errorf("failed to save key: %w", err)
	}

	h.mu.Lock()
	h.kp = kp
	h.mu.Unlock()

	return kp, nil
}

// Unlock decrypts the stored identity with the given passphrase.
func (h *Holder) Unlock(passphrase string) (*KeyPair, error) {
	keyPath := filepath.Join(h.dataDir, "identity", "primary.key")
	enc, err := LoadEncryptedKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load key: %w", err)
	}

	privKey, err := DecryptPrivateKey(enc, passphrase)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt key: %w", err)
	}

	kp := KeyPairFromPrivateKey(privKey)

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
