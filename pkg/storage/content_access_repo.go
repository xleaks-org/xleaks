package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// TrackContentAccess inserts or updates a content_access row. If the CID
// already exists, the last_accessed time and access_count are updated. The
// is_pinned flag is elevated when requested, but ordinary access does not
// implicitly clear an existing pin.
func (db *DB) TrackContentAccess(cid []byte, isPinned bool) error {
	now := time.Now().UnixMilli()
	pinnedInt := 0
	if isPinned {
		pinnedInt = 1
	}
	_, err := db.Exec(
		`INSERT INTO content_access (cid, last_accessed, access_count, is_pinned)
		 VALUES (?, ?, 1, ?)
		 ON CONFLICT(cid) DO UPDATE SET
		     last_accessed = excluded.last_accessed,
		     access_count = content_access.access_count + 1,
		     is_pinned = CASE
		         WHEN content_access.is_pinned = 1 OR excluded.is_pinned = 1 THEN 1
		         ELSE 0
		     END`,
		cid, now, pinnedInt,
	)
	if err != nil {
		return fmt.Errorf("track content access: %w", err)
	}
	return nil
}

// SetContentPinned explicitly sets the pin state for a tracked content row.
func (db *DB) SetContentPinned(cid []byte, pinned bool) error {
	now := time.Now().UnixMilli()
	pinnedInt := 0
	if pinned {
		pinnedInt = 1
	}
	_, err := db.Exec(
		`INSERT INTO content_access (cid, last_accessed, access_count, is_pinned)
		 VALUES (?, ?, 1, ?)
		 ON CONFLICT(cid) DO UPDATE SET
		     is_pinned = excluded.is_pinned`,
		cid, now, pinnedInt,
	)
	if err != nil {
		return fmt.Errorf("set content pinned: %w", err)
	}
	return nil
}

