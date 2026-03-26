package content

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestGenerateMediaThumbnail_VideoPlaceholder(t *testing.T) {
	thumb, cid, err := GenerateMediaThumbnail(nil, "video/mp4", 75)
	if err != nil {
		t.Fatalf("GenerateMediaThumbnail(video): %v", err)
	}
	if len(thumb) == 0 || len(cid) == 0 {
		t.Fatal("expected thumbnail bytes and CID")
	}
	if !bytes.HasPrefix(thumb, []byte{0xff, 0xd8, 0xff}) {
		t.Fatal("expected JPEG thumbnail output")
	}
	if !ValidateCID(cid, thumb) {
		t.Fatal("expected thumbnail CID to match bytes")
	}
}

func TestGenerateThumbnailWithQuality_Image(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 8), G: uint8(y * 8), B: 120, A: 255})
		}
	}

	var src bytes.Buffer
	if err := png.Encode(&src, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}

	thumb, cid, err := GenerateThumbnailWithQuality(src.Bytes(), 60)
	if err != nil {
		t.Fatalf("GenerateThumbnailWithQuality: %v", err)
	}
	if len(thumb) == 0 || len(cid) == 0 {
		t.Fatal("expected thumbnail bytes and CID")
	}
	if !ValidateCID(cid, thumb) {
		t.Fatal("expected thumbnail CID to match bytes")
	}
}
