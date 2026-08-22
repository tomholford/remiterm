package tui

import (
	"strings"
	"testing"
	"time"

	"remiterm/internal/api"
)

func detailTestModel(msgs ...api.Message) Model {
	m := New(nil, time.Hour)
	m.focus = focusMessages
	m.width = 80
	m.height = 40
	for _, msg := range msgs {
		m.store.upsert(msg)
	}
	m.layout()
	return m
}

func TestOpenAndCloseDetail(t *testing.T) {
	t.Parallel()
	m := detailTestModel(
		api.Message{ID: "A", Text: "root", CreatedAt: 100, Author: api.Author{Handle: "alice"}},
		api.Message{ID: "B", Text: "child of A", CreatedAt: 200, ReplyToID: "A", Author: api.Author{Handle: "bob"}},
	)
	m.selected = m.store.indexOf("B")

	got, _ := m.openSelectedDetail()
	m = got.(Model)
	if !m.detail.open {
		t.Fatal("detail should be open")
	}
	if m.detail.focusID != "B" {
		t.Fatalf("focusID = %q", m.detail.focusID)
	}
	if fi := focusIndex(m.detail.chain); fi < 0 || m.detail.selected != fi {
		t.Fatalf("selected=%d focusIndex=%d", m.detail.selected, fi)
	}
	// Viewport content is detail, not the plain list stream.
	if !strings.Contains(m.content, "message detail") {
		t.Fatalf("content missing detail header: %q", m.content)
	}
	if !strings.Contains(m.content, "child of A") {
		t.Fatalf("content missing body: %q", m.content)
	}

	// Select ancestor in chain, then Esc — list should land on focus B, not A.
	for i, it := range m.detail.chain {
		if it.Msg.ID == "A" {
			m.detail.selected = i
			break
		}
	}
	got, _ = m.closeDetail()
	m = got.(Model)
	if m.detail.open {
		t.Fatal("detail should be closed")
	}
	if m.store.idAt(m.selected) != "B" {
		t.Fatalf("list selected id = %q want B", m.store.idAt(m.selected))
	}
	if m.focus != focusMessages {
		t.Fatalf("focus = %v", m.focus)
	}
}

func TestDetailReRoot(t *testing.T) {
	t.Parallel()
	m := detailTestModel(
		api.Message{ID: "A", Text: "root", CreatedAt: 100, Author: api.Author{Handle: "alice"}},
		api.Message{ID: "B", Text: "mid", CreatedAt: 200, ReplyToID: "A", Author: api.Author{Handle: "bob"}},
		api.Message{ID: "C", Text: "leaf", CreatedAt: 300, ReplyToID: "B", Author: api.Author{Handle: "cara"}},
	)
	got, _ := m.openDetail("C")
	m = got.(Model)

	// Move selection to ancestor B and re-root.
	for i, it := range m.detail.chain {
		if it.Msg.ID == "B" {
			m.detail.selected = i
			break
		}
	}
	got, _ = m.detailReRoot()
	m = got.(Model)
	if m.detail.focusID != "B" {
		t.Fatalf("focusID = %q want B", m.detail.focusID)
	}
	// Children of B should include C.
	foundC := false
	for _, it := range m.detail.chain {
		if it.Kind == chainChild && it.Msg.ID == "C" {
			foundC = true
		}
	}
	if !foundC {
		t.Fatalf("chain after re-root missing child C: %+v", chainKinds(m.detail.chain))
	}

	// Esc selects re-rooted focus B in the list.
	got, _ = m.closeDetail()
	m = got.(Model)
	if m.store.idAt(m.selected) != "B" {
		t.Fatalf("list selected = %q", m.store.idAt(m.selected))
	}
}

func TestDetailReplyClosesAndStartsCompose(t *testing.T) {
	t.Parallel()
	m := detailTestModel(
		api.Message{ID: "A", Text: "hello world", CreatedAt: 100, Author: api.Author{Handle: "alice"}},
	)
	got, _ := m.openDetail("A")
	m = got.(Model)
	got, _ = m.detailReply()
	m = got.(Model)
	if m.detail.open {
		t.Fatal("detail should close on reply")
	}
	if m.replyToID != "A" {
		t.Fatalf("replyToID = %q", m.replyToID)
	}
	if m.focus != focusCompose {
		t.Fatalf("focus = %v want compose", m.focus)
	}
	if !strings.Contains(m.replyLabel, "alice") {
		t.Fatalf("replyLabel = %q", m.replyLabel)
	}
}

func TestDetailMissingParentPlaceholder(t *testing.T) {
	t.Parallel()
	m := detailTestModel(
		api.Message{ID: "B", Text: "orphan", CreatedAt: 200, ReplyToID: "0", Author: api.Author{Handle: "bob"}},
	)
	got, _ := m.openDetail("B")
	m = got.(Model)
	if !strings.Contains(m.content, "parent not loaded") {
		t.Fatalf("want missing parent copy: %q", m.content)
	}
	// Re-root on placeholder is a no-op.
	m.detail.selected = 0 // missing parent row
	got, _ = m.detailReRoot()
	m = got.(Model)
	if m.detail.focusID != "B" {
		t.Fatalf("focus should stay B, got %q", m.detail.focusID)
	}
}

