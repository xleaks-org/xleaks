package social

import (
	"fmt"
)

// GetRepostCount returns the number of reposts for the given post CID.
// This recalculates counts by querying the database.
func (s *PostService) GetRepostCount(postCID []byte) (int, error) {
	var repostCount int
	err := s.storage.QueryRow(
		`SELECT COUNT(*) FROM posts WHERE repost_of = ?`,
		postCID,
	).Scan(&repostCount)
	if err != nil {
		return 0, fmt.Errorf("count reposts: %w", err)
	}
	return repostCount, nil
}

// IsReposted returns true if the author has already reposted the given post.
func (s *PostService) IsReposted(author, postCID []byte) bool {
	var n int
	err := s.storage.QueryRow(
		`SELECT 1 FROM posts WHERE author = ? AND repost_of = ? LIMIT 1`,
		author, postCID,
	).Scan(&n)
	return err == nil
}
