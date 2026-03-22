package content

import (
	"bytes"
	"fmt"
	"time"
	"unicode/utf8"

	pb "github.com/xleaks/xleaks/proto/gen"
	"google.golang.org/protobuf/proto"
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

// ValidatePost validates all rules for a Post message per the XLeaks specification.
func ValidatePost(post *pb.Post, verify SignatureVerifier) error {
	if post == nil {
		return fmt.Errorf("post is nil")
	}

	// Author must be a valid 32-byte public key.
	if len(post.Author) != Ed25519PublicKeySize {
		return fmt.Errorf("author must be %d bytes, got %d", Ed25519PublicKeySize, len(post.Author))
	}

	// Content max 5000 UTF-8 characters.
	if utf8.RuneCountInString(post.Content) > MaxContentLength {
		return fmt.Errorf("content exceeds %d characters", MaxContentLength)
	}

	// Content must not be empty unless media_cids or repost_of is non-empty.
	if post.Content == "" && len(post.MediaCids) == 0 && len(post.RepostOf) == 0 {
		return fmt.Errorf("content must not be empty unless media_cids or repost_of is set")
	}

	// media_cids max 10 items.
	if len(post.MediaCids) > MaxMediaCIDs {
		return fmt.Errorf("media_cids exceeds maximum of %d items", MaxMediaCIDs)
	}

	// reply_to and repost_of are mutually exclusive.
	if len(post.ReplyTo) > 0 && len(post.RepostOf) > 0 {
		return fmt.Errorf("reply_to and repost_of are mutually exclusive")
	}

	// Timestamp must not be too far in the future or too old.
	if err := validateTimestamp(post.Timestamp); err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}

	// Signature must be present and valid size.
	if len(post.Signature) != Ed25519SignatureSize {
		return fmt.Errorf("signature must be %d bytes, got %d", Ed25519SignatureSize, len(post.Signature))
	}

	// Compute the signing payload: serialized message with id and signature zeroed.
	sigPayload, err := postSigningPayload(post)
	if err != nil {
		return fmt.Errorf("failed to compute signing payload: %w", err)
	}

	// Verify signature.
	if verify != nil && !verify(post.Author, sigPayload, post.Signature) {
		return fmt.Errorf("invalid signature")
	}

	// Verify CID: id must equal the SHA-256 multihash of the signing payload.
	if len(post.Id) > 0 {
		expectedCID, err := ComputeCID(sigPayload)
		if err != nil {
			return fmt.Errorf("failed to compute CID: %w", err)
		}
		if !bytes.Equal(post.Id, expectedCID) {
			return fmt.Errorf("id does not match content hash")
		}
	}

	return nil
}

// ValidateReaction validates all rules for a Reaction message.
func ValidateReaction(reaction *pb.Reaction, verify SignatureVerifier) error {
	if reaction == nil {
		return fmt.Errorf("reaction is nil")
	}

	if len(reaction.Author) != Ed25519PublicKeySize {
		return fmt.Errorf("author must be %d bytes, got %d", Ed25519PublicKeySize, len(reaction.Author))
	}

	if len(reaction.Target) == 0 {
		return fmt.Errorf("target must not be empty")
	}

	// reaction_type must be "like" in v1.0.
	if reaction.ReactionType != "like" {
		return fmt.Errorf("reaction_type must be \"like\", got %q", reaction.ReactionType)
	}

	if err := validateTimestamp(reaction.Timestamp); err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}

	if len(reaction.Signature) != Ed25519SignatureSize {
		return fmt.Errorf("signature must be %d bytes, got %d", Ed25519SignatureSize, len(reaction.Signature))
	}

	sigPayload, err := reactionSigningPayload(reaction)
	if err != nil {
		return fmt.Errorf("failed to compute signing payload: %w", err)
	}

	if verify != nil && !verify(reaction.Author, sigPayload, reaction.Signature) {
		return fmt.Errorf("invalid signature")
	}

	if len(reaction.Id) > 0 {
		expectedCID, err := ComputeCID(sigPayload)
		if err != nil {
			return fmt.Errorf("failed to compute CID: %w", err)
		}
		if !bytes.Equal(reaction.Id, expectedCID) {
			return fmt.Errorf("id does not match content hash")
		}
	}

	return nil
}

