package content

import (
	"bytes"
	"fmt"
	"time"
)

const (
	// MaxContentLength is the maximum number of UTF-8 characters in a post.
	MaxContentLength = 5000

	// MaxMediaCIDs is the maximum number of media CIDs per post.
	MaxMediaCIDs = 10

	// MaxDisplayNameLength is the maximum number of UTF-8 characters in a display name.
	MaxDisplayNameLength = 50

	// MaxBioLength is the maximum number of UTF-8 characters in a bio.
	MaxBioLength = 500

	// MaxWebsiteLength is the maximum number of characters in a website URL.
	MaxWebsiteLength = 200

	// MaxFutureSkew is the maximum allowed clock skew into the future (5 minutes).
	MaxFutureSkew = 5 * time.Minute

	// DefaultMaxPastAge is the default maximum age for messages (30 days).
	DefaultMaxPastAge = 30 * 24 * time.Hour

	// Ed25519PublicKeySize is the expected size of an ed25519 public key.
	Ed25519PublicKeySize = 32

	// Ed25519SignatureSize is the expected size of an ed25519 signature.
	Ed25519SignatureSize = 64

	// NaClNonceSize is the expected size of a NaCl nonce.
	NaClNonceSize = 24
)

// SignatureVerifier is a function that verifies an ed25519 signature.
// It is defined as a function type to avoid circular imports with the identity package.
type SignatureVerifier func(pubkey, message, signature []byte) bool

// MaxPastAge controls how far in the past a message timestamp is allowed to be.
// It defaults to DefaultMaxPastAge (30 days). During historical sync, set
// HistoricalSyncMode to true to bypass this check.
var MaxPastAge = DefaultMaxPastAge

// HistoricalSyncMode disables the MaxPastAge check so that old messages can be
// accepted during historical synchronisation.
var HistoricalSyncMode bool

// validateTimestamp checks that a millisecond unix timestamp is not more than
// MaxFutureSkew (5 min) in the future and not more than MaxPastAge (30 days)
// in the past. The past-age check is skipped when HistoricalSyncMode is true.
func validateTimestamp(tsMillis uint64) error {
	ts := time.UnixMilli(int64(tsMillis))
	now := time.Now()

	if ts.After(now.Add(MaxFutureSkew)) {
		return fmt.Errorf("timestamp %v is more than %v in the future", ts, MaxFutureSkew)
	}

	if !HistoricalSyncMode && ts.Before(now.Add(-MaxPastAge)) {
		return fmt.Errorf("timestamp %v is more than %v in the past", ts, MaxPastAge)
	}

	return nil
}

// verifyCID checks that the given id matches the SHA-256 multihash of payload.
// It is a no-op when id is nil/empty.
func verifyCID(id, payload []byte) error {
	if len(id) == 0 {
		return nil
	}
	expectedCID, err := ComputeCID(payload)
	if err != nil {
		return fmt.Errorf("failed to compute CID: %w", err)
	}
	if !bytes.Equal(id, expectedCID) {
		return fmt.Errorf("id does not match content hash")
	}
	return nil
}
