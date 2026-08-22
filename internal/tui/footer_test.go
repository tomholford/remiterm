package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"remiterm/internal/api"
)

func TestFooterKeyHints(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		focus        focusArea
		profileOpen  bool
		detailOpen   bool
		settingsOpen bool
		previewOpen  bool
		wantSub      []string // must all be present
		wantNot      []string // must all be absent
		wantEmpty    bool
	}{
		{
			name:  "compose",
			focus: focusCompose,
			wantSub: []string{
				"Enter send",
				"Ctrl+J newline",
				"Tab messages",
				"Ctrl+G help",
				"Ctrl+S settings",
				"Ctrl+C quit",
			},
			wantNot: []string{
				"R reply",
				"p profile",
				"o open",
				"q quit",
				"? help",
			},
		},
		{
			name:  "messages",
			focus: focusMessages,
			wantSub: []string{
				"j/k",
				"Enter",
				"r reply",
				"p profile",
				"m me",
				"o/i",
				"Tab compose",
				"?",
				"q",
			},
			wantNot: []string{
				"Enter send",
				"Enter detail",
				"o open",
				"i preview",
				"? help",
				"q quit",
			},
		},
		{
			name:       "detail open",
			focus:      focusMessages,
			detailOpen: true,
			wantSub: []string{
				"j/k",
				"Enter",
				"r reply",
				"o/i",
				"p profile",
				"Esc",
				"?",
			},
			wantNot: []string{
				"Enter send",
				"Tab compose",
				"q quit",
				"Esc back",
				"j/k chain",
			},
		},
		{
			name:        "profile open (compose focus)",
			focus:       focusCompose,
			profileOpen: true,
			wantEmpty:   true,
		},
		{
			name:        "profile open (messages focus)",
			focus:       focusMessages,
			profileOpen: true,
			wantEmpty:   true,
		},
		{
			name:        "profile wins over detail",
			focus:       focusMessages,
			profileOpen: true,
			detailOpen:  true,
			wantEmpty:   true,
		},
		{
			name:         "settings open",
			focus:        focusMessages,
			settingsOpen: true,
			wantSub: []string{
				"j/k select",
				"Enter cycle",
				"Esc close",
			},
			wantNot: []string{
				"Enter send",
				"Enter detail",
				"q quit",
			},
		},
		{
			name:         "profile wins over settings",
			focus:        focusMessages,
			profileOpen:  true,
			settingsOpen: true,
			wantEmpty:    true,
		},
		{
			name:        "preview open",
			focus:       focusMessages,
			previewOpen: true,
			wantEmpty:   true,
		},
		{
			name:        "preview wins over profile",
			focus:       focusMessages,
			profileOpen: true,
			previewOpen: true,
			wantEmpty:   true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := footerKeyHints(tc.focus, tc.profileOpen, tc.detailOpen, tc.settingsOpen, tc.previewOpen)
			if tc.wantEmpty {
				if got != "" {
					t.Fatalf("want empty, got %q", got)
				}
				return
			}
			for _, s := range tc.wantSub {
				if !strings.Contains(got, s) {
					t.Fatalf("missing %q in %q", s, got)
				}
			}
			for _, s := range tc.wantNot {
				if strings.Contains(got, s) {
					t.Fatalf("unexpected %q in %q", s, got)
				}
			}
		})
	}
}

func TestRenderMainOmitsModeTag(t *testing.T) {
	t.Parallel()
	tags := []string{"[compose]", "[messages]", "[detail]"}

	t.Run("compose", func(t *testing.T) {
		t.Parallel()
		m := focusReadyModel()
		m.focus = focusCompose
		plain := stripANSI(m.renderMain())
		for _, tag := range tags {
			if strings.Contains(plain, tag) {
				t.Fatalf("footer has %s: %q", tag, plain)
			}
		}
		if !strings.Contains(plain, "Enter send") {
			t.Fatalf("missing hints: %q", plain)
		}
		if !strings.Contains(plain, "Ctrl+G help") {
			t.Fatalf("missing compose help chord: %q", plain)
		}
		if !strings.Contains(plain, "Ctrl+S settings") {
			t.Fatalf("missing compose settings chord: %q", plain)
		}
	})

	t.Run("messages", func(t *testing.T) {
		t.Parallel()
		m := focusReadyModel()
		m.focus = focusMessages
		m.textarea.Blur()
		plain := stripANSI(m.renderMain())
		for _, tag := range tags {
			if strings.Contains(plain, tag) {
				t.Fatalf("footer has %s: %q", tag, plain)
			}
		}
		if !strings.Contains(plain, "Tab compose") {
			t.Fatalf("missing hints: %q", plain)
		}
	})

	t.Run("detail", func(t *testing.T) {
		t.Parallel()
		m := focusReadyModel()
		m.store.upsert(api.Message{
			ID: "A", Text: "root", CreatedAt: 100,
			Author: api.Author{Handle: "alice"},
		})
		m.selected = 0
		got, _ := m.openSelectedDetail()
		m = got.(Model)
		plain := stripANSI(m.renderMain())
		for _, tag := range tags {
			if strings.Contains(plain, tag) {
				t.Fatalf("footer has %s: %q", tag, plain)
			}
		}
		if !strings.Contains(plain, "Esc") {
			t.Fatalf("missing hints: %q", plain)
		}
	})
}

