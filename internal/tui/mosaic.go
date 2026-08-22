package tui

import (
	"image"
	"strings"

	"github.com/charmbracelet/x/mosaic"
)

// mosaicPixelsPerCell is Charm mosaic's sampling window: one cell is a 2×2
// pixel block (halfblocks). Width/Height on mosaic.New are pixel budgets,
// so a cell budget of (w,h) must be requested as (2w, 2h) or the art lands
// at about half size inside the overlay.
const mosaicPixelsPerCell = 2

// renderMosaic draws img as Unicode halfblocks, at most maxW×maxH cells.
func renderMosaic(img image.Image, maxW, maxH int) string {
	if img == nil {
		return ""
	}
	if maxW < 1 {
		maxW = 1
	}
	if maxH < 1 {
		maxH = 1
	}
	m := mosaic.New().Width(maxW * mosaicPixelsPerCell).Height(maxH * mosaicPixelsPerCell)
	return strings.TrimRight(m.Render(img), "\n")
}
