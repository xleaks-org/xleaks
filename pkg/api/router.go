package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/xleaks/xleaks/pkg/api/handlers"
	"github.com/xleaks/xleaks/pkg/content"
	"github.com/xleaks/xleaks/pkg/feed"
	"github.com/xleaks/xleaks/pkg/identity"
	"github.com/xleaks/xleaks/pkg/social"
	"github.com/xleaks/xleaks/pkg/storage"
)

// HandlerDeps contains all dependencies needed by API handlers.
type HandlerDeps struct {
	DB       *storage.DB
	CAS      *content.ContentStore
	KeyPair  *identity.KeyPair
	Posts    *social.PostService
	Reactions *social.ReactionService
	Profiles *social.ProfileService
	DMs      *social.DMService
	Notifs   *social.NotificationService
	Feed     *feed.Manager
	Timeline *feed.Timeline
}

// NewRouter creates the chi router with all API routes.
func NewRouter(deps *HandlerDeps, wsHub *WSHub) http.Handler {
	r := chi.NewRouter()

	r.Use(chiMiddleware.Logger)
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.RequestID)

	h := handlers.New(deps.DB, deps.CAS, deps.KeyPair, deps.Posts, deps.Reactions,
		deps.Profiles, deps.DMs, deps.Notifs, deps.Feed, deps.Timeline)

	r.Route("/api", func(r chi.Router) {
		// Identity
		r.Post("/identity/create", h.CreateIdentity)
		r.Post("/identity/import", h.ImportIdentity)
		r.Post("/identity/unlock", h.UnlockIdentity)
		r.Get("/identity/active", h.GetActiveIdentity)
		r.Post("/identity/lock", h.LockIdentity)
		r.Get("/identity/list", h.ListIdentities)
		r.Put("/identity/switch/{pubkey}", h.SwitchIdentity)
		r.Get("/identity/export", h.ExportIdentity)

		// Posts
		r.Post("/posts", h.CreatePost)
		r.Get("/posts/{cid}", h.GetPost)
		r.Get("/posts/{cid}/thread", h.GetThread)
		r.Get("/posts/{cid}/reactions", h.GetPostReactions)
		r.Get("/users/{pubkey}/posts", h.GetUserPosts)

		// Feed
		r.Get("/feed", h.GetFeed)

		// Social actions
		r.Post("/reactions", h.CreateReaction)
		r.Post("/repost", h.CreateRepost)
		r.Post("/follow/{pubkey}", h.Follow)
		r.Delete("/follow/{pubkey}", h.Unfollow)
		r.Get("/following", h.GetFollowing)
		r.Get("/users/{pubkey}/followers", h.GetFollowers)

		// Profiles
		r.Get("/profile", h.GetOwnProfile)
		r.Put("/profile", h.UpdateProfile)
		r.Get("/users/{pubkey}", h.GetUserProfile)

		// Search & Discovery
		r.Get("/search", h.Search)
		r.Get("/trending", h.GetTrending)
		r.Get("/explore", h.Explore)

		// DMs
		r.Get("/dm", h.ListConversations)
		r.Get("/dm/{pubkey}", h.GetConversation)
		r.Post("/dm/{pubkey}", h.SendDM)

		// Notifications
		r.Get("/notifications", h.GetNotifications)
		r.Put("/notifications/read", h.MarkAllNotificationsRead)
		r.Put("/notifications/{id}/read", h.MarkNotificationRead)
		r.Get("/notifications/unread-count", h.GetUnreadCount)

		// Media
		r.Post("/media", h.UploadMedia)
		r.Get("/media/{cid}", h.GetMedia)
		r.Get("/media/{cid}/thumbnail", h.GetMediaThumbnail)
		r.Get("/media/{cid}/status", h.GetMediaStatus)

		// Node status
		r.Get("/node/status", h.GetNodeStatus)
		r.Get("/node/peers", h.GetNodePeers)
		r.Get("/node/config", h.GetNodeConfig)
		r.Put("/node/config", h.UpdateNodeConfig)
	})

	// WebSocket
	r.Get("/ws", wsHub.HandleWebSocket)

	return r
}
