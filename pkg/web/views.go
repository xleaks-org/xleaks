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

	pv := PostView{
		ID:            cidHex,
		AuthorName:    e.AuthorName,
		AuthorInitial: getInitial(e.AuthorName),
		ShortPubkey:   shortenHex(authorHex),
		Content:       e.Post.Content,
		RelativeTime:  formatRelativeTime(e.Post.Timestamp),
		LikeCount:     e.LikeCount,
		ReplyCount:    e.ReplyCount,
		RepostCount:   e.RepostCount,
		IsLiked:       e.IsLiked,
	}
	// Check if current user has reposted
	if kp := h.identity.Get(); kp != nil {
		pv.IsReposted = h.db.HasReacted(kp.PublicKeyBytes(), e.Post.CID, "repost")
	}

	// Populate reply-to metadata if this post is a reply.
	if len(e.Post.ReplyTo) > 0 {
		pv.ReplyTo = hex.EncodeToString(e.Post.ReplyTo)
		if parent, err := h.db.GetPost(e.Post.ReplyTo); err == nil && parent != nil {
			parentName := hex.EncodeToString(parent.Author)
			if profile, err := h.db.GetProfile(parent.Author); err == nil && profile != nil && profile.DisplayName != "" {
				parentName = profile.DisplayName
			}
			pv.ReplyToAuthor = parentName
		}
	}

	// Populate repost metadata if this post is a repost.
	if len(e.Post.RepostOf) > 0 {
		pv.RepostOf = hex.EncodeToString(e.Post.RepostOf)
		if original, err := h.db.GetPost(e.Post.RepostOf); err == nil && original != nil {
			repostAuthor := hex.EncodeToString(original.Author)
			if profile, err := h.db.GetProfile(original.Author); err == nil && profile != nil && profile.DisplayName != "" {
				repostAuthor = profile.DisplayName
			}
			pv.RepostAuthor = repostAuthor
		}
	}

	return pv
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

	var isLiked, isReposted bool
	if kp := h.identity.Get(); kp != nil {
		isLiked = h.db.HasReacted(kp.PublicKeyBytes(), p.CID, "like")
		isReposted = h.db.HasReacted(kp.PublicKeyBytes(), p.CID, "repost")
	}

	pv := PostView{
		ID:            cidHex,
		AuthorName:    authorName,
		AuthorInitial: getInitial(authorName),
		ShortPubkey:   shortenHex(authorHex),
		Content:       p.Content,
		RelativeTime:  formatRelativeTime(p.Timestamp),
		LikeCount:     likeCount,
		IsLiked:       isLiked,
		IsReposted:    isReposted,
	}

	// Populate reply-to metadata if this post is a reply.
	if len(p.ReplyTo) > 0 {
		pv.ReplyTo = hex.EncodeToString(p.ReplyTo)
		if parent, err := h.db.GetPost(p.ReplyTo); err == nil && parent != nil {
			parentName := hex.EncodeToString(parent.Author)
			if prof, err := h.db.GetProfile(parent.Author); err == nil && prof != nil && prof.DisplayName != "" {
				parentName = prof.DisplayName
			}
			pv.ReplyToAuthor = parentName
		}
	}

	// Populate repost metadata if this post is a repost.
	if len(p.RepostOf) > 0 {
		pv.RepostOf = hex.EncodeToString(p.RepostOf)
		if original, err := h.db.GetPost(p.RepostOf); err == nil && original != nil {
			repostAuthor := hex.EncodeToString(original.Author)
			if prof, err := h.db.GetProfile(original.Author); err == nil && prof != nil && prof.DisplayName != "" {
				repostAuthor = prof.DisplayName
			}
			pv.RepostAuthor = repostAuthor
		}
	}

	return pv
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
