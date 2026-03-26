package content

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"strings"

	_ "golang.org/x/image/webp"

	"golang.org/x/image/draw"
)

const (
	ThumbnailMaxWidth    = 320
	ThumbnailMaxSize     = 100 * 1024 // 100KB
	ThumbnailJPEGQuality = 80
)

// GenerateThumbnail creates a JPEG thumbnail from image data using the default quality.
func GenerateThumbnail(imageData []byte) ([]byte, []byte, error) {
	return GenerateThumbnailWithQuality(imageData, ThumbnailJPEGQuality)
}

// GenerateMediaThumbnail creates a JPEG thumbnail for supported image or video data.
func GenerateMediaThumbnail(data []byte, mimeType string, quality int) ([]byte, []byte, error) {
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return GenerateThumbnailWithQuality(data, quality)
	case strings.HasPrefix(mimeType, "video/"):
		return generateVideoPlaceholderThumbnail(quality)
	default:
		return nil, nil, fmt.Errorf("thumbnail generation unsupported for %s", mimeType)
	}
}

// GenerateThumbnailWithQuality creates a JPEG thumbnail from image data.
// The thumbnail is resized to ThumbnailMaxWidth pixels wide, maintaining aspect ratio.
// Returns the thumbnail JPEG data and its CID.
func GenerateThumbnailWithQuality(imageData []byte, quality int) ([]byte, []byte, error) {
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

	return encodeThumbnail(dst, quality)
}

func generateVideoPlaceholderThumbnail(quality int) ([]byte, []byte, error) {
	const height = 180

	img := image.NewRGBA(image.Rect(0, 0, ThumbnailMaxWidth, height))
	top := color.RGBA{0x13, 0x17, 0x20, 0xff}
	bottom := color.RGBA{0x29, 0x33, 0x40, 0xff}
	for y := 0; y < height; y++ {
		t := float64(y) / float64(height-1)
		row := color.RGBA{
			R: uint8(float64(top.R)*(1-t) + float64(bottom.R)*t),
			G: uint8(float64(top.G)*(1-t) + float64(bottom.G)*t),
			B: uint8(float64(top.B)*(1-t) + float64(bottom.B)*t),
			A: 0xff,
		}
		for x := 0; x < ThumbnailMaxWidth; x++ {
			img.SetRGBA(x, y, row)
		}
	}

	panel := color.RGBA{0x00, 0x00, 0x00, 0x55}
	for y := 20; y < height-20; y++ {
		for x := 20; x < ThumbnailMaxWidth-20; x++ {
			img.Set(x, y, panel)
		}
	}

	play := color.RGBA{0xff, 0xff, 0xff, 0xdd}
	cx := ThumbnailMaxWidth / 2
	cy := height / 2
	size := 34
	for x := 0; x < size; x++ {
		maxY := (x * size) / size
		for y := -maxY; y <= maxY; y++ {
			img.Set(cx-size/3+x, cy+y, play)
		}
	}

	return encodeThumbnail(img, quality)
}

func encodeThumbnail(src image.Image, quality int) ([]byte, []byte, error) {
	quality = clampThumbnailQuality(quality)
	var buf bytes.Buffer
	for quality >= 10 {
		buf.Reset()
		if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: quality}); err != nil {
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

func clampThumbnailQuality(quality int) int {
	if quality < 10 {
		return 10
	}
	if quality > 100 {
		return 100
	}
	return quality
}
