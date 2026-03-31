package handlers

import (
	"encoding/hex"
	"net/http"

	"github.com/xleaks-org/xleaks/pkg/feed"
	"github.com/xleaks-org/xleaks/pkg/p2p"
	"github.com/xleaks-org/xleaks/pkg/social"
	"github.com/xleaks-org/xleaks/pkg/storage"
)

// createPostRequest is the JSON body for POST /api/posts.
type createPostRequest struct {
	Content   string   `json:"content"`
	MediaCIDs []string `json:"media_cids"`
	ReplyTo   string   `json:"reply_to"`
}

// CreatePost handles POST /api/posts.
func (h *Handler) CreatePost(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireIdentity(w); !ok {
		return
	}

	var req createPostRequest
	if err := parseJSON(w, r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := social.ValidatePostContent(req.Content); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Decode media CIDs from hex strings.
	var mediaCIDs [][]byte
	for _, cidHex := range req.MediaCIDs {
		cid, err := hex.DecodeString(cidHex)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid media CID hex: "+cidHex)
			return
		}
		mediaCIDs = append(mediaCIDs, cid)
	}

	// Decode reply_to CID if present.
	var replyTo []byte
	if req.ReplyTo != "" {
		var err error
		replyTo, err = hex.DecodeString(req.ReplyTo)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid reply_to hex")
			return
		}
	}

	post, err := h.posts.CreatePost(r.Context(), req.Content, mediaCIDs, replyTo)
	if err != nil {
		respondInternalError(w, "failed to create post", err, "failed to create post")
		return
	}

	postData := map[string]interface{}{
		"id":         hex.EncodeToString(post.Id),
		"author":     hex.EncodeToString(post.Author),
		"content":    post.Content,
		"media_cids": hexSlice(post.MediaCids),
		"timestamp":  post.Timestamp,
		"tags":       post.Tags,
	}
	if h.ensureTopic != nil {
		_ = h.ensureTopic(p2p.ReactionsTopic(hex.EncodeToString(post.Id)))
	}
	h.emit(EventNewPost, postData)
	respondJSON(w, http.StatusCreated, postData)
}

// GetPost handles GET /api/posts/{cid}.
func (h *Handler) GetPost(w http.ResponseWriter, r *http.Request) {
	cidBytes, err := parseHexParam(r, "cid")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	postRow, err := h.db.GetPost(cidBytes)
	if err != nil {
		respondError(w, http.StatusNotFound, "post not found")
		return
	}
	if postRow == nil {
		respondError(w, http.StatusNotFound, "post not found")
		return
	}
	if h.ensureTopic != nil {
		_ = h.ensureTopic(p2p.ReactionsTopic(hex.EncodeToString(cidBytes)))
	}

	likeCount, _ := h.db.GetReactionCount(cidBytes)
	mediaCIDs := h.mediaCIDsForPost(cidBytes)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"id":         hex.EncodeToString(postRow.CID),
		"author":     hex.EncodeToString(postRow.Author),
		"content":    postRow.Content,
		"media_cids": mediaCIDs,
		"reply_to":   hexOrEmpty(postRow.ReplyTo),
		"repost_of":  hexOrEmpty(postRow.RepostOf),
		"timestamp":  postRow.Timestamp,
		"like_count": likeCount,
	})
}

// GetThread handles GET /api/posts/{cid}/thread.
func (h *Handler) GetThread(w http.ResponseWriter, r *http.Request) {
	cidBytes, err := parseHexParam(r, "cid")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	thread, err := h.posts.GetThread(r.Context(), cidBytes)
	if err != nil {
		respondNotFoundError(w, "failed to load thread", err, "thread not found")
		return
	}

	respondJSON(w, http.StatusOK, threadNodeToMap(h.db, thread))
}

