package tui

import (
	"testing"

	"charm.land/bubbles/v2/viewport"
)

func TestPrefetchLineBudget(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		h    int
		want int
	}{
		{name: "zero height uses min", h: 0, want: prefetchMinLines},
		{name: "negative height uses min", h: -3, want: prefetchMinLines},
		{name: "short terminal", h: 10, want: 20}, // 2×10
		{name: "tall terminal", h: 80, want: 160},
		{name: "tiny height floors to min", h: 2, want: prefetchMinLines},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := prefetchLineBudget(tc.h)
			if got != tc.want {
				t.Fatalf("prefetchLineBudget(%d)=%d want %d", tc.h, got, tc.want)
			}
		})
	}
}

func TestNearOlderEdge(t *testing.T) {
	t.Parallel()
	// H=20 → budget 40
	if !nearOlderEdge(0, 20) {
		t.Fatal("top is near")
	}
	if !nearOlderEdge(39, 20) {
		t.Fatal("just inside budget")
	}
	if nearOlderEdge(40, 20) {
		t.Fatal("at budget boundary should not be near")
	}
	if nearOlderEdge(100, 20) {
		t.Fatal("far from top")
	}
	if !nearOlderEdge(-1, 20) {
		t.Fatal("negative yOffset treated as top")
	}
}

func TestNeedsOlderPrefetch(t *testing.T) {
	t.Parallel()
	// H=20 → budget 40; need total < 20+40=60 to force fill.
	t.Run("short content always needs fill", func(t *testing.T) {
		t.Parallel()
		if !needsOlderPrefetch(0, 20, 30) {
			t.Fatal("content shorter than viewport+budget should prefetch")
		}
	})
	t.Run("enough content far from top", func(t *testing.T) {
		t.Parallel()
		if needsOlderPrefetch(100, 20, 500) {
			t.Fatal("scrolled down should not prefetch")
		}
	})
	t.Run("enough content near top", func(t *testing.T) {
		t.Parallel()
		if !needsOlderPrefetch(5, 20, 500) {
			t.Fatal("near top with deep history should prefetch")
		}
	})
	t.Run("exactly filled runway at top still near", func(t *testing.T) {
		t.Parallel()
		// total == height+budget → not forced by short-content branch;
		// yOffset 0 is still within budget.
		if !needsOlderPrefetch(0, 20, 60) {
			t.Fatal("yOffset 0 should still be near edge")
		}
	})
	t.Run("just outside budget", func(t *testing.T) {
		t.Parallel()
		if needsOlderPrefetch(40, 20, 500) {
			t.Fatal("yOffset at budget should not prefetch")
		}
	})
}

func TestAnchorAfterPrepend(t *testing.T) {
	t.Parallel()
	vp := viewport.New(viewport.WithWidth(40), viewport.WithHeight(10))
	// 20 lines of content; scroll down a bit.
	var body string
	for i := 0; i < 20; i++ {
		body += "line\n"
	}
	vp.SetContent(body)
	vp.SetYOffset(5)
	yBefore := vp.YOffset()
	linesBefore := vp.TotalLineCount()

	// Prepend 4 lines and re-anchor.
	vp.SetContent("a\nb\nc\nd\n" + body)
	anchorAfterPrepend(&vp, yBefore, linesBefore)
	delta := vp.TotalLineCount() - linesBefore
	if delta <= 0 {
		t.Fatalf("expected positive delta, got %d (before=%d after=%d)", delta, linesBefore, vp.TotalLineCount())
	}
	if vp.YOffset() != yBefore+delta {
		t.Fatalf("YOffset=%d want %d", vp.YOffset(), yBefore+delta)
	}
}

func TestLineSpans(t *testing.T) {
	t.Parallel()
	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		if got := lineSpans(nil, 1, 1); len(got) != 0 {
			t.Fatalf("len=%d want 0", len(got))
		}
	})
	t.Run("prefix and no gap", func(t *testing.T) {
		t.Parallel()
		got := lineSpans([]string{"a", "b\nc", "d"}, 1, 0)
		want := []lineSpan{
			{start: 1, height: 1},
			{start: 2, height: 2},
			{start: 4, height: 1},
		}
		if len(got) != len(want) {
			t.Fatalf("len=%d want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("span[%d]=%+v want %+v", i, got[i], want[i])
			}
		}
	})
	t.Run("gap between items", func(t *testing.T) {
		t.Parallel()
		got := lineSpans([]string{"aa", "bb"}, 2, 1)
		if got[0] != (lineSpan{start: 2, height: 1}) {
			t.Fatalf("span[0]=%+v", got[0])
		}
		if got[1] != (lineSpan{start: 4, height: 1}) {
			t.Fatalf("span[1]=%+v want start 4 (2+1+gap 1)", got[1])
		}
	})
	t.Run("negative prefix and gap clamp to zero", func(t *testing.T) {
		t.Parallel()
		got := lineSpans([]string{"x"}, -3, -1)
		if got[0] != (lineSpan{start: 0, height: 1}) {
			t.Fatalf("span[0]=%+v", got[0])
		}
	})
}

func TestRevealSpan(t *testing.T) {
	t.Parallel()
	newVP := func() viewport.Model {
		vp := viewport.New(viewport.WithWidth(20), viewport.WithHeight(10))
		var body string
		for i := 0; i < 30; i++ {
			body += "x\n"
		}
		vp.SetContent(body)
		vp.SetYOffset(15) // visible [15, 25)
		return vp
	}

	t.Run("already visible stays put", func(t *testing.T) {
		t.Parallel()
		vp := newVP()
		revealSpan(&vp, lineSpan{start: 18, height: 2})
		if vp.YOffset() != 15 {
			t.Fatalf("YOffset=%d want 15", vp.YOffset())
		}
	})
	t.Run("above snaps to start", func(t *testing.T) {
		t.Parallel()
		vp := newVP()
		revealSpan(&vp, lineSpan{start: 10, height: 2})
		if vp.YOffset() != 10 {
			t.Fatalf("YOffset=%d want 10", vp.YOffset())
		}
	})
	t.Run("below snaps so end is at bottom", func(t *testing.T) {
		t.Parallel()
		vp := newVP()
		revealSpan(&vp, lineSpan{start: 24, height: 3}) // end=27 > 25
		if vp.YOffset() != 17 {                         // 27-10
			t.Fatalf("YOffset=%d want 17", vp.YOffset())
		}
	})
	t.Run("zero height is no-op", func(t *testing.T) {
		t.Parallel()
		vp := newVP()
		revealSpan(&vp, lineSpan{start: 0, height: 0})
		if vp.YOffset() != 15 {
			t.Fatalf("YOffset=%d want 15", vp.YOffset())
		}
	})
}
