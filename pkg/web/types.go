package web

import (
	"context"
	"embed"

	"github.com/xleaks-org/xleaks/pkg/identity"
)

//go:embed templates/*.html
var templateFS embed.FS

// UserInfo contains display information about the current user for templates.
type UserInfo struct {
	DisplayName string
	Address     string
	Pubkey      string
	ShortPubkey string
	Bio         string
	Website     string
	AvatarURL   string
}

// PostView is a template-friendly representation of a post.
type PostView struct {
	ID            string
	AuthorName    string
	AuthorInitial string
	AuthorPubkey  string
	ShortPubkey   string
	Content       string
	RelativeTime  string
	LikeCount     int
	ReplyCount    int
	RepostCount   int
	Media         []MediaView

	IsLiked    bool // whether the current user has liked this post
	IsReposted bool // whether the current user has reposted this post

	ReplyTo       string // hex CID of parent post (empty if top-level)
	ReplyToAuthor string // display name of parent post author
	RepostOf      string // hex CID of original post (empty if original)
	RepostAuthor  string // display name of original post author
}

// MediaView is a template-friendly representation of a post attachment.
type MediaView struct {
	CID          string
	URL          string
	ThumbnailURL string
	MimeType     string
	IsImage      bool
	IsVideo      bool
	IsAudio      bool
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
	Website     string
	AvatarURL   string
	BannerURL   string
}

// IdentityView is a template-friendly representation of a local identity.
type IdentityView struct {
	Pubkey      string
	ShortPubkey string
	Address     string
	DisplayName string
	IsActive    bool
}

// SearchUserView is a template-friendly user search result.
type SearchUserView struct {
	DisplayName string
	Pubkey      string
	ShortPubkey string
	Initial     string
	Bio         string
	Website     string
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

// RepostFunc is a callback to create a repost (a post with repost_of set).
type RepostFunc func(ctx context.Context, targetCIDHex string) (id string, err error)

// ReactFunc creates a signed reaction for the active session identity.
type ReactFunc func(ctx context.Context, kp *identity.KeyPair, targetCID []byte) error

// FollowFunc updates follow state for the active session identity.
type FollowFunc func(ctx context.Context, kp *identity.KeyPair, targetPubkey []byte) error

// UpdateProfileFunc updates the signed profile for the active session identity.
type UpdateProfileFunc func(ctx context.Context, kp *identity.KeyPair, displayName, bio, website string, avatarCID, bannerCID []byte) error

// SendDMFunc sends an encrypted direct message for the active session identity.
type SendDMFunc func(ctx context.Context, kp *identity.KeyPair, recipientPubkey []byte, content string) error

// NodeStatusFunc is a callback that returns live node status without making
// an HTTP round-trip to the API server. Returns peer count, uptime in seconds,
// storage used bytes, storage limit bytes, and subscription count.
type NodeStatusFunc func() (peers int, uptimeSecs float64, storageUsed, storageLimit int64, subscriptions int)
