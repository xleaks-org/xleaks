package content

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestComputeCID(t *testing.T) {
	data := []byte("hello xleaks")
	cid, err := ComputeCID(data)
	if err != nil {
		t.Fatalf("ComputeCID() error: %v", err)
	}
	if len(cid) == 0 {
		t.Fatal("CID should not be empty")
	}

	// Same data should produce same CID.
	cid2, err := ComputeCID(data)
	if err != nil {
		t.Fatalf("ComputeCID() second call error: %v", err)
	}
	if !bytes.Equal(cid, cid2) {
		t.Error("same data should produce same CID")
	}

	// Different data should produce different CID.
	cid3, err := ComputeCID([]byte("different data"))
	if err != nil {
		t.Fatalf("ComputeCID() different data error: %v", err)
	}
	if bytes.Equal(cid, cid3) {
		t.Error("different data should produce different CID")
	}
}

func TestCIDHexRoundTrip(t *testing.T) {
	data := []byte("test data")
	cid, err := ComputeCID(data)
	if err != nil {
		t.Fatalf("ComputeCID() error: %v", err)
	}

	hexStr := CIDToHex(cid)
	decoded, err := HexToCID(hexStr)
	if err != nil {
		t.Fatalf("HexToCID() error: %v", err)
	}

	if !bytes.Equal(cid, decoded) {
		t.Error("hex round-trip produced different CID")
	}
}

func TestValidateCID(t *testing.T) {
	data := []byte("test content")
	cid, _ := ComputeCID(data)

	if !ValidateCID(cid, data) {
		t.Error("ValidateCID should return true for matching data")
	}

	if ValidateCID(cid, []byte("tampered")) {
		t.Error("ValidateCID should return false for non-matching data")
	}
}

func TestContentStore(t *testing.T) {
	dir := t.TempDir()
	store, err := NewContentStore(dir)
	if err != nil {
		t.Fatalf("NewContentStore() error: %v", err)
	}

	data := []byte("stored content")
	cid, _ := ComputeCID(data)

	// Put.
	if err := store.Put(cid, data); err != nil {
		t.Fatalf("Put() error: %v", err)
	}

	// Has.
	if !store.Has(cid) {
		t.Error("Has() should return true after Put")
	}

	// Get.
	retrieved, err := store.Get(cid)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if !bytes.Equal(data, retrieved) {
		t.Error("Get() returned different data than Put()")
	}

	// Verify sharded directory structure.
	hexCID := CIDToHex(cid)
	expectedPath := filepath.Join(dir, hexCID[:2], hexCID)
	if _, err := os.Stat(expectedPath); err != nil {
		t.Errorf("expected file at sharded path %s: %v", expectedPath, err)
	}

	// Delete.
	if err := store.Delete(cid); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
	if store.Has(cid) {
		t.Error("Has() should return false after Delete")
	}
}

func TestChunkFile(t *testing.T) {
	// Create data that spans multiple chunks.
	data := make([]byte, ChunkSize*2+100)
	for i := range data {
		data[i] = byte(i % 256)
	}

	chunks, err := ChunkFile(data)
	if err != nil {
		t.Fatalf("ChunkFile() error: %v", err)
	}

	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}

	if len(chunks[0].Data) != ChunkSize {
		t.Errorf("first chunk size = %d, want %d", len(chunks[0].Data), ChunkSize)
	}
	if len(chunks[1].Data) != ChunkSize {
		t.Errorf("second chunk size = %d, want %d", len(chunks[1].Data), ChunkSize)
	}
	if len(chunks[2].Data) != 100 {
		t.Errorf("third chunk size = %d, want 100", len(chunks[2].Data))
	}

	// Each chunk should have a valid CID.
	for i, chunk := range chunks {
		if !ValidateCID(chunk.CID, chunk.Data) {
			t.Errorf("chunk %d CID does not match its data", i)
		}
		if chunk.Index != i {
			t.Errorf("chunk %d index = %d, want %d", i, chunk.Index, i)
		}
	}
}

func TestAssembleChunks(t *testing.T) {
	data := make([]byte, ChunkSize+500)
	for i := range data {
		data[i] = byte(i % 256)
	}

	chunks, _ := ChunkFile(data)
	assembled, err := AssembleChunks(chunks)
	if err != nil {
		t.Fatalf("AssembleChunks() error: %v", err)
	}

	if !bytes.Equal(data, assembled) {
		t.Error("assembled data does not match original")
	}
}

func TestChunkFileTooLarge(t *testing.T) {
	data := make([]byte, MaxMediaSize+1)
	_, err := ChunkFile(data)
	if err == nil {
		t.Error("ChunkFile() should error for data exceeding MaxMediaSize")
	}
}

func TestChunkFileEmpty(t *testing.T) {
	_, err := ChunkFile([]byte{})
	if err == nil {
		t.Error("ChunkFile() should error for empty data")
	}
}
