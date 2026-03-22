package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
)

type KeyPair struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
}

func GenerateKeyPair() (*KeyPair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ed25519 key pair: %w", err)
	}
	return &KeyPair{
		PrivateKey: priv,
		PublicKey:  pub,
	}, nil
}

func KeyPairFromSeed(seed []byte) (*KeyPair, error) {
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("seed must be %d bytes, got %d", ed25519.SeedSize, len(seed))
	}
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)
	return &KeyPair{
		PrivateKey: priv,
		PublicKey:  pub,
	}, nil
}

func KeyPairFromPrivateKey(privKey ed25519.PrivateKey) *KeyPair {
	pub := privKey.Public().(ed25519.PublicKey)
	return &KeyPair{
		PrivateKey: privKey,
		PublicKey:  pub,
	}
}

func (kp *KeyPair) PublicKeyBytes() []byte {
	return []byte(kp.PublicKey)
}
