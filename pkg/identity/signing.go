package identity

import (
	"crypto/ed25519"
	"crypto/sha512"
	"fmt"
)

func Sign(privateKey ed25519.PrivateKey, message []byte) []byte {
	return ed25519.Sign(privateKey, message)
}

func Verify(publicKey ed25519.PublicKey, message []byte, signature []byte) bool {
	return ed25519.Verify(publicKey, message, signature)
}

func SignProtoMessage(kp *KeyPair, msg []byte) ([]byte, error) {
	if kp == nil {
		return nil, fmt.Errorf("key pair is nil")
	}
	if len(kp.PrivateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key size: got %d, want %d", len(kp.PrivateKey), ed25519.PrivateKeySize)
	}

	hash := sha512.Sum512_256(msg)
	signature := ed25519.Sign(kp.PrivateKey, hash[:])
	return signature, nil
}
