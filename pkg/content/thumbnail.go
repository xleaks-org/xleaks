package content

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"

	"golang.org/x/image/draw"
)

const (
	ThumbnailMaxWidth   = 320
	ThumbnailMaxSize    = 100 * 1024 // 100KB
	ThumbnailJPEGQuality = 80
)

// GenerateThumbnail creates a JPEG thumbnail from image data.
// The thumbnail is resized to ThumbnailMaxWidth pixels wide, maintaining aspect ratio.
// Returns the thumbnail JPEG data and its CID.
func GenerateThumbnail(imageData []byte) ([]byte, []byte, error) {
	src, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode image: %w", err)
	}

	bounds := src.Bounds()
	srcWidth := bounds.Dx()
	srcHeight := bounds.Dy()

	// Calculate new dimensions maintaining aspect ratio.
	newWidth := ThumbnailMaxWidth
	if srcWidth < newWidth {
		newWidth = srcWidth
	}
	newHeight := (srcHeight * newWidth) / srcWidth

	// Resize using high-quality CatmullRom interpolation.
	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)

	// Encode as JPEG, adjusting quality to fit within ThumbnailMaxSize.
	quality := ThumbnailJPEGQuality
	var buf bytes.Buffer
	for quality >= 10 {
		buf.Reset()
		if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: quality}); err != nil {
			return nil, nil, fmt.Errorf("failed to encode thumbnail: %w", err)
		}
		if buf.Len() <= ThumbnailMaxSize {
			break
		}
		quality -= 10
	}

	thumbData := buf.Bytes()
	cid, err := ComputeCID(thumbData)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to compute thumbnail CID: %w", err)
	}

	return thumbData, cid, nil
}
