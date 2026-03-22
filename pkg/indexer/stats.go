package indexer

import (
	"fmt"
	"time"

	"github.com/xleaks-org/xleaks/pkg/storage"
)

// NetworkStats holds aggregate network statistics.
type NetworkStats struct {
	TotalPosts     int64
	TotalUsers     int64
	TotalReactions int64
	ActiveUsers24h int64
}

// StatsCollector gathers network-wide statistics from the database.
type StatsCollector struct {
	db *storage.DB
}

// NewStatsCollector creates a new StatsCollector.
func NewStatsCollector(db *storage.DB) *StatsCollector {
	return &StatsCollector{db: db}
}

// GetStats queries the database for aggregate network statistics.
func (sc *StatsCollector) GetStats() (*NetworkStats, error) {
	stats := &NetworkStats{}

	// Total posts.
	err := sc.db.QueryRow(`SELECT COUNT(*) FROM posts`).Scan(&stats.TotalPosts)
	if err != nil {
		return nil, fmt.Errorf("count posts: %w", err)
	}

	// Total unique users (distinct authors in posts + profiles).
	err = sc.db.QueryRow(`SELECT COUNT(*) FROM profiles`).Scan(&stats.TotalUsers)
	if err != nil {
		return nil, fmt.Errorf("count users: %w", err)
	}

	// Total reactions.
	err = sc.db.QueryRow(`SELECT COUNT(*) FROM reactions`).Scan(&stats.TotalReactions)
	if err != nil {
		return nil, fmt.Errorf("count reactions: %w", err)
	}

	// Active users in last 24h (distinct authors who posted).
	since := time.Now().Add(-24 * time.Hour).UnixMilli()
	err = sc.db.QueryRow(
		`SELECT COUNT(DISTINCT author) FROM posts WHERE timestamp >= ?`,
		since,
	).Scan(&stats.ActiveUsers24h)
	if err != nil {
		return nil, fmt.Errorf("count active users: %w", err)
	}

	return stats, nil
}
