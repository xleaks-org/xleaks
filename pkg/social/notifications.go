package social

import (
	"fmt"
	"time"

	"github.com/xleaks/xleaks/pkg/storage"
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
func (s *NotificationService) GetNotifications(before int64, limit int) ([]NotificationView, error) {
	rows, err := s.storage.GetNotifications(before, limit)
	if err != nil {
		return nil, fmt.Errorf("get notifications: %w", err)
	}

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

		// Enrich with actor profile info if available.
		profile, err := s.storage.GetProfile(row.Actor)
		if err == nil && profile != nil {
			view.ActorDisplayName = profile.DisplayName
			view.ActorAvatarCID = profile.AvatarCID
		}

		views = append(views, view)
	}

	return views, nil
}
