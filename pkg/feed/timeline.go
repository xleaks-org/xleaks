package feed

import (
	"encoding/hex"
	"fmt"

	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/storage"
)

// TimelineEntry represents a single entry in the feed timeline.
type TimelineEntry struct {
	Post         storage.PostRow
	AuthorName   string
	LikeCount    int
	ReplyCount   int
	RepostCount  int
	IsLiked      bool // Whether the current user has liked this post
	IsReposted   bool // Whether the current user has reposted this post
}

// Timeline assembles the chronological feed from local database.
type Timeline struct {
	db       *storage.DB
	identity *identity.Holder
}

// NewTimeline creates a new Timeline assembler.
func NewTimeline(db *storage.DB, idHolder *identity.Holder) *Timeline {
	return &Timeline{
		db:       db,
		identity: idHolder,
	}
}

// GetFeed returns the home feed — posts from followed publishers, paginated.
func (t *Timeline) GetFeed(before int64, limit int) ([]TimelineEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	subs, err := t.db.GetSubscriptions()
	if err != nil {
		return nil, fmt.Errorf("get subscriptions: %w", err)
	}

	authors := make([][]byte, len(subs))
	for i, sub := range subs {
		authors[i] = sub.Pubkey
	}

	// Include own posts in feed (get current pubkey dynamically).
	if kp := t.identity.Get(); kp != nil {
		authors = append(authors, kp.PublicKeyBytes())
	}

	posts, err := t.db.GetFeed(authors, before, limit)
	if err != nil {
		return nil, fmt.Errorf("get feed: %w", err)
	}

	return t.enrichPosts(posts)
}

// GetGlobalFeed returns all posts regardless of follow status, paginated.
func (t *Timeline) GetGlobalFeed(before int64, limit int) ([]TimelineEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	posts, err := t.db.GetAllPosts(before, limit)
	if err != nil {
		return nil, fmt.Errorf("get global feed: %w", err)
	}

	return t.enrichPosts(posts)
}

// GetUserPosts returns posts by a specific user, paginated.
func (t *Timeline) GetUserPosts(pubkey []byte, before int64, limit int) ([]TimelineEntry, error) {
	if limit <= 0 {
		limit = 20
	}

	posts, err := t.db.GetPostsByAuthor(pubkey, before, limit)
	if err != nil {
		return nil, fmt.Errorf("get user posts: %w", err)
	}

	return t.enrichPosts(posts)
}

// enrichPosts adds reaction counts and profile info to raw post rows.
func (t *Timeline) enrichPosts(posts []storage.PostRow) ([]TimelineEntry, error) {
	entries := make([]TimelineEntry, 0, len(posts))

	var ownPubkey []byte
	if kp := t.identity.Get(); kp != nil {
		ownPubkey = kp.PublicKeyBytes()
	}

	for _, post := range posts {
		entry := TimelineEntry{Post: post}

		// Get author profile.
		profile, err := t.db.GetProfile(post.Author)
		if err == nil && profile != nil {
			entry.AuthorName = profile.DisplayName
		} else {
			entry.AuthorName = hex.EncodeToString(post.Author)[:16] + "..."
		}

		// Get reaction counts (likes, replies, reposts).
		likes, replies, reposts, err := t.db.GetFullReactionCounts(post.CID)
		if err == nil {
			entry.LikeCount = likes
			entry.ReplyCount = replies
			entry.RepostCount = reposts
		}

		// Check if current user has liked/reposted.
		entry.IsLiked = t.db.HasReacted(ownPubkey, post.CID, "like")

		entries = append(entries, entry)
	}

	return entries, nil
}
