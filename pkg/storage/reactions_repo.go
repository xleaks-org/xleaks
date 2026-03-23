package storage

import (
	"database/sql"
	"fmt"
	"strings"
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

// UpdateReactionCountTx recalculates reaction counts within an existing transaction.
func (db *DB) UpdateReactionCountTx(tx *sql.Tx, postCID []byte) error {
	var likeCount, replyCount, repostCount int

	err := tx.QueryRow(
		`SELECT COUNT(*) FROM reactions WHERE target = ? AND reaction_type = 'like'`,
		postCID,
	).Scan(&likeCount)
	if err != nil {
		return fmt.Errorf("count likes tx: %w", err)
	}

	err = tx.QueryRow(
		`SELECT COUNT(*) FROM posts WHERE reply_to = ?`,
		postCID,
	).Scan(&replyCount)
	if err != nil {
		return fmt.Errorf("count replies tx: %w", err)
	}

	err = tx.QueryRow(
		`SELECT COUNT(*) FROM posts WHERE repost_of = ?`,
		postCID,
	).Scan(&repostCount)
	if err != nil {
		return fmt.Errorf("count reposts tx: %w", err)
	}

	_, err = tx.Exec(
		`INSERT INTO reaction_counts (post_cid, like_count, reply_count, repost_count)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(post_cid) DO UPDATE SET
		     like_count = excluded.like_count,
		     reply_count = excluded.reply_count,
		     repost_count = excluded.repost_count`,
		postCID, likeCount, replyCount, repostCount,
	)
	if err != nil {
		return fmt.Errorf("upsert reaction counts tx: %w", err)
	}
	return nil
}

// GetFullReactionCounts returns the like, reply, and repost counts for a
// given post CID from the reaction_counts table.
func (db *DB) GetFullReactionCounts(target []byte) (likes, replies, reposts int, err error) {
	err = db.QueryRow(
		`SELECT COALESCE(like_count,0), COALESCE(reply_count,0), COALESCE(repost_count,0) FROM reaction_counts WHERE post_cid = ?`,
		target,
	).Scan(&likes, &replies, &reposts)
	if err == sql.ErrNoRows {
		return 0, 0, 0, nil
	}
	return
}

// ReactionCounts holds the like, reply, and repost counts for a post.
type ReactionCounts struct {
	Likes   int
	Replies int
	Reposts int
}

// GetReactionCountsBatch returns reaction counts for multiple post CIDs in a
// single query. The returned map is keyed by the hex encoding of each CID.
func (db *DB) GetReactionCountsBatch(cids [][]byte) (map[string]ReactionCounts, error) {
	result := make(map[string]ReactionCounts, len(cids))
	if len(cids) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(cids))
	args := make([]interface{}, len(cids))
	for i, cid := range cids {
		placeholders[i] = "?"
		args[i] = cid
	}

	query := fmt.Sprintf(
		`SELECT post_cid, COALESCE(like_count,0), COALESCE(reply_count,0), COALESCE(repost_count,0)
		 FROM reaction_counts WHERE post_cid IN (%s)`,
		strings.Join(placeholders, ","),
	)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("get reaction counts batch: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid []byte
		var rc ReactionCounts
		if err := rows.Scan(&cid, &rc.Likes, &rc.Replies, &rc.Reposts); err != nil {
			return nil, fmt.Errorf("scan reaction counts batch: %w", err)
		}
		result[fmt.Sprintf("%x", cid)] = rc
	}
	return result, rows.Err()
}

// InsertReactionTx inserts a new reaction within an existing transaction,
// ignoring duplicates (same author, target, and reaction_type).
func (db *DB) InsertReactionTx(tx *sql.Tx, cid, author, target []byte, reactionType string, timestamp int64) error {
	_, err := tx.Exec(
		`INSERT OR IGNORE INTO reactions (cid, author, target, reaction_type, timestamp)
		 VALUES (?, ?, ?, ?, ?)`,
		cid, author, target, reactionType, timestamp,
	)
	if err != nil {
		return fmt.Errorf("insert reaction tx: %w", err)
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
