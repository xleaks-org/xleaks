package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// PostRow represents a single row from the posts table.
type PostRow struct {
	CID        []byte
	Author     []byte
	Content    string
	ReplyTo    []byte
	RepostOf   []byte
	Timestamp  int64
	Signature  []byte
	ReceivedAt int64
}

// InsertPost inserts a new post into the posts table.
func (db *DB) InsertPost(cid, author []byte, content string, replyTo, repostOf []byte, timestamp int64, signature []byte) error {
	receivedAt := time.Now().UnixMilli()
	_, err := db.Exec(
		`INSERT INTO posts (cid, author, content, reply_to, repost_of, timestamp, signature, received_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		cid, author, content, nilIfEmpty(replyTo), nilIfEmpty(repostOf), timestamp, signature, receivedAt,
	)
	if err != nil {
		return fmt.Errorf("insert post: %w", err)
	}
	return nil
}

// GetPost retrieves a single post by its CID.
func (db *DB) GetPost(cid []byte) (*PostRow, error) {
	row := db.QueryRow(
		`SELECT cid, author, content, reply_to, repost_of, timestamp, signature, received_at
		 FROM posts WHERE cid = ?`, cid,
	)
	return scanPostRow(row)
}

// GetPostsByAuthor returns posts by a given author, paginated by timestamp
// (descending). Pass 0 for before to start from the most recent.
func (db *DB) GetPostsByAuthor(author []byte, before int64, limit int) ([]PostRow, error) {
	if before == 0 {
		before = time.Now().UnixMilli() + 1
	}
	rows, err := db.Query(
		`SELECT cid, author, content, reply_to, repost_of, timestamp, signature, received_at
		 FROM posts
		 WHERE author = ? AND timestamp < ?
		 ORDER BY timestamp DESC
		 LIMIT ?`,
		author, before, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get posts by author: %w", err)
	}
	defer rows.Close()
	return scanPostRows(rows)
}

// GetFeed returns posts from the given set of followed authors, paginated by
// timestamp (descending). Pass 0 for before to start from the most recent.
func (db *DB) GetFeed(followedAuthors [][]byte, before int64, limit int) ([]PostRow, error) {
	if len(followedAuthors) == 0 {
		return nil, nil
	}
	if before == 0 {
		before = time.Now().UnixMilli() + 1
	}

	placeholders := make([]string, len(followedAuthors))
	args := make([]interface{}, 0, len(followedAuthors)+2)
	for i, a := range followedAuthors {
		placeholders[i] = "?"
		args = append(args, a)
	}
	args = append(args, before, limit)

	query := fmt.Sprintf(
		`SELECT cid, author, content, reply_to, repost_of, timestamp, signature, received_at
		 FROM posts
		 WHERE author IN (%s) AND timestamp < ?
		 ORDER BY timestamp DESC
		 LIMIT ?`,
		strings.Join(placeholders, ","),
	)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("get feed: %w", err)
	}
	defer rows.Close()
	return scanPostRows(rows)
}

// GetThread returns all direct replies to the given post CID, ordered by
// timestamp ascending.
func (db *DB) GetThread(cid []byte) ([]PostRow, error) {
	rows, err := db.Query(
		`SELECT cid, author, content, reply_to, repost_of, timestamp, signature, received_at
		 FROM posts
		 WHERE reply_to = ?
		 ORDER BY timestamp ASC`,
		cid,
	)
	if err != nil {
		return nil, fmt.Errorf("get thread: %w", err)
	}
	defer rows.Close()
	return scanPostRows(rows)
}

// PostExists returns true if a post with the given CID exists in the database.
func (db *DB) PostExists(cid []byte) bool {
	var n int
	err := db.QueryRow(`SELECT 1 FROM posts WHERE cid = ? LIMIT 1`, cid).Scan(&n)
	return err == nil
}

// scanPostRow scans a single *sql.Row into a PostRow.
func scanPostRow(row *sql.Row) (*PostRow, error) {
	var p PostRow
	err := row.Scan(&p.CID, &p.Author, &p.Content, &p.ReplyTo, &p.RepostOf, &p.Timestamp, &p.Signature, &p.ReceivedAt)
	if err != nil {
		return nil, fmt.Errorf("scan post row: %w", err)
	}
	return &p, nil
}

// scanPostRows scans multiple rows into a slice of PostRow.
func scanPostRows(rows *sql.Rows) ([]PostRow, error) {
	var posts []PostRow
	for rows.Next() {
		var p PostRow
		if err := rows.Scan(&p.CID, &p.Author, &p.Content, &p.ReplyTo, &p.RepostOf, &p.Timestamp, &p.Signature, &p.ReceivedAt); err != nil {
			return nil, fmt.Errorf("scan post rows: %w", err)
		}
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

// InsertPostTags bulk-inserts rows into the post_tags table, linking a post to
// its hashtags. Duplicate (post_cid, tag) pairs are silently ignored.
func (db *DB) InsertPostTags(postCID []byte, tags []string) error {
	if len(tags) == 0 {
		return nil
	}

	// Build a single INSERT with multiple value tuples for efficiency.
	placeholders := make([]string, len(tags))
	args := make([]interface{}, 0, len(tags)*2)
	for i, tag := range tags {
		placeholders[i] = "(?, ?)"
		args = append(args, postCID, tag)
	}

	query := fmt.Sprintf(
		`INSERT OR IGNORE INTO post_tags (post_cid, tag) VALUES %s`,
		strings.Join(placeholders, ", "),
	)

	if _, err := db.Exec(query, args...); err != nil {
		return fmt.Errorf("insert post tags: %w", err)
	}
	return nil
}

// GetPostsByTag returns posts that have been tagged with the given hashtag,
// paginated by timestamp (descending). Pass 0 for before to start from the
// most recent.
func (db *DB) GetPostsByTag(tag string, before int64, limit int) ([]PostRow, error) {
	if before == 0 {
		before = time.Now().UnixMilli() + 1
	}
	rows, err := db.Query(
		`SELECT p.cid, p.author, p.content, p.reply_to, p.repost_of, p.timestamp, p.signature, p.received_at
		 FROM posts p
		 INNER JOIN post_tags pt ON p.cid = pt.post_cid
		 WHERE pt.tag = ? AND p.timestamp < ?
		 ORDER BY p.timestamp DESC
		 LIMIT ?`,
		tag, before, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get posts by tag: %w", err)
	}
	defer rows.Close()
	return scanPostRows(rows)
}

// InsertPostTx inserts a new post within an existing transaction.
func (db *DB) InsertPostTx(tx *sql.Tx, cid, author []byte, content string, replyTo, repostOf []byte, timestamp int64, signature []byte) error {
	receivedAt := time.Now().UnixMilli()
	_, err := tx.Exec(
		`INSERT INTO posts (cid, author, content, reply_to, repost_of, timestamp, signature, received_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		cid, author, content, nilIfEmpty(replyTo), nilIfEmpty(repostOf), timestamp, signature, receivedAt,
	)
	if err != nil {
		return fmt.Errorf("insert post tx: %w", err)
	}
	return nil
}

// InsertPostTagsTx bulk-inserts post tag rows within an existing transaction.
func (db *DB) InsertPostTagsTx(tx *sql.Tx, postCID []byte, tags []string) error {
	if len(tags) == 0 {
		return nil
	}

	placeholders := make([]string, len(tags))
	args := make([]interface{}, 0, len(tags)*2)
	for i, tag := range tags {
		placeholders[i] = "(?, ?)"
		args = append(args, postCID, tag)
	}

	query := fmt.Sprintf(
		`INSERT OR IGNORE INTO post_tags (post_cid, tag) VALUES %s`,
		strings.Join(placeholders, ", "),
	)

	if _, err := tx.Exec(query, args...); err != nil {
		return fmt.Errorf("insert post tags tx: %w", err)
	}
	return nil
}

// GetAllDescendantReplies returns all posts that are replies to any of the given
// CIDs. This is used to batch-fetch an entire thread tree in a single query
// instead of issuing per-node queries.
func (db *DB) GetAllDescendantReplies(rootCID []byte) ([]PostRow, error) {
	// We use a recursive CTE to get all replies in the tree rooted at rootCID.
	rows, err := db.Query(
		`WITH RECURSIVE thread(cid) AS (
		     SELECT cid FROM posts WHERE reply_to = ?
		     UNION ALL
		     SELECT p.cid FROM posts p INNER JOIN thread t ON p.reply_to = t.cid
		 )
		 SELECT p.cid, p.author, p.content, p.reply_to, p.repost_of, p.timestamp, p.signature, p.received_at
		 FROM posts p
		 INNER JOIN thread t ON p.cid = t.cid
		 ORDER BY p.timestamp ASC`,
		rootCID,
	)
	if err != nil {
		return nil, fmt.Errorf("get all descendant replies: %w", err)
	}
	defer rows.Close()
	return scanPostRows(rows)
}

// CountPostsByAuthor returns the number of posts by the given author.
func (db *DB) CountPostsByAuthor(author []byte) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM posts WHERE author = ?", author).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count posts by author: %w", err)
	}
	return count, nil
}

