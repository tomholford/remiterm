package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"remiterm/internal/api"
)

const (
	// replySnippetMax is the max rune length for parent text in reply
	// previews (compose banner and chat lines).
	replySnippetMax = 40
	// replyCompactWidth: below this, parent + child share one line.
	replyCompactWidth = 60
)

// authorHandle prefers handle, then display name, then "?".
// Identity only — own-message color and profile lookup use this, not the
// display label from authorLabel.
func authorHandle(a api.Author) string {
	if strings.TrimSpace(a.Handle) != "" {
		return strings.TrimSpace(a.Handle)
	}
	if strings.TrimSpace(a.DisplayName) != "" {
		return strings.TrimSpace(a.DisplayName)
	}
	return "?"
}

// authorLabel is the visible name for chat rows, reply prefixes, and banners.
// Empty display name falls back to handle; empty everything is "?".
func authorLabel(a api.Author, mode string) string {
	handle := strings.TrimSpace(a.Handle)
	display := strings.TrimSpace(a.DisplayName)
	switch mode {
	case "display_name":
		if display != "" {
			return display
		}
		if handle != "" {
			return handle
		}
		return "?"
	case "both":
		switch {
		case handle != "" && display != "" && display != handle:
			return display + " (@" + handle + ")"
		case handle != "":
			return handle
		case display != "":
			return display
		default:
			return "?"
		}
	default: // handle
		if handle != "" {
			return handle
		}
		if display != "" {
			return display
		}
		return "?"
	}
}

// truncateSnippet collapses newlines to spaces and truncates to max runes.
func truncateSnippet(text string, max int) string {
	s := strings.ReplaceAll(text, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if max <= 0 {
		return s
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "…"
}

// formatTimestamp renders t in now's location.
// Absolute layouts prefix "Jan 2" (or "Jan 2 2006") when t is not today.
func formatTimestamp(t, now time.Time, layout string) string {
	if t.IsZero() {
		return ""
	}
	loc := now.Location()
	if loc == nil {
		loc = time.Local
	}
	t = t.In(loc)
	now = now.In(loc)
	if layout == "relative" {
		return relativeTime(t, now)
	}
	clock := t.Format(absoluteLayout(layout))
	if sameCalendarDay(t, now) {
		return clock
	}
	if t.Year() == now.Year() {
		return t.Format("Jan 2") + " " + clock
	}
	return t.Format("Jan 2 2006") + " " + clock
}

func absoluteLayout(layout string) string {
	switch layout {
	case "15:04:05":
		return "15:04:05"
	case "3:04pm":
		return "3:04pm"
	default:
		return "15:04"
	}
}

func relativeTime(t, now time.Time) string {
	if !t.Before(now) {
		return "now"
	}
	d := now.Sub(t)
	switch {
	case d < time.Second:
		return "now"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	if sameCalendarDay(t, now.AddDate(0, 0, -1)) {
		return "yesterday"
	}
	if t.Year() == now.Year() {
		return t.Format("Jan 2")
	}
	return t.Format("Jan 2 2006")
}

func sameCalendarDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// replySnippetLabel is the parent preview for compose/chat.
// handle / display_name: "@label: snippet". both: "display (@handle): snippet"
// (the @ already lives inside the label).
func replySnippetLabel(parent api.Message, mode string) string {
	label := authorLabel(parent.Author, mode)
	snippet := truncateSnippet(parent.Text, replySnippetMax)
	if mode == "both" {
		return label + ": " + snippet
	}
	return "@" + label + ": " + snippet
}
