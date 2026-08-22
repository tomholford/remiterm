package tui

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"strings"
	"testing"
)

func solidPNG(t *testing.T, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDecodeStillPNG(t *testing.T) {
	t.Parallel()
	raw := solidPNG(t, color.RGBA{R: 255, A: 255})
	img, format, err := decodeStill(bytes.NewReader(raw), previewMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	if format != "png" {
		t.Fatalf("format %q", format)
	}
	r, _, _, _ := img.At(0, 0).RGBA()
	if r>>8 != 255 {
		t.Fatalf("pixel r=%d", r>>8)
	}
}

func TestDecodeStillGIFFirstFrame(t *testing.T) {
	t.Parallel()
	red := image.NewPaletted(image.Rect(0, 0, 4, 4), []color.Color{
		color.RGBA{R: 255, A: 255},
		color.RGBA{B: 255, A: 255},
	})
	blue := image.NewPaletted(image.Rect(0, 0, 4, 4), red.Palette)
	for y := range 4 {
		for x := range 4 {
			red.SetColorIndex(x, y, 0)
			blue.SetColorIndex(x, y, 1)
		}
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, &gif.GIF{
		Image: []*image.Paletted{red, blue},
		Delay: []int{10, 10},
	}); err != nil {
		t.Fatal(err)
	}
	img, format, err := decodeStill(bytes.NewReader(buf.Bytes()), previewMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	if format != "gif" {
		t.Fatalf("format %q", format)
	}
	r, g, b, _ := img.At(0, 0).RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Fatalf("want first-frame red, got %d,%d,%d", r>>8, g>>8, b>>8)
	}
}

func TestDecodeStillOversize(t *testing.T) {
	t.Parallel()
	raw := solidPNG(t, color.White)
	_, _, err := decodeStill(bytes.NewReader(raw), 8)
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("err=%v", err)
	}
}

func TestDecodeStillUnknown(t *testing.T) {
	t.Parallel()
	_, _, err := decodeStill(strings.NewReader("not an image"), previewMaxBytes)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeStillReadError(t *testing.T) {
	t.Parallel()
	_, _, err := decodeStill(badReader{}, previewMaxBytes)
	if err == nil {
		t.Fatal("expected error")
	}
}

type badReader struct{}

func (badReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}
