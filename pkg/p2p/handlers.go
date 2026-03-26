package p2p

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"

	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/identity"
	pb "github.com/xleaks-org/xleaks/proto/gen"
	"google.golang.org/protobuf/proto"
)

// PostIndexer is an optional interface for indexing posts and profiles
// as they are received over the P2P network.
type PostIndexer interface {
	IndexPost(post *pb.Post) error
	IndexProfile(profile *pb.Profile) error
}

// MessageProcessor handles incoming P2P messages by deserializing,
// validating, and storing them.
type MessageProcessor struct {
	db             StorageWriter
	cas            ContentWriter
	notifier       Notifier
	indexer        PostIndexer
	broadcast      func(eventType string, data interface{})
	autoFetchMedia bool
	fetchContent   func(ctx context.Context, cidHex string) ([]byte, error)
}

// StorageWriter defines the storage operations needed for message processing.
type StorageWriter interface {
	InsertPost(cid, author []byte, content string, replyTo, repostOf []byte, timestamp int64, signature []byte) error
	InsertPostMedia(postCID, mediaCID []byte, position int) error
	InsertMediaObject(cid, author []byte, mimeType string, size uint64, chunkCount uint32, width, height, duration uint32, thumbnailCID []byte, timestamp int64) error
	UpsertProfile(pubkey []byte, displayName, bio string, avatarCID, bannerCID []byte, website string, version uint64, updatedAt int64) error
	InsertReaction(cid, author, target []byte, reactionType string, timestamp int64) error
	InsertFollowEvent(author, target []byte, action string, timestamp int64) error
	InsertDM(cid, author, recipient, encryptedContent, nonce []byte, timestamp int64) error
	PostExists(cid []byte) bool
	GetProfileVersion(pubkey []byte) (version uint64, found bool, err error)
	UpdateReactionCount(postCID []byte) error
	UpdateFollowerCount(pubkey []byte) error
	SetMediaFetched(cid []byte) error
	ShouldPinAuthor(author []byte) (bool, error)
	TrackContentForAuthor(cid, author []byte) error
	TrackReactionContent(cid, author, target []byte) error
	TrackContentForDM(cid, author, recipient []byte) error
	TrackContentForMedia(cid, mediaObjectCID []byte) error
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
	NotifyRepost(actor, targetCID, repostCID []byte) error
	NotifyFollow(actor, target []byte) error
	NotifyDM(actor, recipient []byte) error
}

// NewMessageProcessor creates a new MessageProcessor.
func NewMessageProcessor(db StorageWriter, cas ContentWriter, notifier Notifier) *MessageProcessor {
	return &MessageProcessor{db: db, cas: cas, notifier: notifier}
}

// SetIndexer sets the post indexer. Must be called before message processing starts.
func (mp *MessageProcessor) SetIndexer(idx PostIndexer) {
	mp.indexer = idx
}

// SetBroadcaster sets the WebSocket broadcast function. Must be called before message processing starts.
func (mp *MessageProcessor) SetBroadcaster(fn func(eventType string, data interface{})) {
	mp.broadcast = fn
}

// SetAutoFetchMedia enables or disables eager media fetching. Must be called before message processing starts.
func (mp *MessageProcessor) SetAutoFetchMedia(enabled bool) {
	mp.autoFetchMedia = enabled
}

