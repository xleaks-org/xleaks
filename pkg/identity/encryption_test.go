package identity

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptDM(t *testing.T) {
	sender, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() sender error: %v", err)
	}

	recipient, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() recipient error: %v", err)
	}

	plaintext := []byte("Hello, this is a secret message for XLeaks!")

	ciphertext, nonce, err := EncryptDM(sender.PrivateKey, recipient.PublicKey, plaintext)
	if err != nil {
		t.Fatalf("EncryptDM() error: %v", err)
	}

	if bytes.Equal(ciphertext, plaintext) {
		t.Error("ciphertext should not equal plaintext")
	}

	decrypted, err := DecryptDM(recipient.PrivateKey, sender.PublicKey, ciphertext, nonce)
	if err != nil {
		t.Fatalf("DecryptDM() error: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("decrypted text does not match: got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptDMWrongRecipient(t *testing.T) {
	sender, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() sender error: %v", err)
	}

	recipient, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() recipient error: %v", err)
	}

	wrongRecipient, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() wrongRecipient error: %v", err)
	}

	plaintext := []byte("secret message")
	ciphertext, nonce, err := EncryptDM(sender.PrivateKey, recipient.PublicKey, plaintext)
	if err != nil {
		t.Fatalf("EncryptDM() error: %v", err)
	}

	// Try to decrypt with the wrong recipient's key.
	_, err = DecryptDM(wrongRecipient.PrivateKey, sender.PublicKey, ciphertext, nonce)
	if err == nil {
		t.Error("expected error when decrypting with wrong recipient key")
	}
}

func TestEncryptDMEmptyMessage(t *testing.T) {
	sender, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() sender error: %v", err)
	}

	recipient, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() recipient error: %v", err)
	}

	ciphertext, nonce, err := EncryptDM(sender.PrivateKey, recipient.PublicKey, []byte{})
	if err != nil {
		t.Fatalf("EncryptDM() error: %v", err)
	}

	decrypted, err := DecryptDM(recipient.PrivateKey, sender.PublicKey, ciphertext, nonce)
	if err != nil {
		t.Fatalf("DecryptDM() error: %v", err)
	}

	if len(decrypted) != 0 {
		t.Errorf("expected empty decrypted message, got %d bytes", len(decrypted))
	}
}

func TestEd25519ToX25519Deterministic(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}

	x1 := Ed25519PrivateKeyToX25519(kp.PrivateKey)
	x2 := Ed25519PrivateKeyToX25519(kp.PrivateKey)
	if x1 != x2 {
		t.Error("X25519 private key conversion should be deterministic")
	}

	pub1, err := Ed25519PublicKeyToX25519(kp.PublicKey)
	if err != nil {
		t.Fatalf("Ed25519PublicKeyToX25519() error: %v", err)
	}
	pub2, err := Ed25519PublicKeyToX25519(kp.PublicKey)
	if err != nil {
		t.Fatalf("Ed25519PublicKeyToX25519() second call error: %v", err)
	}
	if pub1 != pub2 {
		t.Error("X25519 public key conversion should be deterministic")
	}
}

func TestFullMnemonicToEncryptedDMRoundTrip(t *testing.T) {
	// Generate mnemonic -> seed -> key pair -> encrypt DM -> decrypt DM.
	mnemonic, err := GenerateMnemonic()
	if err != nil {
		t.Fatalf("GenerateMnemonic() error: %v", err)
	}

	seed, err := MnemonicToSeed(mnemonic, "integration-test")
	if err != nil {
		t.Fatalf("MnemonicToSeed() error: %v", err)
	}

	sender, err := KeyPairFromSeed(seed)
	if err != nil {
		t.Fatalf("KeyPairFromSeed() error: %v", err)
	}

	recipient, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}

	msg := []byte("end-to-end test message")
	ct, nonce, err := EncryptDM(sender.PrivateKey, recipient.PublicKey, msg)
	if err != nil {
		t.Fatalf("EncryptDM() error: %v", err)
	}

	pt, err := DecryptDM(recipient.PrivateKey, sender.PublicKey, ct, nonce)
	if err != nil {
		t.Fatalf("DecryptDM() error: %v", err)
	}

	if !bytes.Equal(msg, pt) {
		t.Error("full round-trip failed: decrypted message does not match original")
	}
}
