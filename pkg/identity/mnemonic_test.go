package identity

import (
	"bytes"
	"strings"
	"testing"
)

func TestGenerateMnemonic(t *testing.T) {
	mnemonic, err := GenerateMnemonic()
	if err != nil {
		t.Fatalf("GenerateMnemonic() error: %v", err)
	}

	words := strings.Fields(mnemonic)
	if len(words) != 24 {
		t.Errorf("expected 24 words, got %d", len(words))
	}

	if !ValidateMnemonic(mnemonic) {
		t.Error("generated mnemonic failed validation")
	}
}

func TestMnemonicToSeed(t *testing.T) {
	mnemonic, err := GenerateMnemonic()
	if err != nil {
		t.Fatalf("GenerateMnemonic() error: %v", err)
	}

	seed, err := MnemonicToSeed(mnemonic, "")
	if err != nil {
		t.Fatalf("MnemonicToSeed() error: %v", err)
	}

	if len(seed) != 32 {
		t.Errorf("expected 32-byte seed, got %d bytes", len(seed))
	}

	// Same mnemonic + passphrase should produce the same seed.
	seed2, err := MnemonicToSeed(mnemonic, "")
	if err != nil {
		t.Fatalf("MnemonicToSeed() second call error: %v", err)
	}
	if !bytes.Equal(seed, seed2) {
		t.Error("same mnemonic should produce the same seed")
	}

	// Different passphrase should produce a different seed.
	seed3, err := MnemonicToSeed(mnemonic, "my-passphrase")
	if err != nil {
		t.Fatalf("MnemonicToSeed() with passphrase error: %v", err)
	}
	if bytes.Equal(seed, seed3) {
		t.Error("different passphrase should produce a different seed")
	}
}

func TestMnemonicToSeedInvalid(t *testing.T) {
	_, err := MnemonicToSeed("invalid mnemonic words here", "")
	if err == nil {
		t.Error("expected error for invalid mnemonic, got nil")
	}
}

func TestValidateMnemonic(t *testing.T) {
	if ValidateMnemonic("not a valid mnemonic") {
		t.Error("expected false for invalid mnemonic")
	}

	mnemonic, err := GenerateMnemonic()
	if err != nil {
		t.Fatalf("GenerateMnemonic() error: %v", err)
	}
	if !ValidateMnemonic(mnemonic) {
		t.Error("expected true for valid mnemonic")
	}

	// Test with leading/trailing whitespace.
	if !ValidateMnemonic("  " + mnemonic + "  ") {
		t.Error("expected true for valid mnemonic with whitespace")
	}
}

func TestMnemonicToKeyPairIntegration(t *testing.T) {
	mnemonic, err := GenerateMnemonic()
	if err != nil {
		t.Fatalf("GenerateMnemonic() error: %v", err)
	}

	seed, err := MnemonicToSeed(mnemonic, "test-passphrase")
	if err != nil {
		t.Fatalf("MnemonicToSeed() error: %v", err)
	}

	kp, err := KeyPairFromSeed(seed)
	if err != nil {
		t.Fatalf("KeyPairFromSeed() error: %v", err)
	}

	// Verify the key pair works for signing.
	msg := []byte("hello xleaks")
	sig := Sign(kp.PrivateKey, msg)
	if !Verify(kp.PublicKey, msg, sig) {
		t.Error("signature verification failed for mnemonic-derived key pair")
	}
}
