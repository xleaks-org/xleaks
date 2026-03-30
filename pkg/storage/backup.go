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
	if err := ensurePrivateDir(destDir); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}

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

	dst, err := os.CreateTemp(destDir, "xleaks-backup-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create backup temp file: %w", err)
	}
	tempPath := dst.Name()
	defer os.Remove(tempPath)

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return "", fmt.Errorf("copy db: %w", err)
	}
	if err := dst.Sync(); err != nil {
		dst.Close()
		return "", fmt.Errorf("sync backup: %w", err)
	}
	if err := dst.Close(); err != nil {
		return "", fmt.Errorf("close backup: %w", err)
	}
	if err := os.Rename(tempPath, backupPath); err != nil {
		return "", fmt.Errorf("finalize backup: %w", err)
	}
	if err := syncDirectory(destDir); err != nil {
		return "", fmt.Errorf("sync backup directory: %w", err)
	}

	return backupPath, nil
}

func ensurePrivateDir(path string) error {
	info, err := os.Stat(path)
	switch {
	case err == nil:
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", path)
		}
		if info.Mode().Perm() != 0o700 {
			if err := os.Chmod(path, 0o700); err != nil {
				return fmt.Errorf("set directory permissions: %w", err)
			}
		}
		return nil
	case !os.IsNotExist(err):
		return fmt.Errorf("stat directory: %w", err)
	}

	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("set directory permissions: %w", err)
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync parent directory: %w", err)
	}
	return nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync directory: %w", err)
	}
	return nil
}
