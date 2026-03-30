package identity

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

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

	pubkeyHex := h.migrationPubkeyHex()

	// Ensure keys directory exists.
	if err := os.MkdirAll(h.keysDir(), 0o700); err != nil {
		return fmt.Errorf("create keys directory: %w", err)
	}

	newPath := h.keyFilePath(pubkeyHex)
	if err := h.copyLegacyKey(legacyPath, newPath); err != nil {
		return err
	}

	// Remove legacy file after successful migration.
	_ = os.Remove(legacyPath)

	// Write active file if it doesn't exist and we have a pubkey.
	if pubkeyHex != "legacy" {
		if _, err := os.Stat(h.activeFilePath()); os.IsNotExist(err) {
			_ = writeOwnerOnlyFile(h.activeFilePath(), []byte(pubkeyHex))
		}
	}

	return nil
}

// migrationPubkeyHex determines the pubkey hex for migration. It checks the
// in-memory key pair first, then the active file, and falls back to "legacy".
func (h *Holder) migrationPubkeyHex() string {
	h.mu.RLock()
	if h.kp != nil {
		pubkeyHex := hex.EncodeToString(h.kp.PublicKeyBytes())
		h.mu.RUnlock()
		return pubkeyHex
	}
	h.mu.RUnlock()

	// Try reading the active file.
	data, err := os.ReadFile(h.activeFilePath())
	if err == nil && len(data) > 0 {
		return string(data)
	}

	return "legacy"
}

// copyLegacyKey copies the legacy key to the new path if the destination does
// not already exist.
func (h *Holder) copyLegacyKey(legacyPath, newPath string) error {
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		return nil // Destination already exists.
	}
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		return fmt.Errorf("read legacy key: %w", err)
	}
	if err := writeOwnerOnlyFile(newPath, data); err != nil {
		return fmt.Errorf("write migrated key: %w", err)
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
	if err := os.MkdirAll(identityDir, 0o700); err != nil {
		return fmt.Errorf("create identity directory: %w", err)
	}
	return writeOwnerOnlyFile(h.activeFilePath(), []byte(pubkeyHex))
}