// ValidateProfile validates all rules for a Profile message.
func ValidateProfile(profile *pb.Profile, verify SignatureVerifier) error {
	if profile == nil {
		return fmt.Errorf("profile is nil")
	}

	if len(profile.Author) != Ed25519PublicKeySize {
		return fmt.Errorf("author must be %d bytes, got %d", Ed25519PublicKeySize, len(profile.Author))
	}

	if profile.DisplayName == "" {
		return fmt.Errorf("display_name must not be empty")
	}

	if utf8.RuneCountInString(profile.DisplayName) > MaxDisplayNameLength {
		return fmt.Errorf("display_name exceeds %d characters", MaxDisplayNameLength)
	}

	if utf8.RuneCountInString(profile.Bio) > MaxBioLength {
		return fmt.Errorf("bio exceeds %d characters", MaxBioLength)
	}

	if len(profile.Website) > MaxWebsiteLength {
		return fmt.Errorf("website exceeds %d characters", MaxWebsiteLength)
	}

	if err := validateTimestamp(profile.Timestamp); err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}

	if len(profile.Signature) != Ed25519SignatureSize {
		return fmt.Errorf("signature must be %d bytes, got %d", Ed25519SignatureSize, len(profile.Signature))
	}

	sigPayload, err := profileSigningPayload(profile)
	if err != nil {
		return fmt.Errorf("failed to compute signing payload: %w", err)
	}

	if verify != nil && !verify(profile.Author, sigPayload, profile.Signature) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

// ValidateFollowEvent validates all rules for a FollowEvent message.
func ValidateFollowEvent(event *pb.FollowEvent, verify SignatureVerifier) error {
	if event == nil {
		return fmt.Errorf("follow event is nil")
	}

	if len(event.Author) != Ed25519PublicKeySize {
		return fmt.Errorf("author must be %d bytes, got %d", Ed25519PublicKeySize, len(event.Author))
	}

	if len(event.Target) != Ed25519PublicKeySize {
		return fmt.Errorf("target must be %d bytes, got %d", Ed25519PublicKeySize, len(event.Target))
	}

	// A user must not follow themselves.
	if bytes.Equal(event.Author, event.Target) {
		return fmt.Errorf("a user cannot follow themselves")
	}

	// Action must be "follow" or "unfollow".
	if event.Action != "follow" && event.Action != "unfollow" {
		return fmt.Errorf("action must be \"follow\" or \"unfollow\", got %q", event.Action)
	}

	if err := validateTimestamp(event.Timestamp); err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}

	if len(event.Signature) != Ed25519SignatureSize {
		return fmt.Errorf("signature must be %d bytes, got %d", Ed25519SignatureSize, len(event.Signature))
	}

	sigPayload, err := followEventSigningPayload(event)
	if err != nil {
		return fmt.Errorf("failed to compute signing payload: %w", err)
	}

	if verify != nil && !verify(event.Author, sigPayload, event.Signature) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

