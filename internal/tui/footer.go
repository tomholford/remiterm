package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// statusSlotWidth is the always-on right cluster for transients
// ("sent", "sending…", "opened in browser") so they cannot change
// hint wrap width. Sized to the longest of those strings.
const statusSlotWidth = 17

// footerKeyHints returns key hints that work in the current mode.
// Empty when a modal is open — the card already shows its own hints.
func footerKeyHints(focus focusArea, profileOpen, detailOpen, settingsOpen, previewOpen bool) string {
	if previewOpen || profileOpen {
		return ""
	}
	if settingsOpen {
		return "j/k select · Enter cycle · Esc close"
	}
	if detailOpen {
		return "j/k · Enter · r reply · o/i · p profile · Esc · ?"
	}
	switch focus {
	case focusMessages:
		return "j/k · Enter · r reply · p profile · m me · o/i · Tab compose · ? · q"
	default:
		return "Enter send · Ctrl+J newline · Tab messages · Ctrl+G help · Ctrl+S settings · Ctrl+C quit"
	}
}

// footerText is the left cluster: loading tip + key hints. Errors and
// transients are laid out separately so a flash cannot reflow this string.
func (m Model) footerText() string {
	var parts []string
	if m.loadingOld && m.viewport.AtTop() {
		parts = append(parts, strings.TrimSpace(m.oldSpinner.View())+" loading older")
	}
	hints := footerKeyHints(m.focus, m.profile.open, m.detail.open, m.settings.open, m.preview.open)
	if hints != "" {
		parts = append(parts, hints)
	}
	return strings.Join(parts, " · ")
}

func (m Model) renderFooter() string {
	if m.errMsg != "" {
		st := styleErr
		if m.width > 0 {
			st = st.Width(m.width)
		}
		return st.Render(m.errMsg)
	}

	left := m.footerText()
	if m.width <= 0 {
		if m.status == "" {
			return styleFooter.Render(left)
		}
		if left == "" {
			return styleFooter.Render(m.status)
		}
		return styleFooter.Render(left + "  ·  " + m.status)
	}

	hPad := styleFooter.GetHorizontalPadding()
	inner := m.width - hPad
	if inner < 1 {
		inner = 1
	}
	slotW := statusSlotWidth
	leftW := inner - slotW
	if leftW < 1 {
		leftW = 1
		slotW = inner - leftW
		if slotW < 0 {
			slotW = 0
		}
	}

	leftView := lipgloss.NewStyle().Width(leftW).Render(left)
	rightView := lipgloss.NewStyle().
		Width(slotW).
		AlignHorizontal(lipgloss.Right).
		Render(clipCellWidth(m.status, slotW))
	joined := lipgloss.JoinHorizontal(lipgloss.Top, leftView, rightView)
	return styleFooter.Width(m.width).Render(joined)
}

func (m Model) footerRows() int {
	h := lipgloss.Height(m.renderFooter())
	if h < 1 {
		return footerHeight
	}
	return h
}

// clipCellWidth truncates s to at most w display cells so the status
// cluster cannot wrap and steal a footer row.
func clipCellWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(w).Render(s)
}
