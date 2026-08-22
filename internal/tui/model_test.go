package tui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"remiterm/internal/api"
	"remiterm/internal/cache"
)

func testModelWithHistory() Model {
	m := New(nil, time.Hour)
	m.store.hasMore = true
	m.loadingOld = false
	m.lastOldFetchAt = time.Time{}
	m.oldLoadBurstUsed = 0
	m.focus = focusMessages
	m.width = 80
	m.height = 40
	m.layout()
	return m
}

func TestCanLoadOlder(t *testing.T) {
	t.Parallel()

	t.Run("first auto allowed", func(t *testing.T) {
		m := testModelWithHistory()
		if !m.canLoadOlder(false) {
			t.Fatal("expected first auto load allowed")
		}
	})

	t.Run("blocked while in flight", func(t *testing.T) {
		m := testModelWithHistory()
		m.loadingOld = true
		if m.canLoadOlder(false) || m.canLoadOlder(true) {
			t.Fatal("in-flight must block auto and force")
		}
	})

	t.Run("blocked without hasMore", func(t *testing.T) {
		m := testModelWithHistory()
		m.store.hasMore = false
		if m.canLoadOlder(false) || m.canLoadOlder(true) {
			t.Fatal("hasMore false must block")
		}
	})

	t.Run("burst free then cooldown", func(t *testing.T) {
		m := testModelWithHistory()
		m.oldLoadBurstUsed = oldLoadBurstFree
		m.lastOldFetchAt = time.Now()
		if m.canLoadOlder(false) {
			t.Fatal("after burst, cooldown should block auto")
		}
		if !m.canLoadOlder(true) {
			t.Fatal("force should bypass cooldown")
		}
		m.lastOldFetchAt = time.Now().Add(-oldLoadCooldown - time.Millisecond)
		if !m.canLoadOlder(false) {
			t.Fatal("expected auto after cooldown")
		}
	})

	t.Run("within burst ignores cooldown", func(t *testing.T) {
		m := testModelWithHistory()
		m.oldLoadBurstUsed = oldLoadBurstFree - 1
		m.lastOldFetchAt = time.Now()
		if !m.canLoadOlder(false) {
			t.Fatal("burst remaining should ignore cooldown")
		}
	})
}

func TestTryLoadOlder(t *testing.T) {
	t.Parallel()

	t.Run("starts fetch and increments burst", func(t *testing.T) {
		m := testModelWithHistory()
		cmd := m.tryLoadOlder(false)
		if cmd == nil {
			t.Fatal("expected cmd")
		}
		if !m.loadingOld {
			t.Fatal("loadingOld should be set")
		}
		if m.oldLoadBurstUsed != 1 {
			t.Fatalf("burst used = %d, want 1", m.oldLoadBurstUsed)
		}
		// Cooldown clock is set on completion, not start.
		if !m.lastOldFetchAt.IsZero() {
			t.Fatal("lastOldFetchAt should remain zero until completion")
		}
	})

	t.Run("second auto while in flight is nil", func(t *testing.T) {
		m := testModelWithHistory()
		if m.tryLoadOlder(false) == nil {
			t.Fatal("first should start")
		}
		if m.tryLoadOlder(false) != nil {
			t.Fatal("second while in-flight should be nil")
		}
	})

	t.Run("serial pages within burst after complete", func(t *testing.T) {
		m := testModelWithHistory()
		for i := 0; i < oldLoadBurstFree; i++ {
			if m.tryLoadOlder(false) == nil {
				t.Fatalf("burst page %d should start", i+1)
			}
			m.loadingOld = false // simulate pollMsg older done
			m.lastOldFetchAt = time.Now()
		}
		// Burst exhausted; cooling from completion.
		if m.tryLoadOlder(false) != nil {
			t.Fatal("post-burst cooldown should block")
		}
		m.lastOldFetchAt = time.Now().Add(-oldLoadCooldown - time.Millisecond)
		if m.tryLoadOlder(false) == nil {
			t.Fatal("expected auto after cooldown")
		}
	})

	t.Run("force while idle starts even if cooling after burst", func(t *testing.T) {
		m := testModelWithHistory()
		m.oldLoadBurstUsed = oldLoadBurstFree
		m.lastOldFetchAt = time.Now()
		cmd := m.tryLoadOlder(true)
		if cmd == nil {
			t.Fatal("force should start")
		}
		if !m.loadingOld {
			t.Fatal("force should set loadingOld")
		}
		// force does not burn burst budget
		if m.oldLoadBurstUsed != oldLoadBurstFree {
			t.Fatalf("force should not change burst used, got %d", m.oldLoadBurstUsed)
		}
	})

	t.Run("force while in flight is nil", func(t *testing.T) {
		m := testModelWithHistory()
		m.loadingOld = true
		if m.tryLoadOlder(true) != nil {
			t.Fatal("force must not overlap in-flight")
		}
	})
}

