package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"fmt"

	"filippo.io/edwards25519"
	"golang.org/x/crypto/nacl/box"
)

// Ed25519PrivateKeyToX25519 converts an ed25519 private key to an X25519 private key.
// It follows RFC 7748: SHA-512 the ed25519 seed (first 32 bytes), then clamp the
// first 32 bytes of the hash to produce the X25519 scalar.
func Ed25519PrivateKeyToX25519(privKey ed25519.PrivateKey) [32]byte {
	// The ed25519 private key is seed || public key (64 bytes).
	// The seed is the first 32 bytes.
	seed := privKey.Seed()

	// SHA-512 the seed, as specified by RFC 8032 / RFC 7748.
	h := sha512.Sum512(seed)

	// Clamp the first 32 bytes per RFC 7748.
	h[0] &= 248
	h[31] &= 127
	h[31] |= 64

	var x25519Key [32]byte
	copy(x25519Key[:], h[:32])
	return x25519Key
}

// Ed25519PublicKeyToX25519 converts an ed25519 public key to an X25519 public key.
// It uses the birational map from the Ed25519 Edwards curve point to the Montgomery
// form (Curve25519) using filippo.io/edwards25519.
func Ed25519PublicKeyToX25519(pubKey ed25519.PublicKey) ([32]byte, error) {
	var x25519Key [32]byte

	// Parse the ed25519 public key as an Edwards25519 point.
	point, err := new(edwards25519.Point).SetBytes(pubKey)
	if err != nil {
		return x25519Key, fmt.Errorf("failed to parse ed25519 public key: %w", err)
	}

	// Convert to Montgomery form (X25519 / Curve25519).
	montBytes := point.BytesMontgomery()
	copy(x25519Key[:], montBytes)

	return x25519Key, nil
}

// EncryptDM encrypts a direct message from sender to recipient using NaCl box
// (X25519 key agreement + XSalsa20-Poly1305).
//
// The ed25519 keys are converted to X25519 for the Diffie-Hellman exchange.
// A random 24-byte nonce is generated and returned alongside the ciphertext.
func EncryptDM(senderPrivKey ed25519.PrivateKey, recipientPubKey ed25519.PublicKey, plaintext []byte) (ciphertext []byte, nonce [24]byte, err error) {
	// Convert keys to X25519.
	senderX25519 := Ed25519PrivateKeyToX25519(senderPrivKey)
	recipientX25519, err := Ed25519PublicKeyToX25519(recipientPubKey)
	if err != nil {
		return nil, nonce, fmt.Errorf("failed to convert recipient public key: %w", err)
	}

	// Generate a random 24-byte nonce.
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, nonce, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt with NaCl box.
	ciphertext = box.Seal(nil, plaintext, &nonce, &recipientX25519, &senderX25519)

	return ciphertext, nonce, nil
}

// DecryptDM decrypts a direct message from sender to recipient using NaCl box
// (X25519 key agreement + XSalsa20-Poly1305).
//
// The ed25519 keys are converted to X25519 for the Diffie-Hellman exchange.
func DecryptDM(recipientPrivKey ed25519.PrivateKey, senderPubKey ed25519.PublicKey, ciphertext []byte, nonce [24]byte) ([]byte, error) {
	// Convert keys to X25519.
	recipientX25519 := Ed25519PrivateKeyToX25519(recipientPrivKey)
	senderX25519, err := Ed25519PublicKeyToX25519(senderPubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert sender public key: %w", err)
	}

	// Decrypt with NaCl box.
	plaintext, ok := box.Open(nil, ciphertext, &nonce, &senderX25519, &recipientX25519)
	if !ok {
		return nil, fmt.Errorf("decryption failed: invalid ciphertext or keys")
	}

	return plaintext, nil
}
