package tui

import "charm.land/lipgloss/v2"

// Prefetch runway: keep ~prefetchScreens viewport heights of content near
// the older (top) edge so backscroll does not stall waiting on the network.
// Scales with short and tall terminals.
const (
	prefetchScreens  = 2.0
	prefetchMinLines = 8
)

// prefetchLineBudget is how many lines of "runway" above the visible window
// we try to keep. Scales with terminal height.
func prefetchLineBudget(viewportHeight int) int {
	if viewportHeight <= 0 {
		return prefetchMinLines
	}
	n := int(float64(viewportHeight) * prefetchScreens)
	if n < prefetchMinLines {
		return prefetchMinLines
	}
	return n
}

// needsOlderPrefetch reports whether the viewport is close enough to the older
// edge (or still too short to scroll) that another older page should load.
// yOffset is lines scrolled down from the top; totalLines is content height.
func needsOlderPrefetch(yOffset, viewportHeight, totalLines int) bool {
	if yOffset < 0 {
		yOffset = 0
	}
	budget := prefetchLineBudget(viewportHeight)
	if viewportHeight <= 0 {
		return yOffset < budget
	}
	// Fill until there is a full viewport plus runway of scrollable history.
	if totalLines < viewportHeight+budget {
		return true
	}
	return yOffset < budget
}

// nearOlderEdge is true when YOffset is within the prefetch budget.
// Prefer needsOlderPrefetch when total line count is known.
func nearOlderEdge(yOffset, viewportHeight int) bool {
	if yOffset < 0 {
		yOffset = 0
	}
	return yOffset < prefetchLineBudget(viewportHeight)
}

// anchorAfterPrepend keeps the visible content stable when lines are inserted
// above the viewport (older history merge). Call after SetContent with the
// YOffset and TotalLineCount captured before the prepended content was set.
func anchorAfterPrepend(vp interface {
	TotalLineCount() int
	SetYOffset(int)
}, yBefore, linesBefore int) {
	delta := vp.TotalLineCount() - linesBefore
	if delta > 0 {
		vp.SetYOffset(yBefore + delta)
	}
}

// lineSpan is the viewport line range of one rendered item (list row or
// detail chain row). end is exclusive: [start, start+height).
type lineSpan struct {
	start  int
	height int
}

func (s lineSpan) end() int { return s.start + s.height }

// lineSpans maps already-wrapped item blocks onto viewport line indexes.
// prefixLines are rows above the first item (tips, titles). gapLines are
// extra blank rows between items, not counting each block's own lines.
func lineSpans(blocks []string, prefixLines, gapLines int) []lineSpan {
	if prefixLines < 0 {
		prefixLines = 0
	}
	if gapLines < 0 {
		gapLines = 0
	}
	out := make([]lineSpan, len(blocks))
	line := prefixLines
	for i, block := range blocks {
		h := lipgloss.Height(block)
		out[i] = lineSpan{start: line, height: h}
		line += h
		if i < len(blocks)-1 {
			line += gapLines
		}
	}
	return out
}

// revealSpan scrolls vp just enough so span is inside the visible window.
// No-op when the span is already fully visible or has no height.
func revealSpan(vp interface {
	YOffset() int
	Height() int
	SetYOffset(int)
}, span lineSpan) {
	if span.height <= 0 {
		return
	}
	top := vp.YOffset()
	h := vp.Height()
	if h <= 0 {
		return
	}
	bot := top + h
	if span.start < top {
		vp.SetYOffset(span.start)
		return
	}
	if span.end() > bot {
		vp.SetYOffset(span.end() - h)
	}
}