func TestResetOldLoadBurst(t *testing.T) {
	t.Parallel()
	m := testModelWithHistory()
	m.oldLoadBurstUsed = oldLoadBurstFree
	m.resetOldLoadBurst()
	if m.oldLoadBurstUsed != 0 {
		t.Fatal("reset should clear burst used")
	}
	m.lastOldFetchAt = time.Now()
	if !m.canLoadOlder(false) {
		t.Fatal("after reset, burst should allow load despite fresh lastOldFetchAt")
	}
}

func TestMaybePrefetchOlder(t *testing.T) {
	t.Parallel()

	t.Run("compose focus never prefetches", func(t *testing.T) {
		m := testModelWithHistory()
		m.focus = focusCompose
		seedShortHistory(&m, 5)
		if m.maybePrefetchOlder() != nil {
			t.Fatal("compose must not prefetch")
		}
	})

	t.Run("near top with hasMore starts load", func(t *testing.T) {
		m := testModelWithHistory()
		seedShortHistory(&m, 5)
		m.viewport.GotoTop()
		cmd := m.maybePrefetchOlder()
		if cmd == nil {
			t.Fatal("expected prefetch near top")
		}
		if !m.loadingOld {
			t.Fatal("loadingOld should be set")
		}
	})

	t.Run("far from top resets burst and skips", func(t *testing.T) {
		m := testModelWithHistory()
		seedTallHistory(&m, 200)
		m.oldLoadBurstUsed = oldLoadBurstFree
		m.viewport.GotoBottom()
		if m.maybePrefetchOlder() != nil {
			t.Fatal("far from top should not prefetch")
		}
		if m.oldLoadBurstUsed != 0 {
			t.Fatal("leave zone should reset burst")
		}
	})

	t.Run("in flight does not double-fetch", func(t *testing.T) {
		m := testModelWithHistory()
		seedShortHistory(&m, 5)
		m.viewport.GotoTop()
		if m.maybePrefetchOlder() == nil {
			t.Fatal("first prefetch")
		}
		if m.maybePrefetchOlder() != nil {
			t.Fatal("second while in-flight should be nil")
		}
	})
}

func TestOlderPollMsgAnchorsAndCooldown(t *testing.T) {
	t.Parallel()
	m := testModelWithHistory()
	seedTallHistory(&m, 80)
	// Select a mid-list message by id so we can prove remap after prepend.
	keepID := m.store.idAt(40)
	if keepID == "" {
		t.Fatal("expected id at 40")
	}
	m.selected = 40
	// Scroll into the middle so a prepend must move YOffset.
	m.viewport.SetYOffset(15)
	yBefore := m.viewport.YOffset()
	if yBefore == 0 {
		t.Fatal("need non-zero offset for anchor check")
	}

	// Simulate an older page completing with more messages.
	m.loadingOld = true
	m.oldLoadBurstUsed = 1
	older := api.ListMessagesResponse{
		Messages: []api.Message{
			{ID: "old-a", Text: "older a", Author: api.Author{Handle: "a"}, CreatedAt: 1},
			{ID: "old-b", Text: "older b", Author: api.Author{Handle: "b"}, CreatedAt: 2},
			{ID: "old-c", Text: "older c", Author: api.Author{Handle: "c"}, CreatedAt: 3},
		},
		HasMore:    true,
		NextCursor: "old-a",
	}
	next, cmd := m.Update(pollMsg{res: older, older: true})
	m = next.(Model)
	if m.loadingOld {
		t.Fatal("loadingOld should clear")
	}
	if m.lastOldFetchAt.IsZero() {
		t.Fatal("completion should set lastOldFetchAt")
	}
	if m.viewport.YOffset() <= yBefore {
		t.Fatalf("expected YOffset to increase after prepend: before=%d after=%d", yBefore, m.viewport.YOffset())
	}
	if m.store.idAt(m.selected) != keepID {
		t.Fatalf("selection should stay on %q, got idx=%d id=%q", keepID, m.selected, m.store.idAt(m.selected))
	}
	if m.selected != 43 { // 40 + 3 older messages
		t.Fatalf("selected index = %d, want 43 after 3 prepended", m.selected)
	}
	// Burst exhausted + fresh completion → cooldown blocks immediate auto.
	if m.canLoadOlder(false) {
		t.Fatal("should be in cooldown after completion with burst used")
	}
	_ = cmd
}

