package storage

import (
	"fmt"
)

// SubscriptionRow represents a single row from the subscriptions table.
type SubscriptionRow struct {
	Pubkey        []byte
	FollowedAt    int64
	SyncCompleted bool
}

// AddSubscription adds a subscription (follow) for the given public key.
func (db *DB) AddSubscription(pubkey []byte, followedAt int64) error {
	_, err := db.Exec(
		`INSERT OR IGNORE INTO subscriptions (pubkey, followed_at) VALUES (?, ?)`,
		pubkey, followedAt,
	)
	if err != nil {
		return fmt.Errorf("add subscription: %w", err)
	}
	return nil
}

// RemoveSubscription removes a subscription (unfollow) for the given public key.
func (db *DB) RemoveSubscription(pubkey []byte) error {
	_, err := db.Exec(`DELETE FROM subscriptions WHERE pubkey = ?`, pubkey)
	if err != nil {
		return fmt.Errorf("remove subscription: %w", err)
	}
	return nil
}

// GetSubscriptions returns all current subscriptions ordered by follow date.
func (db *DB) GetSubscriptions() ([]SubscriptionRow, error) {
	rows, err := db.Query(
		`SELECT pubkey, followed_at, sync_completed FROM subscriptions ORDER BY followed_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("get subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []SubscriptionRow
	for rows.Next() {
		var s SubscriptionRow
		var syncInt int
		if err := rows.Scan(&s.Pubkey, &s.FollowedAt, &syncInt); err != nil {
			return nil, fmt.Errorf("scan subscription row: %w", err)
		}
		s.SyncCompleted = syncInt == 1
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

// CountSubscriptions returns the total number of subscriptions without fetching all rows.
func (db *DB) CountSubscriptions() (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM subscriptions").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count subscriptions: %w", err)
	}
	return count, nil
}

// IsSubscribed returns true if the user is currently subscribed to the given
// public key.
func (db *DB) IsSubscribed(pubkey []byte) bool {
	var n int
	err := db.QueryRow(`SELECT 1 FROM subscriptions WHERE pubkey = ? LIMIT 1`, pubkey).Scan(&n)
	return err == nil
}

// InsertFollowEvent records a follow or unfollow event observed on the network.
// On conflict (same author + target), the row is updated to reflect the latest
// action.
func (db *DB) InsertFollowEvent(author, target []byte, action string, timestamp int64) error {
	_, err := db.Exec(
		`INSERT INTO follow_events (author, target, action, timestamp)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(author, target) DO UPDATE SET
		     action = excluded.action,
		     timestamp = excluded.timestamp`,
		author, target, action, timestamp,
	)
	if err != nil {
		return fmt.Errorf("insert follow event: %w", err)
	}
	return nil
}

// GetFollowers returns the public keys of all users who currently follow the
// given pubkey (based on the latest follow_events with action = 'follow').
func (db *DB) GetFollowers(pubkey []byte) ([][]byte, error) {
	rows, err := db.Query(
		`SELECT author FROM follow_events WHERE target = ? AND action = 'follow'`,
		pubkey,
	)
	if err != nil {
		return nil, fmt.Errorf("get followers: %w", err)
	}
	defer rows.Close()

	var followers [][]byte
	for rows.Next() {
		var author []byte
		if err := rows.Scan(&author); err != nil {
			return nil, fmt.Errorf("scan follower: %w", err)
		}
		followers = append(followers, author)
	}
	return followers, rows.Err()
}

// GetFollowing returns the public keys of all users that the given pubkey is
// currently following (based on follow_events with action = 'follow').
func (db *DB) GetFollowing(pubkey []byte) ([][]byte, error) {
	rows, err := db.Query(
		`SELECT target FROM follow_events WHERE author = ? AND action = 'follow'`,
		pubkey,
	)
	if err != nil {
		return nil, fmt.Errorf("get following: %w", err)
	}
	defer rows.Close()

	var following [][]byte
	for rows.Next() {
		var target []byte
		if err := rows.Scan(&target); err != nil {
			return nil, fmt.Errorf("scan following: %w", err)
		}
		following = append(following, target)
	}
	return following, rows.Err()
}

// MarkSyncCompleted sets sync_completed=1 for the given subscription pubkey.
func (db *DB) MarkSyncCompleted(pubkey []byte) error {
	_, err := db.Exec(`UPDATE subscriptions SET sync_completed = 1 WHERE pubkey = ?`, pubkey)
	if err != nil {
		return fmt.Errorf("mark sync completed: %w", err)
	}
	return nil
}

// GetPendingSyncs returns subscriptions where sync_completed=0.
func (db *DB) GetPendingSyncs() ([]SubscriptionRow, error) {
	rows, err := db.Query(
		`SELECT pubkey, followed_at, sync_completed FROM subscriptions WHERE sync_completed = 0 ORDER BY followed_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("get pending syncs: %w", err)
	}
	defer rows.Close()

	var subs []SubscriptionRow
	for rows.Next() {
		var s SubscriptionRow
		var syncInt int
		if err := rows.Scan(&s.Pubkey, &s.FollowedAt, &syncInt); err != nil {
			return nil, fmt.Errorf("scan pending sync row: %w", err)
		}
		s.SyncCompleted = syncInt == 1
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

// UpdateFollowerCount recalculates and upserts the follower and following
// counts for the given public key into the follower_counts table.
func (db *DB) UpdateFollowerCount(pubkey []byte) error {
	var followerCount, followingCount int

	err := db.QueryRow(
		`SELECT COUNT(*) FROM follow_events WHERE target = ? AND action = 'follow'`,
		pubkey,
	).Scan(&followerCount)
	if err != nil {
		return fmt.Errorf("count followers: %w", err)
	}

	err = db.QueryRow(
		`SELECT COUNT(*) FROM follow_events WHERE author = ? AND action = 'follow'`,
		pubkey,
	).Scan(&followingCount)
	if err != nil {
		return fmt.Errorf("count following: %w", err)
	}

	_, err = db.Exec(
		`INSERT INTO follower_counts (pubkey, follower_count, following_count)
		 VALUES (?, ?, ?)
		 ON CONFLICT(pubkey) DO UPDATE SET
		     follower_count = excluded.follower_count,
		     following_count = excluded.following_count`,
		pubkey, followerCount, followingCount,
	)
	if err != nil {
		return fmt.Errorf("upsert follower counts: %w", err)
	}
	return nil
}
