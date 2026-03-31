package identity

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// MarshalExportIdentity renders the encrypted-key export payload and filename
// for a given identity pubkey.
func MarshalExportIdentity(pubkeyHex string, enc *EncryptedKey) ([]byte, string, error) {
	pubkeyBytes, err := hex.DecodeString(pubkeyHex)
	if err != nil {
		return nil, "", fmt.Errorf("decode pubkey: %w", err)
	}

	address, err := PubKeyToAddress(pubkeyBytes)
	if err != nil {
		return nil, "", fmt.Errorf("derive address: %w", err)
	}

	body, err := json.MarshalIndent(map[string]interface{}{
		"pubkey":        pubkeyHex,
		"address":       address,
		"encrypted_key": enc,
	}, "", "  ")
	if err != nil {
		return nil, "", fmt.Errorf("marshal export identity: %w", err)
	}

	return body, pubkeyHex + ".xleaks-key.json", nil
}
