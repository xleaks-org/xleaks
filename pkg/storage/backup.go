package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Backup creates a consistent copy of the database using SQLite's backup mechanism.
// It returns the path to the backup file.
func (db *DB) Backup(destDir string) (string, error) {
	timestamp := time.Now().Format("20060102-150405")
	backupPath := filepath.Join(destDir, fmt.Sprintf("xleaks-backup-%s.db", timestamp))

	// Use SQLite's .backup command via a checkpoint + file copy.
	// First, checkpoint the WAL to ensure all data is in the main DB file.
	_, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	if err != nil {
		return "", fmt.Errorf("wal checkpoint: %w", err)
	}

	// Copy the database file.
	srcPath := db.Path()
	src, err := os.Open(srcPath)
	if err != nil {
		return "", fmt.Errorf("open source db: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(backupPath)
	if err != nil {
		return "", fmt.Errorf("create backup: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("copy db: %w", err)
	}

	return backupPath, nil
}
