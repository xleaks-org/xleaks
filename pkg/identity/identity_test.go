package identity

import (
	"bytes"
	"crypto/ed25519"
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}
	if len(kp.PrivateKey) != ed25519.PrivateKeySize {
		t.Errorf("private key size = %d, want %d", len(kp.PrivateKey), ed25519.PrivateKeySize)
	}
	if len(kp.PublicKey) != ed25519.PublicKeySize {
		t.Errorf("public key size = %d, want %d", len(kp.PublicKey), ed25519.PublicKeySize)
	}
}

func TestKeyPairFromSeed(t *testing.T) {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}

	kp1, err := KeyPairFromSeed(seed)
	if err != nil {
		t.Fatalf("KeyPairFromSeed() error: %v", err)
	}

	kp2, err := KeyPairFromSeed(seed)
	if err != nil {
		t.Fatalf("KeyPairFromSeed() error: %v", err)
	}

	if !bytes.Equal(kp1.PublicKey, kp2.PublicKey) {
		t.Error("same seed produced different public keys")
	}
	if !bytes.Equal(kp1.PrivateKey, kp2.PrivateKey) {
		t.Error("same seed produced different private keys")
	}
}

func TestKeyPairFromSeedInvalidLength(t *testing.T) {
	_, err := KeyPairFromSeed(make([]byte, 16))
	if err == nil {
		t.Error("KeyPairFromSeed() with 16-byte seed should return error")
	}
}

func TestKeyPairFromPrivateKey(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}

	kp2 := KeyPairFromPrivateKey(kp.PrivateKey)
	if !bytes.Equal(kp.PublicKey, kp2.PublicKey) {
		t.Error("KeyPairFromPrivateKey produced different public key")
	}
}

func TestPublicKeyBytes(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}

	pkb := kp.PublicKeyBytes()
	if len(pkb) != 32 {
		t.Errorf("PublicKeyBytes() length = %d, want 32", len(pkb))
	}
	if !bytes.Equal(pkb, []byte(kp.PublicKey)) {
		t.Error("PublicKeyBytes() returned different bytes than PublicKey")
	}
}

func TestSignAndVerify(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}

	message := []byte("hello xleaks")
	sig := Sign(kp.PrivateKey, message)

	if !Verify(kp.PublicKey, message, sig) {
		t.Error("Verify() returned false for valid signature")
	}

	if Verify(kp.PublicKey, []byte("tampered"), sig) {
		t.Error("Verify() returned true for tampered message")
	}

	otherKP, _ := GenerateKeyPair()
	if Verify(otherKP.PublicKey, message, sig) {
		t.Error("Verify() returned true for wrong public key")
	}
}

func TestSignProtoMessage(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}

	msg := []byte("serialized protobuf payload")
	sig, err := SignProtoMessage(kp, msg)
	if err != nil {
		t.Fatalf("SignProtoMessage() error: %v", err)
	}

	if len(sig) != ed25519.SignatureSize {
		t.Errorf("signature size = %d, want %d", len(sig), ed25519.SignatureSize)
	}
}

func TestSignProtoMessageNilKeyPair(t *testing.T) {
	_, err := SignProtoMessage(nil, []byte("test"))
	if err == nil {
		t.Error("SignProtoMessage() with nil key pair should return error")
	}
}

func TestPubKeyToAddressAndBack(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}

	addr, err := PubKeyToAddress(kp.PublicKeyBytes())
	if err != nil {
		t.Fatalf("PubKeyToAddress() error: %v", err)
	}

	if len(addr) < len(addressHRP) {
		t.Fatalf("address too short: %s", addr)
	}

	if addr[:len(addressHRP)] != addressHRP {
		t.Errorf("address prefix = %q, want %q", addr[:len(addressHRP)], addressHRP)
	}

	decoded, err := AddressToPubKey(addr)
	if err != nil {
		t.Fatalf("AddressToPubKey() error: %v", err)
	}

	if !bytes.Equal(decoded, kp.PublicKeyBytes()) {
		t.Error("round-trip encoding/decoding produced different public key")
	}
}

func TestPubKeyToAddressInvalidLength(t *testing.T) {
	_, err := PubKeyToAddress(make([]byte, 16))
	if err == nil {
		t.Error("PubKeyToAddress() with 16-byte key should return error")
	}
}

func TestAddressToPubKeyInvalidPrefix(t *testing.T) {
	kp, _ := GenerateKeyPair()
	addr, _ := PubKeyToAddress(kp.PublicKeyBytes())

	tampered := "wrong1" + addr[len(addressHRP):]
	_, err := AddressToPubKey(tampered)
	if err == nil {
		t.Error("AddressToPubKey() with wrong prefix should return error")
	}
}

func TestAddressDeterministic(t *testing.T) {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i + 42)
	}

	kp, _ := KeyPairFromSeed(seed)
	addr1, err := PubKeyToAddress(kp.PublicKeyBytes())
	if err != nil {
		t.Fatalf("PubKeyToAddress() error: %v", err)
	}

	addr2, err := PubKeyToAddress(kp.PublicKeyBytes())
	if err != nil {
		t.Fatalf("PubKeyToAddress() error: %v", err)
	}

	if addr1 != addr2 {
		t.Errorf("same public key produced different addresses: %s vs %s", addr1, addr2)
	}
}

func TestBech32InvalidChecksum(t *testing.T) {
	kp, _ := GenerateKeyPair()
	addr, _ := PubKeyToAddress(kp.PublicKeyBytes())

	corrupted := addr[:len(addr)-1] + "q"
	if corrupted == addr {
		corrupted = addr[:len(addr)-1] + "p"
	}

	_, err := AddressToPubKey(corrupted)
	if err == nil {
		t.Error("AddressToPubKey() with corrupted checksum should return error")
	}
}
