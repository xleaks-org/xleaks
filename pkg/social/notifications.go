package social

import (
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
	storage *storage.DB
}

// NewNotificationService creates a new NotificationService.
func NewNotificationService(db *storage.DB) *NotificationService {
	return &NotificationService{
		storage: db,
	}
}

// NotifyLike creates a notification for a like reaction.
func (s *NotificationService) NotifyLike(actor, targetCID, reactionCID []byte) error {
	if err := s.storage.InsertNotification("like", actor, targetCID, reactionCID, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("notify like: %w", err)
	}
	return nil
}

// NotifyReply creates a notification for a reply.
func (s *NotificationService) NotifyReply(actor, targetCID, replyCID []byte) error {
	if err := s.storage.InsertNotification("reply", actor, targetCID, replyCID, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("notify reply: %w", err)
	}
	return nil
}

// NotifyRepost creates a notification for a repost.
func (s *NotificationService) NotifyRepost(actor, targetCID, repostCID []byte) error {
	if err := s.storage.InsertNotification("repost", actor, targetCID, repostCID, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("notify repost: %w", err)
	}
	return nil
}

// NotifyFollow creates a notification for a follow event.
func (s *NotificationService) NotifyFollow(actor []byte) error {
	if err := s.storage.InsertNotification("follow", actor, nil, nil, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("notify follow: %w", err)
	}
	return nil
}

// NotifyDM creates a notification for a direct message.
func (s *NotificationService) NotifyDM(actor []byte) error {
	if err := s.storage.InsertNotification("dm", actor, nil, nil, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("notify dm: %w", err)
	}
	return nil
}

// GetNotifications returns enriched notifications with actor profile info.
// Uses a local profile cache to avoid querying the same actor multiple times.
func (s *NotificationService) GetNotifications(before int64, limit int) ([]NotificationView, error) {
	rows, err := s.storage.GetNotifications(before, limit)
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