func TestRenderMessagesStableDuringLoad(t *testing.T) {
	t.Parallel()
	m := testModelWithHistory()
	seedShortHistory(&m, 3)
	m.loadingOld = false
	idle := m.renderMessages()
	m.loadingOld = true
	loading := m.renderMessages()
	if idle != loading {
		t.Fatal("message pane content must not change when loadingOld toggles (footer owns spinner)")
	}
	if strings.Contains(loading, "loading older") {
		t.Fatal("loading tip must not live in scroll content")
	}
}

func TestIndexOfAndIDAt(t *testing.T) {
	t.Parallel()
	m := testModelWithHistory()
	seedShortHistory(&m, 5)
	if m.store.indexOf("m2") != 2 {
		t.Fatalf("indexOf m2 = %d", m.store.indexOf("m2"))
	}
	if m.store.idAt(2) != "m2" {
		t.Fatalf("idAt 2 = %q", m.store.idAt(2))
	}
	if m.store.indexOf("nope") != -1 {
		t.Fatal("unknown id")
	}
	if m.store.idAt(-1) != "" || m.store.idAt(99) != "" {
		t.Fatal("out of range idAt")
	}
}

func TestSetTransientStatus(t *testing.T) {
	t.Parallel()

	t.Run("sets status and returns tick cmd", func(t *testing.T) {
		m := New(nil, time.Hour)
		cmd := m.setTransientStatus("sent")
		if m.status != "sent" {
			t.Fatalf("status = %q, want sent", m.status)
		}
		if m.statusGen != 1 {
			t.Fatalf("statusGen = %d, want 1", m.statusGen)
		}
		if cmd == nil {
			t.Fatal("expected non-nil clear tick cmd")
		}
	})

	t.Run("empty clears without tick", func(t *testing.T) {
		m := New(nil, time.Hour)
		m.status = "sending…"
		m.statusGen = 3
		cmd := m.setTransientStatus("")
		if m.status != "" {
			t.Fatalf("status = %q, want empty", m.status)
		}
		if m.statusGen != 4 {
			t.Fatalf("statusGen = %d, want 4", m.statusGen)
		}
		if cmd != nil {
			t.Fatal("empty status must not schedule a tick")
		}
	})
}

func TestClearStatusMsg(t *testing.T) {
	t.Parallel()

	t.Run("matching gen clears status", func(t *testing.T) {
		m := New(nil, time.Hour)
		_ = m.setTransientStatus("sent")
		gen := m.statusGen
		next, _ := m.Update(clearStatusMsg{gen: gen})
		m = next.(Model)
		if m.status != "" {
			t.Fatalf("status = %q, want empty", m.status)
		}
	})

	t.Run("stale gen leaves newer status", func(t *testing.T) {
		m := New(nil, time.Hour)
		_ = m.setTransientStatus("sent")
		oldGen := m.statusGen
		_ = m.setTransientStatus("opened in browser")
		next, _ := m.Update(clearStatusMsg{gen: oldGen})
		m = next.(Model)
		if m.status != "opened in browser" {
			t.Fatalf("status = %q, want opened in browser", m.status)
		}
	})

	t.Run("same string re-set not wiped by older tick", func(t *testing.T) {
		m := New(nil, time.Hour)
		_ = m.setTransientStatus("sent")
		oldGen := m.statusGen
		_ = m.setTransientStatus("sent")
		next, _ := m.Update(clearStatusMsg{gen: oldGen})
		m = next.(Model)
		if m.status != "sent" {
			t.Fatalf("status = %q, want sent (newer gen)", m.status)
		}
		// Current gen still clears.
		next, _ = m.Update(clearStatusMsg{gen: m.statusGen})
		m = next.(Model)
		if m.status != "" {
			t.Fatalf("status = %q after matching clear", m.status)
		}
	})
}

