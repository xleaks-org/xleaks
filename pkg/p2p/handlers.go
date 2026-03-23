package p2p

import (
	"context"
	"fmt"
	"log"

	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/identity"
	pb "github.com/xleaks-org/xleaks/proto/gen"
	"google.golang.org/protobuf/proto"
)

// MessageProcessor handles incoming P2P messages by deserializing,
// validating, and storing them.
type MessageProcessor struct {
	db       StorageWriter
	cas      ContentWriter
	notifier Notifier
}

// StorageWriter defines the storage operations needed for message processing.
type StorageWriter interface {
	InsertPost(cid, author []byte, content string, replyTo, repostOf []byte, timestamp int64, signature []byte) error
	UpsertProfile(pubkey []byte, displayName, bio string, avatarCID, bannerCID []byte, website string, version uint64, updatedAt int64) error
	InsertReaction(cid, author, target []byte, reactionType string, timestamp int64) error
	InsertFollowEvent(author, target []byte, action string, timestamp int64) error
	InsertDM(cid, author, recipient, encryptedContent, nonce []byte, timestamp int64) error
	PostExists(cid []byte) bool
	GetProfileVersion(pubkey []byte) (version uint64, found bool, err error)
	UpdateReactionCount(postCID []byte) error
}

// ContentWriter defines content-addressed storage operations.
type ContentWriter interface {
	Put(cid []byte, data []byte) error
	Has(cid []byte) bool
}

// Notifier creates notifications for relevant events.
type Notifier interface {
	NotifyLike(actor, targetCID, reactionCID []byte) error
	NotifyReply(actor, targetCID, replyCID []byte) error
	NotifyFollow(actor []byte) error
	NotifyDM(actor []byte) error
}

// NewMessageProcessor creates a new MessageProcessor.
func NewMessageProcessor(db StorageWriter, cas ContentWriter, notifier Notifier) *MessageProcessor {
	return &MessageProcessor{db: db, cas: cas, notifier: notifier}
}

// HandleMessage deserializes an Envelope and routes to the appropriate handler.
func (mp *MessageProcessor) HandleMessage(ctx context.Context, data []byte) error {
	var env pb.Envelope
	if err := proto.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("unmarshal envelope: %w", err)
	}
	switch payload := env.Payload.(type) {
	case *pb.Envelope_Post:
		return mp.handlePost(ctx, payload.Post)
	case *pb.Envelope_Reaction:
		return mp.handleReaction(ctx, payload.Reaction)
	case *pb.Envelope_Profile:
		return mp.handleProfile(ctx, payload.Profile)
	case *pb.Envelope_FollowEvent:
		return mp.handleFollow(ctx, payload.FollowEvent)
	case *pb.Envelope_DirectMessage:
		return mp.handleDM(ctx, payload.DirectMessage)
	default:
		return nil // Unknown payload type, ignore.
	}
}

func (mp *MessageProcessor) handlePost(_ context.Context, post *pb.Post) error {
	if err := content.ValidatePost(post, verifySignature); err != nil {
		return fmt.Errorf("validate post: %w", err)
	}

	// Dedup: skip if already stored.
	if mp.db.PostExists(post.Id) {
		return nil
	}

	// Store the serialized proto in CAS.
	raw, err := proto.Marshal(post)
	if err != nil {
		return fmt.Errorf("marshal post for CAS: %w", err)
	}
	if err := mp.cas.Put(post.Id, raw); err != nil {
		return fmt.Errorf("CAS put post: %w", err)
	}

	// Store in DB.
	if err := mp.db.InsertPost(
		post.Id, post.Author, post.Content,
		post.ReplyTo, post.RepostOf,
		int64(post.Timestamp), post.Signature,
	); err != nil {
		return fmt.Errorf("insert post: %w", err)
	}

	// If this is a reply, create a notification and update counts on target.
	if len(post.ReplyTo) > 0 {
		if err := mp.notifier.NotifyReply(post.Author, post.ReplyTo, post.Id); err != nil {
			log.Printf("notify reply error: %v", err)
		}
		if err := mp.db.UpdateReactionCount(post.ReplyTo); err != nil {
			log.Printf("update reaction count (reply): %v", err)
		}
	}
	return nil
}

