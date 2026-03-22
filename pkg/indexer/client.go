package indexer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// IndexerClient is an HTTP client for querying remote indexer nodes.
// It tries each known indexer in order, skipping unavailable ones,
// and caches responses for 60 seconds.
type IndexerClient struct {
	mu            sync.RWMutex
	httpClient    *http.Client
	knownIndexers []string // base URLs like "http://host:7471"
	cache         map[string]*cachedResponse
}

type cachedResponse struct {
	data      interface{}
	expiresAt time.Time
}

// ClientSearchResponse is the response returned by the indexer search endpoint.
type ClientSearchResponse struct {
	Results []ClientSearchHit `json:"results"`
	Total   int               `json:"total"`
}

// ClientSearchHit represents a single search result from the indexer.
type ClientSearchHit struct {
	ID      string  `json:"id"`
	Type    string  `json:"type"` // "post" or "user"
	Score   float64 `json:"score"`
	Content string  `json:"content,omitempty"`
	Author  string  `json:"author,omitempty"`
	Name    string  `json:"name,omitempty"`
}

// ClientTrendingResponse is the response returned by the indexer trending endpoint.
type ClientTrendingResponse struct {
	Posts  []ClientTrendingPost `json:"posts"`
	Tags   []ClientTrendingTag  `json:"tags"`
	Window string               `json:"window"`
}

// ClientTrendingPost represents a trending post from the indexer.
type ClientTrendingPost struct {
	CID         string  `json:"cid_hex,omitempty"`
	Author      string  `json:"author,omitempty"`
	Content     string  `json:"content,omitempty"`
	LikeCount   int     `json:"like_count"`
	RepostCount int     `json:"repost_count"`
	ReplyCount  int     `json:"reply_count"`
	Score       float64 `json:"score"`
	Timestamp   int64   `json:"timestamp"`
}

// ClientTrendingTag represents a trending tag from the indexer.
type ClientTrendingTag struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// ClientPublisher represents a publisher from the explore endpoint.
type ClientPublisher struct {
	Pubkey         string `json:"pubkey"`
	FollowerCount  int    `json:"follower_count"`
	FollowingCount int    `json:"following_count"`
	DisplayName    string `json:"display_name"`
	Bio            string `json:"bio"`
}

// NewIndexerClient creates a new IndexerClient with an HTTP client
// configured with a 5-second timeout.
func NewIndexerClient() *IndexerClient {
	return &IndexerClient{
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		knownIndexers: make([]string, 0),
		cache:         make(map[string]*cachedResponse),
	}
}

// AddIndexer adds a single indexer base URL to the list of known indexers.
func (c *IndexerClient) AddIndexer(baseURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Avoid duplicates.
	for _, u := range c.knownIndexers {
		if u == baseURL {
			return
		}
	}
	c.knownIndexers = append(c.knownIndexers, baseURL)
}

// SetIndexers replaces the list of known indexer base URLs.
func (c *IndexerClient) SetIndexers(urls []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.knownIndexers = make([]string, len(urls))
	copy(c.knownIndexers, urls)
}

// Available returns true if at least one indexer is known.
func (c *IndexerClient) Available() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.knownIndexers) > 0
}

// SearchPosts queries the first available indexer for post search results.
func (c *IndexerClient) SearchPosts(query string, page, pageSize int) (*ClientSearchResponse, error) {
	cacheKey := fmt.Sprintf("search:posts:%s:%d:%d", query, page, pageSize)
	if cached := c.getFromCache(cacheKey); cached != nil {
		return cached.(*ClientSearchResponse), nil
	}

	params := url.Values{}
	params.Set("q", query)
	params.Set("type", "posts")
	params.Set("page", strconv.Itoa(page))
	params.Set("page_size", strconv.Itoa(pageSize))

	var raw map[string]json.RawMessage
	if err := c.queryIndexer("/api/search", params, &raw); err != nil {
		return nil, err
	}

	resp := &ClientSearchResponse{}
	if results, ok := raw["results"]; ok {
		json.Unmarshal(results, &resp.Results)
	}
	if total, ok := raw["total"]; ok {
		json.Unmarshal(total, &resp.Total)
	}

	c.putInCache(cacheKey, resp)
	return resp, nil
}

