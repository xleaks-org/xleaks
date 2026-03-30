package content

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"strings"

	_ "golang.org/x/image/webp"
)

// MediaMetadata holds extracted metadata for a media file.
type MediaMetadata struct {
	Width    uint32
	Height   uint32
	Duration uint32 // seconds, 0 for images
	MimeType string
}

// ExtractImageMetadata reads image dimensions from the data without decoding
// the full image. It uses image.DecodeConfig which only reads the header.
func ExtractImageMetadata(data []byte) (*MediaMetadata, error) {
	return ExtractImageMetadataReader(bytes.NewReader(data))
}

// ExtractImageMetadataReader reads image dimensions from the reader without
// decoding the full image.
func ExtractImageMetadataReader(r io.Reader) (*MediaMetadata, error) {
	cfg, format, err := image.DecodeConfig(r)
	if err != nil {
		return nil, err
	}
	if err := ValidateImageDimensions(cfg.Width, cfg.Height); err != nil {
		return nil, err
	}
	mimeType := "image/" + format
	return &MediaMetadata{
		Width:    uint32(cfg.Width),
		Height:   uint32(cfg.Height),
		MimeType: mimeType,
	}, nil
}

// ExtractMediaMetadata attempts to extract metadata from any supported media
// type. For images it extracts width and height. For video and audio types it
// returns best-effort metadata (dimensions and duration set to 0) since pure-Go
// video/audio parsing is complex and would require additional dependencies.
// Returns nil if no metadata could be extracted.
func ExtractMediaMetadata(data []byte, mimeType string) *MediaMetadata {
	if strings.HasPrefix(mimeType, "image/") {
		meta, err := ExtractImageMetadata(data)
		if err != nil {
			// Return basic metadata with the known MIME type.
			return &MediaMetadata{MimeType: mimeType}
		}
		// Preserve the detected MIME type from http.DetectContentType if the
		// image decoder returns a more generic format name.
		if meta.MimeType == "" {
			meta.MimeType = mimeType
		}
		return meta
	}

	if strings.HasPrefix(mimeType, "video/") || strings.HasPrefix(mimeType, "audio/") {
		// Best-effort: return the MIME type with zero dimensions/duration.
		// Full video/audio metadata extraction would require ffprobe or
		// a dedicated container parser.
		return &MediaMetadata{MimeType: mimeType}
	}

	return nil
}
