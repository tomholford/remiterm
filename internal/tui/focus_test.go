package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"remiterm/internal/api"
)

func TestGutterStyle(t *testing.T) {
	t.Parallel()
	active := gutterStyle(true).Render(gutterGlyph)
	idle := gutterStyle(false).Render(gutterGlyph)
	if active == idle {
		t.Fatal("active and idle gutter styles must differ")
	}
	if stripANSI(active) != gutterGlyph || stripANSI(idle) != gutterGlyph {
		t.Fatalf("glyph: active %q idle %q", stripANSI(active), stripANSI(idle))
	}
}

func TestWithLeftGutter(t *testing.T) {
	t.Parallel()

	t.Run("single line width plus one", func(t *testing.T) {
		t.Parallel()
		in := "hello"
		got := withLeftGutter(in, true)
		plain := stripANSI(got)
		if !strings.HasPrefix(plain, gutterGlyph) {
			t.Fatalf("missing gutter: %q", plain)
		}
		if lipgloss.Width(plain) != lipgloss.Width(in)+1 {
			t.Fatalf("width %d want %d (%q)", lipgloss.Width(plain), lipgloss.Width(in)+1, plain)
		}
		if !strings.HasSuffix(plain, in) {
			t.Fatalf("lost content: %q", plain)
		}
	})

	t.Run("prefixes every line", func(t *testing.T) {
		t.Parallel()
		got := withLeftGutter("one\ntwo\nthree", false)
		lines := strings.Split(stripANSI(got), "\n")
		if len(lines) != 3 {
			t.Fatalf("lines: %v", lines)
		}
		for _, line := range lines {
			if !strings.HasPrefix(line, gutterGlyph) {
				t.Fatalf("missing gutter on %q", line)
			}
		}
	})

	t.Run("blank lines still get a gutter", func(t *testing.T) {
		t.Parallel()
		got := withLeftGutter("a\n\nb", true)
		lines := strings.Split(stripANSI(got), "\n")
		if len(lines) != 3 {
			t.Fatalf("lines: %v", lines)
		}
		if lines[1] != gutterGlyph {
			t.Fatalf("blank line gutter: %q", lines[1])
		}
	})

	t.Run("empty content is one gutter cell", func(t *testing.T) {
		t.Parallel()
		got := stripANSI(withLeftGutter("", false))
		if got != gutterGlyph {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("active vs idle differ", func(t *testing.T) {
		t.Parallel()
		if withLeftGutter("x", true) == withLeftGutter("x", false) {
			t.Fatal("active and idle framed output must differ")
		}
	})
}

func TestRegionActive(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		focus    focusArea
		detail   bool
		wantMsg  bool
		wantComp bool
	}{
		{name: "compose", focus: focusCompose, wantComp: true},
		{name: "messages", focus: focusMessages, wantMsg: true},
		{name: "detail", focus: focusMessages, detail: true, wantMsg: true},
		{name: "detail wins over compose focus", focus: focusCompose, detail: true, wantMsg: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := Model{focus: tc.focus, detail: detailView{open: tc.detail}}
			if got := m.messagesRegionActive(); got != tc.wantMsg {
				t.Fatalf("messagesRegionActive=%v want %v", got, tc.wantMsg)
			}
			if got := m.composeRegionActive(); got != tc.wantComp {
				t.Fatalf("composeRegionActive=%v want %v", got, tc.wantComp)
			}
		})
	}
}

func focusReadyModel() Model {
	m := New(nil, 5*time.Second)
	m.width = 80
	m.height = 24
	m.ready = true
	m.layout()
	m.redraw()
	return m
}

func TestComposePlaceholderOmitsHints(t *testing.T) {
	t.Parallel()
	m := New(nil, time.Hour)
	if m.textarea.Placeholder != "message…" {
		t.Fatalf("placeholder %q", m.textarea.Placeholder)
	}
}

func TestComposePromptFocusStyle(t *testing.T) {
	t.Parallel()
	m := New(nil, time.Hour)
	styles := m.textarea.Styles()
	if styles.Focused.Prompt.GetForeground() != colorAccent {
		t.Fatalf("focused prompt fg %v want %v", styles.Focused.Prompt.GetForeground(), colorAccent)
	}
	if styles.Blurred.Prompt.GetForeground() != colorDim {
		t.Fatalf("blurred prompt fg %v want %v", styles.Blurred.Prompt.GetForeground(), colorDim)
	}
}

func TestRenderMainFocusGutter(t *testing.T) {
	t.Parallel()

	t.Run("compose idles viewport and accents prompt", func(t *testing.T) {
		t.Parallel()
		m := focusReadyModel()
		m.focus = focusCompose
		m.textarea.Focus()
		got := m.renderMain()
		idle := withLeftGutter(m.viewport.View(), false)
		active := withLeftGutter(m.viewport.View(), true)
		if !strings.Contains(got, idle) {
			t.Fatal("compose focus should idle-gutter the viewport")
		}
		if strings.Contains(got, active) {
			t.Fatal("compose focus should not accent the viewport")
		}
	})

	t.Run("messages accents viewport", func(t *testing.T) {
		t.Parallel()
		m := focusReadyModel()
		m.focus = focusMessages
		m.textarea.Blur()
		got := m.renderMain()
		if !strings.Contains(got, withLeftGutter(m.viewport.View(), true)) {
			t.Fatal("messages focus should accent the viewport")
		}
		if strings.Contains(got, withLeftGutter(m.viewport.View(), false)) {
			t.Fatal("messages focus should not idle-gutter the viewport")
		}
	})

	t.Run("detail accents viewport", func(t *testing.T) {
		t.Parallel()
		m := focusReadyModel()
		m.store.upsert(api.Message{
			ID: "A", Text: "root", CreatedAt: 100,
			Author: api.Author{Handle: "alice"},
		})
		m.selected = 0
		got, _ := m.openSelectedDetail()
		m = got.(Model)
		out := m.renderMain()
		if !strings.Contains(out, withLeftGutter(m.viewport.View(), true)) {
			t.Fatal("detail should accent the viewport")
		}
	})

	t.Run("tab does not change viewport width", func(t *testing.T) {
		t.Parallel()
		m := focusReadyModel()
		m.focus = focusCompose
		m.layout()
		w := m.viewport.Width()
		if w != m.contentWidth() {
			t.Fatalf("viewport width %d want contentWidth %d", w, m.contentWidth())
		}
		m.focus = focusMessages
		m.layout()
		if m.viewport.Width() != w {
			t.Fatalf("viewport width changed on focus: %d -> %d", w, m.viewport.Width())
		}
	})

	t.Run("reply banner follows compose focus", func(t *testing.T) {
		t.Parallel()
		m := focusReadyModel()
		m.replyToID = "A"
		m.replyLabel = "@alice: hi"
		banner := styleBanner.Render("↳ reply " + m.replyLabel + "  (esc cancel)")
		m.focus = focusCompose
		got := m.renderMain()
		if !strings.Contains(got, withLeftGutter(banner, true)) {
			t.Fatal("compose+reply should accent the banner gutter")
		}
		m.focus = focusMessages
		got = m.renderMain()
		if !strings.Contains(got, withLeftGutter(banner, false)) {
			t.Fatal("messages+reply should idle the banner gutter")
		}
	})

	t.Run("profile overlay keeps card and underlying gutter", func(t *testing.T) {
		t.Parallel()
		m := focusReadyModel()
		m.focus = focusMessages
		m.profile = profileModal{open: true, loading: true, handle: "milady"}
		base := m.renderMain()
		if !strings.Contains(base, withLeftGutter(m.viewport.View(), true)) {
			t.Fatal("renderMain should still gutter under profile")
		}
		got := m.renderProfileOverlay(base)
		if !strings.Contains(got, "@milady") {
			t.Fatalf("missing profile card: %q", stripANSI(got))
		}
		// Rounded border still marks the card as the attention winner.
		if !strings.Contains(got, "╭") {
			t.Fatalf("profile card missing rounded border: %q", got)
		}
	})

	t.Run("help has no region gutter", func(t *testing.T) {
		t.Parallel()
		m := focusReadyModel()
		m.showHelp = true
		plain := stripANSI(m.View().Content)
		if strings.Contains(plain, gutterGlyph) {
			t.Fatalf("help should not show region gutter: %q", plain)
		}
		if !strings.Contains(plain, "remiterm help") {
			t.Fatalf("missing help: %q", plain)
		}
	})
}