func TestDetailAncestorUsesTimestampPref(t *testing.T) {
	t.Parallel()
	parentWhen := time.Date(2020, 1, 2, 15, 4, 0, 0, time.Local)
	m := detailTestModel(
		api.Message{ID: "A", Text: "root", CreatedAt: parentWhen.UnixMilli(), Author: api.Author{Handle: "alice"}},
		api.Message{ID: "B", Text: "child", CreatedAt: time.Now().UnixMilli(), ReplyToID: "A", Author: api.Author{Handle: "bob"}},
	)
	m.prefs.TimestampFormat = "15:04"
	got, _ := m.openDetail("B")
	m = got.(Model)
	want := formatTimestamp(parentWhen, time.Now(), "15:04")
	plain := stripANSI(m.content)
	if want == "" || !strings.Contains(plain, want) {
		t.Fatalf("ancestor missing stamp %q in %q", want, plain)
	}
}

func TestDetailChainAuthorLabel(t *testing.T) {
	t.Parallel()
	m := detailTestModel(
		api.Message{
			ID: "1", Text: "hi", CreatedAt: 100,
			Author: api.Author{Handle: "milady", DisplayName: "milady maker"},
		},
	)
	m.prefs.AuthorLabel = "both"
	got, _ := m.openDetail("1")
	m = got.(Model)
	plain := stripANSI(m.content)
	if !strings.Contains(plain, "milady maker (@milady)") {
		t.Fatalf("detail should use both label: %q", plain)
	}
}

func TestDetailLongTimestampOnFocus(t *testing.T) {
	t.Parallel()
	// Fixed millis: 2026-08-11 14:19:15 UTC
	const ts int64 = 1786457955022
	m := detailTestModel(
		api.Message{ID: "1", Text: "hi", CreatedAt: ts, Author: api.Author{Handle: "x"}},
	)
	m.prefs.TimestampFormat = "relative"
	got, _ := m.openDetail("1")
	m = got.(Model)
	// Local format includes date portion.
	if !strings.Contains(m.content, "2026-08-11") && !strings.Contains(m.content, "2026-") {
		// Timezone may shift calendar day; at least long form has seconds.
		if !strings.Contains(m.content, ":15") {
			t.Fatalf("expected long timestamp in content: %q", m.content)
		}
	}
}

func TestOpenSelectedDetailEmptyStore(t *testing.T) {
	t.Parallel()
	m := detailTestModel()
	got, _ := m.openSelectedDetail()
	m = got.(Model)
	if m.detail.open {
		t.Fatal("should not open on empty store")
	}
}

func TestEnsureDetailSelectedVisibleScrollsOnFirstOcclusion(t *testing.T) {
	t.Parallel()
	// Deep chain of tall focus-style bodies so the viewport must scroll.
	var msgs []api.Message
	for i := 0; i < 20; i++ {
		msg := api.Message{
			ID:        string(rune('A' + i)),
			Text:      "chain row body that wraps and eats vertical space · " + strings.Repeat("pad ", 12),
			CreatedAt: int64(100 + i),
			Author:    api.Author{Handle: "u"},
		}
		if i > 0 {
			msg.ReplyToID = string(rune('A' + i - 1))
		}
		msgs = append(msgs, msg)
	}
	m := detailTestModel(msgs...)
	got, _ := m.openDetail(msgs[len(msgs)-1].ID)
	m = got.(Model)
	m.detail.selected = len(m.detail.chain) - 1
	m.redraw()
	m.viewport.GotoBottom()

	for step := 0; step < len(m.detail.chain); step++ {
		if m.detail.selected <= 0 {
			t.Fatal("never left the viewport")
		}
		m.detail.selected--
		m.redraw()
		span := m.itemSpans[m.detail.selected]
		top := m.viewport.YOffset()
		if span.start >= top {
			m.ensureDetailSelectedVisible()
			continue
		}
		m.ensureDetailSelectedVisible()
		if m.viewport.YOffset() != span.start {
			t.Fatalf("first occlusion: YOffset=%d want %d (step %d)",
				m.viewport.YOffset(), span.start, step)
		}
		return
	}
	t.Fatal("detail selection never started above the viewport")
}

func TestRefreshDetailChainKeepsSelection(t *testing.T) {
	t.Parallel()
	m := detailTestModel(
		api.Message{ID: "A", Text: "root", CreatedAt: 100, Author: api.Author{Handle: "alice"}},
	)
	got, _ := m.openDetail("A")
	m = got.(Model)
	// Poll adds a child while detail is open.
	m.store.upsert(api.Message{ID: "B", Text: "reply", CreatedAt: 200, ReplyToID: "A", Author: api.Author{Handle: "bob"}})
	m.redraw()
	found := false
	for _, it := range m.detail.chain {
		if it.Kind == chainChild && it.Msg.ID == "B" {
			found = true
		}
	}
	if !found {
		t.Fatalf("refresh should add child: %+v", chainKinds(m.detail.chain))
	}
	if m.detail.focusID != "A" {
		t.Fatalf("focusID = %q", m.detail.focusID)
	}
}
