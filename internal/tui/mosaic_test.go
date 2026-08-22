package tui

import (
	"image"
	"image/color"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestRenderMosaicBounds(t *testing.T) {
	t.Parallel()
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			img.Set(x, y, color.RGBA{R: uint8(x * 4), G: uint8(y * 4), A: 255})
		}
	}
	const maxW, maxH = 20, 10
	out := renderMosaic(img, maxW, maxH)
	if out == "" {
		t.Fatal("empty mosaic")
	}
	w, h := lipgloss.Width(out), lipgloss.Height(out)
	if w > maxW {
		t.Fatalf("width %d > %d", w, maxW)
	}
	if h > maxH {
		t.Fatalf("height %d > %d", h, maxH)
	}
	// mosaic samples 2×2 pixels per cell; a naive Width(w).Height(h) only
	// fills about half the budget (see TestRenderMosaicFillsCellBudget).
	if w < maxW-1 {
		t.Fatalf("width %d, want about %d", w, maxW)
	}
	if h < maxH-1 {
		t.Fatalf("height %d, want about %d", h, maxH)
	}
}

func TestRenderMosaicNilAndClamp(t *testing.T) {
	t.Parallel()
	if got := renderMosaic(nil, 10, 10); got != "" {
		t.Fatalf("%q", got)
	}
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	out := renderMosaic(img, 0, 0)
	if out == "" {
		t.Fatal("clamped render should still produce output")
	}
}