// GetLRUContent returns CIDs of the least recently used non-pinned content,
// ordered by last_accessed ascending (oldest first), limited to `limit` rows.
func (db *DB) GetLRUContent(limit int) ([][]byte, error) {
	rows, err := db.Query(
		`SELECT cid FROM content_access
		 WHERE is_pinned = 0
		 ORDER BY last_accessed ASC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get LRU content: %w", err)
	}
	defer rows.Close()

	var cids [][]byte
	for rows.Next() {
		var cid []byte
		if err := rows.Scan(&cid); err != nil {
			return nil, fmt.Errorf("scan LRU content row: %w", err)
		}
		cids = append(cids, cid)
	}
	return cids, rows.Err()
}

// DeleteContentAccess removes a content_access row by CID.
func (db *DB) DeleteContentAccess(cid []byte) error {
	_, err := db.Exec(`DELETE FROM content_access WHERE cid = ?`, cid)
	if err != nil {
		return fmt.Errorf("delete content access: %w", err)
	}
	return nil
}

// IsLocalIdentity reports whether the public key belongs to a locally stored identity.
func (db *DB) IsLocalIdentity(pubkey []byte) (bool, error) {
	if len(pubkey) == 0 {
		return false, nil
	}
	var exists int
	err := db.QueryRow(`SELECT 1 FROM identities WHERE pubkey = ? LIMIT 1`, pubkey).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check local identity: %w", err)
	}
	return true, nil
}

// HasLocalSubscription reports whether any local identity follows the given public key.
func (db *DB) HasLocalSubscription(pubkey []byte) (bool, error) {
	if len(pubkey) == 0 {
		return false, nil
	}
	var exists int
	err := db.QueryRow(`SELECT 1 FROM subscriptions WHERE pubkey = ? LIMIT 1`, pubkey).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check local subscription: %w", err)
	}
	return true, nil
}

// ShouldPinAuthor reports whether content authored by the given public key must stay pinned.
func (db *DB) ShouldPinAuthor(author []byte) (bool, error) {
	isLocal, err := db.IsLocalIdentity(author)
	if err != nil || isLocal {
		return isLocal, err
	}
	return db.HasLocalSubscription(author)
}

// ShouldPinDM reports whether a DM should stay pinned because it involves a local identity.
func (db *DB) ShouldPinDM(author, recipient []byte) (bool, error) {
	isLocalAuthor, err := db.IsLocalIdentity(author)
	if err != nil || isLocalAuthor {
		return isLocalAuthor, err
	}
	return db.IsLocalIdentity(recipient)
}

// TrackContentForAuthor tracks content and pins it when authored by a followed or local identity.
func (db *DB) TrackContentForAuthor(cid, author []byte) error {
	pinned, err := db.ShouldPinAuthor(author)
	if err != nil {
		return err
	}
	return db.TrackContentAccess(cid, pinned)
}

// TrackReactionContent tracks a reaction and pins it when either the actor is
// local/followed or the target post belongs to a local/followed author.
func (db *DB) TrackReactionContent(cid, author, target []byte) error {
	pinned, err := db.ShouldPinAuthor(author)
	if err != nil {
		return err
	}
	if !pinned && len(target) > 0 {
		targetAuthor, err := db.lookupPostAuthor(target)
		if err != nil {
			return err
		}
		if len(targetAuthor) > 0 {
			pinned, err = db.ShouldPinAuthor(targetAuthor)
			if err != nil {
				return err
			}
		}
	}
	return db.TrackContentAccess(cid, pinned)
}

// TrackContentForDM tracks a DM and pins it when either participant is local.
func (db *DB) TrackContentForDM(cid, author, recipient []byte) error {
	pinned, err := db.ShouldPinDM(author, recipient)
	if err != nil {
		return err
	}
	return db.TrackContentAccess(cid, pinned)
}

// TrackContentForMedia tracks raw media bytes, thumbnails, or chunks using the
// author from the parent media object when available.
func (db *DB) TrackContentForMedia(cid, mediaObjectCID []byte) error {
	author, err := db.lookupMediaAuthor(cid, mediaObjectCID)
	if err != nil {
		return err
	}
	if len(author) == 0 {
		return db.TrackContentAccess(cid, false)
	}
	return db.TrackContentForAuthor(cid, author)
}

// SetPinnedForAuthor updates the pin state for content attributed to the author.
func (db *DB) SetPinnedForAuthor(author []byte, pinned bool) error {
	if len(author) == 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	pinnedInt := 0
	if pinned {
		pinnedInt = 1
	}

	statements := []struct {
		query string
		args  []interface{}
	}{
		{
			query: `INSERT INTO content_access (cid, last_accessed, access_count, is_pinned)
			        VALUES (?, ?, 1, ?)
			        ON CONFLICT(cid) DO UPDATE SET is_pinned = excluded.is_pinned`,
			args: []interface{}{author, now, pinnedInt},
		},
		{
			query: `INSERT INTO content_access (cid, last_accessed, access_count, is_pinned)
			        SELECT cid, ?, 1, ? FROM posts WHERE author = ?
			        ON CONFLICT(cid) DO UPDATE SET is_pinned = excluded.is_pinned`,
			args: []interface{}{now, pinnedInt, author},
		},
		{
			query: `INSERT INTO content_access (cid, last_accessed, access_count, is_pinned)
			        SELECT cid, ?, 1, ? FROM media_objects WHERE author = ?
			        ON CONFLICT(cid) DO UPDATE SET is_pinned = excluded.is_pinned`,
			args: []interface{}{now, pinnedInt, author},
		},
		{
			query: `INSERT INTO content_access (cid, last_accessed, access_count, is_pinned)
			        SELECT thumbnail_cid, ?, 1, ? FROM media_objects
			        WHERE author = ? AND thumbnail_cid IS NOT NULL
			        ON CONFLICT(cid) DO UPDATE SET is_pinned = excluded.is_pinned`,
			args: []interface{}{now, pinnedInt, author},
		},
		{
			query: `INSERT INTO content_access (cid, last_accessed, access_count, is_pinned)
			        SELECT cid, ?, 1, ? FROM reactions WHERE author = ?
			        ON CONFLICT(cid) DO UPDATE SET is_pinned = excluded.is_pinned`,
			args: []interface{}{now, pinnedInt, author},
		},
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt.query, stmt.args...); err != nil {
			return fmt.Errorf("set pinned for author: %w", err)
		}
	}
	return nil
}

func (db *DB) lookupPostAuthor(cid []byte) ([]byte, error) {
	if len(cid) == 0 {
		return nil, nil
	}
	var author []byte
	err := db.QueryRow(`SELECT author FROM posts WHERE cid = ? LIMIT 1`, cid).Scan(&author)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lookup post author: %w", err)
	}
	return author, nil
}

func (db *DB) lookupMediaAuthor(cid, mediaObjectCID []byte) ([]byte, error) {
	lookupCID := cid
	if len(mediaObjectCID) > 0 {
		lookupCID = mediaObjectCID
	}
	if len(lookupCID) == 0 {
		return nil, nil
	}

	var author []byte
	err := db.QueryRow(
		`SELECT author FROM media_objects
		 WHERE cid = ? OR thumbnail_cid = ?
		 LIMIT 1`,
		nilIfEmpty(lookupCID), nilIfEmpty(cid),
	).Scan(&author)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lookup media author: %w", err)
	}
	return author, nil
}