// TagCount represents a hashtag and its usage count.
type TagCount struct {
	Tag   string
	Count int
}

// GetTrendingTags returns the most frequently used hashtags, ordered by
// occurrence count descending.
func (db *DB) GetTrendingTags(limit int) ([]TagCount, error) {
	rows, err := db.Query(
		`SELECT tag, COUNT(*) AS cnt FROM post_tags GROUP BY tag ORDER BY cnt DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get trending tags: %w", err)
	}
	defer rows.Close()

	var tags []TagCount
	for rows.Next() {
		var tc TagCount
		if err := rows.Scan(&tc.Tag, &tc.Count); err != nil {
			return nil, fmt.Errorf("scan trending tag: %w", err)
		}
		tags = append(tags, tc)
	}
	return tags, rows.Err()
}

// SearchPostsByContent returns posts whose content matches the given query
// using a SQL LIKE search, paginated by timestamp descending.
func (db *DB) SearchPostsByContent(query string, limit int) ([]PostRow, error) {
	pattern := "%" + query + "%"
	rows, err := db.Query(
		`SELECT cid, author, content, reply_to, repost_of, timestamp, signature, received_at
		 FROM posts
		 WHERE content LIKE ?
		 ORDER BY timestamp DESC
		 LIMIT ?`,
		pattern, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search posts by content: %w", err)
	}
	defer rows.Close()
	return scanPostRows(rows)
}

// nilIfEmpty returns nil if the byte slice is empty, otherwise returns the
// slice as-is. This ensures that empty byte slices are stored as NULL in
// SQLite rather than as empty blobs.
func nilIfEmpty(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return b
}
