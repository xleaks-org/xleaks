package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/xleaks-org/xleaks/pkg/api/handlers"
	"github.com/xleaks-org/xleaks/pkg/api/middleware"
	"github.com/xleaks-org/xleaks/pkg/config"
	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/feed"
	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/indexer"
	"github.com/xleaks-org/xleaks/pkg/p2p"
	"github.com/xleaks-org/xleaks/pkg/social"
	"github.com/xleaks-org/xleaks/pkg/storage"
)

// HandlerDeps contains all dependencies needed by API handlers.
type HandlerDeps struct {
	DB             *storage.DB
	CAS            *content.ContentStore
	KeyPair        *identity.KeyPair
	IdentityHolder *identity.Holder
	Posts          *social.PostService
	Reactions      *social.ReactionService
	Profiles       *social.ProfileService
	DMs            *social.DMService
	Follows        *social.FollowService
	Notifs         *social.NotificationService
	Feed           *feed.Manager
	Timeline       *feed.Timeline
	P2PHost        *p2p.Host
	Config         *config.Config
	ConfigPath     string
	IndexerClient  *indexer.IndexerClient
	IdentityChange func(*identity.KeyPair)
	EnsureTopic    func(string) error
	WebHandler     chi.Router // optional: Go-based web UI routes
}

// NewRouter creates the chi router with all API routes.
func NewRouter(deps *HandlerDeps, wsHub *WSHub) http.Handler {
	r := chi.NewRouter()

	r.Use(chiMiddleware.Logger)
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.RequestID)

	// Per-route rate limiting.
	rl := middleware.NewRouteRateLimiter()
	rl.AddLimit("POST /api/posts", 10, time.Minute)
	rl.AddLimit("POST /api/reactions", 30, time.Minute)
	rl.AddLimit("POST /api/dm", 30, time.Minute)
	rl.AddLimit("POST /api/media", 5, time.Minute)
	rl.AddLimit("POST /api/identity/create", 5, time.Minute)
	rl.AddLimit("POST /api/identity/import", 5, time.Minute)
	rl.AddLimit("POST /api/identity/unlock", 10, time.Minute)
	rl.AddLimit("GET", 120, time.Minute)
	rl.SetGlobalLimit(300, time.Minute)
	r.Use(rl.Middleware)

	h := handlers.New(deps.DB, deps.CAS, deps.KeyPair, deps.Posts, deps.Reactions,
		deps.Profiles, deps.DMs, deps.Follows, deps.Notifs, deps.Feed, deps.Timeline)
	// Wire WebSocket event broadcasts into API handlers.
	h.SetBroadcaster(func(eventType string, data interface{}) {
		wsHub.Broadcast(WSEvent{Type: eventType, Data: data})
	})
	if deps.IdentityHolder != nil {
		h.SetIdentityHolder(deps.IdentityHolder)
	}
	if deps.P2PHost != nil {
		h.SetP2PHost(deps.P2PHost)
	}
	if deps.Config != nil {
		h.SetConfig(deps.Config, deps.ConfigPath)
	}
	if deps.IndexerClient != nil {
		h.SetIndexerClient(deps.IndexerClient)
	}
	if deps.IdentityChange != nil {
		h.SetIdentityChangeFunc(deps.IdentityChange)
	}
	if deps.EnsureTopic != nil {
		h.SetTopicSubscriber(deps.EnsureTopic)
	}

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

	// Web UI (Go templates + htmx)
	if deps.WebHandler != nil {
		r.Mount("/", deps.WebHandler)
	}

	return r
}
