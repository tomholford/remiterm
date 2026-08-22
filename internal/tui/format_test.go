package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"remiterm/internal/api"
)

func TestAuthorHandle(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		a    api.Author
		want string
	}{
		{"handle", api.Author{Handle: "milady", DisplayName: "Milady Maker"}, "milady"},
		{"display fallback", api.Author{DisplayName: "Only Display"}, "Only Display"},
		{"empty", api.Author{}, "?"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := authorHandle(tc.a); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestTruncateSnippet(t *testing.T) {
	t.Parallel()

	t.Run("short unchanged", func(t *testing.T) {
		t.Parallel()
		if got := truncateSnippet("hello", 40); got != "hello" {
			t.Fatalf("%q", got)
		}
	})

	t.Run("collapses newlines and spaces", func(t *testing.T) {
		t.Parallel()
		got := truncateSnippet("what is\nit?  :D", 40)
		if got != "what is it? :D" {
			t.Fatalf("%q", got)
		}
	})

	t.Run("truncates at max runes", func(t *testing.T) {
		t.Parallel()
		// 45 ascii chars → 40 + ellipsis
		in := strings.Repeat("a", 45)
		got := truncateSnippet(in, 40)
		if got != strings.Repeat("a", 40)+"…" {
			t.Fatalf("%q (len runes wrong)", got)
		}
	})

	t.Run("utf8 safe", func(t *testing.T) {
		t.Parallel()
		got := truncateSnippet(strings.Repeat("世", 10), 5)
		if got != strings.Repeat("世", 5)+"…" {
			t.Fatalf("got %q", got)
		}
	})
}

func TestAuthorLabel(t *testing.T) {
	t.Parallel()
	both := api.Author{Handle: "milady", DisplayName: "milady maker"}
	cases := []struct {
		name string
		a    api.Author
		mode string
		want string
	}{
		{"handle default", both, "handle", "milady"},
		{"handle empty mode", both, "", "milady"},
		{"display_name", both, "display_name", "milady maker"},
		{"both", both, "both", "milady maker (@milady)"},
		{"display_name empty display", api.Author{Handle: "milady"}, "display_name", "milady"},
		{"both empty display", api.Author{Handle: "milady"}, "both", "milady"},
		{"both names match", api.Author{Handle: "milady", DisplayName: "milady"}, "both", "milady"},
		{"handle empty handle", api.Author{DisplayName: "Only Display"}, "handle", "Only Display"},
		{"empty", api.Author{}, "handle", "?"},
		{"empty both", api.Author{}, "both", "?"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := authorLabel(tc.a, tc.mode); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestReplySnippetLabel(t *testing.T) {
	t.Parallel()
	msg := api.Message{
		Author: api.Author{Handle: "Key", DisplayName: "Key Holder"},
		Text:   "what is it? :D",
	}
	if got := replySnippetLabel(msg, "handle"); got != "@Key: what is it? :D" {
		t.Fatalf("handle: %q", got)
	}
	if got := replySnippetLabel(msg, "display_name"); got != "@Key Holder: what is it? :D" {
		t.Fatalf("display_name: %q", got)
	}
	if got := replySnippetLabel(msg, "both"); got != "Key Holder (@Key): what is it? :D" {
		t.Fatalf("both: %q", got)
	}
}

func stripANSI(s string) string {
	// Minimal CSI strip for lipgloss output in tests.
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			// ESC [ ... final byte
			i++
			if i < len(s) && s[i] == '[' {
				i++
				for i < len(s) {
					c := s[i]
					i++
					if c >= 0x40 && c <= 0x7e {
						break
					}
				}
				continue
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func testFormatModel(width int, msgs ...api.Message) Model {
	m := New(nil, 0)
	m.width = width
	m.height = 40
	for _, msg := range msgs {
		m.store.upsert(msg)
	}
	return m
}

func TestFormatMessageReplySnippet(t *testing.T) {
	t.Parallel()

	parent := api.Message{
		ID:        "p1",
		Author:    api.Author{Handle: "Key"},
		Text:      "what is it? :D",
		CreatedAt: 1_700_000_000_000,
	}
	child := api.Message{
		ID:        "c1",
		Author:    api.Author{Handle: "kpoji"},
		Text:      "Eth chan daily",
		ReplyToID: "p1",
		CreatedAt: 1_700_000_100_000,
	}

	t.Run("two-line when wide and parent present", func(t *testing.T) {
		t.Parallel()
		m := testFormatModel(80, parent, child)
		out := stripANSI(m.formatMessage(child, false))
		lines := strings.Split(out, "\n")
		if len(lines) < 2 {
			t.Fatalf("expected two lines, got %q", out)
		}
		if !strings.Contains(lines[0], "↳") || !strings.Contains(lines[0], "@Key: what is it? :D") {
			t.Fatalf("parent line: %q", lines[0])
		}
		if !strings.Contains(lines[1], "kpoji") || !strings.Contains(lines[1], "Eth chan daily") {
			t.Fatalf("child line: %q", lines[1])
		}
		// Child line should not re-prefix ↳@
		if strings.Contains(lines[1], "↳") {
			t.Fatalf("child should not have ↳: %q", lines[1])
		}
	})

	t.Run("compact when narrow", func(t *testing.T) {
		t.Parallel()
		m := testFormatModel(50, parent, child)
		out := stripANSI(m.formatMessage(child, false))
		// Width wrap may introduce newlines and pad spaces; collapse for content checks.
		flat := strings.Join(strings.Fields(out), " ")
		if !strings.Contains(flat, "↳@Key: what is it? :D") {
			t.Fatalf("missing snippet: %q", out)
		}
		if !strings.Contains(flat, "Eth chan daily") {
			t.Fatalf("missing body: %q", out)
		}
		if !strings.Contains(flat, "kpoji") {
			t.Fatalf("missing handle: %q", out)
		}
		// Compact path is single logical line (no deliberate parent/child split).
		// Soft-wrap newlines from lipgloss are OK; a parent-only first line is not.
		if strings.Contains(out, "\n") && strings.Contains(strings.Split(out, "\n")[0], "↳@Key") &&
			!strings.Contains(strings.Split(out, "\n")[0], "kpoji") {
			t.Fatalf("expected compact single-line layout, got multi-line block: %q", out)
		}
	})

	t.Run("parent missing", func(t *testing.T) {
		t.Parallel()
		orphan := api.Message{
			ID:        "o1",
			Author:    api.Author{Handle: "bob"},
			Text:      "hello",
			ReplyToID: "missing",
			CreatedAt: 1,
		}
		m := testFormatModel(80, orphan)
		out := stripANSI(m.formatMessage(orphan, false))
		if !strings.Contains(out, "↳") {
			t.Fatalf("expected ↳: %q", out)
		}
		if strings.Contains(out, "↳@") || strings.Contains(out, "↳ @") {
			t.Fatalf("missing parent must not invent snippet label: %q", out)
		}
		if !strings.Contains(out, "hello") {
			t.Fatalf("expected body: %q", out)
		}
	})

	t.Run("long parent truncated", func(t *testing.T) {
		t.Parallel()
		long := api.Message{
			ID:     "lp",
			Author: api.Author{Handle: "alice"},
			Text:   strings.Repeat("x", 50),
		}
		reply := api.Message{
			ID:        "lr",
			Author:    api.Author{Handle: "bob"},
			Text:      "ok",
			ReplyToID: "lp",
			CreatedAt: 1,
		}
		m := testFormatModel(80, long, reply)
		out := stripANSI(m.formatMessage(reply, false))
		want := "@alice: " + strings.Repeat("x", 40) + "…"
		if !strings.Contains(out, want) {
			t.Fatalf("expected truncated snippet in %q", out)
		}
		if strings.Contains(out, strings.Repeat("x", 41)) {
			t.Fatalf("should not include untruncated run: %q", out)
		}
	})

	t.Run("parent newlines collapsed", func(t *testing.T) {
		t.Parallel()
		p := api.Message{
			ID:     "nl",
			Author: api.Author{Handle: "carol"},
			Text:   "line one\nline two",
		}
		c := api.Message{
			ID: "nlr", Author: api.Author{Handle: "bob"}, Text: "yep",
			ReplyToID: "nl", CreatedAt: 1,
		}
		m := testFormatModel(80, p, c)
		out := stripANSI(m.formatMessage(c, false))
		if !strings.Contains(out, "@carol: line one line two") {
			t.Fatalf("%q", out)
		}
		if strings.Contains(strings.Split(out, "\n")[0], "↵") {
			t.Fatalf("parent snippet should not use ↵: %q", out)
		}
	})

	t.Run("empty parent handle uses display name", func(t *testing.T) {
		t.Parallel()
		p := api.Message{
			ID:     "dh",
			Author: api.Author{DisplayName: "Only Name"},
			Text:   "hi",
		}
		c := api.Message{
			ID: "dhr", Author: api.Author{Handle: "bob"}, Text: "yo",
			ReplyToID: "dh", CreatedAt: 1,
		}
		m := testFormatModel(80, p, c)
		out := stripANSI(m.formatMessage(c, false))
		if !strings.Contains(out, "@Only Name: hi") {
			t.Fatalf("%q", out)
		}
	})

	t.Run("non-reply unchanged shape", func(t *testing.T) {
		t.Parallel()
		plain := api.Message{
			ID: "n1", Author: api.Author{Handle: "alice"}, Text: "gm",
			CreatedAt: 1,
		}
		m := testFormatModel(80, plain)
		out := stripANSI(m.formatMessage(plain, false))
		if strings.Contains(out, "↳") {
			t.Fatalf("plain should not have ↳: %q", out)
		}
		if !strings.Contains(out, "alice") || !strings.Contains(out, "gm") {
			t.Fatalf("%q", out)
		}
		if strings.Count(out, "\n") != 0 {
			// width wrap might still wrap very long — "gm" is short
			if strings.Contains(out, "\n") {
				// single short line expected
				t.Fatalf("expected single line: %q", out)
			}
		}
	})
}

func TestFormatMessageSelectedContrast(t *testing.T) {
	t.Parallel()
	msg := api.Message{
		ID: "s1", Author: api.Author{Handle: "alice"}, Text: "gm",
		CreatedAt: 1,
	}
	m := testFormatModel(80, msg)
	idle := m.formatMessage(msg, false)
	sel := m.formatMessage(msg, true)
	if idle == sel {
		t.Fatal("selected row must render differently from idle")
	}
	if stripANSI(sel) == sel {
		t.Fatal("selected row should carry a background wash")
	}
	if !strings.Contains(stripANSI(sel), "alice") || !strings.Contains(stripANSI(sel), "gm") {
		t.Fatalf("selected lost content: %q", stripANSI(sel))
	}
	bg := styleSelected.GetBackground()
	if bg == nil {
		t.Fatal("selected needs a background wash")
	}
	if bg == lipgloss.Color("236") {
		t.Fatal("selected background 236 is too faint")
	}
	if bg != colorSelected {
		t.Fatalf("selected background %v want %v", bg, colorSelected)
	}
}

func TestFormatMessageAuthorLabel(t *testing.T) {
	t.Parallel()
	msg := api.Message{
		ID:        "a1",
		Author:    api.Author{Handle: "milady", DisplayName: "milady maker"},
		Text:      "gm",
		CreatedAt: 1,
	}
	cases := []struct {
		mode string
		want string
		not  string
	}{
		{mode: "handle", want: "milady", not: "milady maker"},
		{mode: "display_name", want: "milady maker", not: "(@milady)"},
		{mode: "both", want: "milady maker (@milady)"},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			t.Parallel()
			m := testFormatModel(80, msg)
			m.prefs.AuthorLabel = tc.mode
			out := stripANSI(m.formatMessage(msg, false))
			if !strings.Contains(out, tc.want) {
				t.Fatalf("missing %q in %q", tc.want, out)
			}
			if tc.not != "" && strings.Contains(out, tc.not) {
				t.Fatalf("unexpected %q in %q", tc.not, out)
			}
			if !strings.Contains(out, "gm") {
				t.Fatalf("missing body: %q", out)
			}
		})
	}
}

func TestFormatMessageOwnStyleUsesHandle(t *testing.T) {
	t.Parallel()
	msg := api.Message{
		ID:        "own",
		Author:    api.Author{Handle: "milady", DisplayName: "milady maker"},
		Text:      "gm",
		CreatedAt: 1,
	}
	m := testFormatModel(80, msg)
	m.meHandle = "milady"
	m.prefs.AuthorLabel = "display_name"
	got := m.formatMessage(msg, false)
	if !strings.Contains(got, styleHandleOwn.Render("milady maker")) {
		t.Fatalf("own row should style display label as own: %q", stripANSI(got))
	}
	if strings.Contains(got, styleHandle.Render("milady maker")) {
		t.Fatalf("own row used stranger style: %q", stripANSI(got))
	}
}

func TestFormatMessageReplySnippetAuthorLabel(t *testing.T) {
	t.Parallel()
	parent := api.Message{
		ID:        "p1",
		Author:    api.Author{Handle: "Key", DisplayName: "Key Holder"},
		Text:      "what is it? :D",
		CreatedAt: 1,
	}
	child := api.Message{
		ID:        "c1",
		Author:    api.Author{Handle: "kpoji"},
		Text:      "Eth chan daily",
		ReplyToID: "p1",
		CreatedAt: 2,
	}
	m := testFormatModel(80, parent, child)
	m.prefs.AuthorLabel = "both"
	out := stripANSI(m.formatMessage(child, false))
	if !strings.Contains(out, "Key Holder (@Key): what is it? :D") {
		t.Fatalf("parent snippet: %q", out)
	}
	if strings.Contains(out, "@Key Holder (@Key)") {
		t.Fatalf("both should not double-@: %q", out)
	}
}

func TestFormatTimestamp(t *testing.T) {
	t.Parallel()
	loc := time.FixedZone("test", -5*3600)
	now := time.Date(2026, 8, 19, 15, 4, 5, 0, loc)
	today := time.Date(2026, 8, 19, 9, 1, 2, 0, loc)
	yesterday := time.Date(2026, 8, 18, 22, 0, 0, 0, loc)
	yesterdayMorning := time.Date(2026, 8, 18, 9, 0, 0, 0, loc)
	lastMonth := time.Date(2026, 7, 4, 12, 30, 0, 0, loc)
	lastYear := time.Date(2025, 12, 31, 23, 59, 1, 0, loc)

	cases := []struct {
		name   string
		t      time.Time
		layout string
		want   string
	}{
		{"15:04 today", today, "15:04", "09:01"},
		{"15:04:05 today", today, "15:04:05", "09:01:02"},
		{"3:04pm today", today, "3:04pm", "9:01am"},
		{"empty layout today", today, "", "09:01"},
		{"15:04 yesterday", yesterday, "15:04", "Aug 18 22:00"},
		{"15:04 last month", lastMonth, "15:04", "Jul 4 12:30"},
		{"15:04 last year", lastYear, "15:04", "Dec 31 2025 23:59"},
		{"relative now", now, "relative", "now"},
		{"relative 0ns", now.Add(-time.Millisecond), "relative", "now"},
		{"relative seconds", now.Add(-12 * time.Second), "relative", "12s"},
		{"relative minutes", now.Add(-3 * time.Minute), "relative", "3m"},
		{"relative hours", now.Add(-5 * time.Hour), "relative", "5h"},
		{"relative 23h", now.Add(-23 * time.Hour), "relative", "23h"},
		{"relative yesterday", yesterdayMorning, "relative", "yesterday"},
		{"relative last month", lastMonth, "relative", "Jul 4"},
		{"relative last year", lastYear, "relative", "Dec 31 2025"},
		{"zero", time.Time{}, "15:04", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := formatTimestamp(tc.t, now, tc.layout); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestFormatMessageTimestampFormat(t *testing.T) {
	t.Parallel()
	when := time.Date(2020, 1, 2, 15, 4, 5, 0, time.Local)
	msg := api.Message{
		ID:        "t1",
		Author:    api.Author{Handle: "alice"},
		Text:      "gm",
		CreatedAt: when.UnixMilli(),
	}
	m := testFormatModel(80, msg)
	m.prefs.TimestampFormat = "3:04pm"
	out := stripANSI(m.formatMessage(msg, false))
	want := formatTimestamp(messageTime(msg.CreatedAt), time.Now(), "3:04pm")
	if want == "" || !strings.Contains(out, want) {
		t.Fatalf("missing stamp %q in %q", want, out)
	}
	if !strings.Contains(out, "alice") || !strings.Contains(out, "gm") {
		t.Fatalf("lost content: %q", out)
	}
}

func TestReplyBannerFollowsAuthorLabel(t *testing.T) {
	t.Parallel()
	msg := api.Message{
		ID:     "A",
		Author: api.Author{Handle: "alice", DisplayName: "Alice A"},
		Text:   "hello world",
	}
	m := testFormatModel(80, msg)
	got, _ := m.startReplyTo(msg)
	m = got.(Model)
	if got := m.replyBannerLabel(); got != "@alice: hello world" {
		t.Fatalf("default banner = %q", got)
	}
	m.prefs.AuthorLabel = "display_name"
	if got := m.replyBannerLabel(); got != "@Alice A: hello world" {
		t.Fatalf("display banner = %q", got)
	}
	m.prefs.AuthorLabel = "both"
	if got := m.replyBannerLabel(); got != "Alice A (@alice): hello world" {
		t.Fatalf("both banner = %q", got)
	}
}
