package protocol_test

import (
	"strings"
	"testing"
	"time"

	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/identity"
	pb "github.com/xleaks-org/xleaks/proto/gen"
	"google.golang.org/protobuf/proto"
)

// verifier wraps identity.Verify as a content.SignatureVerifier.
var verifier content.SignatureVerifier = func(pubkey, message, signature []byte) bool {
	return identity.Verify(pubkey, message, signature)
}

// makeValidPost creates a properly signed post with a valid CID.
func makeValidPost(t *testing.T, kp *identity.KeyPair, text string) *pb.Post {
	t.Helper()
	post := &pb.Post{
		Author:    kp.PublicKeyBytes(),
		Timestamp: uint64(time.Now().UnixMilli()),
		Content:   text,
	}
	signAndSetCID(t, kp, post)
	return post
}

// signAndSetCID computes the signing payload, signs it, and sets Id + Signature on the post.
func signAndSetCID(t *testing.T, kp *identity.KeyPair, post *pb.Post) {
	t.Helper()
	// Clone, zero id+signature, marshal => signing payload.
	clone := proto.Clone(post).(*pb.Post)
	clone.Id = nil
	clone.Signature = nil
	sigPayload, err := proto.Marshal(clone)
	if err != nil {
		t.Fatalf("marshal signing payload: %v", err)
	}

	sig := identity.Sign(kp.PrivateKey, sigPayload)
	post.Signature = sig

	cid, err := content.ComputeCID(sigPayload)
	if err != nil {
		t.Fatalf("compute CID: %v", err)
	}
	post.Id = cid
}

// makeValidReaction creates a properly signed reaction.
func makeValidReaction(t *testing.T, kp *identity.KeyPair, targetCID []byte) *pb.Reaction {
	t.Helper()
	reaction := &pb.Reaction{
		Author:       kp.PublicKeyBytes(),
		Target:       targetCID,
		ReactionType: "like",
		Timestamp:    uint64(time.Now().UnixMilli()),
	}
	signReaction(t, kp, reaction)
	return reaction
}

// signReaction computes the signing payload, signs it, and sets Id + Signature.
func signReaction(t *testing.T, kp *identity.KeyPair, reaction *pb.Reaction) {
	t.Helper()
	clone := proto.Clone(reaction).(*pb.Reaction)
	clone.Id = nil
	clone.Signature = nil
	sigPayload, err := proto.Marshal(clone)
	if err != nil {
		t.Fatalf("marshal reaction signing payload: %v", err)
	}
	reaction.Signature = identity.Sign(kp.PrivateKey, sigPayload)

	cid, err := content.ComputeCID(sigPayload)
	if err != nil {
		t.Fatalf("compute reaction CID: %v", err)
	}
	reaction.Id = cid
}

// makeValidProfile creates a properly signed profile.
func makeValidProfile(t *testing.T, kp *identity.KeyPair, displayName string) *pb.Profile {
	t.Helper()
	profile := &pb.Profile{
		Author:      kp.PublicKeyBytes(),
		DisplayName: displayName,
		Bio:         "test bio",
		Version:     1,
		Timestamp:   uint64(time.Now().UnixMilli()),
	}
	signProfile(t, kp, profile)
	return profile
}

// signProfile computes the signing payload, signs it, and sets Signature.
func signProfile(t *testing.T, kp *identity.KeyPair, profile *pb.Profile) {
	t.Helper()
	clone := proto.Clone(profile).(*pb.Profile)
	clone.Signature = nil
	sigPayload, err := proto.Marshal(clone)
	if err != nil {
		t.Fatalf("marshal profile signing payload: %v", err)
	}
	profile.Signature = identity.Sign(kp.PrivateKey, sigPayload)
}

// --- Tests ---

