package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// pollStaleAfter is the minimum grace after a successful poll before the
// header may show "offline". When the poll interval is longer, we wait
// at least one full interval (see onlineLabel) so a quiet demo poll does
// not look offline.
const pollStaleAfter = 5 * time.Second

// onlineLabel is the poll health indicator for the header.
// lastOK is the time of the last successful poll; pollFailed is true when the
// most recent attempt failed. pollInterval is the live-poll period. now is
// injected for tests.
func onlineLabel(lastOK time.Time, pollFailed bool, now time.Time, pollInterval time.Duration) string {
	if lastOK.IsZero() {
		if pollFailed {
			return "offline"
		}
		return "connecting…"
	}
	if pollFailed {
		return "offline"
	}
	staleAfter := pollStaleAfter
	if pollInterval > staleAfter {
		staleAfter = pollInterval
	}
	if now.Sub(lastOK) > staleAfter {
		return "offline"
	}
	return "online"
}

func styleOnlineLabel(label string) string {
	switch label {
	case "online":
		return styleConnOnline.Render(label)
	case "offline":
		return styleConnOffline.Render(label)
	default:
		return styleConnConnecting.Render(label)
	}
}

// renderHeader draws the title cluster on the left and the poll-health
// indicator on the right of a single line.
func (m Model) renderHeader() string {
	left := styleHeader.Render("remiterm · global")
	if m.meHandle != "" {
		left += styleStatus.Render(" @" + m.meHandle)
	}
	if m.pendingNew > 0 {
		left += styleBanner.Render(fmt.Sprintf("  ↓ %d new", m.pendingNew))
	}

	label := onlineLabel(m.lastPoll, m.pollFailed, time.Now(), m.poll)
	right := styleOnlineLabel(label)

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}
