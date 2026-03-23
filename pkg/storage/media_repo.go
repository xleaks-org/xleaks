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

// InsertMediaObject inserts a new media object metadata record.
func (db *DB) InsertMediaObject(cid, author []byte, mimeType string, size uint64, chunkCount uint32, width, height, duration uint32, thumbnailCID []byte, timestamp int64) error {
	_, err := db.Exec(
		`INSERT OR IGNORE INTO media_objects (cid, author, mime_type, size, chunk_count, width, height, duration, thumbnail_cid, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
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
