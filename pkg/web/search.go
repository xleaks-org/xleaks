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

	if len(results) == 0 && h.indexerClient != nil && h.indexerClient.Available() {
		idxResults, err := h.indexerClient.SearchPosts(q, 1, limit)
		if err == nil && idxResults != nil {
			for _, hit := range idxResults.Results {
				results = append(results, PostView{
					ID:            hit.ID,
					AuthorName:    shortenHex(hit.Author),
					AuthorInitial: getInitial(hit.Author),
					AuthorPubkey:  hit.Author,
					ShortPubkey:   shortenHex(hit.Author),
					Content:       hit.Content,
					RelativeTime:  "indexer",
				})
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

	if profiles, err := h.db.GetAllProfiles(); err == nil {
		for _, profile := range profiles {
			pubkeyHex := hex.EncodeToString(profile.Pubkey)
			if !profileMatchesQuery(profile, query, pubkeyHex) {
				continue
			}
			results = append(results, searchUserView(profile))
			seen[pubkeyHex] = struct{}{}
			if len(results) >= limit {
				return results
			}
		}
	}

	if h.indexerClient != nil && h.indexerClient.Available() && len(results) < limit {
		idxResults, err := h.indexerClient.SearchUsers(q, 1, limit)
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
				})
				seen[pubkeyHex] = struct{}{}
				if len(results) >= limit {
					break
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
