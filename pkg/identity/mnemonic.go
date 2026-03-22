package identity

import (
	"crypto/sha512"
	"fmt"
	"strings"

	"github.com/tyler-smith/go-bip39"
	"golang.org/x/crypto/pbkdf2"
)

// GenerateMnemonic generates a 24-word BIP39 mnemonic from 256-bit entropy.
func GenerateMnemonic() (string, error) {
	entropy, err := bip39.NewEntropy(256)
	if err != nil {
		return "", fmt.Errorf("failed to generate entropy: %w", err)
	}

	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return "", fmt.Errorf("failed to generate mnemonic: %w", err)
	}

	return mnemonic, nil
}

// MnemonicToSeed derives a 32-byte ed25519 seed from a BIP39 mnemonic and passphrase.
// It uses PBKDF2 as specified in BIP39: the mnemonic is the password, "mnemonic"+passphrase
// is the salt, with 2048 iterations of HMAC-SHA512. The first 32 bytes of the output are
// returned as the ed25519 seed.
func MnemonicToSeed(mnemonic string, passphrase string) ([]byte, error) {
	mnemonic = strings.TrimSpace(mnemonic)
	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, fmt.Errorf("invalid mnemonic")
	}

	// BIP39 spec: PBKDF2(password=mnemonic, salt="mnemonic"+passphrase, iterations=2048, dkLen=64, prf=HMAC-SHA512)
	salt := "mnemonic" + passphrase
	derived := pbkdf2.Key([]byte(mnemonic), []byte(salt), 2048, 64, sha512.New)

	// Take the first 32 bytes as the ed25519 seed.
	seed := make([]byte, 32)
	copy(seed, derived[:32])

	return seed, nil
}

// ValidateMnemonic checks whether the given mnemonic is a valid BIP39 mnemonic.
func ValidateMnemonic(mnemonic string) bool {
	return bip39.IsMnemonicValid(strings.TrimSpace(mnemonic))
}
