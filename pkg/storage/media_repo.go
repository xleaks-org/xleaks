package storage

import (
	"database/sql"
	"fmt"
)

// MediaObjectRow represents a single row from the media_objects table.
type MediaObjectRow struct {
	CID          []byte
	Author       []byte
	MimeType     string
	Size         uint64
	ChunkCount   uint32
	Width        uint32
	Height       uint32
	Duration     uint32
	ThumbnailCID []byte
	Timestamp    int64
	FullyFetched bool
}

// PostMediaRow represents a media attachment associated with a post.
type PostMediaRow struct {
	MediaObjectRow
	Position int
}

// InsertMediaObject inserts a new media object metadata record.
func (db *DB) InsertMediaObject(cid, author []byte, mimeType string, size uint64, chunkCount uint32, width, height, duration uint32, thumbnailCID []byte, timestamp int64) error {
	_, err := db.Exec(
		`INSERT INTO media_objects (cid, author, mime_type, size, chunk_count, width, height, duration, thumbnail_cid, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(cid) DO UPDATE SET
		     author = excluded.author,
		     mime_type = excluded.mime_type,
		     size = excluded.size,
		     chunk_count = excluded.chunk_count,
		     width = excluded.width,
		     height = excluded.height,
		     duration = excluded.duration,
		     thumbnail_cid = excluded.thumbnail_cid,
		     timestamp = excluded.timestamp`,
		cid, author, mimeType, size, chunkCount, width, height, duration, nilIfEmpty(thumbnailCID), timestamp,
	)
	if err != nil {
		return fmt.Errorf("insert media object: %w", err)
	}
	return nil
}

// GetMediaObject retrieves a media object by its CID. Returns nil if not found.
func (db *DB) GetMediaObject(cid []byte) (*MediaObjectRow, error) {
	var m MediaObjectRow
	var fetchedInt int
	err := db.QueryRow(
		`SELECT cid, author, mime_type, size, chunk_count, width, height, duration, thumbnail_cid, timestamp, fully_fetched
		 FROM media_objects WHERE cid = ?`,
		cid,
	).Scan(&m.CID, &m.Author, &m.MimeType, &m.Size, &m.ChunkCount, &m.Width, &m.Height, &m.Duration, &m.ThumbnailCID, &m.Timestamp, &fetchedInt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get media object: %w", err)
	}
	m.FullyFetched = fetchedInt == 1
	return &m, nil
}

// SetMediaFetched marks a media object as fully fetched (all chunks stored
// locally).
func (db *DB) SetMediaFetched(cid []byte) error {
	_, err := db.Exec(`UPDATE media_objects SET fully_fetched = 1 WHERE cid = ?`, cid)
	if err != nil {
		return fmt.Errorf("set media fetched: %w", err)
	}
	return nil
}

// InsertPostMedia links a media object to a post at a given position.
func (db *DB) InsertPostMedia(postCID, mediaCID []byte, position int) error {
	_, err := db.Exec(
		`INSERT OR IGNORE INTO post_media (post_cid, media_cid, position) VALUES (?, ?, ?)`,
		postCID, mediaCID, position,
	)
	if err != nil {
		return fmt.Errorf("insert post media: %w", err)
	}
	return nil
}

// InsertPostMediaTx links a media object to a post within an existing transaction.
func (db *DB) InsertPostMediaTx(tx *sql.Tx, postCID, mediaCID []byte, position int) error {
	_, err := tx.Exec(
		`INSERT OR IGNORE INTO post_media (post_cid, media_cid, position) VALUES (?, ?, ?)`,
		postCID, mediaCID, position,
	)
	if err != nil {
		return fmt.Errorf("insert post media tx: %w", err)
	}
	return nil
}

// GetPostMedia returns media attachments linked to a post ordered by position.
func (db *DB) GetPostMedia(postCID []byte) ([]PostMediaRow, error) {
	rows, err := db.Query(
		`SELECT pm.media_cid, pm.position,
		        COALESCE(mo.author, x''),
		        COALESCE(mo.mime_type, ''),
		        COALESCE(mo.size, 0),
		        COALESCE(mo.chunk_count, 0),
		        COALESCE(mo.width, 0),
		        COALESCE(mo.height, 0),
		        COALESCE(mo.duration, 0),
		        mo.thumbnail_cid,
		        COALESCE(mo.timestamp, 0),
		        COALESCE(mo.fully_fetched, 0)
		 FROM post_media pm
		 LEFT JOIN media_objects mo ON mo.cid = pm.media_cid
		 WHERE pm.post_cid = ?
		 ORDER BY pm.position ASC`,
		postCID,
	)
	if err != nil {
		return nil, fmt.Errorf("get post media: %w", err)
	}
	defer rows.Close()

	var items []PostMediaRow
	for rows.Next() {
		var row PostMediaRow
		var fetchedInt int
		if err := rows.Scan(
			&row.CID,
			&row.Position,
			&row.Author,
			&row.MimeType,
			&row.Size,
			&row.ChunkCount,
			&row.Width,
			&row.Height,
			&row.Duration,
			&row.ThumbnailCID,
			&row.Timestamp,
			&fetchedInt,
		); err != nil {
			return nil, fmt.Errorf("scan post media row: %w", err)
		}
		row.FullyFetched = fetchedInt == 1
		items = append(items, row)
	}
	return items, rows.Err()
}
