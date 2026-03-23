package main

import (
	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/feed"
	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/indexer"
	"github.com/xleaks-org/xleaks/pkg/social"
	"github.com/xleaks-org/xleaks/pkg/storage"
)

// ServiceBundle holds all social services, feed manager, timeline, and indexer
// client initialised during startup.
type ServiceBundle struct {
	Posts     *social.PostService
	Reactions *social.ReactionService
	Profiles  *social.ProfileService
	DMs       *social.DMService
	Notifs    *social.NotificationService
	Feed      *feed.Manager
	Timeline  *feed.Timeline
	Indexer   *indexer.IndexerClient
}

// setupServices creates all social services, the feed manager, timeline, and
// indexer client.
func setupServices(
	db *storage.DB,
	cas *content.ContentStore,
	kp *identity.KeyPair,
	idHolder *identity.Holder,
) *ServiceBundle {
	return &ServiceBundle{
		Posts:     social.NewPostService(db, cas, kp),
		Reactions: social.NewReactionService(db, kp),
		Profiles:  social.NewProfileService(db, kp),
		DMs:       social.NewDMService(db, kp),
		Notifs:    social.NewNotificationService(db),
		Feed:      feed.NewManager(db),
		Timeline:  feed.NewTimeline(db, kp.PublicKeyBytes()),
		Indexer:   indexer.NewIndexerClient(),
	}
}
