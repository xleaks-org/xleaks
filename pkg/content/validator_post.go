package content

import (
	"fmt"
	"unicode/utf8"

	pb "github.com/xleaks-org/xleaks/proto/gen"
	"google.golang.org/protobuf/proto"
)

// ValidatePost validates all rules for a Post message per the XLeaks specification.
func ValidatePost(post *pb.Post, verify SignatureVerifier) error {
	if post == nil {
		return fmt.Errorf("post is nil")
	}

	if len(post.Author) != Ed25519PublicKeySize {
		return fmt.Errorf("author must be %d bytes, got %d", Ed25519PublicKeySize, len(post.Author))
	}

	if utf8.RuneCountInString(post.Content) > MaxContentLength {
		return fmt.Errorf("content exceeds %d characters", MaxContentLength)
	}

	if post.Content == "" && len(post.MediaCids) == 0 && len(post.RepostOf) == 0 {
		return fmt.Errorf("content must not be empty unless media_cids or repost_of is set")
	}

	if len(post.MediaCids) > MaxMediaCIDs {
		return fmt.Errorf("media_cids exceeds maximum of %d items", MaxMediaCIDs)
	}

	if len(post.ReplyTo) > 0 && len(post.RepostOf) > 0 {
		return fmt.Errorf("reply_to and repost_of are mutually exclusive")
	}

	if err := validateTimestamp(post.Timestamp); err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}

	if len(post.Signature) != Ed25519SignatureSize {
		return fmt.Errorf("signature must be %d bytes, got %d", Ed25519SignatureSize, len(post.Signature))
	}

	sigPayload, err := postSigningPayload(post)
	if err != nil {
		return fmt.Errorf("failed to compute signing payload: %w", err)
	}

	if verify != nil && !verify(post.Author, sigPayload, post.Signature) {
		return fmt.Errorf("invalid signature")
	}

	return verifyCID(post.Id, sigPayload)
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
