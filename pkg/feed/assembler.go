package feed

import (
	"encoding/hex"

	"github.com/xleaks-org/xleaks/pkg/storage"
)

// FeedAssembler builds feed data from the local database, handling
// enrichment with profiles, reaction counts, and user-specific state.
type FeedAssembler struct {
	db        *storage.DB
	ownPubkey []byte
}

// NewFeedAssembler creates a new FeedAssembler.
func NewFeedAssembler(db *storage.DB, ownPubkey []byte) *FeedAssembler {
	return &FeedAssembler{
		db:        db,
		ownPubkey: ownPubkey,
	}
}

// AssemblePost enriches a single PostRow with profile/reaction data.
func (fa *FeedAssembler) AssemblePost(post storage.PostRow) (TimelineEntry, error) {
	entry := TimelineEntry{Post: post}

	// Get author profile for display name.
	profile, err := fa.db.GetProfile(post.Author)
	if err == nil && profile != nil {
		entry.AuthorName = profile.DisplayName
	} else {
		entry.AuthorName = hex.EncodeToString(post.Author)[:16] + "..."
	}

	// Get reaction counts.
	likes, err := fa.db.GetReactionCount(post.CID)
	if err == nil {
		entry.LikeCount = likes
	}

	// Check if current user has liked/reposted this post.
	entry.IsLiked = fa.db.HasReacted(fa.ownPubkey, post.CID, "like")
	entry.IsReposted = fa.db.HasReacted(fa.ownPubkey, post.CID, "repost")

	return entry, nil
}

// AssemblePosts enriches multiple PostRows.
func (fa *FeedAssembler) AssemblePosts(posts []storage.PostRow) ([]TimelineEntry, error) {
	entries := make([]TimelineEntry, 0, len(posts))
	for _, post := range posts {
		entry, err := fa.AssemblePost(post)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
