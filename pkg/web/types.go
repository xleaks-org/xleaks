package web

import (
	"context"
	"embed"
)

//go:embed templates/*.html
var templateFS embed.FS

// UserInfo contains display information about the current user for templates.
type UserInfo struct {
	DisplayName string
	Address     string
	Pubkey      string
	ShortPubkey string
}

// PostView is a template-friendly representation of a post.
type PostView struct {
	ID            string
	AuthorName    string
	AuthorInitial string
	ShortPubkey   string
	Content       string
	RelativeTime  string
	LikeCount     int
	ReplyCount    int
	RepostCount   int
}

// NotificationView is a template-friendly representation of a notification.
type NotificationView struct {
	Type         string
	ActorName    string
	ActorInitial string
	RelativeTime string
	Read         bool
}

// ConversationView is a template-friendly representation of a DM conversation.
type ConversationView struct {
	PeerPubkey   string
	PeerName     string
	PeerInitial  string
	Preview      string
	RelativeTime string
	UnreadCount  int
}

// ProfileView holds profile data for the profile page template.
type ProfileView struct {
	DisplayName string
	Pubkey      string
	ShortPubkey string
	Initial     string
	Bio         string
}

// MessageView is a template-friendly representation of a single DM message.
type MessageView struct {
	Content      string
	IsSent       bool
	RelativeTime string
}

// StatusData holds formatted node status for the template.
type StatusData struct {
	Peers         int
	Uptime        string
	StorageUsed   string
	StorageMax    string
	Subscriptions int
}

// WordSlot represents a word slot in the seed phrase confirmation UI.
type WordSlot struct {
	Word  string
	Blank bool
}

// CreatePostFunc is a callback to create a post, avoiding direct dependency on social package.
// The replyTo parameter is the hex-encoded CID of the parent post (empty for top-level posts).
type CreatePostFunc func(ctx context.Context, content string, replyTo string) (id string, err error)

// NodeStatusFunc is a callback that returns live node status without making
// an HTTP round-trip to the API server. Returns peer count, uptime in seconds,
// storage used bytes, storage limit bytes, and subscription count.
type NodeStatusFunc func() (peers int, uptimeSecs float64, storageUsed, storageLimit int64, subscriptions int)
