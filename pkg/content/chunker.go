package content

import (
	"fmt"
	"io"
)

// supportedMediaTypes lists the MIME types accepted for media uploads.
var supportedMediaTypes = map[string]bool{
	// Images
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
	// Video
	"video/mp4":  true,
	"video/webm": true,
	// Audio
	"audio/mpeg": true,
	"audio/ogg":  true,
	"audio/wav":  true,
}

// ValidateMediaType checks whether the given MIME type is in the set of
// supported media formats. Returns an error for unsupported types.
func ValidateMediaType(mimeType string) error {
	if !supportedMediaTypes[mimeType] {
		return fmt.Errorf("unsupported media type: %s", mimeType)
	}
	return nil
}

const (
	// ChunkSize is the maximum size of a single media chunk (256KB).
	ChunkSize = 262144

	// MaxMediaSize is the maximum allowed media file size (100MB).
	MaxMediaSize = 100 * 1024 * 1024
)

// Chunk represents a single chunk of a media file with its content-addressed
// identifier, sequence index, and raw data.
type Chunk struct {
	CID   []byte
	Index int
	Data  []byte
}

// ChunkFile splits data into ChunkSize-byte chunks and computes a CID for each
// chunk. Returns an error if data exceeds MaxMediaSize.
func ChunkFile(data []byte) ([]Chunk, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("cannot chunk empty data")
	}
	if len(data) > MaxMediaSize {
		return nil, fmt.Errorf("data exceeds maximum media size: %d > %d", len(data), MaxMediaSize)
	}

	var chunks []Chunk
	for i := 0; i < len(data); i += ChunkSize {
		end := i + ChunkSize
		if end > len(data) {
			end = len(data)
		}
		chunkData := data[i:end]

		cid, err := ComputeCID(chunkData)
		if err != nil {
			return nil, fmt.Errorf("failed to compute CID for chunk %d: %w", len(chunks), err)
		}

		chunks = append(chunks, Chunk{
			CID:   cid,
			Index: len(chunks),
			Data:  chunkData,
		})
	}
	return chunks, nil
}

// ChunkReader reads from r and splits the content into ChunkSize-byte chunks,
// computing a CID for each. Returns an error if the total data exceeds
// MaxMediaSize.
func ChunkReader(r io.Reader) ([]Chunk, error) {
	var chunks []Chunk
	totalRead := 0
	index := 0

	for {
		buf := make([]byte, ChunkSize)
		n, err := io.ReadFull(r, buf)
		if n > 0 {
			totalRead += n
			if totalRead > MaxMediaSize {
				return nil, fmt.Errorf("data exceeds maximum media size: read at least %d > %d", totalRead, MaxMediaSize)
			}

			chunkData := buf[:n]
			cid, cidErr := ComputeCID(chunkData)
			if cidErr != nil {
				return nil, fmt.Errorf("failed to compute CID for chunk %d: %w", index, cidErr)
			}

			chunks = append(chunks, Chunk{
				CID:   cid,
				Index: index,
				Data:  chunkData,
			})
			index++
		}

		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read data: %w", err)
		}
	}

	if len(chunks) == 0 {
		return nil, fmt.Errorf("cannot chunk empty data")
	}

	return chunks, nil
}
