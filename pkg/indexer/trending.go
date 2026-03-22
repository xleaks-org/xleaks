package indexer

import (
	"fmt"
	"time"

	"github.com/xleaks-org/xleaks/pkg/storage"
)

// TrendingEngine computes trending content over rolling time windows.
type TrendingEngine struct {
	db *storage.DB
}

// NewTrendingEngine creates a new TrendingEngine.
func NewTrendingEngine(db *storage.DB) *TrendingEngine {
	return &TrendingEngine{db: db}
}

// TrendingPost represents a post with its engagement metrics and score.
type TrendingPost struct {
	CIDHex      string
	Author      string
	Content     string
	LikeCount   int
	RepostCount int
	ReplyCount  int
	Score       float64
	Timestamp   int64
}

// TrendingTag represents a hashtag with its usage count.
type TrendingTag struct {
	Tag   string
	Count int
}

// parseWindow converts a window string like "1h", "6h", "24h", "7d" into a
// Unix millisecond timestamp representing the start of the window.
func parseWindow(window string) (int64, error) {
	now := time.Now()
	switch window {
	case "1h":
		return now.Add(-1 * time.Hour).UnixMilli(), nil
	case "6h":
		return now.Add(-6 * time.Hour).UnixMilli(), nil
	case "24h":
		return now.Add(-24 * time.Hour).UnixMilli(), nil
	case "7d":
		return now.Add(-7 * 24 * time.Hour).UnixMilli(), nil
	default:
		return 0, fmt.Errorf("unsupported window: %s (valid: 1h, 6h, 24h, 7d)", window)
	}
}

// GetTrendingPosts returns the most engaged posts in the given time window.
// Score is computed as: like_count + repost_count*2 + reply_count*3.
func (te *TrendingEngine) GetTrendingPosts(window string, limit int) ([]TrendingPost, error) {
	since, err := parseWindow(window)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 20
	}

	rows, err := te.db.Query(
		`SELECT p.cid, p.author, p.content, p.timestamp,
		        COALESCE(rc.like_count, 0) AS like_count,
		        COALESCE(rc.repost_count, 0) AS repost_count,
		        COALESCE(rc.reply_count, 0) AS reply_count,
		        (COALESCE(rc.like_count, 0) + COALESCE(rc.repost_count, 0) * 2 + COALESCE(rc.reply_count, 0) * 3) AS score
		 FROM posts p
		 LEFT JOIN reaction_counts rc ON p.cid = rc.post_cid
		 WHERE p.timestamp >= ?
		   AND p.reply_to IS NULL
		   AND p.repost_of IS NULL
		 ORDER BY score DESC, p.timestamp DESC
		 LIMIT ?`,
		since, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get trending posts: %w", err)
	}
	defer rows.Close()

	var posts []TrendingPost
	for rows.Next() {
		var tp TrendingPost
		var cidBytes, authorBytes []byte
		if err := rows.Scan(
			&cidBytes, &authorBytes, &tp.Content, &tp.Timestamp,
			&tp.LikeCount, &tp.RepostCount, &tp.ReplyCount, &tp.Score,
		); err != nil {
			return nil, fmt.Errorf("scan trending post: %w", err)
		}
		tp.CIDHex = fmt.Sprintf("%x", cidBytes)
		tp.Author = fmt.Sprintf("%x", authorBytes)
		posts = append(posts, tp)
	}
	return posts, rows.Err()
}

// GetTrendingTags returns the most used hashtags in the given time window.
func (te *TrendingEngine) GetTrendingTags(window string, limit int) ([]TrendingTag, error) {
	since, err := parseWindow(window)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 20
	}

	rows, err := te.db.Query(
		`SELECT pt.tag, COUNT(*) AS cnt
		 FROM post_tags pt
		 INNER JOIN posts p ON pt.post_cid = p.cid
		 WHERE p.timestamp >= ?
		 GROUP BY pt.tag
		 ORDER BY cnt DESC
		 LIMIT ?`,
		since, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get trending tags: %w", err)
	}
	defer rows.Close()

	var tags []TrendingTag
	for rows.Next() {
		var t TrendingTag
		if err := rows.Scan(&t.Tag, &t.Count); err != nil {
			return nil, fmt.Errorf("scan trending tag: %w", err)
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}
