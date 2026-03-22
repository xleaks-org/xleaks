package storage

import (
	"fmt"
)

// ReactionRow represents a single row from the reactions table.
type ReactionRow struct {
	CID          []byte
	Author       []byte
	Target       []byte
	ReactionType string
	Timestamp    int64
}

// InsertReaction inserts a new reaction, ignoring duplicates (same author,
// target, and reaction_type).
func (db *DB) InsertReaction(cid, author, target []byte, reactionType string, timestamp int64) error {
	_, err := db.Exec(
		`INSERT OR IGNORE INTO reactions (cid, author, target, reaction_type, timestamp)
		 VALUES (?, ?, ?, ?, ?)`,
		cid, author, target, reactionType, timestamp,
	)
	if err != nil {
		return fmt.Errorf("insert reaction: %w", err)
	}
	return nil
}

// GetReactions returns all reactions targeting the given post CID.
func (db *DB) GetReactions(target []byte) ([]ReactionRow, error) {
	rows, err := db.Query(
		`SELECT cid, author, target, reaction_type, timestamp
		 FROM reactions
		 WHERE target = ?
		 ORDER BY timestamp ASC`,
		target,
	)
	if err != nil {
		return nil, fmt.Errorf("get reactions: %w", err)
	}
	defer rows.Close()

	var reactions []ReactionRow
	for rows.Next() {
		var r ReactionRow
		if err := rows.Scan(&r.CID, &r.Author, &r.Target, &r.ReactionType, &r.Timestamp); err != nil {
			return nil, fmt.Errorf("scan reaction row: %w", err)
		}
		reactions = append(reactions, r)
	}
	return reactions, rows.Err()
}

// GetReactionCount returns the materialized like count for a given post CID
// from the reaction_counts table.
func (db *DB) GetReactionCount(target []byte) (likes int, err error) {
	err = db.QueryRow(
		`SELECT COALESCE(like_count, 0) FROM reaction_counts WHERE post_cid = ?`,
		target,
	).Scan(&likes)
	if err != nil {
		// No row means zero likes.
		likes = 0
		err = nil
	}
	return likes, err
}

// UpdateReactionCount recalculates the like, reply, and repost counts for a
// given post CID and upserts them into the reaction_counts table.
func (db *DB) UpdateReactionCount(postCID []byte) error {
	var likeCount, replyCount, repostCount int

	err := db.QueryRow(
		`SELECT COUNT(*) FROM reactions WHERE target = ? AND reaction_type = 'like'`,
		postCID,
	).Scan(&likeCount)
	if err != nil {
		return fmt.Errorf("count likes: %w", err)
	}

	err = db.QueryRow(
		`SELECT COUNT(*) FROM posts WHERE reply_to = ?`,
		postCID,
	).Scan(&replyCount)
	if err != nil {
		return fmt.Errorf("count replies: %w", err)
	}

	err = db.QueryRow(
		`SELECT COUNT(*) FROM posts WHERE repost_of = ?`,
		postCID,
	).Scan(&repostCount)
	if err != nil {
		return fmt.Errorf("count reposts: %w", err)
	}

	_, err = db.Exec(
		`INSERT INTO reaction_counts (post_cid, like_count, reply_count, repost_count)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(post_cid) DO UPDATE SET
		     like_count = excluded.like_count,
		     reply_count = excluded.reply_count,
		     repost_count = excluded.repost_count`,
		postCID, likeCount, replyCount, repostCount,
	)
	if err != nil {
		return fmt.Errorf("upsert reaction counts: %w", err)
	}
	return nil
}

// HasReacted returns true if the given author has already reacted to the
// target with the specified reaction type.
func (db *DB) HasReacted(author, target []byte, reactionType string) bool {
	var n int
	err := db.QueryRow(
		`SELECT 1 FROM reactions
		 WHERE author = ? AND target = ? AND reaction_type = ?
		 LIMIT 1`,
		author, target, reactionType,
	).Scan(&n)
	return err == nil
}