func TestSentMsgStatus(t *testing.T) {
	t.Parallel()

	t.Run("success sets sent with clear cmd", func(t *testing.T) {
		m := New(nil, time.Hour)
		m.status = "sending…"
		msg := api.Message{
			ID: "n1", Text: "hi", Author: api.Author{Handle: "me"}, CreatedAt: 1,
		}
		next, cmd := m.Update(sentMsg{msg: msg})
		m = next.(Model)
		if m.status != "sent" {
			t.Fatalf("status = %q, want sent", m.status)
		}
		if cmd == nil {
			t.Fatal("expected clear tick cmd after sent")
		}
	})

	t.Run("error clears sending status", func(t *testing.T) {
		m := New(nil, time.Hour)
		m.status = "sending…"
		next, _ := m.Update(sentMsg{err: fmt.Errorf("boom")})
		m = next.(Model)
		if m.status != "" {
			t.Fatalf("status = %q, want empty after send error", m.status)
		}
		if m.errMsg == "" {
			t.Fatal("expected errMsg set")
		}
	})
}

func seedShortHistory(m *Model, n int) {
	msgs := make([]api.Message, n)
	for i := 0; i < n; i++ {
		msgs[i] = api.Message{
			ID:        fmt.Sprintf("m%d", i),
			Text:      "msg",
			Author:    api.Author{Handle: "u"},
			CreatedAt: int64(1000 + i),
		}
	}
	res := api.ListMessagesResponse{Messages: msgs, HasMore: true, NextCursor: msgs[0].ID}
	m.store.mergeAPI(res, false)
	m.redraw()
}

func TestLineSpansMatchRenderedMessages(t *testing.T) {
	t.Parallel()
	m := testModelWithHistory()
	seedMixedHeightHistory(&m, 50)
	m.selected = 40
	m.focus = focusMessages
	m.redraw()

	if len(m.itemSpans) != m.store.len() {
		t.Fatalf("spans=%d store=%d", len(m.itemSpans), m.store.len())
	}
	lines := strings.Split(m.content, "\n")
	list := m.store.list()
	for i, msg := range list {
		span := m.itemSpans[i]
		formatted := m.formatMessage(msg, i == m.selected && m.focus == focusMessages)
		want := strings.Split(formatted, "\n")
		if span.end() > len(lines) {
			t.Fatalf("msg %d span [%d,%d) exceeds content lines %d", i, span.start, span.end(), len(lines))
		}
		got := lines[span.start:span.end()]
		if len(got) != len(want) {
			t.Fatalf("msg %d height=%d want %d", i, len(got), len(want))
		}
		for j := range want {
			if got[j] != want[j] {
				t.Fatalf("msg %d line %d: got %q want %q", i, j, got[j], want[j])
			}
		}
	}
}

