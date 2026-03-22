package indexer

import (
	"fmt"
	"strconv"

	"github.com/blevesearch/bleve/v2"
)

// SearchIndex provides full-text search over posts and profiles using Bleve.
type SearchIndex struct {
	index bleve.Index
}

// SearchResult represents a single search hit.
type SearchResult struct {
	ID    string
	Score float64
}

// postDocument is the internal document shape indexed for posts.
type postDocument struct {
	Author    string   `json:"author"`
	Content   string   `json:"content"`
	Tags      []string `json:"tags"`
	Timestamp int64    `json:"timestamp"`
	Type      string   `json:"type"`
}

// profileDocument is the internal document shape indexed for profiles.
type profileDocument struct {
	DisplayName string `json:"display_name"`
	Bio         string `json:"bio"`
	Type        string `json:"type"`
}

// NewSearchIndex opens or creates a Bleve index at the given path.
func NewSearchIndex(path string) (*SearchIndex, error) {
	idx, err := bleve.Open(path)
	if err != nil {
		// Index doesn't exist yet; create a new one.
		mapping := bleve.NewIndexMapping()
		idx, err = bleve.New(path, mapping)
		if err != nil {
			return nil, fmt.Errorf("create search index: %w", err)
		}
	}
	return &SearchIndex{index: idx}, nil
}

// Close closes the underlying Bleve index.
func (si *SearchIndex) Close() error {
	return si.index.Close()
}

// IndexPost adds a post to the search index.
func (si *SearchIndex) IndexPost(id string, author string, content string, tags []string, timestamp int64) error {
	doc := postDocument{
		Author:    author,
		Content:   content,
		Tags:      tags,
		Timestamp: timestamp,
		Type:      "post",
	}
	return si.index.Index(id, doc)
}

// IndexProfile adds a profile to the search index.
func (si *SearchIndex) IndexProfile(pubkeyHex string, displayName string, bio string) error {
	doc := profileDocument{
		DisplayName: displayName,
		Bio:         bio,
		Type:        "profile",
	}
	return si.index.Index(pubkeyHex, doc)
}

// SearchPosts searches for posts matching the query. Returns results and
// the total number of matches.
func (si *SearchIndex) SearchPosts(query string, page int, pageSize int) ([]SearchResult, int, error) {
	q := bleve.NewQueryStringQuery(query)
	req := bleve.NewSearchRequestOptions(q, pageSize, page*pageSize, false)

	// Add a type filter to only return posts.
	typeQuery := bleve.NewTermQuery("post")
	typeQuery.SetField("type")
	combined := bleve.NewConjunctionQuery(q, typeQuery)
	req = bleve.NewSearchRequestOptions(combined, pageSize, page*pageSize, false)

	res, err := si.index.Search(req)
	if err != nil {
		return nil, 0, fmt.Errorf("search posts: %w", err)
	}

	results := make([]SearchResult, 0, len(res.Hits))
	for _, hit := range res.Hits {
		results = append(results, SearchResult{
			ID:    hit.ID,
			Score: hit.Score,
		})
	}
	return results, int(res.Total), nil
}

// SearchUsers searches for profiles matching the query. Returns results and
// the total number of matches.
func (si *SearchIndex) SearchUsers(query string, page int, pageSize int) ([]SearchResult, int, error) {
	q := bleve.NewQueryStringQuery(query)

	// Add a type filter to only return profiles.
	typeQuery := bleve.NewTermQuery("profile")
	typeQuery.SetField("type")
	combined := bleve.NewConjunctionQuery(q, typeQuery)
	req := bleve.NewSearchRequestOptions(combined, pageSize, page*pageSize, false)

	res, err := si.index.Search(req)
	if err != nil {
		return nil, 0, fmt.Errorf("search users: %w", err)
	}

	results := make([]SearchResult, 0, len(res.Hits))
	for _, hit := range res.Hits {
		results = append(results, SearchResult{
			ID:    hit.ID,
			Score: hit.Score,
		})
	}
	return results, int(res.Total), nil
}

// parsePageParam converts a string page number to int, defaulting to 0.
func parsePageParam(s string) int {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
