package social

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/storage"
	pb "github.com/xleaks-org/xleaks/proto/gen"
	"google.golang.org/protobuf/proto"
)

var hashtagRe = regexp.MustCompile(`#(\w+)`)

// PostService handles post creation, signing, and publishing.
type PostService struct {
	storage       *storage.DB
	cas           *content.ContentStore
	identity      *identity.KeyPair
	notifications *NotificationService
	publisher     Publisher
}

// NewPostService creates a new PostService with the given dependencies.
func NewPostService(db *storage.DB, cas *content.ContentStore, kp *identity.KeyPair) *PostService {
	return &PostService{
		storage:  db,
		cas:      cas,
		identity: kp,
	}
}

// SetNotifications sets the notification service for reply notifications.
func (s *PostService) SetNotifications(ns *NotificationService) {
	s.notifications = ns
}

// SetIdentity updates the active key pair used for signing.
func (s *PostService) SetIdentity(kp *identity.KeyPair) {
	s.identity = kp
}

// SetPublisher configures the optional outbound P2P publisher.
func (s *PostService) SetPublisher(publisher Publisher) {
	s.publisher = publisher
}

// CreatePost creates, signs, and stores a new post.
func (s *PostService) CreatePost(ctx context.Context, text string, mediaCIDs [][]byte, replyTo []byte) (*pb.Post, error) {
	kp, err := activeIdentity(s.identity)
	if err != nil {
		return nil, err
	}

	// Extract hashtags from the text.
	tags := extractHashtags(text)

	// Build the Post protobuf.
	post := &pb.Post{
		Author:    kp.PublicKeyBytes(),
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
	sig, err := identity.SignProtoMessage(kp, sigPayload)
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

	// Store post, tags, media, and reaction counts in a single transaction.
	err = s.storage.WithTransaction(func(tx *sql.Tx) error {
		if err := s.storage.InsertPostTx(tx, cid, post.Author, post.Content, post.ReplyTo, post.RepostOf, int64(post.Timestamp), post.Signature); err != nil {
			return fmt.Errorf("store post in DB: %w", err)
		}
		if len(post.Tags) > 0 {
			if err := s.storage.InsertPostTagsTx(tx, post.Id, post.Tags); err != nil {
				return fmt.Errorf("store post tags: %w", err)
			}
		}
		for i, mediaCID := range mediaCIDs {
			if err := s.storage.InsertPostMediaTx(tx, cid, mediaCID, i); err != nil {
				return fmt.Errorf("store post media link: %w", err)
			}
		}
		if len(replyTo) > 0 {
			if err := s.storage.UpdateReactionCountTx(tx, replyTo); err != nil {
				return fmt.Errorf("update parent reply count: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := s.storage.TrackContentForAuthor(post.Id, post.Author); err != nil {
		return nil, fmt.Errorf("track post content: %w", err)
	}

	// Send reply notification to the parent post's author (if replying).
	if len(replyTo) > 0 && s.notifications != nil {
		parentPost, err := s.storage.GetPost(replyTo)
		if err == nil && parentPost != nil {
			// Don't notify yourself.
			if !bytes.Equal(parentPost.Author, post.Author) {
				if err := s.notifications.NotifyReply(post.Author, replyTo, post.Id); err != nil {
					log.Printf("failed to send reply notification: %v", err)
				}
			}
		}
	}

	if err := publishPost(ctx, s.publisher, post); err != nil {
		log.Printf("publish post: %v", err)
	}

	return post, nil
}

// CreateRepost creates a repost of an existing post.
func (s *PostService) CreateRepost(ctx context.Context, originalCID []byte) (*pb.Post, error) {
	kp, err := activeIdentity(s.identity)
	if err != nil {
		return nil, err
	}

	// Verify the original post exists.
	if !s.storage.PostExists(originalCID) {
		return nil, fmt.Errorf("original post not found")
	}

	post := &pb.Post{
		Author:    kp.PublicKeyBytes(),
		Timestamp: uint64(time.Now().UnixMilli()),
		RepostOf:  originalCID,
	}

	// Compute signing payload.
	sigPayload, err := signingPayloadPost(post)
	if err != nil {
		return nil, fmt.Errorf("compute signing payload: %w", err)
	}

	// Sign.
	sig, err := identity.SignProtoMessage(kp, sigPayload)
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

	// Store in database within a transaction.
	err = s.storage.WithTransaction(func(tx *sql.Tx) error {
		if err := s.storage.InsertPostTx(tx, cid, post.Author, post.Content, post.ReplyTo, post.RepostOf, int64(post.Timestamp), post.Signature); err != nil {
			return fmt.Errorf("store repost in DB: %w", err)
		}
		if err := s.storage.UpdateReactionCountTx(tx, originalCID); err != nil {
			return fmt.Errorf("update repost count: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := s.storage.TrackContentForAuthor(post.Id, post.Author); err != nil {
		return nil, fmt.Errorf("track repost content: %w", err)
	}

	if err := publishPost(ctx, s.publisher, post); err != nil {
		log.Printf("publish repost: %v", err)
	}

	return post, nil
}

const maxTagsPerPost = 20
const maxTagLength = 100

// extractHashtags extracts unique hashtags from text content.
// Limited to 20 tags of max 100 chars each.
func extractHashtags(text string) []string {
	matches := hashtagRe.FindAllStringSubmatch(text, -1)
	seen := make(map[string]bool)
	var tags []string
	for _, match := range matches {
		tag := strings.ToLower(match[1])
		if len(tag) > maxTagLength {
			continue
		}
		if !seen[tag] {
			seen[tag] = true
			tags = append(tags, tag)
			if len(tags) >= maxTagsPerPost {
				break
			}
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
