package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
)

func TestOnlineLabel(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 11, 16, 33, 41, 0, time.UTC)
	const defaultPoll = 5 * time.Second
	cases := []struct {
		name       string
		lastOK     time.Time
		pollFailed bool
		poll       time.Duration
		want       string
	}{
		{name: "initial", poll: defaultPoll, want: "connecting…"},
		{name: "first fail", pollFailed: true, poll: defaultPoll, want: "offline"},
		{name: "fresh success", lastOK: now.Add(-2 * time.Second), poll: defaultPoll, want: "online"},
		{name: "exactly 5s still online", lastOK: now.Add(-pollStaleAfter), poll: defaultPoll, want: "online"},
		{name: "stale success", lastOK: now.Add(-pollStaleAfter - time.Second), poll: defaultPoll, want: "offline"},
		{name: "fail after success", lastOK: now.Add(-time.Second), pollFailed: true, poll: defaultPoll, want: "offline"},
		// Demo harness uses a long poll so backscroll stays quiet; success
		// within that interval must still read as online.
		{name: "long poll still online", lastOK: now.Add(-10 * time.Minute), poll: time.Hour, want: "online"},
		{name: "long poll stale after interval", lastOK: now.Add(-time.Hour - time.Second), poll: time.Hour, want: "offline"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := onlineLabel(tc.lastOK, tc.pollFailed, now, tc.poll)
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestRenderMainConnectionPlacement(t *testing.T) {
	t.Parallel()

	readyModel := func() Model {
		m := New(nil, 5*time.Second)
		m.width = 80
		m.height = 24
		m.ready = true
		m.meHandle = "me"
		m.layout()
		return m
	}

	headerFooter := func(t *testing.T, got string) (header, rest string) {
		t.Helper()
		lines := strings.Split(got, "\n")
		if len(lines) < 2 {
			t.Fatalf("too few lines: %q", got)
		}
		return lines[0], strings.Join(lines[1:], "\n")
	}

	t.Run("online in header not footer", func(t *testing.T) {
		m := readyModel()
		m.lastPoll = time.Now()
		m.pollFailed = false
		got := m.renderMain()
		header, footer := headerFooter(t, got)
		plain := stripANSI(header)
		if !strings.Contains(plain, "remiterm") {
			t.Fatalf("header missing title: %q", plain)
		}
		if !strings.HasSuffix(strings.TrimRight(plain, " "), "online") {
			t.Fatalf("header should end with online: %q", plain)
		}
		if lipgloss.Width(header) != m.width {
			t.Fatalf("header width %d want %d", lipgloss.Width(header), m.width)
		}
		footPlain := stripANSI(footer)
		for _, word := range []string{"online", "offline", "connecting"} {
			if strings.Contains(footPlain, word) {
				t.Fatalf("footer still has poll health %q: %q", word, footPlain)
			}
		}
		for _, tag := range []string{"[compose]", "[messages]", "[detail]"} {
			if strings.Contains(footPlain, tag) {
				t.Fatalf("footer still has mode tag %s: %q", tag, footPlain)
			}
		}
		if !strings.Contains(footPlain, "Enter send") {
			t.Fatalf("footer missing hints: %q", footPlain)
		}
	})

	t.Run("connecting before first poll", func(t *testing.T) {
		m := readyModel()
		got := m.renderMain()
		header, footer := headerFooter(t, got)
		plain := stripANSI(header)
		if !strings.Contains(plain, "connecting…") {
			t.Fatalf("header missing connecting: %q", plain)
		}
		if strings.Contains(stripANSI(footer), "connecting") {
			t.Fatalf("footer still has connecting: %q", footer)
		}
	})

	t.Run("offline after failed poll", func(t *testing.T) {
		m := readyModel()
		m.pollFailed = true
		got := m.renderMain()
		header, footer := headerFooter(t, got)
		plain := stripANSI(header)
		if !strings.HasSuffix(strings.TrimRight(plain, " "), "offline") {
			t.Fatalf("header should end with offline: %q", plain)
		}
		if strings.Contains(stripANSI(footer), "offline") {
			t.Fatalf("footer still has offline: %q", footer)
		}
	})

	t.Run("pending new stays on header left", func(t *testing.T) {
		m := readyModel()
		m.lastPoll = time.Now()
		m.pendingNew = 3
		got := m.renderMain()
		plain := stripANSI(strings.Split(got, "\n")[0])
		if !strings.Contains(plain, "↓ 3 new") {
			t.Fatalf("header missing pending new: %q", plain)
		}
		if !strings.HasSuffix(strings.TrimRight(plain, " "), "online") {
			t.Fatalf("header should still end with online: %q", plain)
		}
	})
}
