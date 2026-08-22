package tui

import (
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"remiterm/internal/api"
)

// Settings are TUI-editable prefs. Persistence is wired separately.
type Settings struct {
	AuthorLabel     string
	TimestampFormat string
	Theme           string
	PollInterval    time.Duration
}

// PersistFunc writes settings. Nil means in-memory only.
type PersistFunc func(Settings) error

type settingsView struct {
	open     bool
	selected int
}

type settingDef struct {
	key    string
	label  string
	values []string
}

var settingDefs = []settingDef{
	{key: "author_label", label: "Author label", values: []string{"handle", "display_name", "both"}},
	{key: "timestamp_format", label: "Timestamp", values: []string{"15:04", "15:04:05", "3:04pm", "relative"}},
	{key: "theme", label: "Theme", values: []string{"default", "dim", "high-contrast", "monokai", "tomorrow-night", "dracula"}},
	{key: "poll_interval", label: "Poll interval", values: []string{"1s", "2s", "3s", "5s", "10s", "15s", "30s", "60s"}},
}

func defaultSettings(poll time.Duration) Settings {
	if poll <= 0 {
		poll = 5 * time.Second
	}
	return Settings{
		AuthorLabel:     "handle",
		TimestampFormat: "15:04",
		Theme:           "default",
		PollInterval:    poll,
	}
}

func (s Settings) valueOf(key string) string {
	switch key {
	case "author_label":
		return s.AuthorLabel
	case "timestamp_format":
		return s.TimestampFormat
	case "theme":
		return s.Theme
	case "poll_interval":
		if s.PollInterval <= 0 {
			return ""
		}
		return s.PollInterval.String()
	default:
		return ""
	}
}

func (s *Settings) setValue(key, val string) {
	switch key {
	case "author_label":
		s.AuthorLabel = val
	case "timestamp_format":
		s.TimestampFormat = val
	case "theme":
		s.Theme = val
	case "poll_interval":
		d, err := time.ParseDuration(val)
		if err == nil {
			s.PollInterval = d
		}
	}
}

func cycleValue(values []string, current string, delta int) string {
	if len(values) == 0 {
		return current
	}
	i := slices.Index(values, current)
	if i < 0 {
		if delta < 0 {
			return values[len(values)-1]
		}
		return values[0]
	}
	n := len(values)
	return values[((i+delta)%n+n)%n]
}

func (m Model) openSettings() (tea.Model, tea.Cmd) {
	m.settings.open = true
	if m.settings.selected < 0 || m.settings.selected >= len(settingDefs) {
		m.settings.selected = 0
	}
	m.showHelp = false
	m.redraw()
	return m, nil
}

func (m Model) closeSettings() (tea.Model, tea.Cmd) {
	m.settings.open = false
	m.redraw()
	return m, nil
}

func (m Model) handleSettingsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		return m.closeSettings()
	case "ctrl+c":
		return m, tea.Quit
	case "?", "ctrl+g":
		m.showHelp = true
		return m, nil
	case "j", "down":
		if m.settings.selected < len(settingDefs)-1 {
			m.settings.selected++
			m.redraw()
		}
		return m, nil
	case "k", "up":
		if m.settings.selected > 0 {
			m.settings.selected--
			m.redraw()
		}
		return m, nil
	case "enter", "l", "right":
		return m.cycleSetting(1)
	case "h", "left":
		return m.cycleSetting(-1)
	}
	return m, nil
}

func (m Model) cycleSetting(delta int) (tea.Model, tea.Cmd) {
	if m.settings.selected < 0 || m.settings.selected >= len(settingDefs) {
		return m, nil
	}
	def := settingDefs[m.settings.selected]
	next := cycleValue(def.values, m.prefs.valueOf(def.key), delta)
	m.prefs.setValue(def.key, next)
	m.applyLivePrefs()
	if m.persist != nil {
		if err := m.persist(m.prefs); err != nil {
			m.errMsg = "save settings: " + err.Error()
		} else {
			m.errMsg = ""
		}
	}
	m.redraw()
	return m, nil
}

