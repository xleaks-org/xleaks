package storage

import (
	"fmt"
	"time"
)

// TrackContentAccess inserts or updates a content_access row. If the CID
// already exists, the last_accessed time and access_count are updated. The
// is_pinned flag is set to the provided value.
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
		     is_pinned = excluded.is_pinned`,
		cid, now, pinnedInt,
	)
	if err != nil {
		return fmt.Errorf("track content access: %w", err)
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