// GetPostReactions handles GET /api/posts/{cid}/reactions.
func (h *Handler) GetPostReactions(w http.ResponseWriter, r *http.Request) {
	cidBytes, err := parseHexParam(r, "cid")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	reactions, err := h.db.GetReactions(cidBytes)
	if err != nil {
		respondInternalError(w, "failed to load post reactions", err, "failed to load reactions")
		return
	}

	result := make([]map[string]interface{}, 0, len(reactions))
	for _, rx := range reactions {
		result = append(result, map[string]interface{}{
			"id":            hex.EncodeToString(rx.CID),
			"author":        hex.EncodeToString(rx.Author),
			"target":        hex.EncodeToString(rx.Target),
			"reaction_type": rx.ReactionType,
			"timestamp":     rx.Timestamp,
		})
	}

	respondJSON(w, http.StatusOK, result)
}

// GetUserPosts handles GET /api/users/{pubkey}/posts.
func (h *Handler) GetUserPosts(w http.ResponseWriter, r *http.Request) {
	pubkey, err := parseHexParam(r, "pubkey")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	before, limit, err := parsePagination(r, 20)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	entries, err := h.timeline.GetUserPosts(pubkey, before, limit)
	if err != nil {
		respondInternalError(w, "failed to load user posts", err, "failed to load user posts")
		return
	}

	respondJSON(w, http.StatusOK, timelineEntriesToJSON(h.db, entries))
}

// hexOrEmpty returns the hex encoding of b, or an empty string if b is nil/empty.
func hexOrEmpty(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return hex.EncodeToString(b)
}

// threadNodeToMap converts a ThreadNode tree to a JSON-friendly map.
func threadNodeToMap(db mediaLookup, node *social.ThreadNode) map[string]interface{} {
	if node == nil {
		return nil
	}

	children := make([]map[string]interface{}, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, threadNodeToMap(db, child))
	}

	result := map[string]interface{}{
		"post": map[string]interface{}{
			"id":         hex.EncodeToString(node.Post.Id),
			"author":     hex.EncodeToString(node.Post.Author),
			"content":    node.Post.Content,
			"media_cids": mediaCIDsForPost(db, node.Post.Id),
			"timestamp":  node.Post.Timestamp,
			"reply_to":   hexOrEmpty(node.Post.ReplyTo),
			"repost_of":  hexOrEmpty(node.Post.RepostOf),
		},
		"reply_count": node.ReplyCount,
		"like_count":  node.LikeCount,
		"children":    children,
	}
	return result
}

// timelineEntriesToJSON converts timeline entries to JSON-friendly slices.
func timelineEntriesToJSON(db mediaLookup, entries []feed.TimelineEntry) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(entries))
	for _, e := range entries {
		result = append(result, map[string]interface{}{
			"id":           hex.EncodeToString(e.Post.CID),
			"author":       hex.EncodeToString(e.Post.Author),
			"author_name":  e.AuthorName,
			"content":      e.Post.Content,
			"media_cids":   mediaCIDsForPost(db, e.Post.CID),
			"reply_to":     hexOrEmpty(e.Post.ReplyTo),
			"repost_of":    hexOrEmpty(e.Post.RepostOf),
			"timestamp":    e.Post.Timestamp,
			"like_count":   e.LikeCount,
			"reply_count":  e.ReplyCount,
			"repost_count": e.RepostCount,
			"is_liked":     e.IsLiked,
			"is_reposted":  e.IsReposted,
		})
	}
	return result
}

type mediaLookup interface {
	GetPostMedia(postCID []byte) ([]storage.PostMediaRow, error)
}

func (h *Handler) mediaCIDsForPost(postCID []byte) []string {
	return mediaCIDsForPost(h.db, postCID)
}

func mediaCIDsForPost(db mediaLookup, postCID []byte) []string {
	if db == nil || len(postCID) == 0 {
		return nil
	}
	items, err := db.GetPostMedia(postCID)
	if err != nil {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, hex.EncodeToString(item.CID))
	}
	return result
}