func TestEnsureSelectedVisibleStaysPutWhileOnScreen(t *testing.T) {
	t.Parallel()
	m := testModelWithHistory()
	seedMixedHeightHistory(&m, 50)
	m.selected = m.store.len() - 1
	m.redraw()
	m.viewport.GotoBottom()

	moved := 0
	for step := 0; step < 8; step++ {
		if m.selected <= 0 {
			t.Fatal("ran out of rows before finishing stay-put walk")
		}
		m.selected--
		m.redraw()
		if m.selected >= len(m.itemSpans) {
			t.Fatalf("selected=%d spans=%d", m.selected, len(m.itemSpans))
		}
		span := m.itemSpans[m.selected]
		top := m.viewport.YOffset()
		bot := top + m.viewport.Height()
		if span.start < top || span.end() > bot {
			t.Fatalf("step %d: selected left the window before ensure; span=[%d,%d) view=[%d,%d)",
				step, span.start, span.end(), top, bot)
		}
		m.ensureSelectedVisible()
		if m.viewport.YOffset() != top {
			t.Fatalf("step %d: YOffset moved %d → %d while span still on screen",
				step, top, m.viewport.YOffset())
		}
		moved++
	}
	if moved == 0 {
		t.Fatal("expected to walk some on-screen rows")
	}
}

func TestEnsureSelectedVisibleScrollsOnFirstOcclusion(t *testing.T) {
	t.Parallel()
	m := testModelWithHistory()
	seedMixedHeightHistory(&m, 50)
	m.selected = m.store.len() - 1
	m.redraw()
	m.viewport.GotoBottom()

	for step := 0; step < m.store.len(); step++ {
		if m.selected <= 0 {
			t.Fatal("never left the viewport")
		}
		m.selected--
		m.redraw()
		span := m.itemSpans[m.selected]
		top := m.viewport.YOffset()
		if span.start >= top {
			m.ensureSelectedVisible()
			continue
		}
		m.ensureSelectedVisible()
		if m.viewport.YOffset() != span.start {
			t.Fatalf("first occlusion: YOffset=%d want span.start=%d (step %d)",
				m.viewport.YOffset(), span.start, step)
		}
		return
	}
	t.Fatal("selection never started above the viewport")
}

func TestJumpKeysMoveSelection(t *testing.T) {
	t.Parallel()

	t.Run("G selects latest then k stays near bottom", func(t *testing.T) {
		t.Parallel()
		m := testModelWithHistory()
		seedTallHistory(&m, 80)
		mid := 40
		m.selected = mid
		m.stickToBottom = false
		m.redraw()
		m.viewport.GotoTop()
		if m.viewport.AtBottom() {
			t.Fatal("need to start away from bottom")
		}

		m = press(m, "G")
		last := m.store.len() - 1
		if m.selected != last {
			t.Fatalf("G selected=%d want %d", m.selected, last)
		}
		if !m.stickToBottom {
			t.Fatal("G should stick to bottom")
		}
		if !m.viewport.AtBottom() {
			t.Fatal("G should scroll to bottom")
		}

		m = press(m, "k")
		if m.selected != last-1 {
			t.Fatalf("k after G selected=%d want %d (old mid was %d)", m.selected, last-1, mid)
		}
	})

	t.Run("g selects oldest then j stays near top", func(t *testing.T) {
		t.Parallel()
		m := testModelWithHistory()
		seedTallHistory(&m, 80)
		mid := 40
		m.selected = mid
		m.stickToBottom = true
		m.redraw()
		m.viewport.GotoBottom()
		if m.viewport.AtTop() {
			t.Fatal("need to start away from top")
		}

		m = press(m, "g")
		if m.selected != 0 {
			t.Fatalf("g selected=%d want 0", m.selected)
		}
		if m.stickToBottom {
			t.Fatal("g should unstick from bottom")
		}
		if !m.viewport.AtTop() {
			t.Fatal("g should scroll to top")
		}

		m = press(m, "j")
		if m.selected != 1 {
			t.Fatalf("j after g selected=%d want 1 (old mid was %d)", m.selected, mid)
		}
	})

	t.Run("empty store does not panic", func(t *testing.T) {
		t.Parallel()
		m := testModelWithHistory()
		m.selected = -1
		m = press(m, "G")
		if m.selected != -1 {
			t.Fatalf("G empty selected=%d want -1", m.selected)
		}
		m = press(m, "g")
		if m.selected != -1 {
			t.Fatalf("g empty selected=%d want -1", m.selected)
		}
	})

	t.Run("compose focus does not move selection", func(t *testing.T) {
		t.Parallel()
		m := testModelWithHistory()
		seedTallHistory(&m, 20)
		m.selected = 5
		m.focus = focusCompose
		m.textarea.Focus()
		m.redraw()

		m = press(m, "G")
		if m.selected != 5 {
			t.Fatalf("compose G selected=%d want 5", m.selected)
		}
		m = press(m, "g")
		if m.selected != 5 {
			t.Fatalf("compose g selected=%d want 5", m.selected)
		}
	})
}

