package content

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func ensureDirectory(path string, mode os.FileMode) error {
	info, err := os.Stat(path)
	switch {
	case err == nil:
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", path)
		}
		if info.Mode().Perm() != mode {
			if err := os.Chmod(path, mode); err != nil {
				return fmt.Errorf("set directory permissions: %w", err)
			}
		}
		return nil
	case !os.IsNotExist(err):
		return fmt.Errorf("stat directory: %w", err)
	}

	if err := os.MkdirAll(path, mode); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("set directory permissions: %w", err)
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync parent directory: %w", err)
	}
	return nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tempPath := f.Name()
	defer os.Remove(tempPath)

	if err := f.Chmod(mode); err != nil {
		f.Close()
		return fmt.Errorf("set file permissions: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("write file: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("sync file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace file: %w", err)
	}
	if err := syncDirectory(dir); err != nil {
		return fmt.Errorf("sync directory: %w", err)
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
		if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.EOPNOTSUPP) {
			return nil
		}
		return fmt.Errorf("sync directory: %w", err)
	}
	return nil
}