func TestFooterWrapsAndFits(t *testing.T) {
	t.Parallel()
	m := focusReadyModel()
	m.focus = focusMessages
	m.textarea.Blur()
	oneRow := m.viewport.Height()

	m.errMsg = strings.Repeat("e", 400)
	m.redraw()
	got := m.renderMain()
	plain := stripANSI(got)
	if !strings.Contains(plain, strings.Repeat("e", 20)) {
		t.Fatalf("missing wrapped error: %q", plain)
	}
	if m.viewport.Height() >= oneRow {
		t.Fatalf("wrapped footer should shrink viewport: one=%d wrap=%d", oneRow, m.viewport.Height())
	}
	if h := lipgloss.Height(got); h > m.height {
		t.Fatalf("render height %d > terminal %d", h, m.height)
	}
}

func TestFooterStatusDoesNotReflowViewport(t *testing.T) {
	t.Parallel()
	statuses := []string{"sent", "opened in browser", "sending…"}
	for _, focus := range []focusArea{focusCompose, focusMessages} {
		t.Run(focusName(focus), func(t *testing.T) {
			t.Parallel()
			m := focusReadyModel()
			m.focus = focus
			if focus == focusMessages {
				m.textarea.Blur()
			}
			m.redraw()
			idleVP := m.viewport.Height()
			idleFooter := lipgloss.Height(m.renderFooter())

			for _, status := range statuses {
				m.status = status
				m.redraw()
				if m.viewport.Height() != idleVP {
					t.Fatalf("status %q changed viewport: idle=%d got=%d", status, idleVP, m.viewport.Height())
				}
				if h := lipgloss.Height(m.renderFooter()); h != idleFooter {
					t.Fatalf("status %q changed footer height: idle=%d got=%d", status, idleFooter, h)
				}
				plain := stripANSI(m.renderFooter())
				if !strings.Contains(plain, status) {
					t.Fatalf("missing %q in %q", status, plain)
				}
				if !strings.Contains(plain, "Tab") {
					t.Fatalf("hints dropped while status %q: %q", status, plain)
				}
			}

			m.status = ""
			m.redraw()
			if m.viewport.Height() != idleVP {
				t.Fatalf("clear changed viewport: idle=%d got=%d", idleVP, m.viewport.Height())
			}
			plain := stripANSI(m.renderFooter())
			if strings.Contains(plain, "sent") {
				t.Fatalf("idle footer still has sent: %q", plain)
			}
		})
	}
}

func TestFooterStatusSlotIdleIsBlank(t *testing.T) {
	t.Parallel()
	// Compose hints fit on one row near width 100 without a slot, but wrap
	// once 17 cells are reserved. Idle must already pay that reservation.
	m := focusReadyModel()
	m.focus = focusCompose
	m.width = 100
	m.height = 24
	m.status = ""
	m.redraw()
	idleRows := m.footerRows()
	idleVP := m.viewport.Height()
	idlePlain := stripANSI(m.renderFooter())
	if strings.Contains(idlePlain, "sent") || strings.Contains(idlePlain, "sending") {
		t.Fatalf("idle footer leaked status: %q", idlePlain)
	}

	m.status = "sent"
	m.redraw()
	if m.footerRows() != idleRows {
		t.Fatalf("idle must reserve the slot: idle rows=%d sent rows=%d", idleRows, m.footerRows())
	}
	if m.viewport.Height() != idleVP {
		t.Fatalf("viewport jumped: idle=%d sent=%d", idleVP, m.viewport.Height())
	}
	if !strings.Contains(stripANSI(m.renderFooter()), "sent") {
		t.Fatalf("missing sent: %q", stripANSI(m.renderFooter()))
	}
}

func TestClipCellWidth(t *testing.T) {
	t.Parallel()
	if got := clipCellWidth("opened in browser", statusSlotWidth); got != "opened in browser" {
		t.Fatalf("longest status clipped: %q", got)
	}
	long := strings.Repeat("x", statusSlotWidth+10)
	got := clipCellWidth(long, statusSlotWidth)
	if lipgloss.Width(got) > statusSlotWidth {
		t.Fatalf("width %d want <= %d (%q)", lipgloss.Width(got), statusSlotWidth, got)
	}
	if clipCellWidth("sent", 0) != "" {
		t.Fatal("zero width should clip to empty")
	}
}

func focusName(f focusArea) string {
	if f == focusMessages {
		return "messages"
	}
	return "compose"
}

func TestHelpHotkey(t *testing.T) {
	t.Parallel()

	t.Run("ctrl+g opens help from compose", func(t *testing.T) {
		t.Parallel()
		m := New(nil, time.Hour)
		m = press(m, "a")
		if m.textarea.Value() != "a" {
			t.Fatalf("typed %q", m.textarea.Value())
		}
		m = press(m, "ctrl+g")
		if !m.showHelp {
			t.Fatal("ctrl+g in compose should open help")
		}
		if m.textarea.Value() != "a" {
			t.Fatalf("draft changed: %q", m.textarea.Value())
		}
	})

	t.Run("question mark types in compose", func(t *testing.T) {
		t.Parallel()
		m := New(nil, time.Hour)
		m = press(m, "?")
		if m.showHelp {
			t.Fatal("? in compose should type, not open help")
		}
		if m.textarea.Value() != "?" {
			t.Fatalf("compose should type ?, got %q", m.textarea.Value())
		}
	})

	t.Run("question mark opens help from messages", func(t *testing.T) {
		t.Parallel()
		m := settingsTestModel()
		m = press(m, "?")
		if !m.showHelp {
			t.Fatal("? in messages should open help")
		}
	})

	t.Run("closes help", func(t *testing.T) {
		t.Parallel()
		for _, key := range []string{"ctrl+g", "?", "esc", "q"} {
			m := New(nil, time.Hour)
			m = press(m, "ctrl+g")
			if !m.showHelp {
				t.Fatalf("%s setup: help should be open", key)
			}
			m = press(m, key)
			if m.showHelp {
				t.Fatalf("%s should close help", key)
			}
		}
	})
}