func TestEnsureSelectedVisibleScrollsDownPastBottom(t *testing.T) {
	t.Parallel()
	m := testModelWithHistory()
	seedMixedHeightHistory(&m, 50)
	m.selected = 0
	m.redraw()
	m.viewport.GotoTop()

	for step := 0; step < m.store.len(); step++ {
		if m.selected >= m.store.len()-1 {
			t.Fatal("never crossed the bottom edge")
		}
		m.selected++
		m.redraw()
		span := m.itemSpans[m.selected]
		top := m.viewport.YOffset()
		bot := top + m.viewport.Height()
		if span.end() <= bot {
			m.ensureSelectedVisible()
			continue
		}
		m.ensureSelectedVisible()
		want := span.end() - m.viewport.Height()
		if m.viewport.YOffset() != want {
			t.Fatalf("first overflow: YOffset=%d want %d (step %d)",
				m.viewport.YOffset(), want, step)
		}
		return
	}
	t.Fatal("selection never extended past the viewport bottom")
}

func seedTallHistory(m *Model, n int) {
	msgs := make([]api.Message, n)
	for i := 0; i < n; i++ {
		msgs[i] = api.Message{
			ID:        fmt.Sprintf("t%d", i),
			Text:      "line " + fmt.Sprintf("%d", i) + " " + strings.Repeat("pad ", 8),
			Author:    api.Author{Handle: "u"},
			CreatedAt: int64(1000 + i),
		}
	}
	res := api.ListMessagesResponse{Messages: msgs, HasMore: true, NextCursor: msgs[0].ID}
	m.store.mergeAPI(res, false)
	m.redraw()
}

// seedMixedHeightHistory plants short older rows and tall recent replies so
// the old index-ratio scroller would leave the highlight before scrolling.
func seedMixedHeightHistory(m *Model, n int) {
	msgs := make([]api.Message, n)
	for i := 0; i < n; i++ {
		text := "ok"
		if i >= n-15 {
			text = "demo msg from @u — scroll up for history ↵ extra padding so tall terminals still need to scroll ↵ third line keep going"
		}
		msg := api.Message{
			ID:        fmt.Sprintf("mix%d", i),
			Text:      text,
			Author:    api.Author{Handle: "u"},
			CreatedAt: int64(1000 + i),
		}
		if i >= n-15 && i > 0 {
			msg.ReplyToID = fmt.Sprintf("mix%d", i-1)
		}
		msgs[i] = msg
	}
	res := api.ListMessagesResponse{Messages: msgs, HasMore: true, NextCursor: msgs[0].ID}
	m.store.mergeAPI(res, false)
	m.redraw()
}