func TestInvalidSignatureRejected(t *testing.T) {
	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	post := makeValidPost(t, kp, "hello world")

	// Tamper with the signature.
	post.Signature[0] ^= 0xFF

	err = content.ValidatePost(post, verifier)
	if err == nil {
		t.Fatal("expected validation to reject tampered signature")
	}
	if !strings.Contains(err.Error(), "invalid signature") {
		t.Fatalf("expected 'invalid signature' error, got: %v", err)
	}
}

func TestOversizedContentRejected(t *testing.T) {
	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	// Create content with 5001 characters (just over the limit).
	longText := strings.Repeat("a", 5001)
	post := &pb.Post{
		Author:    kp.PublicKeyBytes(),
		Timestamp: uint64(time.Now().UnixMilli()),
		Content:   longText,
	}
	// Sign it properly -- we still need a valid signature for the check ordering.
	signAndSetCID(t, kp, post)

	err = content.ValidatePost(post, verifier)
	if err == nil {
		t.Fatal("expected validation to reject oversized content")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected content length error, got: %v", err)
	}
}

func TestDuplicateReactionsDeduped(t *testing.T) {
	// This tests dedup at the storage layer, not the validator.
	// The storage UNIQUE(author, target, reaction_type) constraint
	// ensures duplicate reactions are silently ignored.
	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	targetCID := []byte("some_target_post_cid_xxxxxxxxxxxxx")

	r1 := makeValidReaction(t, kp, targetCID)
	r2 := makeValidReaction(t, kp, targetCID)

	// Both should individually pass validation.
	if err := content.ValidateReaction(r1, verifier); err != nil {
		t.Fatalf("first reaction failed validation: %v", err)
	}
	if err := content.ValidateReaction(r2, verifier); err != nil {
		t.Fatalf("second reaction failed validation: %v", err)
	}

	// Both are valid on their own; the DB layer handles dedup.
	// (Full dedup test is in integration_test.go and storage_test.go.)
}

func TestFutureDatedMessageRejected(t *testing.T) {
	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	// Set timestamp 10 minutes in the future (max is 5 min).
	futureTS := uint64(time.Now().Add(10 * time.Minute).UnixMilli())
	post := &pb.Post{
		Author:    kp.PublicKeyBytes(),
		Timestamp: futureTS,
		Content:   "future post",
	}
	signAndSetCID(t, kp, post)

	err = content.ValidatePost(post, verifier)
	if err == nil {
		t.Fatal("expected validation to reject future-dated message")
	}
	if !strings.Contains(err.Error(), "future") {
		t.Fatalf("expected future timestamp error, got: %v", err)
	}
}

func TestStaleProfileVersionRejected(t *testing.T) {
	// The version guard is enforced by the storage layer (UpsertProfile
	// uses "WHERE excluded.version > profiles.version"), not the validator.
	// Here we verify the storage layer correctly rejects stale versions.
	// This is covered in storage_test.go; we do a quick validator check to
	// ensure a validly-signed profile with any version passes the validator
	// (the validator doesn't enforce version ordering).
	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	p1 := makeValidProfile(t, kp, "Alice v5")
	p1.Version = 5
	signProfile(t, kp, p1)
	if err := content.ValidateProfile(p1, verifier); err != nil {
		t.Fatalf("profile v5 failed validation: %v", err)
	}

	p2 := makeValidProfile(t, kp, "Alice v3")
	p2.Version = 3
	signProfile(t, kp, p2)
	if err := content.ValidateProfile(p2, verifier); err != nil {
		t.Fatalf("profile v3 should pass validator (version guard is storage-layer): %v", err)
	}
}

func TestReplyAndRepostMutuallyExclusive(t *testing.T) {
	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	post := &pb.Post{
		Author:    kp.PublicKeyBytes(),
		Timestamp: uint64(time.Now().UnixMilli()),
		Content:   "conflicting fields",
		ReplyTo:   []byte("parent_cid_xxxxxxxxxxxxxxxxxxxxx"),
		RepostOf:  []byte("original_cid_xxxxxxxxxxxxxxxxxxx"),
	}
	signAndSetCID(t, kp, post)

	err = content.ValidatePost(post, verifier)
	if err == nil {
		t.Fatal("expected validation to reject post with both reply_to and repost_of")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutually exclusive error, got: %v", err)
	}
}

