package identity

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/crypto/argon2"
)

// Argon2Params holds the parameters for the Argon2id key derivation function.
type Argon2Params struct {
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"`
	Threads uint8  `json:"threads"`
}

// EncryptedKey holds all the data needed to decrypt a private key.
type EncryptedKey struct {
	Salt         []byte       `json:"salt"`
	Nonce        []byte       `json:"nonce"`
	Ciphertext   []byte       `json:"ciphertext"`
	Argon2Params Argon2Params `json:"argon2_params"`
}

// EncryptPrivateKey encrypts an ed25519 private key using Argon2id + AES-256-GCM.
//
// The passphrase is used with Argon2id (time=3, memory=64MB, threads=4) to derive
// a 32-byte encryption key. A random 16-byte salt is generated for Argon2, and AES-256-GCM
// handles authenticated encryption with a random nonce.
func EncryptPrivateKey(privateKey ed25519.PrivateKey, passphrase string) (*EncryptedKey, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key size: got %d, want %d", len(privateKey), ed25519.PrivateKeySize)
	}

	// Generate a random 16-byte salt for Argon2id.
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	params := Argon2Params{
		Time:    3,
		Memory:  64 * 1024, // 64 MB in KiB
		Threads: 4,
	}

	// Derive the encryption key using Argon2id.
	key := argon2.IDKey([]byte(passphrase), salt, params.Time, params.Memory, params.Threads, 32)

	// Create AES-256-GCM cipher.
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate a random nonce for GCM.
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the private key.
	ciphertext := gcm.Seal(nil, nonce, []byte(privateKey), nil)

	return &EncryptedKey{
		Salt:         salt,
		Nonce:        nonce,
		Ciphertext:   ciphertext,
		Argon2Params: params,
	}, nil
}

// DecryptPrivateKey decrypts an encrypted private key using the stored parameters and the passphrase.
func DecryptPrivateKey(enc *EncryptedKey, passphrase string) (ed25519.PrivateKey, error) {
	if enc == nil {
		return nil, fmt.Errorf("encrypted key is nil")
	}

	// Re-derive the encryption key using the stored salt and parameters.
	key := argon2.IDKey(
		[]byte(passphrase),
		enc.Salt,
		enc.Argon2Params.Time,
		enc.Argon2Params.Memory,
		enc.Argon2Params.Threads,
		32,
	)

	// Create AES-256-GCM cipher.
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt the private key.
	plaintext, err := gcm.Open(nil, enc.Nonce, enc.Ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed (wrong passphrase?): %w", err)
	}

	if len(plaintext) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("decrypted key has invalid size: got %d, want %d", len(plaintext), ed25519.PrivateKeySize)
	}

	return ed25519.PrivateKey(plaintext), nil
}

// SaveEncryptedKey serializes the encrypted key to JSON and writes it to the given path.
func SaveEncryptedKey(enc *EncryptedKey, path string) error {
	if enc == nil {
		return fmt.Errorf("encrypted key is nil")
	}

	data, err := json.MarshalIndent(enc, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal encrypted key: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write encrypted key to %s: %w", path, err)
	}

	return nil
}

// LoadEncryptedKey reads an encrypted key from the given path and deserializes it from JSON.
func LoadEncryptedKey(path string) (*EncryptedKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read encrypted key from %s: %w", path, err)
	}

	var enc EncryptedKey
	if err := json.Unmarshal(data, &enc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal encrypted key: %w", err)
	}

	return &enc, nil
}
