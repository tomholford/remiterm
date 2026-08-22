package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"remiterm/internal/api"
)

func settingsTestModel() Model {
	m := New(nil, 5*time.Second)
	m.focus = focusMessages
	m.width = 80
	m.height = 40
	m.textarea.Blur()
	m.layout()
	return m
}

func press(m Model, key string) Model {
	got, _ := m.handleKey(keyMsg(key))
	return got.(Model)
}

func keyMsg(s string) tea.KeyPressMsg {
	switch s {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEsc}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	case "ctrl+g":
		return tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl}
	case "ctrl+s":
		return tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	default:
		r := []rune(s)
		return tea.KeyPressMsg{Text: s, Code: r[0]}
	}
}

func TestOpenCloseSettings(t *testing.T) {
	t.Parallel()
	m := settingsTestModel()

	m = press(m, "s")
	if !m.settings.open {
		t.Fatal("s from messages should open settings")
	}
	if !strings.Contains(m.content, "Author label") {
		t.Fatalf("content missing settings list: %q", m.content)
	}

	m = press(m, "esc")
	if m.settings.open {
		t.Fatal("esc should close settings")
	}

	m = press(m, "s")
	m = press(m, "q")
	if m.settings.open {
		t.Fatal("q should close settings")
	}

	m = New(nil, time.Hour)
	m.width, m.height = 80, 40
	m.layout()
	m = press(m, "s")
	if m.settings.open {
		t.Fatal("s in compose should type, not open settings")
	}
	if !strings.Contains(m.textarea.Value(), "s") {
		t.Fatalf("compose should type s, got %q", m.textarea.Value())
	}

	m = New(nil, time.Hour)
	m.width, m.height = 80, 40
	m.layout()
	m = press(m, "a")
	m = press(m, "ctrl+s")
	if !m.settings.open {
		t.Fatal("ctrl+s in compose should open settings")
	}
	if m.textarea.Value() != "a" {
		t.Fatalf("draft changed: %q", m.textarea.Value())
	}
}

func TestSettingsCycle(t *testing.T) {
	t.Parallel()
	m := settingsTestModel()
	m = press(m, "s")
	if m.settings.selected != 0 {
		t.Fatalf("selected = %d", m.settings.selected)
	}
	if m.prefs.AuthorLabel != "handle" {
		t.Fatalf("start = %q", m.prefs.AuthorLabel)
	}

	m = press(m, "enter")
	if m.prefs.AuthorLabel != "display_name" {
		t.Fatalf("enter = %q", m.prefs.AuthorLabel)
	}
	if m.settings.selected != 0 {
		t.Fatalf("selection moved: %d", m.settings.selected)
	}

	m = press(m, "l")
	if m.prefs.AuthorLabel != "both" {
		t.Fatalf("l = %q", m.prefs.AuthorLabel)
	}
	m = press(m, "l")
	if m.prefs.AuthorLabel != "handle" {
		t.Fatalf("wrap next = %q", m.prefs.AuthorLabel)
	}

	m = press(m, "h")
	if m.prefs.AuthorLabel != "both" {
		t.Fatalf("h wrap = %q", m.prefs.AuthorLabel)
	}
}

func TestSettingsSwallowsKeys(t *testing.T) {
	t.Parallel()
	m := settingsTestModel()
	m.store.upsert(api.Message{
		ID:     "1",
		Text:   "hi",
		Author: api.Author{Handle: "alice"},
	})
	m.selected = 0
	m = press(m, "s")

	m = press(m, "p")
	if m.profile.open {
		t.Fatal("p should not open profile from settings")
	}
	m = press(m, "r")
	if m.replyToID != "" {
		t.Fatal("r should not start a reply from settings")
	}
	m = press(m, "o")
	if m.errMsg != "" {
		t.Fatalf("o should be swallowed, err=%q", m.errMsg)
	}
	if !m.settings.open {
		t.Fatal("settings should stay open")
	}
}

func TestSettingsPersistCalled(t *testing.T) {
	t.Parallel()
	m := settingsTestModel()
	var got []Settings
	m.AttachSettings(m.prefs, func(s Settings) error {
		got = append(got, s)
		return nil
	})
	m = press(m, "s")
	m = press(m, "enter")
	if len(got) != 1 {
		t.Fatalf("persist calls = %d", len(got))
	}
	if got[0].AuthorLabel != "display_name" {
		t.Fatalf("%+v", got[0])
	}

	m2 := settingsTestModel()
	m2 = press(m2, "s")
	m2 = press(m2, "enter")
	if m2.prefs.AuthorLabel != "display_name" {
		t.Fatal("nil persist should still update memory")
	}
}