func TestAttachCacheSeedsStore(t *testing.T) {
	db, err := cache.Open(filepath.Join(t.TempDir(), "chat.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	if err = db.SaveMessage(ctx, api.Message{ID: "1", Text: "a", CreatedAt: 100}); err != nil {
		t.Fatal(err)
	}
	if err = db.SaveMessage(ctx, api.Message{ID: "2", Text: "b", CreatedAt: 200}); err != nil {
		t.Fatal(err)
	}
	m := New(nil, time.Hour)
	m.AttachCache(db)
	list := m.store.list()
	if len(list) != 2 || list[0].ID != "1" || list[1].ID != "2" {
		t.Fatalf("%+v", list)
	}
	if !m.store.hasMore {
		t.Fatal("seed should leave hasMore so backscroll still tries the API")
	}
}

func TestNewWithoutCacheEmpty(t *testing.T) {
	m := New(nil, time.Hour)
	if m.store.len() != 0 {
		t.Fatalf("len %d", m.store.len())
	}
}

func TestPollAndSentPersistMessages(t *testing.T) {
	db, err := cache.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	m := New(nil, time.Hour)
	m.AttachCache(db)

	next, _ := m.Update(pollMsg{res: api.ListMessagesResponse{
		Messages: []api.Message{{ID: "9", Text: "hi", CreatedAt: 100}},
	}})
	m = next.(Model)
	next, _ = m.Update(sentMsg{msg: api.Message{ID: "10", Text: "sent", CreatedAt: 200}})
	_ = next

	got, err := db.LatestMessages(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "9" || got[1].ID != "10" {
		t.Fatalf("%+v", got)
	}
}

func TestOlderPollPersistsMessages(t *testing.T) {
	db, err := cache.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	m := New(nil, time.Hour)
	m.AttachCache(db)
	m.store.mergeAPI(api.ListMessagesResponse{
		Messages: []api.Message{{ID: "20", Text: "now", CreatedAt: 200}},
	}, false)
	m.loadingOld = true
	next, _ := m.Update(pollMsg{
		older: true,
		res: api.ListMessagesResponse{
			Messages:   []api.Message{{ID: "10", Text: "old", CreatedAt: 100}},
			HasMore:    true,
			NextCursor: "10",
		},
	})
	_ = next
	got, err := db.LatestMessages(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "10" {
		t.Fatalf("%+v", got)
	}
}

func TestOlderPollErrorServesCache(t *testing.T) {
	db, err := cache.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	for i := 1; i <= 50; i++ {
		msg := api.Message{ID: fmt.Sprintf("%02d", i), Text: "x", CreatedAt: int64(i * 10)}
		if err = db.SaveMessage(ctx, msg); err != nil {
			t.Fatal(err)
		}
	}
	m := New(nil, time.Hour)
	m.cacheDB = db
	var live []api.Message
	for i := 20; i <= 50; i++ {
		live = append(live, api.Message{ID: fmt.Sprintf("%02d", i), Text: "x", CreatedAt: int64(i * 10)})
	}
	m.store.mergeAPI(api.ListMessagesResponse{Messages: live, HasMore: true, NextCursor: "20"}, false)
	keepID := "20"
	m.selected = m.store.indexOf(keepID)
	m.loadingOld = true
	m.width = 80
	m.height = 40
	m.layout()
	m.redraw()

	next, _ := m.Update(pollMsg{older: true, err: errors.New("offline")})
	got := next.(Model)
	if got.pollFailed {
		t.Fatal("older cache fallback must not set pollFailed")
	}
	if got.store.idAt(0) != "01" {
		t.Fatalf("oldest %q", got.store.idAt(0))
	}
	if got.store.idAt(got.selected) != keepID {
		t.Fatalf("selection %q idx=%d", got.store.idAt(got.selected), got.selected)
	}
	if got.errMsg != "" {
		t.Fatalf("errMsg %q", got.errMsg)
	}
}

func TestOlderPollErrorWithoutCacheKeepsError(t *testing.T) {
	m := New(nil, time.Hour)
	m.store.mergeAPI(api.ListMessagesResponse{
		Messages: []api.Message{{ID: "20", Text: "now", CreatedAt: 200}},
		HasMore:  true,
	}, false)
	m.loadingOld = true
	next, _ := m.Update(pollMsg{older: true, err: errors.New("offline")})
	got := next.(Model)
	if got.store.len() != 1 {
		t.Fatalf("len %d", got.store.len())
	}
	if got.errMsg != "offline" {
		t.Fatalf("errMsg %q", got.errMsg)
	}
	if got.pollFailed {
		t.Fatal("older error must not set pollFailed")
	}
}

func TestPersistErrorDoesNotFailPoll(t *testing.T) {
	db, err := cache.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	m := New(nil, time.Hour)
	m.AttachCache(db)
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	next, _ := m.Update(pollMsg{res: api.ListMessagesResponse{
		Messages: []api.Message{{ID: "9", Text: "hi", CreatedAt: 1}},
	}})
	got := next.(Model)
	if got.pollFailed {
		t.Fatal("persist error must not set pollFailed")
	}
	if got.store.len() != 1 {
		t.Fatalf("store len %d", got.store.len())
	}
}