// SearchUsers queries the first available indexer for user search results.
func (c *IndexerClient) SearchUsers(query string, page, pageSize int) (*ClientSearchResponse, error) {
	cacheKey := fmt.Sprintf("search:users:%s:%d:%d", query, page, pageSize)
	if cached := c.getFromCache(cacheKey); cached != nil {
		return cached.(*ClientSearchResponse), nil
	}

	params := url.Values{}
	params.Set("q", query)
	params.Set("type", "users")
	params.Set("page", strconv.Itoa(page))
	params.Set("page_size", strconv.Itoa(pageSize))

	var raw map[string]json.RawMessage
	if err := c.queryIndexer("/api/search", params, &raw); err != nil {
		return nil, err
	}

	resp := &ClientSearchResponse{}
	if results, ok := raw["results"]; ok {
		json.Unmarshal(results, &resp.Results)
	}
	if total, ok := raw["total"]; ok {
		json.Unmarshal(total, &resp.Total)
	}

	c.putInCache(cacheKey, resp)
	return resp, nil
}

// GetTrending queries the first available indexer for trending posts and tags.
func (c *IndexerClient) GetTrending(window string, limit int) (*ClientTrendingResponse, error) {
	cacheKey := fmt.Sprintf("trending:%s:%d", window, limit)
	if cached := c.getFromCache(cacheKey); cached != nil {
		return cached.(*ClientTrendingResponse), nil
	}

	resp := &ClientTrendingResponse{Window: window}

	// Fetch trending posts.
	postParams := url.Values{}
	postParams.Set("window", window)
	postParams.Set("limit", strconv.Itoa(limit))
	postParams.Set("type", "posts")

	var postRaw map[string]json.RawMessage
	if err := c.queryIndexer("/api/trending", postParams, &postRaw); err == nil {
		if posts, ok := postRaw["posts"]; ok {
			json.Unmarshal(posts, &resp.Posts)
		}
	}

	// Fetch trending tags.
	tagParams := url.Values{}
	tagParams.Set("window", window)
	tagParams.Set("limit", strconv.Itoa(limit))
	tagParams.Set("type", "tags")

	var tagRaw map[string]json.RawMessage
	if err := c.queryIndexer("/api/trending", tagParams, &tagRaw); err == nil {
		if tags, ok := tagRaw["tags"]; ok {
			json.Unmarshal(tags, &resp.Tags)
		}
	}

	c.putInCache(cacheKey, resp)
	return resp, nil
}

// GetExplorePublishers queries the first available indexer for suggested publishers.
func (c *IndexerClient) GetExplorePublishers(limit int) ([]ClientPublisher, error) {
	cacheKey := fmt.Sprintf("explore:publishers:%d", limit)
	if cached := c.getFromCache(cacheKey); cached != nil {
		return cached.([]ClientPublisher), nil
	}

	params := url.Values{}
	params.Set("limit", strconv.Itoa(limit))

	var raw map[string]json.RawMessage
	if err := c.queryIndexer("/api/explore/publishers", params, &raw); err != nil {
		return nil, err
	}

	var publishers []ClientPublisher
	if pubs, ok := raw["publishers"]; ok {
		json.Unmarshal(pubs, &publishers)
	}
	if publishers == nil {
		publishers = []ClientPublisher{}
	}

	c.putInCache(cacheKey, publishers)
	return publishers, nil
}

// queryIndexer tries each known indexer in order, returning the first
// successful response. Unavailable indexers are skipped.
func (c *IndexerClient) queryIndexer(path string, params url.Values, result interface{}) error {
	c.mu.RLock()
	indexers := make([]string, len(c.knownIndexers))
	copy(indexers, c.knownIndexers)
	c.mu.RUnlock()

	if len(indexers) == 0 {
		return fmt.Errorf("no indexer nodes available")
	}

	var lastErr error
	for _, base := range indexers {
		u := base + path + "?" + params.Encode()
		resp, err := c.httpClient.Get(u)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("indexer %s returned status %d", base, resp.StatusCode)
			continue
		}

		err = json.NewDecoder(resp.Body).Decode(result)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		return nil
	}

	return fmt.Errorf("all indexers unavailable: %w", lastErr)
}

// getFromCache returns a cached value if it exists and has not expired.
func (c *IndexerClient) getFromCache(key string) interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.cache[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil
	}
	return entry.data
}

// putInCache stores a value in the cache with a 60-second TTL.
func (c *IndexerClient) putInCache(key string, data interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[key] = &cachedResponse{
		data:      data,
		expiresAt: time.Now().Add(60 * time.Second),
	}
}
