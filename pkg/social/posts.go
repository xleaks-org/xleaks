package social

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/xleaks/xleaks/pkg/content"
	"github.com/xleaks/xleaks/pkg/identity"
	"github.com/xleaks/xleaks/pkg/storage"
	pb "github.com/xleaks/xleaks/proto/gen"
	"google.golang.org/protobuf/proto"
)

var hashtagRe = regexp.MustCompile(`#(\w+)`)

// PostService handles post creation, signing, and publishing.
type PostService struct {
	storage  *storage.DB
	cas      *content.ContentStore
	identity *identity.KeyPair
}

// NewPostService creates a new PostService with the given dependencies.
func NewPostService(db *storage.DB, cas *content.ContentStore, kp *identity.KeyPair) *PostService {
	return &PostService{
		storage:  db,
		cas:      cas,
		identity: kp,
	}
}

// CreatePost creates, signs, and stores a new post.
func (s *PostService) CreatePost(ctx context.Context, text string, mediaCIDs [][]byte, replyTo []byte) (*pb.Post, error) {
	// Extract hashtags from the text.
	tags := extractHashtags(text)

	// Build the Post protobuf.
	post := &pb.Post{
		Author:    s.identity.PublicKeyBytes(),
		Timestamp: uint64(time.Now().UnixMilli()),
		Content:   text,
		MediaCids: mediaCIDs,
		ReplyTo:   replyTo,
		Tags:      tags,
	}

	// Compute signing payload (marshal with id+signature zeroed).
	sigPayload, err := signingPayloadPost(post)
	if err != nil {
		return nil, fmt.Errorf("compute signing payload: %w", err)
	}

	// Sign with identity.
	sig, err := identity.SignProtoMessage(s.identity, sigPayload)
	if err != nil {
		return nil, fmt.Errorf("sign post: %w", err)
	}
	post.Signature = sig

	// Compute CID from signing payload.
	cid, err := content.ComputeCID(sigPayload)
	if err != nil {
		return nil, fmt.Errorf("compute CID: %w", err)
	}
	post.Id = cid

	// Validate the post.
	verifier := func(pubkey, message, signature []byte) bool {
		return identity.Verify(pubkey, message, signature)
	}
	if err := content.ValidatePost(post, verifier); err != nil {
		return nil, fmt.Errorf("validate post: %w", err)
	}

	// Store in CAS.
	fullData, err := proto.Marshal(post)
	if err != nil {
		return nil, fmt.Errorf("marshal post for CAS: %w", err)
	}
	if err := s.cas.Put(cid, fullData); err != nil {
		return nil, fmt.Errorf("store in CAS: %w", err)
	}

	// Store in database.
	if err := s.storage.InsertPost(cid, post.Author, post.Content, post.ReplyTo, post.RepostOf, int64(post.Timestamp), post.Signature); err != nil {
		return nil, fmt.Errorf("store post in DB: %w", err)
	}

	// Store hashtags.
	if len(post.Tags) > 0 {
		if err := s.storage.InsertPostTags(post.Id, post.Tags); err != nil {
			return nil, fmt.Errorf("store post tags: %w", err)
		}
	}

	// Store media references.
	for i, mediaCID := range mediaCIDs {
		if err := s.storage.InsertPostMedia(cid, mediaCID, i); err != nil {
			return nil, fmt.Errorf("store post media link: %w", err)
		}
	}

	// Update reply count on the parent if this is a reply.
	if len(replyTo) > 0 {
		if err := s.storage.UpdateReactionCount(replyTo); err != nil {
			return nil, fmt.Errorf("update parent reply count: %w", err)
		}
	}

	return post, nil
}

// CreateRepost creates a repost of an existing post.
func (s *PostService) CreateRepost(ctx context.Context, originalCID []byte) (*pb.Post, error) {
	// Verify the original post exists.
	if !s.storage.PostExists(originalCID) {
		return nil, fmt.Errorf("original post not found")
	}

	post := &pb.Post{
		Author:    s.identity.PublicKeyBytes(),
		Timestamp: uint64(time.Now().UnixMilli()),
		RepostOf:  originalCID,
	}

	// Compute signing payload.
	sigPayload, err := signingPayloadPost(post)
	if err != nil {
		return nil, fmt.Errorf("compute signing payload: %w", err)
	}

	// Sign.
	sig, err := identity.SignProtoMessage(s.identity, sigPayload)
	if err != nil {
		return nil, fmt.Errorf("sign repost: %w", err)
	}
	post.Signature = sig

	// Compute CID.
	cid, err := content.ComputeCID(sigPayload)
	if err != nil {
		return nil, fmt.Errorf("compute CID: %w", err)
	}
	post.Id = cid

	// Validate.
	verifier := func(pubkey, message, signature []byte) bool {
		return identity.Verify(pubkey, message, signature)
	}
	if err := content.ValidatePost(post, verifier); err != nil {
		return nil, fmt.Errorf("validate repost: %w", err)
	}

	// Store in CAS.
	fullData, err := proto.Marshal(post)
	if err != nil {
		return nil, fmt.Errorf("marshal repost for CAS: %w", err)
	}
	if err := s.cas.Put(cid, fullData); err != nil {
		return nil, fmt.Errorf("store repost in CAS: %w", err)
	}

	// Store in database.
	if err := s.storage.InsertPost(cid, post.Author, post.Content, post.ReplyTo, post.RepostOf, int64(post.Timestamp), post.Signature); err != nil {
		return nil, fmt.Errorf("store repost in DB: %w", err)
	}

	// Update repost count on the original post.
	if err := s.storage.UpdateReactionCount(originalCID); err != nil {
		return nil, fmt.Errorf("update repost count: %w", err)
	}

	return post, nil
}

// extractHashtags extracts unique hashtags from text content.
func extractHashtags(text string) []string {
	matches := hashtagRe.FindAllStringSubmatch(text, -1)
	seen := make(map[string]bool)
	var tags []string
	for _, match := range matches {
		tag := strings.ToLower(match[1])
		if !seen[tag] {
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	return tags
}

// signingPayloadPost returns the serialized Post with id and signature zeroed.
func signingPayloadPost(post *pb.Post) ([]byte, error) {
	clone := proto.Clone(post).(*pb.Post)
	clone.Id = nil
	clone.Signature = nil
	data, err := proto.Marshal(clone)
	if err != nil {
		return nil, fmt.Errorf("marshal post for signing: %w", err)
	}
	return data, nil
}
