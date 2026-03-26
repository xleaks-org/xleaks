package web

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/xleaks-org/xleaks/pkg/feed"
	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/indexer"
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
		AuthorPubkey:  authorHex,
		ShortPubkey:   shortenHex(authorHex),
		Content:       e.Post.Content,
		RelativeTime:  formatRelativeTime(e.Post.Timestamp),
		LikeCount:     e.LikeCount,
		ReplyCount:    e.ReplyCount,
		RepostCount:   e.RepostCount,
		IsLiked:       e.IsLiked,
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
	pv.Media = h.postMediaViews(e.Post.CID)

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

	likeCount, replyCount, repostCount, _ := h.db.GetFullReactionCounts(p.CID)

	pv := PostView{
		ID:            cidHex,
		AuthorName:    authorName,
		AuthorInitial: getInitial(authorName),
		AuthorPubkey:  authorHex,
		ShortPubkey:   shortenHex(authorHex),
		Content:       p.Content,
		RelativeTime:  formatRelativeTime(p.Timestamp),
		LikeCount:     likeCount,
		ReplyCount:    replyCount,
		RepostCount:   repostCount,
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
	pv.Media = h.postMediaViews(p.CID)

	return pv
}

func (h *Handler) trendingHitToView(hit indexer.ClientTrendingPost) PostView {
	authorName := shortenHex(hit.Author)
	if authorBytes, err := hex.DecodeString(hit.Author); err == nil {
		if profile, err := h.db.GetProfile(authorBytes); err == nil && profile != nil && profile.DisplayName != "" {
			authorName = profile.DisplayName
		}
	}

	return PostView{
		ID:            hit.CID,
		AuthorName:    authorName,
		AuthorInitial: getInitial(authorName),
		AuthorPubkey:  hit.Author,
		ShortPubkey:   shortenHex(hit.Author),
		Content:       hit.Content,
		RelativeTime:  formatRelativeTime(hit.Timestamp),
		LikeCount:     hit.LikeCount,
		ReplyCount:    hit.ReplyCount,
		RepostCount:   hit.RepostCount,
	}
}

// buildNewPostView creates a PostView for a freshly created post.
func (h *Handler) buildNewPostView(r *http.Request, postID, content string) PostView {
	user := h.currentUser(r)
	authorName := "Anonymous"
	authorInitial := "A"
	shortPubkey := ""
	authorPubkey := ""
	if user != nil {
		authorName = user.DisplayName
		authorInitial = getInitial(user.DisplayName)
		shortPubkey = user.ShortPubkey
		authorPubkey = user.Pubkey
	}

	return PostView{
		ID:            postID,
		AuthorName:    authorName,
		AuthorInitial: authorInitial,
		AuthorPubkey:  authorPubkey,
		ShortPubkey:   shortPubkey,
		Content:       content,
		RelativeTime:  "just now",
	}
}

// buildMessageViews converts DB message rows to template views (oldest first).
func buildMessageViews(kp *identity.KeyPair, msgs []storage.DMRow) []MessageView {
	views := make([]MessageView, 0, len(msgs))
	ownPubkey := kp.PublicKeyBytes()
	ownHex := hex.EncodeToString(ownPubkey)
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		content := decryptDMContent(kp, m)
		views = append(views, MessageView{
			Content:      content,
			IsSent:       hex.EncodeToString(m.Author) == ownHex,
			RelativeTime: formatRelativeTime(m.Timestamp),
		})
	}
	return views
}

func decryptDMContent(kp *identity.KeyPair, row storage.DMRow) string {
	if kp == nil {
		return "(encrypted)"
	}

	peerPubkey := row.Author
	if bytes.Equal(row.Author, kp.PublicKeyBytes()) {
		peerPubkey = row.Recipient
	}
	if len(row.Nonce) != 24 {
		return "(encrypted)"
	}
	var nonce [24]byte
	copy(nonce[:], row.Nonce)

	plaintext, err := identity.DecryptDM(kp.PrivateKey, ed25519.PublicKey(peerPubkey), row.EncryptedContent, nonce)
	if err != nil {
		return "(encrypted)"
	}
	return string(plaintext)
}

func (h *Handler) postMediaViews(postCID []byte) []MediaView {
	items, err := h.db.GetPostMedia(postCID)
	if err != nil || len(items) == 0 {
		return nil
	}

	views := make([]MediaView, 0, len(items))
	for _, item := range items {
		cidHex := hex.EncodeToString(item.CID)
		mimeType := item.MimeType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		view := MediaView{
			CID:          cidHex,
			URL:          "/api/media/" + cidHex,
			ThumbnailURL: "/api/media/" + cidHex + "/thumbnail",
			MimeType:     mimeType,
			IsImage:      strings.HasPrefix(mimeType, "image/"),
			IsVideo:      strings.HasPrefix(mimeType, "video/"),
			IsAudio:      strings.HasPrefix(mimeType, "audio/"),
		}
		views = append(views, view)
	}
	return views
}
