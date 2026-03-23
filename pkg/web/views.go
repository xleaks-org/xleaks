package web

import (
	"encoding/hex"

	"github.com/xleaks-org/xleaks/pkg/feed"
	"github.com/xleaks-org/xleaks/pkg/storage"
)

// entryToView converts a feed.TimelineEntry to a PostView.
func (h *Handler) entryToView(e *feed.TimelineEntry) PostView {
	cidHex := hex.EncodeToString(e.Post.CID)
	authorHex := hex.EncodeToString(e.Post.Author)

	return PostView{
		ID:            cidHex,
		AuthorName:    e.AuthorName,
		AuthorInitial: getInitial(e.AuthorName),
		ShortPubkey:   shortenHex(authorHex),
		Content:       e.Post.Content,
		RelativeTime:  formatRelativeTime(e.Post.Timestamp),
		LikeCount:     e.LikeCount,
		ReplyCount:    e.ReplyCount,
		RepostCount:   e.RepostCount,
	}
}

// postRowToView converts a storage.PostRow to a PostView (fetching profile data).
func (h *Handler) postRowToView(p *storage.PostRow) PostView {
	cidHex := hex.EncodeToString(p.CID)
	authorHex := hex.EncodeToString(p.Author)

	authorName := authorHex[:16] + "..."
	profile, err := h.db.GetProfile(p.Author)
	if err == nil && profile != nil && profile.DisplayName != "" {
		authorName = profile.DisplayName
	}

	likeCount, _ := h.db.GetReactionCount(p.CID)

	return PostView{
		ID:            cidHex,
		AuthorName:    authorName,
		AuthorInitial: getInitial(authorName),
		ShortPubkey:   shortenHex(authorHex),
		Content:       p.Content,
		RelativeTime:  formatRelativeTime(p.Timestamp),
		LikeCount:     likeCount,
	}
}

// buildNewPostView creates a PostView for a freshly created post.
func (h *Handler) buildNewPostView(postID, content string) PostView {
	user := h.currentUser()
	authorName := "Anonymous"
	authorInitial := "A"
	shortPubkey := ""
	if user != nil {
		authorName = user.DisplayName
		authorInitial = getInitial(user.DisplayName)
		shortPubkey = user.ShortPubkey
	}

	return PostView{
		ID:            postID,
		AuthorName:    authorName,
		AuthorInitial: authorInitial,
		ShortPubkey:   shortPubkey,
		Content:       content,
		RelativeTime:  "just now",
	}
}

// buildMessageViews converts DB message rows to template views (oldest first).
func buildMessageViews(msgs []storage.DMRow, ownPubkey []byte) []MessageView {
	views := make([]MessageView, 0, len(msgs))
	ownHex := hex.EncodeToString(ownPubkey)
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		views = append(views, MessageView{
			Content:      "(encrypted)",
			IsSent:       hex.EncodeToString(m.Author) == ownHex,
			RelativeTime: formatRelativeTime(m.Timestamp),
		})
	}
	return views
}
