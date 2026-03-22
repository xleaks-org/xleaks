package content

import (
	"fmt"
	"sort"
)

// AssembleChunks reassembles ordered chunks into a complete file. Chunks are
// sorted by their Index before concatenation.
func AssembleChunks(chunks []Chunk) ([]byte, error) {
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no chunks to assemble")
	}

	// Sort chunks by index to ensure correct ordering.
	sorted := make([]Chunk, len(chunks))
	copy(sorted, chunks)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Index < sorted[j].Index
	})

	// Verify indices are contiguous starting from 0.
	for i, c := range sorted {
		if c.Index != i {
			return nil, fmt.Errorf("missing chunk at index %d (got index %d)", i, c.Index)
		}
	}

	// Calculate total size and assemble.
	totalSize := 0
	for _, c := range sorted {
		totalSize += len(c.Data)
	}

	result := make([]byte, 0, totalSize)
	for _, c := range sorted {
		result = append(result, c.Data...)
	}

	return result, nil
}

// ValidateChunks verifies that every chunk's CID matches the SHA-256 multihash
// of its data. Returns an error describing the first invalid chunk found.
func ValidateChunks(chunks []Chunk) error {
	for _, c := range chunks {
		if !ValidateCID(c.CID, c.Data) {
			return fmt.Errorf("chunk %d CID mismatch: data does not match its CID", c.Index)
		}
	}
	return nil
}
