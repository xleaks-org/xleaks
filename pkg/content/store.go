package content

import (
	"fmt"
	"os"
	"path/filepath"
)

// ContentStore provides content-addressed storage on disk using a sharded
// directory structure. Files are stored under basePath/<first-2-hex-chars>/<full-hex-cid>.
type ContentStore struct {
	basePath string
}

// NewContentStore creates a new ContentStore rooted at basePath, ensuring the
// base directory exists.
func NewContentStore(basePath string) (*ContentStore, error) {
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create content store directory: %w", err)
	}
	return &ContentStore{basePath: basePath}, nil
}

// BasePath returns the root directory of the content store.
func (cs *ContentStore) BasePath() string {
	return cs.basePath
}

// Put stores data under the given CID. The shard directory is created
// automatically if it does not already exist.
func (cs *ContentStore) Put(cid []byte, data []byte) error {
	path := cs.objectPath(cid)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create shard directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write content: %w", err)
	}
	return nil
}

// Get retrieves the data stored under the given CID.
func (cs *ContentStore) Get(cid []byte) ([]byte, error) {
	path := cs.objectPath(cid)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}
	return data, nil
}

// Has reports whether data exists for the given CID.
func (cs *ContentStore) Has(cid []byte) bool {
	path := cs.objectPath(cid)
	_, err := os.Stat(path)
	return err == nil
}

// Delete removes the data stored under the given CID.
func (cs *ContentStore) Delete(cid []byte) error {
	path := cs.objectPath(cid)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to delete content: %w", err)
	}
	return nil
}

// objectPath computes the file path for a CID using a sharded directory
// structure. The first 2 hex characters of the CID form the shard directory
// name, and the full hex CID is the filename.
func (cs *ContentStore) objectPath(cid []byte) string {
	hexCID := CIDToHex(cid)
	shard := hexCID[:2]
	return filepath.Join(cs.basePath, shard, hexCID)
}

// DirSize calculates the total size in bytes of a directory tree.
func DirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		size += info.Size()
		return nil
	})
	return size, err
}
