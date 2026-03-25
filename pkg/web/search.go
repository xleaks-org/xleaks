package web

import "strings"

// performSearch executes a search query and returns matching PostView results.
// It handles hashtag detection (# prefix), local DB search (by tag or content),
// and falls back to the indexer when no local results are found.
func (h *Handler) performSearch(q string, limit int) []PostView {
	var results []PostView

	if strings.HasPrefix(q, "#") {
		tag := strings.TrimPrefix(q, "#")
		posts, err := h.db.GetPostsByTag(tag, 0, limit)
		if err == nil {
			for i := range posts {
				results = append(results, h.postRowToView(&posts[i]))
			}
		}
	} else {
		posts, err := h.db.SearchPostsByContent(q, limit)
		if err == nil {
			for i := range posts {
				results = append(results, h.postRowToView(&posts[i]))
			}
		}
	}

	// WU-6: If no local results, try the indexer for broader search.
	if len(results) == 0 && h.indexerClient != nil && h.indexerClient.Available() {
		idxResults, err := h.indexerClient.SearchPosts(q, 1, limit)
		if err == nil && idxResults != nil {
			for _, hit := range idxResults.Results {
				results = append(results, PostView{
					ID:            hit.ID,
					AuthorName:    shortenHex(hit.Author),
					AuthorInitial: getInitial(hit.Author),
					ShortPubkey:   shortenHex(hit.Author),
					Content:       hit.Content,
					RelativeTime:  "indexer",
				})
			}
		}
	}

	return results
}
