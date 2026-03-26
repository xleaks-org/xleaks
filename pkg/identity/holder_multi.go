package identity

import (
	"encoding/hex"
	"fmt"
	"os"
	"time"
)

// CreateAndSave generates a new identity, encrypts it, and saves to disk.
// Returns the key pair and mnemonic seed phrase.
func (h *Holder) CreateAndSave(passphrase string) (*KeyPair, string, error) {
	_ = h.migrateIfNeeded()

	mnemonic, err := GenerateMnemonic()
	if err != nil {
		return nil, "", fmt.Errorf("generate mnemonic: %w", err)
	}

	seed, err := MnemonicToSeed(mnemonic, "")
	if err != nil {
		return nil, "", fmt.Errorf("derive seed: %w", err)
	}

	kp, err := KeyPairFromSeed(seed)
	if err != nil {
		return nil, "", fmt.Errorf("generate key pair: %w", err)
	}

	if err := h.saveKeyAndRegister(kp, passphrase); err != nil {
		return nil, "", err
	}

	h.activateIdentity(kp)
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
		return nil, fmt.Errorf("derive seed: %w", err)
	}

	kp, err := KeyPairFromSeed(seed)
	if err != nil {
		return nil, fmt.Errorf("generate key pair: %w", err)
	}

	if err := h.saveKeyAndRegister(kp, passphrase); err != nil {
		return nil, err
	}

	h.activateIdentity(kp)
	return kp, nil
}

// activateIdentity sets a key pair as the in-memory active identity and
// writes the active pubkey to disk and DB.
func (h *Holder) activateIdentity(kp *KeyPair) {
	pubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())
	_ = h.writeActivePubkeyHex(pubkeyHex)
	if h.db != nil {
		_ = h.db.SetActiveIdentity(kp.PublicKeyBytes())
	}
	h.mu.Lock()
	h.kp = kp
	h.mu.Unlock()
}

// saveKeyAndRegister encrypts a key pair, saves it to keys/<pubkey>.key,
// registers it in the DB, and sets it as active if it's the first identity.
func (h *Holder) saveKeyAndRegister(kp *KeyPair, passphrase string) error {
	enc, err := EncryptPrivateKey(kp.PrivateKey, passphrase)
	if err != nil {
		return fmt.Errorf("encrypt key: %w", err)
	}
	pubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())
	if err := os.MkdirAll(h.keysDir(), 0o755); err != nil {
		return fmt.Errorf("create keys directory: %w", err)
	}

	keyPath := h.keyFilePath(pubkeyHex)
	if err := SaveEncryptedKey(enc, keyPath); err != nil {
		return fmt.Errorf("save key: %w", err)
	}

	h.registerIdentityInDB(kp)
	return nil
}

// registerIdentityInDB inserts the identity into the database and marks it
// active if it is the first identity.
func (h *Holder) registerIdentityInDB(kp *KeyPair) {
	if h.db == nil {
		return
	}
	isFirst := true
	count, err := h.db.CountIdentities()
	if err == nil && count > 0 {
		isFirst = false
	}
	now := time.Now().UnixMilli()
	_ = h.db.InsertIdentity(kp.PublicKeyBytes(), DefaultDisplayName, isFirst, now)
	if isFirst {
		_ = h.db.SetActiveIdentity(kp.PublicKeyBytes())
	}
}

// ListIdentities returns information about all stored identities.
func (h *Holder) ListIdentities() ([]IdentityInfo, error) {
	if h.db == nil {
		return h.listIdentitiesWithoutDB()
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

// listIdentitiesWithoutDB returns a single-entry list when no DB is available.
func (h *Holder) listIdentitiesWithoutDB() ([]IdentityInfo, error) {
	h.mu.RLock()
	kp := h.kp
	h.mu.RUnlock()
	if kp == nil {
		return []IdentityInfo{}, nil
	}
	pubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())
	address, _ := PubKeyToAddress(kp.PublicKeyBytes())
	return []IdentityInfo{{
		PubkeyHex:   pubkeyHex,
		Address:     address,
		DisplayName: DefaultDisplayName,
		IsActive:    true,
	}}, nil
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
		return fmt.Errorf("load key for %s: %w", pubkeyHex, err)
	}

	privKey, err := DecryptPrivateKey(enc, passphrase)
	if err != nil {
		return fmt.Errorf("decrypt key: %w", err)
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

// ExportIdentity loads the encrypted key material for the given identity.
func (h *Holder) ExportIdentity(pubkeyHex string) (*EncryptedKey, error) {
	keyPath := h.keyFilePath(pubkeyHex)
	enc, err := LoadEncryptedKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("load key for %s: %w", pubkeyHex, err)
	}
	return enc, nil
}

// ExportActiveIdentity loads the encrypted key material for the current active identity.
func (h *Holder) ExportActiveIdentity() (*EncryptedKey, string, error) {
	pubkeyHex, err := h.resolveActivePubkey()
	if err != nil {
		return nil, "", err
	}
	enc, err := h.ExportIdentity(pubkeyHex)
	if err != nil {
		return nil, "", err
	}
	return enc, pubkeyHex, nil
}