// SetMediaFetcher sets the content fetch callback for eager media fetching. Must be called before message processing starts.
func (mp *MessageProcessor) SetMediaFetcher(fn func(ctx context.Context, cidHex string) ([]byte, error)) {
	mp.fetchContent = fn
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
	case *pb.Envelope_MediaObject:
		return mp.handleMediaObject(ctx, payload.MediaObject)
	case *pb.Envelope_MediaChunk:
		return mp.handleMediaChunk(ctx, payload.MediaChunk)
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
	for i, mediaCID := range post.MediaCids {
		if err := mp.db.InsertPostMedia(post.Id, mediaCID, i); err != nil {
			return fmt.Errorf("insert post media: %w", err)
		}
	}
	if err := mp.db.TrackContentForAuthor(post.Id, post.Author); err != nil {
		return fmt.Errorf("track post content: %w", err)
	}

	// Feed into indexer if available.
	if mp.indexer != nil {
		if err := mp.indexer.IndexPost(post); err != nil {
			slog.Warn("failed to index post", "error", err)
		}
	}

	// If this is a reply, create a notification and update counts on target.
	if len(post.ReplyTo) > 0 {
		if err := mp.notifier.NotifyReply(post.Author, post.ReplyTo, post.Id); err != nil {
			slog.Warn("failed to notify reply", "error", err)
		}
		if err := mp.db.UpdateReactionCount(post.ReplyTo); err != nil {
			slog.Warn("failed to update reaction count for reply", "error", err)
		}
	}
	// If this is a repost, notify the original post's author.
	if len(post.RepostOf) > 0 {
		if err := mp.notifier.NotifyRepost(post.Author, post.RepostOf, post.Id); err != nil {
			slog.Warn("failed to notify repost", "error", err)
		}
		if err := mp.db.UpdateReactionCount(post.RepostOf); err != nil {
			slog.Warn("failed to update reaction count for repost", "error", err)
		}
	}
	mp.emit("new_post", map[string]interface{}{
		"id":        hex.EncodeToString(post.Id),
		"author":    hex.EncodeToString(post.Author),
		"reply_to":  hex.EncodeToString(post.ReplyTo),
		"repost_of": hex.EncodeToString(post.RepostOf),
	})
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
	if err := mp.db.TrackReactionContent(reaction.Id, reaction.Author, reaction.Target); err != nil {
		return fmt.Errorf("track reaction content: %w", err)
	}

	// Update materialized counts on the target post.
	if err := mp.db.UpdateReactionCount(reaction.Target); err != nil {
		slog.Warn("failed to update reaction count", "error", err)
	}

	// Notify: a like on a post.
	if err := mp.notifier.NotifyLike(reaction.Author, reaction.Target, reaction.Id); err != nil {
		slog.Warn("failed to notify like", "error", err)
	}
	mp.emit("new_reaction", map[string]interface{}{
		"id":     hex.EncodeToString(reaction.Id),
		"target": hex.EncodeToString(reaction.Target),
		"author": hex.EncodeToString(reaction.Author),
	})
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
	if err := mp.db.TrackContentForAuthor(profile.Author, profile.Author); err != nil {
		return fmt.Errorf("track profile content: %w", err)
	}

	// Feed into indexer if available.
	if mp.indexer != nil {
		if err := mp.indexer.IndexProfile(profile); err != nil {
			slog.Warn("failed to index profile", "error", err)
		}
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
		if err := mp.notifier.NotifyFollow(event.Author, event.Target); err != nil {
			slog.Warn("failed to notify follow", "error", err)
		}
	}
	if err := mp.db.UpdateFollowerCount(event.Author); err != nil {
		slog.Warn("failed to update author follow counts", "error", err)
	}
	if err := mp.db.UpdateFollowerCount(event.Target); err != nil {
		slog.Warn("failed to update target follow counts", "error", err)
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
	if err := mp.db.TrackContentForDM(dm.Id, dm.Author, dm.Recipient); err != nil {
		return fmt.Errorf("track dm content: %w", err)
	}

	if err := mp.notifier.NotifyDM(dm.Author, dm.Recipient); err != nil {
		slog.Warn("failed to notify DM", "error", err)
	}
	mp.emit("new_dm", map[string]interface{}{
		"id":        hex.EncodeToString(dm.Id),
		"author":    hex.EncodeToString(dm.Author),
		"recipient": hex.EncodeToString(dm.Recipient),
	})
	return nil
}

func (mp *MessageProcessor) handleMediaObject(ctx context.Context, obj *pb.MediaObject) error {
	if err := content.ValidateMediaObject(obj, verifySignature); err != nil {
		return fmt.Errorf("validate media object: %w", err)
	}
	if err := mp.db.InsertMediaObject(
		obj.Cid,
		obj.Author,
		obj.MimeType,
		obj.Size,
		obj.ChunkCount,
		obj.Width,
		obj.Height,
		obj.Duration,
		obj.ThumbnailCid,
		int64(obj.Timestamp),
	); err != nil {
		return fmt.Errorf("insert media object: %w", err)
	}
	if mp.autoFetchMedia && mp.fetchContent != nil {
		shouldPin, err := mp.db.ShouldPinAuthor(obj.Author)
		if err != nil {
			return fmt.Errorf("resolve media pin policy: %w", err)
		}
		if shouldPin {
			if err := mp.prefetchMediaContent(ctx, obj); err != nil {
				slog.Warn("failed to prefetch media", "cid", hex.EncodeToString(obj.Cid), "error", err)
			}
		}
	}
	return nil
}

func (mp *MessageProcessor) handleMediaChunk(_ context.Context, chunk *pb.MediaChunk) error {
	if err := content.ValidateMediaChunk(chunk); err != nil {
		return fmt.Errorf("validate media chunk: %w", err)
	}
	if err := mp.cas.Put(chunk.Cid, chunk.Data); err != nil {
		return fmt.Errorf("CAS put media chunk: %w", err)
	}
	if err := mp.db.TrackContentForMedia(chunk.Cid, chunk.ParentCid); err != nil {
		return fmt.Errorf("track media chunk: %w", err)
	}
	return nil
}

// verifySignature is the signature verification function passed to validators.
func verifySignature(pubkey, message, signature []byte) bool {
	return identity.Verify(pubkey, message, signature)
}

func (mp *MessageProcessor) emit(eventType string, data interface{}) {
	if mp.broadcast != nil {
		mp.broadcast(eventType, data)
	}
}

func (mp *MessageProcessor) prefetchMediaContent(ctx context.Context, obj *pb.MediaObject) error {
	items := []struct {
		cid    []byte
		parent []byte
	}{
		{cid: obj.Cid, parent: obj.Cid},
		{cid: obj.ThumbnailCid, parent: obj.Cid},
	}

	for _, item := range items {
		if len(item.cid) == 0 {
			continue
		}
		data, err := mp.fetchContent(ctx, hex.EncodeToString(item.cid))
		if err != nil {
			return fmt.Errorf("fetch %x: %w", item.cid, err)
		}
		if err := mp.cas.Put(item.cid, data); err != nil {
			return fmt.Errorf("store %x: %w", item.cid, err)
		}
		if err := mp.db.TrackContentForMedia(item.cid, item.parent); err != nil {
			return fmt.Errorf("track %x: %w", item.cid, err)
		}
	}

	if err := mp.db.SetMediaFetched(obj.Cid); err != nil {
		return fmt.Errorf("mark media fetched: %w", err)
	}
	return nil
}