// AttachSettings replaces in-memory prefs and the persist callback.
// A nil persist keeps changes in-memory only (tests, remiterm demo).
func (m *Model) AttachSettings(s Settings, persist PersistFunc) {
	if s.AuthorLabel == "" && s.TimestampFormat == "" && s.Theme == "" && s.PollInterval <= 0 {
		s = defaultSettings(m.poll)
	} else {
		if s.AuthorLabel == "" {
			s.AuthorLabel = "handle"
		}
		if s.TimestampFormat == "" {
			s.TimestampFormat = "15:04"
		}
		if s.Theme == "" {
			s.Theme = "default"
		}
		if s.PollInterval <= 0 {
			s.PollInterval = m.poll
		}
	}
	m.prefs = s
	m.applyLivePrefs()
	m.persist = persist
}

// applyLivePrefs pushes in-memory prefs into runtime (poll, theme, widgets).
func (m *Model) applyLivePrefs() {
	if m.prefs.PollInterval > 0 {
		m.poll = m.prefs.PollInterval
	}
	applyTheme(m.prefs.Theme)
	m.refreshThemeWidgets()
}

func (m *Model) refreshThemeWidgets() {
	styles := m.textarea.Styles()
	styles.Focused.Prompt = gutterStyle(true)
	styles.Blurred.Prompt = gutterStyle(false)
	m.textarea.SetStyles(styles)
	m.oldSpinner.Style = lipgloss.NewStyle().Foreground(colorDim)
}

func (m *Model) renderSettings() string {
	title := styleStatus.Render("settings · esc back")
	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n\n")
	prefix := lipgloss.Height(title) + 1

	blocks := make([]string, len(settingDefs))
	for i, def := range settingDefs {
		blocks[i] = m.renderSettingRow(def, i == m.settings.selected)
		b.WriteString(blocks[i])
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.renderThemePreview())
	m.itemSpans = lineSpans(blocks, prefix, 0)
	return b.String()
}

func (m Model) renderThemePreview() string {
	now := time.Now()
	parent := api.Message{
		Author: api.Author{Handle: "milady", DisplayName: "milady maker"},
		Text:   "gm",
	}
	replyBody := styleReply.Render("↳"+replySnippetLabel(parent, m.prefs.AuthorLabel)+" · ") + "yea"

	var b strings.Builder
	b.WriteString(styleStatus.Render("theme preview"))
	b.WriteString("\n")
	lines := []string{
		m.previewChatLine(now.Add(-4*time.Minute), now, api.Author{Handle: "milady", DisplayName: "milady maker"}, "gm", false, false),
		m.previewChatLine(now.Add(-3*time.Minute), now, api.Author{Handle: "you"}, "gn", true, false),
		m.previewChatLine(now.Add(-2*time.Minute), now, api.Author{Handle: "alice", DisplayName: "Alice"}, replyBody, false, false),
		m.previewChatLine(now.Add(-time.Minute), now, api.Author{Handle: "bob", DisplayName: "Bob"}, mediaLabel("", "image"), false, false),
		m.previewChatLine(now, now, api.Author{Handle: "cara", DisplayName: "Cara"}, "this one is selected", false, true),
	}
	b.WriteString(strings.Join(lines, "\n"))
	return b.String()
}

func (m Model) previewChatLine(t, now time.Time, author api.Author, body string, own, selected bool) string {
	hStyle := styleHandle
	if own {
		hStyle = styleHandleOwn
	}
	var b strings.Builder
	b.WriteString(styleTime.Render(formatTimestamp(t, now, m.prefs.TimestampFormat)))
	b.WriteString(" ")
	b.WriteString(hStyle.Render(authorLabel(author, m.prefs.AuthorLabel)))
	b.WriteString(" ")
	b.WriteString(body)
	line := b.String()
	cw := m.contentWidth()
	if selected {
		return styleSelected.Width(max(0, cw)).Render(line)
	}
	if cw > 4 {
		return lipgloss.NewStyle().Width(cw).Render(line)
	}
	return line
}

func (m Model) renderSettingRow(def settingDef, selected bool) string {
	val := m.prefs.valueOf(def.key)
	if val == "" {
		val = "—"
	}
	line := def.label + "  " + styleTime.Render(val)
	cw := m.contentWidth()
	if selected {
		return styleSelected.Width(max(0, cw)).Render(line)
	}
	if cw > 4 {
		return lipgloss.NewStyle().Width(cw).Render(line)
	}
	return line
}