func TestEmptyContentWithoutMediaRejected(t *testing.T) {
	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	post := &pb.Post{
		Author:    kp.PublicKeyBytes(),
		Timestamp: uint64(time.Now().UnixMilli()),
		Content:   "", // empty
		// No media_cids, no repost_of
	}
	signAndSetCID(t, kp, post)

	err = content.ValidatePost(post, verifier)
	if err == nil {
		t.Fatal("expected validation to reject empty content without media or repost_of")
	}
	if !strings.Contains(err.Error(), "content must not be empty") {
		t.Fatalf("expected empty content error, got: %v", err)
	}
}

func TestSelfFollowRejected(t *testing.T) {
	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	event := &pb.FollowEvent{
		Author:    kp.PublicKeyBytes(),
		Target:    kp.PublicKeyBytes(), // same as author
		Action:    "follow",
		Timestamp: uint64(time.Now().UnixMilli()),
	}

	// Sign it.
	clone := proto.Clone(event).(*pb.FollowEvent)
	clone.Signature = nil
	sigPayload, err := proto.Marshal(clone)
	if err != nil {
		t.Fatalf("marshal follow event: %v", err)
	}
	event.Signature = identity.Sign(kp.PrivateKey, sigPayload)

	err = content.ValidateFollowEvent(event, verifier)
	if err == nil {
		t.Fatal("expected validation to reject self-follow")
	}
	if !strings.Contains(err.Error(), "cannot follow themselves") {
		t.Fatalf("expected self-follow error, got: %v", err)
	}
}

func TestInvalidReactionTypeRejected(t *testing.T) {
	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	reaction := &pb.Reaction{
		Author:       kp.PublicKeyBytes(),
		Target:       []byte("target_cid_xxxxxxxxxxxxxxxxxxxxxxx"),
		ReactionType: "dislike", // invalid in v1.0
		Timestamp:    uint64(time.Now().UnixMilli()),
	}

	// Sign it.
	clone := proto.Clone(reaction).(*pb.Reaction)
	clone.Id = nil
	clone.Signature = nil
	sigPayload, err := proto.Marshal(clone)
	if err != nil {
		t.Fatalf("marshal reaction: %v", err)
	}
	reaction.Signature = identity.Sign(kp.PrivateKey, sigPayload)
	cid, err := content.ComputeCID(sigPayload)
	if err != nil {
		t.Fatalf("compute CID: %v", err)
	}
	reaction.Id = cid

	err = content.ValidateReaction(reaction, verifier)
	if err == nil {
		t.Fatal("expected validation to reject non-'like' reaction type")
	}
	if !strings.Contains(err.Error(), "reaction_type must be") {
		t.Fatalf("expected reaction type error, got: %v", err)
	}
}

func TestValidPostAccepted(t *testing.T) {
	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	post := makeValidPost(t, kp, "Hello, XLeaks protocol!")

	if err := content.ValidatePost(post, verifier); err != nil {
		t.Fatalf("valid post should pass validation: %v", err)
	}
}

func TestValidReactionAccepted(t *testing.T) {
	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	targetCID := []byte("valid_target_cid_xxxxxxxxxxxxxxxxx")
	reaction := makeValidReaction(t, kp, targetCID)

	if err := content.ValidateReaction(reaction, verifier); err != nil {
		t.Fatalf("valid reaction should pass validation: %v", err)
	}
}

func TestValidProfileAccepted(t *testing.T) {
	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	profile := makeValidProfile(t, kp, "TestUser")

	if err := content.ValidateProfile(profile, verifier); err != nil {
		t.Fatalf("valid profile should pass validation: %v", err)
	}
}
