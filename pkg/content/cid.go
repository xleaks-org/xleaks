package content

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"

	mh "github.com/multiformats/go-multihash"
)

// ComputeCID computes a SHA-256 multihash CID of the given raw data.
func ComputeCID(data []byte) ([]byte, error) {
	hash, err := mh.Sum(data, mh.SHA2_256, -1)
	if err != nil {
		return nil, fmt.Errorf("failed to compute multihash: %w", err)
	}
	return []byte(hash), nil
}

// ComputeCIDReader computes a SHA-256 multihash CID of the data read from r.
func ComputeCIDReader(r io.Reader) ([]byte, error) {
	hash, err := mh.SumStream(r, mh.SHA2_256, -1)
	if err != nil {
		return nil, fmt.Errorf("failed to compute multihash: %w", err)
	}
	return []byte(hash), nil
}

// CIDToHex returns the hex-encoded string representation of a CID.
func CIDToHex(cid []byte) string {
	return hex.EncodeToString(cid)
}

// HexToCID decodes a hex-encoded string into CID bytes.
func HexToCID(h string) ([]byte, error) {
	b, err := hex.DecodeString(h)
	if err != nil {
		return nil, fmt.Errorf("invalid hex string: %w", err)
	}
	// Validate that the decoded bytes form a valid multihash.
	if _, err := mh.Decode(b); err != nil {
		return nil, fmt.Errorf("decoded bytes are not a valid multihash: %w", err)
	}
	return b, nil
}

// ValidateCID checks whether the given CID matches the SHA-256 multihash of data.
func ValidateCID(cid []byte, data []byte) bool {
	computed, err := ComputeCID(data)
	if err != nil {
		return false
	}
	return bytes.Equal(cid, computed)
}
