package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

const gutterGlyph = "│"

func gutterStyle(active bool) lipgloss.Style {
	c := colorDim
	if active {
		c = colorAccent
	}
	return lipgloss.NewStyle().Foreground(c)
}

func (m Model) messagesRegionActive() bool {
	return m.detail.open || m.focus == focusMessages
}

func (m Model) composeRegionActive() bool {
	return !m.detail.open && m.focus == focusCompose
}

// withLeftGutter prefixes every line of content with a 1-cell │.
// The glyph is always present so Tab does not reflow wrap width;
// only the color changes with active.
func withLeftGutter(content string, active bool) string {
	bar := gutterStyle(active).Render(gutterGlyph)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = bar + line
	}
	return strings.Join(lines, "\n")
}