func (mp *MessageProcessor) handleReaction(_ context.Context, reaction *pb.Reaction) error {
	if err := content.ValidateReaction(reaction, verifySignature); err != nil {
		return fmt.Errorf("validate reaction: %w", err)
	}

	// Dedup: CAS check.
	if mp.cas.Has(reaction.Id) {
		return nil
	}

	raw, err := proto.Marshal(reaction)
	if err != nil {
		return fmt.Errorf("marshal reaction for CAS: %w", err)
	}
	if err := mp.cas.Put(reaction.Id, raw); err != nil {
		return fmt.Errorf("CAS put reaction: %w", err)
	}

	if err := mp.db.InsertReaction(
		reaction.Id, reaction.Author, reaction.Target,
		reaction.ReactionType, int64(reaction.Timestamp),
	); err != nil {
		return fmt.Errorf("insert reaction: %w", err)
	}

	// Update materialized counts on the target post.
	if err := mp.db.UpdateReactionCount(reaction.Target); err != nil {
		log.Printf("update reaction count: %v", err)
	}

	// Notify: a like on a post.
	if err := mp.notifier.NotifyLike(reaction.Author, reaction.Target, reaction.Id); err != nil {
		log.Printf("notify like error: %v", err)
	}
	return nil
}

func (mp *MessageProcessor) handleProfile(_ context.Context, profile *pb.Profile) error {
	if err := content.ValidateProfile(profile, verifySignature); err != nil {
		return fmt.Errorf("validate profile: %w", err)
	}

	// Version guard: only accept if version is higher than stored.
	storedVersion, found, err := mp.db.GetProfileVersion(profile.Author)
	if err != nil {
		return fmt.Errorf("get profile version: %w", err)
	}
	if found && storedVersion >= profile.Version {
		return nil // Stale or duplicate update, ignore.
	}

	raw, err := proto.Marshal(profile)
	if err != nil {
		return fmt.Errorf("marshal profile for CAS: %w", err)
	}
	// Use author pubkey as the CAS key for profiles (latest wins).
	if err := mp.cas.Put(profile.Author, raw); err != nil {
		return fmt.Errorf("CAS put profile: %w", err)
	}

	if err := mp.db.UpsertProfile(
		profile.Author, profile.DisplayName, profile.Bio,
		profile.AvatarCid, profile.BannerCid, profile.Website,
		profile.Version, int64(profile.Timestamp),
	); err != nil {
		return fmt.Errorf("upsert profile: %w", err)
	}
	return nil
}

func (mp *MessageProcessor) handleFollow(_ context.Context, event *pb.FollowEvent) error {
	if err := content.ValidateFollowEvent(event, verifySignature); err != nil {
		return fmt.Errorf("validate follow event: %w", err)
	}

	if err := mp.db.InsertFollowEvent(
		event.Author, event.Target, event.Action, int64(event.Timestamp),
	); err != nil {
		return fmt.Errorf("insert follow event: %w", err)
	}

	if event.Action == "follow" {
		if err := mp.notifier.NotifyFollow(event.Author); err != nil {
			log.Printf("notify follow error: %v", err)
		}
	}
	return nil
}

func (mp *MessageProcessor) handleDM(_ context.Context, dm *pb.DirectMessage) error {
	if err := content.ValidateDirectMessage(dm, verifySignature); err != nil {
		return fmt.Errorf("validate dm: %w", err)
	}

	// Dedup: CAS check.
	if mp.cas.Has(dm.Id) {
		return nil
	}

	raw, err := proto.Marshal(dm)
	if err != nil {
		return fmt.Errorf("marshal dm for CAS: %w", err)
	}
	if err := mp.cas.Put(dm.Id, raw); err != nil {
		return fmt.Errorf("CAS put dm: %w", err)
	}

	if err := mp.db.InsertDM(
		dm.Id, dm.Author, dm.Recipient,
		dm.EncryptedContent, dm.Nonce, int64(dm.Timestamp),
	); err != nil {
		return fmt.Errorf("insert dm: %w", err)
	}

	if err := mp.notifier.NotifyDM(dm.Author); err != nil {
		log.Printf("notify dm error: %v", err)
	}
	return nil
}

// verifySignature is the signature verification function passed to validators.
func verifySignature(pubkey, message, signature []byte) bool {
	return identity.Verify(pubkey, message, signature)
}
