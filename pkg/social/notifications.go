package social

import (
	"bytes"
	"fmt"
	"time"

	"github.com/xleaks-org/xleaks/pkg/storage"
)

// NotificationView represents an enriched notification with actor profile info.
type NotificationView struct {
	Type             string
	ActorPubkey      []byte
	ActorDisplayName string
	ActorAvatarCID   []byte
	TargetPostCID    []byte
	RelatedCID       []byte
	Timestamp        int64
	Read             bool
}

// NotificationService handles notification generation and retrieval.
type NotificationService struct {
	storage   *storage.DB
	broadcast func(eventType string, data interface{})
}

// NewNotificationService creates a new NotificationService.
func NewNotificationService(db *storage.DB) *NotificationService {
	return &NotificationService{
		storage: db,
	}
}

// SetBroadcaster registers a callback for real-time notification events.
func (s *NotificationService) SetBroadcaster(fn func(eventType string, data interface{})) {
	s.broadcast = fn
}

// NotifyLike creates a notification for a like reaction.
func (s *NotificationService) NotifyLike(actor, targetCID, reactionCID []byte) error {
	owner, err := s.ownerForPost(targetCID)
	if err != nil {
		return fmt.Errorf("resolve like owner: %w", err)
	}
	if len(owner) == 0 || bytes.Equal(owner, actor) {
		return nil
	}
	if err := s.storage.InsertNotification(owner, "like", actor, targetCID, reactionCID, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("notify like: %w", err)
	}
	s.emit("like", owner, actor, targetCID, reactionCID)
	return nil
}

// NotifyReply creates a notification for a reply.
func (s *NotificationService) NotifyReply(actor, targetCID, replyCID []byte) error {
	owner, err := s.ownerForPost(targetCID)
	if err != nil {
		return fmt.Errorf("resolve reply owner: %w", err)
	}
	if len(owner) == 0 || bytes.Equal(owner, actor) {
		return nil
	}
	if err := s.storage.InsertNotification(owner, "reply", actor, targetCID, replyCID, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("notify reply: %w", err)
	}
	s.emit("reply", owner, actor, targetCID, replyCID)
	return nil
}

// NotifyRepost creates a notification for a repost.
func (s *NotificationService) NotifyRepost(actor, targetCID, repostCID []byte) error {
	owner, err := s.ownerForPost(targetCID)
	if err != nil {
		return fmt.Errorf("resolve repost owner: %w", err)
	}
	if len(owner) == 0 || bytes.Equal(owner, actor) {
		return nil
	}
	if err := s.storage.InsertNotification(owner, "repost", actor, targetCID, repostCID, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("notify repost: %w", err)
	}
	s.emit("repost", owner, actor, targetCID, repostCID)
	return nil
}

// NotifyFollow creates a notification for a follow event.
func (s *NotificationService) NotifyFollow(actor, target []byte) error {
	if len(target) == 0 || bytes.Equal(actor, target) {
		return nil
	}
	if err := s.storage.InsertNotification(target, "follow", actor, nil, nil, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("notify follow: %w", err)
	}
	s.emit("follow", target, actor, nil, nil)
	return nil
}

// NotifyDM creates a notification for a direct message.
func (s *NotificationService) NotifyDM(actor, recipient []byte) error {
	if len(recipient) == 0 || bytes.Equal(actor, recipient) {
		return nil
	}
	if err := s.storage.InsertNotification(recipient, "dm", actor, nil, nil, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("notify dm: %w", err)
	}
	s.emit("dm", recipient, actor, nil, nil)
	return nil
}

// GetNotifications returns enriched notifications with actor profile info.
// Uses a local profile cache to avoid querying the same actor multiple times.
func (s *NotificationService) GetNotifications(ownerPubkey []byte, before int64, limit int) ([]NotificationView, error) {
	rows, err := s.storage.GetNotifications(ownerPubkey, before, limit)
	if err != nil {
		return nil, fmt.Errorf("get notifications: %w", err)
	}

	// Local profile cache: actor pubkey hex -> *ProfileRow (nil means not found).
	profileCache := make(map[string]*storage.ProfileRow, len(rows))

	views := make([]NotificationView, 0, len(rows))
	for _, row := range rows {
		view := NotificationView{
			Type:          row.Type,
			ActorPubkey:   row.Actor,
			TargetPostCID: row.TargetCID,
			RelatedCID:    row.RelatedCID,
			Timestamp:     row.Timestamp,
			Read:          row.Read,
		}

		// Enrich with actor profile info, using cache to avoid N+1 queries.
		actorHex := fmt.Sprintf("%x", row.Actor)
		profile, cached := profileCache[actorHex]
		if !cached {
			profile, _ = s.storage.GetProfile(row.Actor)
			profileCache[actorHex] = profile // cache nil too to avoid re-querying
		}
		if profile != nil {
			view.ActorDisplayName = profile.DisplayName
			view.ActorAvatarCID = profile.AvatarCID
		}

		views = append(views, view)
	}

	return views, nil
}

func (s *NotificationService) ownerForPost(targetCID []byte) ([]byte, error) {
	if len(targetCID) == 0 {
		return nil, nil
	}
	post, err := s.storage.GetPost(targetCID)
	if err != nil {
		return nil, err
	}
	if post == nil {
		return nil, nil
	}
	return post.Author, nil
}

func (s *NotificationService) emit(notifType string, owner, actor, targetCID, relatedCID []byte) {
	if s.broadcast == nil {
		return
	}
	s.broadcast("new_notification", map[string]interface{}{
		"type":        notifType,
		"owner":       fmt.Sprintf("%x", owner),
		"actor":       fmt.Sprintf("%x", actor),
		"target_cid":  fmt.Sprintf("%x", targetCID),
		"related_cid": fmt.Sprintf("%x", relatedCID),
	})
}
