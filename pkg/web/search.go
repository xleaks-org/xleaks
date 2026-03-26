package web

import (
	"encoding/hex"
	"strings"

	"github.com/xleaks-org/xleaks/pkg/storage"
)

type searchResults struct {
	Posts []PostView
	Users []SearchUserView
}

// performSearch executes a search query and returns both post and user results.
func (h *Handler) performSearch(q string, limit int) searchResults {
	return searchResults{
		Posts: h.searchPosts(q, limit),
		Users: h.searchUsers(q, limit),
	}
}

func (h *Handler) searchPosts(q string, limit int) []PostView {
	results := make([]PostView, 0, limit)
	seen := make(map[string]struct{}, limit)
	indexerQuery := normalizedPostSearchQuery(q)

	if h.indexerClient != nil && h.indexerClient.Available() && indexerQuery != "" {
		idxResults, err := h.indexerClient.SearchPosts(indexerQuery, 0, limit)
		if err == nil && idxResults != nil {
			for _, hit := range idxResults.Results {
				if hit.ID == "" {
					continue
				}
				if _, ok := seen[hit.ID]; ok {
					continue
				}
				author := hit.Author
				if author == "" {
					author = "unknown"
				}
				results = append(results, PostView{
					ID:            hit.ID,
					AuthorName:    shortenHex(author),
					AuthorInitial: getInitial(author),
					AuthorPubkey:  author,
					ShortPubkey:   shortenHex(author),
					Content:       hit.Content,
					RelativeTime:  "indexer",
				})
				seen[hit.ID] = struct{}{}
				if len(results) >= limit {
					break
				}
			}
		}
	}

	if len(results) < limit {
		if strings.HasPrefix(q, "#") {
			tag := strings.TrimPrefix(q, "#")
			posts, err := h.db.GetPostsByTag(tag, 0, limit)
			if err == nil {
				for i := range posts {
					view := h.postRowToView(&posts[i])
					if _, ok := seen[view.ID]; ok {
						continue
					}
					results = append(results, view)
					seen[view.ID] = struct{}{}
					if len(results) >= limit {
						break
					}
				}
			}
		} else {
			posts, err := h.db.SearchPostsByContent(q, limit)
			if err == nil {
				for i := range posts {
					view := h.postRowToView(&posts[i])
					if _, ok := seen[view.ID]; ok {
						continue
					}
					results = append(results, view)
					seen[view.ID] = struct{}{}
					if len(results) >= limit {
						break
					}
				}
			}
		}
	}

	return results
}

func (h *Handler) searchUsers(q string, limit int) []SearchUserView {
	query := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(q, "@")))
	if query == "" {
		return nil
	}

	results := make([]SearchUserView, 0, limit)
	seen := make(map[string]struct{}, limit)

	indexerQuery := normalizedUserSearchQuery(q)
	if h.indexerClient != nil && h.indexerClient.Available() && indexerQuery != "" {
		idxResults, err := h.indexerClient.SearchUsers(indexerQuery, 0, limit)
		if err == nil && idxResults != nil {
			for _, hit := range idxResults.Results {
				pubkeyHex := hit.Author
				if pubkeyHex == "" {
					pubkeyHex = hit.ID
				}
				if pubkeyHex == "" {
					continue
				}
				if _, ok := seen[pubkeyHex]; ok {
					continue
				}
				displayName := hit.Name
				if displayName == "" {
					displayName = shortenHex(pubkeyHex)
				}
				results = append(results, SearchUserView{
					DisplayName: displayName,
					Pubkey:      pubkeyHex,
					ShortPubkey: shortenHex(pubkeyHex),
					Initial:     getInitial(displayName),
					Bio:         hit.Bio,
				})
				seen[pubkeyHex] = struct{}{}
				if len(results) >= limit {
					break
				}
			}
		}
	}

	if len(results) < limit {
		if profiles, err := h.db.GetAllProfiles(); err == nil {
			for _, profile := range profiles {
				pubkeyHex := hex.EncodeToString(profile.Pubkey)
				if _, ok := seen[pubkeyHex]; ok || !profileMatchesQuery(profile, query, pubkeyHex) {
					continue
				}
				results = append(results, searchUserView(profile))
				seen[pubkeyHex] = struct{}{}
				if len(results) >= limit {
					return results
				}
			}
		}
	}

	return results
}

func profileMatchesQuery(profile storage.ProfileRow, query, pubkeyHex string) bool {
	return strings.Contains(strings.ToLower(profile.DisplayName), query) ||
		strings.Contains(strings.ToLower(profile.Bio), query) ||
		strings.Contains(strings.ToLower(profile.Website), query) ||
		strings.Contains(strings.ToLower(pubkeyHex), query)
}

func searchUserView(profile storage.ProfileRow) SearchUserView {
	displayName := profile.DisplayName
	if displayName == "" {
		displayName = "Anonymous"
	}
	pubkeyHex := hex.EncodeToString(profile.Pubkey)
	return SearchUserView{
		DisplayName: displayName,
		Pubkey:      pubkeyHex,
		ShortPubkey: shortenHex(pubkeyHex),
		Initial:     getInitial(displayName),
		Bio:         profile.Bio,
		Website:     profile.Website,
	}
}

func normalizedUserSearchQuery(q string) string {
	return strings.TrimSpace(strings.TrimPrefix(q, "@"))
}

func normalizedPostSearchQuery(q string) string {
	return strings.TrimSpace(strings.TrimPrefix(q, "#"))
}
