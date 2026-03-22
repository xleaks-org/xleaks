package identity

import (
	"bytes"
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptDecryptPrivateKey(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}

	passphrase := "strong-passphrase-123!"

	enc, err := EncryptPrivateKey(kp.PrivateKey, passphrase)
	if err != nil {
		t.Fatalf("EncryptPrivateKey() error: %v", err)
	}

	if len(enc.Salt) != 16 {
		t.Errorf("expected 16-byte salt, got %d", len(enc.Salt))
	}
	if enc.Argon2Params.Time != 3 {
		t.Errorf("expected time=3, got %d", enc.Argon2Params.Time)
	}
	if enc.Argon2Params.Memory != 64*1024 {
		t.Errorf("expected memory=65536, got %d", enc.Argon2Params.Memory)
	}
	if enc.Argon2Params.Threads != 4 {
		t.Errorf("expected threads=4, got %d", enc.Argon2Params.Threads)
	}

	decrypted, err := DecryptPrivateKey(enc, passphrase)
	if err != nil {
		t.Fatalf("DecryptPrivateKey() error: %v", err)
	}

	if !bytes.Equal(kp.PrivateKey, decrypted) {
		t.Error("decrypted key does not match original")
	}
}

func TestDecryptWrongPassphrase(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}

	enc, err := EncryptPrivateKey(kp.PrivateKey, "correct-passphrase")
	if err != nil {
		t.Fatalf("EncryptPrivateKey() error: %v", err)
	}

	_, err = DecryptPrivateKey(enc, "wrong-passphrase")
	if err == nil {
		t.Error("expected error when decrypting with wrong passphrase")
	}
}

func TestSaveLoadEncryptedKey(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}

	passphrase := "save-load-test"
	enc, err := EncryptPrivateKey(kp.PrivateKey, passphrase)
	if err != nil {
		t.Fatalf("EncryptPrivateKey() error: %v", err)
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "key.json")

	if err := SaveEncryptedKey(enc, path); err != nil {
		t.Fatalf("SaveEncryptedKey() error: %v", err)
	}

	// Verify file permissions.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat() error: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected file permissions 0600, got %o", perm)
	}

	loaded, err := LoadEncryptedKey(path)
	if err != nil {
		t.Fatalf("LoadEncryptedKey() error: %v", err)
	}

	// Decrypt the loaded key and verify it matches.
	decrypted, err := DecryptPrivateKey(loaded, passphrase)
	if err != nil {
		t.Fatalf("DecryptPrivateKey() after load error: %v", err)
	}

	if !bytes.Equal(kp.PrivateKey, decrypted) {
		t.Error("decrypted key after save/load does not match original")
	}
}

func TestLoadEncryptedKeyNotFound(t *testing.T) {
	_, err := LoadEncryptedKey("/nonexistent/path/key.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestEncryptPrivateKeyInvalidSize(t *testing.T) {
	_, err := EncryptPrivateKey(ed25519.PrivateKey([]byte("too-short")), "pass")
	if err == nil {
		t.Error("expected error for invalid private key size")
	}
}

func TestDecryptPrivateKeyNilInput(t *testing.T) {
	_, err := DecryptPrivateKey(nil, "pass")
	if err == nil {
		t.Error("expected error for nil encrypted key")
	}
}

func TestSaveEncryptedKeyNilInput(t *testing.T) {
	err := SaveEncryptedKey(nil, "/tmp/test.json")
	if err == nil {
		t.Error("expected error for nil encrypted key")
	}
}