// ValidateDirectMessage validates all rules for a DirectMessage.
func ValidateDirectMessage(dm *pb.DirectMessage, verify SignatureVerifier) error {
	if dm == nil {
		return fmt.Errorf("direct message is nil")
	}

	if len(dm.Author) != Ed25519PublicKeySize {
		return fmt.Errorf("author must be %d bytes, got %d", Ed25519PublicKeySize, len(dm.Author))
	}

	if len(dm.Recipient) != Ed25519PublicKeySize {
		return fmt.Errorf("recipient must be %d bytes, got %d", Ed25519PublicKeySize, len(dm.Recipient))
	}

	// Author and recipient must not be the same.
	if bytes.Equal(dm.Author, dm.Recipient) {
		return fmt.Errorf("author and recipient must not be the same")
	}

	if len(dm.EncryptedContent) == 0 {
		return fmt.Errorf("encrypted_content must not be empty")
	}

	if len(dm.Nonce) != NaClNonceSize {
		return fmt.Errorf("nonce must be %d bytes, got %d", NaClNonceSize, len(dm.Nonce))
	}

	if err := validateTimestamp(dm.Timestamp); err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}

	if len(dm.Signature) != Ed25519SignatureSize {
		return fmt.Errorf("signature must be %d bytes, got %d", Ed25519SignatureSize, len(dm.Signature))
	}

	sigPayload, err := dmSigningPayload(dm)
	if err != nil {
		return fmt.Errorf("failed to compute signing payload: %w", err)
	}

	if verify != nil && !verify(dm.Author, sigPayload, dm.Signature) {
		return fmt.Errorf("invalid signature")
	}

	if len(dm.Id) > 0 {
		expectedCID, err := ComputeCID(sigPayload)
		if err != nil {
			return fmt.Errorf("failed to compute CID: %w", err)
		}
		if !bytes.Equal(dm.Id, expectedCID) {
			return fmt.Errorf("id does not match content hash")
		}
	}

	return nil
}

// validateTimestamp checks that a millisecond unix timestamp is not more than
// MaxFutureSkew (5 min) in the future and not more than MaxPastAge (30 days)
// in the past. The past-age check is skipped when HistoricalSyncMode is true.
func validateTimestamp(tsMillis uint64) error {
	ts := time.UnixMilli(int64(tsMillis))
	now := time.Now()

	// Reject messages with timestamps too far in the future.
	if ts.After(now.Add(MaxFutureSkew)) {
		return fmt.Errorf("timestamp %v is more than %v in the future", ts, MaxFutureSkew)
	}

	// Reject messages with timestamps too far in the past (unless syncing history).
	if !HistoricalSyncMode && ts.Before(now.Add(-MaxPastAge)) {
		return fmt.Errorf("timestamp %v is more than %v in the past", ts, MaxPastAge)
	}

	return nil
}

// postSigningPayload returns the serialized Post with id and signature zeroed,
// suitable for signing or CID computation.
func postSigningPayload(post *pb.Post) ([]byte, error) {
	clone := proto.Clone(post).(*pb.Post)
	clone.Id = nil
	clone.Signature = nil
	data, err := proto.Marshal(clone)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal post: %w", err)
	}
	return data, nil
}

// reactionSigningPayload returns the serialized Reaction with id and signature zeroed.
func reactionSigningPayload(reaction *pb.Reaction) ([]byte, error) {
	clone := proto.Clone(reaction).(*pb.Reaction)
	clone.Id = nil
	clone.Signature = nil
	data, err := proto.Marshal(clone)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal reaction: %w", err)
	}
	return data, nil
}

// profileSigningPayload returns the serialized Profile with signature zeroed.
func profileSigningPayload(profile *pb.Profile) ([]byte, error) {
	clone := proto.Clone(profile).(*pb.Profile)
	clone.Signature = nil
	data, err := proto.Marshal(clone)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal profile: %w", err)
	}
	return data, nil
}

// followEventSigningPayload returns the serialized FollowEvent with signature zeroed.
func followEventSigningPayload(event *pb.FollowEvent) ([]byte, error) {
	clone := proto.Clone(event).(*pb.FollowEvent)
	clone.Signature = nil
	data, err := proto.Marshal(clone)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal follow event: %w", err)
	}
	return data, nil
}

// dmSigningPayload returns the serialized DirectMessage with id and signature zeroed.
func dmSigningPayload(dm *pb.DirectMessage) ([]byte, error) {
	clone := proto.Clone(dm).(*pb.DirectMessage)
	clone.Id = nil
	clone.Signature = nil
	data, err := proto.Marshal(clone)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal direct message: %w", err)
	}
	return data, nil
}
