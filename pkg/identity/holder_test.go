package identity

import (
	"encoding/hex"
	"os"
	"testing"
)

func TestWriteActivePubkeyHexUsesOwnerOnlyPermissions(t *testing.T) {
	holder := NewHolder(t.TempDir())

	if err := holder.writeActivePubkeyHex("abc123"); err != nil {
		t.Fatalf("writeActivePubkeyHex() error = %v", err)
	}

	info, err := os.Stat(holder.activeFilePath())
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("active file permissions = %o, want 600", perm)
	}
}

func TestCreateAndSaveCreatesOwnerOnlyIdentityArtifacts(t *testing.T) {
	holder := NewHolder(t.TempDir())

	kp, _, err := holder.CreateAndSave("passphrase")
	if err != nil {
		t.Fatalf("CreateAndSave() error = %v", err)
	}

	keyPath := holder.keyFilePath(hex.EncodeToString(kp.PublicKeyBytes()))
	keyInfo, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("Stat key file error = %v", err)
	}
	if perm := keyInfo.Mode().Perm(); perm != 0o600 {
		t.Fatalf("key file permissions = %o, want 600", perm)
	}

	keysDirInfo, err := os.Stat(holder.keysDir())
	if err != nil {
		t.Fatalf("Stat keys dir error = %v", err)
	}
	if perm := keysDirInfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("keys dir permissions = %o, want 700", perm)
	}

	activeInfo, err := os.Stat(holder.activeFilePath())
	if err != nil {
		t.Fatalf("Stat active file error = %v", err)
	}
	if perm := activeInfo.Mode().Perm(); perm != 0o600 {
		t.Fatalf("active file permissions = %o, want 600", perm)
	}
}

func TestUnlockMigratedLegacyKeyRenamesPlaceholderAndUpdatesActive(t *testing.T) {
	holder := NewHolder(t.TempDir())

	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	enc, err := EncryptPrivateKey(kp.PrivateKey, "passphrase")
	if err != nil {
		t.Fatalf("EncryptPrivateKey() error = %v", err)
	}
	if err := SaveEncryptedKey(enc, holder.legacyKeyPath()); err != nil {
		t.Fatalf("SaveEncryptedKey() error = %v", err)
	}

	unlocked, err := holder.Unlock("passphrase")
	if err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}
	if unlocked == nil {
		t.Fatal("Unlock() returned nil key pair")
	}

	actualPubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())
	if got := hex.EncodeToString(unlocked.PublicKeyBytes()); got != actualPubkeyHex {
		t.Fatalf("unlocked pubkey = %s, want %s", got, actualPubkeyHex)
	}

	if _, err := os.Stat(holder.keyFilePath(actualPubkeyHex)); err != nil {
		t.Fatalf("Stat actual key file error = %v", err)
	}
	if _, err := os.Stat(holder.keyFilePath("legacy")); !os.IsNotExist(err) {
		t.Fatalf("legacy placeholder key should be removed, stat err = %v", err)
	}

	activePubkey, err := os.ReadFile(holder.activeFilePath())
	if err != nil {
		t.Fatalf("ReadFile(active) error = %v", err)
	}
	if got := string(activePubkey); got != actualPubkeyHex {
		t.Fatalf("active pubkey = %s, want %s", got, actualPubkeyHex)
	}
}

func TestUnlockMigratedLegacyKeyDoesNotOverwriteConflictingDestination(t *testing.T) {
	holder := NewHolder(t.TempDir())

	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	legacyEnc, err := EncryptPrivateKey(kp.PrivateKey, "legacy-pass")
	if err != nil {
		t.Fatalf("EncryptPrivateKey(legacy) error = %v", err)
	}
	if err := SaveEncryptedKey(legacyEnc, holder.keyFilePath("legacy")); err != nil {
		t.Fatalf("SaveEncryptedKey(legacy placeholder) error = %v", err)
	}
	if err := holder.writeActivePubkeyHex("legacy"); err != nil {
		t.Fatalf("writeActivePubkeyHex() error = %v", err)
	}

	conflictingEnc, err := EncryptPrivateKey(kp.PrivateKey, "other-pass")
	if err != nil {
		t.Fatalf("EncryptPrivateKey(conflicting) error = %v", err)
	}
	actualPath := holder.keyFilePath(hex.EncodeToString(kp.PublicKeyBytes()))
	if err := SaveEncryptedKey(conflictingEnc, actualPath); err != nil {
		t.Fatalf("SaveEncryptedKey(actual) error = %v", err)
	}
	originalActualData, err := os.ReadFile(actualPath)
	if err != nil {
		t.Fatalf("ReadFile(actual before unlock) error = %v", err)
	}

	if _, err := holder.Unlock("legacy-pass"); err == nil {
		t.Fatal("Unlock() should fail when migrated key would overwrite a different destination file")
	}
	if holder.Get() != nil {
		t.Fatal("holder should remain locked after failed unlock finalization")
	}

	actualDataAfter, err := os.ReadFile(actualPath)
	if err != nil {
		t.Fatalf("ReadFile(actual after unlock) error = %v", err)
	}
	if string(actualDataAfter) != string(originalActualData) {
		t.Fatal("existing destination key file was modified")
	}

	if _, err := os.Stat(holder.keyFilePath("legacy")); err != nil {
		t.Fatalf("legacy placeholder key should remain after failed finalization, stat err = %v", err)
	}
	activePubkey, err := os.ReadFile(holder.activeFilePath())
	if err != nil {
		t.Fatalf("ReadFile(active) error = %v", err)
	}
	if got := string(activePubkey); got != "legacy" {
		t.Fatalf("active pubkey = %s, want legacy", got)
	}
}

func TestUnlockMigratedLegacyKeyRemovesIdenticalDuplicateDestination(t *testing.T) {
	holder := NewHolder(t.TempDir())

	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	enc, err := EncryptPrivateKey(kp.PrivateKey, "passphrase")
	if err != nil {
		t.Fatalf("EncryptPrivateKey() error = %v", err)
	}
	if err := SaveEncryptedKey(enc, holder.keyFilePath("legacy")); err != nil {
		t.Fatalf("SaveEncryptedKey(legacy placeholder) error = %v", err)
	}
	if err := holder.writeActivePubkeyHex("legacy"); err != nil {
		t.Fatalf("writeActivePubkeyHex() error = %v", err)
	}

	actualPubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())
	actualPath := holder.keyFilePath(actualPubkeyHex)
	if err := SaveEncryptedKey(enc, actualPath); err != nil {
		t.Fatalf("SaveEncryptedKey(actual) error = %v", err)
	}

	if _, err := holder.Unlock("passphrase"); err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}

	if _, err := os.Stat(actualPath); err != nil {
		t.Fatalf("actual key file should remain, stat err = %v", err)
	}
	if _, err := os.Stat(holder.keyFilePath("legacy")); !os.IsNotExist(err) {
		t.Fatalf("legacy placeholder key should be removed, stat err = %v", err)
	}

	activePubkey, err := os.ReadFile(holder.activeFilePath())
	if err != nil {
		t.Fatalf("ReadFile(active) error = %v", err)
	}
	if got := string(activePubkey); got != actualPubkeyHex {
		t.Fatalf("active pubkey = %s, want %s", got, actualPubkeyHex)
	}
}