func TestSettingsPersistError(t *testing.T) {
	t.Parallel()
	m := settingsTestModel()
	m.AttachSettings(m.prefs, func(Settings) error {
		return errors.New("disk full")
	})
	m = press(m, "s")
	m = press(m, "enter")
	if m.prefs.AuthorLabel != "display_name" {
		t.Fatalf("keep in-memory, got %q", m.prefs.AuthorLabel)
	}
	if !strings.Contains(m.errMsg, "disk full") {
		t.Fatalf("errMsg = %q", m.errMsg)
	}
}

func TestSettingsThemePreview(t *testing.T) {
	t.Parallel()
	m := settingsTestModel()
	m = press(m, "s")
	plain := stripANSI(m.content)
	if !strings.Contains(plain, "theme preview") {
		t.Fatalf("missing preview heading: %q", plain)
	}
	for _, want := range []string{"milady", "gm", "you", "gn", "alice", "bob", "cara", "this one is selected"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("missing %q in %q", want, plain)
		}
	}
	if !strings.Contains(plain, "[image") {
		t.Fatalf("missing media badge: %q", plain)
	}
	if !strings.Contains(plain, "↳") {
		t.Fatalf("missing reply prefix: %q", plain)
	}
	if len(m.itemSpans) != len(settingDefs) {
		t.Fatalf("preview must not be selectable, spans=%d want %d", len(m.itemSpans), len(settingDefs))
	}
}

func TestSettingsThemePreviewFollowsAuthorLabel(t *testing.T) {
	t.Parallel()
	m := settingsTestModel()
	m.prefs.AuthorLabel = "both"
	m = press(m, "s")
	plain := stripANSI(m.content)
	if !strings.Contains(plain, "milady maker (@milady)") {
		t.Fatalf("preview should honor author_label: %q", plain)
	}
}

func TestCycleSettingAppliesTheme(t *testing.T) {
	t.Cleanup(func() { applyTheme("default") })
	m := settingsTestModel()
	m.AttachSettings(m.prefs, nil)
	m = press(m, "s")
	m = press(m, "j")
	m = press(m, "j")
	if settingDefs[m.settings.selected].key != "theme" {
		t.Fatalf("selected %q", settingDefs[m.settings.selected].key)
	}
	if m.prefs.Theme != "default" {
		t.Fatalf("start theme = %q", m.prefs.Theme)
	}
	m = press(m, "enter")
	if m.prefs.Theme != "dim" {
		t.Fatalf("cycled theme = %q", m.prefs.Theme)
	}
	if appliedTheme != "dim" {
		t.Fatalf("appliedTheme = %q", appliedTheme)
	}
	dim, _ := paletteFor("dim")
	if colorAccent != dim.accent {
		t.Fatalf("live accent %v want dim %v", colorAccent, dim.accent)
	}
	if m.textarea.Styles().Focused.Prompt.GetForeground() != colorAccent {
		t.Fatal("compose gutter should follow the new theme")
	}
}

func TestAttachSettingsAppliesTheme(t *testing.T) {
	t.Cleanup(func() { applyTheme("default") })
	m := settingsTestModel()
	s := m.prefs
	s.Theme = "high-contrast"
	m.AttachSettings(s, nil)
	if appliedTheme != "high-contrast" {
		t.Fatalf("appliedTheme = %q", appliedTheme)
	}
	hc, _ := paletteFor("high-contrast")
	if colorAccent != hc.accent {
		t.Fatalf("accent %v want %v", colorAccent, hc.accent)
	}
}

func TestSettingsApplyPoll(t *testing.T) {
	t.Parallel()
	m := settingsTestModel()
	m.AttachSettings(m.prefs, nil)
	m = press(m, "s")
	for i := 0; i < 3; i++ {
		m = press(m, "j")
	}
	if settingDefs[m.settings.selected].key != "poll_interval" {
		t.Fatalf("selected %q", settingDefs[m.settings.selected].key)
	}
	m = press(m, "enter")
	if m.prefs.PollInterval != 10*time.Second {
		t.Fatalf("prefs poll = %s", m.prefs.PollInterval)
	}
	if m.poll != 10*time.Second {
		t.Fatalf("m.poll = %s", m.poll)
	}
}

func TestCycleValueUnknownSnaps(t *testing.T) {
	t.Parallel()
	vals := []string{"a", "b", "c"}
	if got := cycleValue(vals, "nope", 1); got != "a" {
		t.Fatalf("next unknown = %q", got)
	}
	if got := cycleValue(vals, "nope", -1); got != "c" {
		t.Fatalf("prev unknown = %q", got)
	}
}
