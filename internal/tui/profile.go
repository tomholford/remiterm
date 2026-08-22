package tui

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"remiterm/internal/api"
)

// profileModal holds overlay state for viewing a user profile.
type profileModal struct {
	open    bool
	loading bool
	poking  bool
	handle  string
	profile api.Profile
	err     string
	status  string

	stats        api.MeStats
	statsLoading bool
	statsHidden  bool
	statsErr     string
	statsGen     uint64
}

type profileMsg struct {
	handle  string
	profile api.Profile
	err     error
	// me is true when this was a GET /me fetch (used to refresh meHandle).
	me bool
}

type pokeMsg struct {
	handle string
	err    error
}

type statsMsg struct {
	gen   uint64
	stats api.MeStats
	err   error
}

// selectedAuthorHandle returns the handle of the currently selected message author.
func (m Model) selectedAuthorHandle() string {
	if m.store == nil || m.store.len() == 0 {
		return ""
	}
	idx := m.selected
	if idx < 0 {
		idx = m.store.len() - 1
	}
	list := m.store.list()
	if idx < 0 || idx >= len(list) {
		return ""
	}
	h := list[idx].Author.Handle
	if h == "" {
		h = list[idx].Author.DisplayName
	}
	return h
}

func (m Model) openSelectedProfile() (tea.Model, tea.Cmd) {
	handle := m.selectedAuthorHandle()
	if handle == "" {
		m.errMsg = "no message selected"
		return m, nil
	}
	return m.openProfile(handle, false)
}

func (m Model) openMeProfile() (tea.Model, tea.Cmd) {
	if m.meHandle != "" {
		return m.openProfile(m.meHandle, true)
	}
	// Unknown handle yet — still open modal and load via GET /me.
	m.profile = profileModal{
		open:    true,
		loading: true,
		handle:  "me",
	}
	m.showHelp = false
	var statsCmd tea.Cmd
	m, statsCmd = m.startMeStats()
	return m, tea.Batch(m.fetchMeProfile(), statsCmd)
}

func (m Model) openProfile(handle string, me bool) (tea.Model, tea.Cmd) {
	handle = strings.TrimSpace(handle)
	if handle == "" {
		m.errMsg = "no handle"
		return m, nil
	}
	cached, hit := m.cachedProfile(handle)
	m.profile = profileModal{
		open:    true,
		loading: !hit,
		handle:  handle,
		profile: cached,
	}
	m.showHelp = false
	if me {
		var statsCmd tea.Cmd
		m, statsCmd = m.startMeStats()
		return m, tea.Batch(m.fetchMeProfile(), statsCmd)
	}
	return m, m.fetchProfile(handle)
}

func (m Model) closeProfile() (tea.Model, tea.Cmd) {
	m.profile = profileModal{}
	return m, nil
}

func (m Model) fetchProfile(handle string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		p, err := m.client.GetUser(ctx, handle)
		return profileMsg{handle: handle, profile: p, err: err}
	}
}

func (m Model) fetchMeProfile() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		// Prefer public GetUser when we know our handle (richer, no extra scope).
		if m.meHandle != "" {
			p, err := m.client.GetUser(ctx, m.meHandle)
			return profileMsg{handle: m.meHandle, profile: p, err: err, me: true}
		}
		p, err := m.client.Me(ctx)
		handle := p.Handle()
		return profileMsg{handle: handle, profile: p, err: err, me: true}
	}
}

func (m Model) startMeStats() (Model, tea.Cmd) {
	if m.client == nil {
		return m, nil
	}
	m.profile.statsLoading = true
	m.profile.statsHidden = false
	m.profile.statsErr = ""
	m.profile.stats = api.MeStats{}
	m.profile.statsGen++
	return m, m.fetchMeStats()
}

func (m Model) fetchMeStats() tea.Cmd {
	gen := m.profile.statsGen
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		s, err := m.client.MeStats(ctx)
		return statsMsg{gen: gen, stats: s, err: err}
	}
}

func (m Model) pokeUser(handle string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, err := m.client.Poke(ctx, handle)
		return pokeMsg{handle: handle, err: err}
	}
}

func (m Model) handleProfileKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		return m.closeProfile()
	case "ctrl+c":
		return m, tea.Quit
	case "o":
		handle := m.profile.handle
		if m.profile.profile.Handle() != "" {
			handle = m.profile.profile.Handle()
		}
		if handle == "" || handle == "me" {
			m.profile.err = "no handle to open"
			return m, nil
		}
		u := "https://www.remilia.net/" + handle
		if err := openURL(u); err != nil {
			m.profile.err = "open: " + err.Error()
			return m, nil
		}
		m.profile.status = "opened in browser"
		m.profile.err = ""
		return m, nil
	case "P":
		if m.profile.loading || m.profile.poking {
			return m, nil
		}
		if m.profile.err != "" && m.profile.profile.Handle() == "" {
			return m, nil
		}
		p := m.profile.profile
		if p.IsOwnProfile {
			m.profile.status = "can't poke yourself"
			return m, nil
		}
		handle := p.Handle()
		if handle == "" {
			handle = m.profile.handle
		}
		if handle == "" || handle == "me" {
			m.profile.err = "no handle to poke"
			return m, nil
		}
		m.profile.poking = true
		m.profile.status = "poking…"
		m.profile.err = ""
		return m, m.pokeUser(handle)
	}
	// Swallow other keys while modal is open.
	return m, nil
}

func (m Model) applyProfileMsg(msg profileMsg) (tea.Model, tea.Cmd) {
	if !m.profile.open {
		return m, nil
	}
	// Ignore stale responses if the user switched targets.
	if msg.handle != "" && m.profile.handle != "" && msg.handle != m.profile.handle && !msg.me {
		return m, nil
	}
	m.profile.loading = false
	if msg.err != nil {
		if m.profile.profile.Handle() != "" {
			return m, nil
		}
		m.profile.err = msg.err.Error()
		return m, nil
	}
	m.profile.profile = msg.profile
	m.profile.err = ""
	m.persistProfile(msg.profile)
	if h := msg.profile.Handle(); h != "" {
		m.profile.handle = h
	}
	if msg.me && msg.profile.Handle() != "" {
		m.meHandle = msg.profile.Handle()
	}
	own := msg.me || msg.profile.IsOwnProfile
	if own && !m.profile.statsLoading && m.profile.statsGen == 0 {
		return m.startMeStats()
	}
	return m, nil
}

func (m Model) applyStatsMsg(msg statsMsg) (tea.Model, tea.Cmd) {
	if !m.profile.open || msg.gen != m.profile.statsGen {
		return m, nil
	}
	m.profile.statsLoading = false
	if msg.err != nil {
		var ae *api.Error
		if errors.As(msg.err, &ae) && ae.Code == "insufficient_scope" {
			m.profile.statsHidden = true
			m.profile.statsErr = ""
			return m, nil
		}
		m.profile.statsErr = "stats unavailable"
		return m, nil
	}
	m.profile.stats = msg.stats
	m.profile.statsHidden = false
	m.profile.statsErr = ""
	return m, nil
}

func (m Model) applyPokeMsg(msg pokeMsg) (tea.Model, tea.Cmd) {
	if !m.profile.open {
		return m, nil
	}
	m.profile.poking = false
	if msg.err != nil {
		m.profile.status = pokeStatusFromError(msg.err)
		return m, nil
	}
	m.profile.status = "poked @" + msg.handle + "!"
	// Reflect local canPoke so the footer hint updates.
	m.profile.profile.ViewerContext.CanPoke = false
	return m, nil
}

// pokeStatusFromError maps API errors to a short modal status line.
func pokeStatusFromError(err error) string {
	if err == nil {
		return ""
	}
	var ae *api.Error
	if errors.As(err, &ae) {
		if ae.Code == "poke_cooldown" || strings.Contains(strings.ToLower(ae.Message), "cooldown") {
			if ae.Message != "" {
				return ae.Message
			}
			return "poke on cooldown"
		}
		if ae.Code == "insufficient_scope" {
			return "need remilia:interact.poke scope"
		}
		if ae.Message != "" {
			return ae.Message
		}
		return ae.Error()
	}
	return err.Error()
}

func (m Model) renderProfileOverlay(base string) string {
	w, h := m.width, m.height
	if w < 20 {
		w = 20
	}
	if h < 8 {
		h = 8
	}
	card := m.renderProfileCard(min(48, w-4))
	cardW := lipgloss.Width(card)
	cardH := lipgloss.Height(card)
	x := (w - cardW) / 2
	y := (h - cardH) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	c := lipgloss.NewCanvas(w, h)
	c.Compose(lipgloss.NewLayer(base))
	c.Compose(lipgloss.NewLayer(card).X(x).Y(y).Z(1))
	return c.Render()
}

func (m Model) renderProfileCard(width int) string {
	if width < 24 {
		width = 24
	}
	var b strings.Builder
	handle := m.profile.handle
	if handle == "" {
		handle = "?"
	}

	if m.profile.loading {
		b.WriteString(styleProfileTitle.Render("@" + handle))
		b.WriteString("\n\n")
		b.WriteString(styleProfileLabel.Render("loading…"))
		b.WriteString("\n\n")
		b.WriteString(styleProfileLabel.Render("esc close"))
		return styleProfileCard.Width(width).Render(b.String())
	}

	if m.profile.err != "" && m.profile.profile.Handle() == "" {
		b.WriteString(styleProfileTitle.Render("@" + handle))
		b.WriteString("\n\n")
		b.WriteString(styleErr.Render(m.profile.err))
		b.WriteString("\n\n")
		b.WriteString(styleProfileLabel.Render("esc close"))
		return styleProfileCard.Width(width).Render(b.String())
	}

	p := m.profile.profile
	if p.Handle() != "" {
		handle = p.Handle()
	}
	b.WriteString(styleProfileTitle.Render("@" + handle))
	if p.IsOwnProfile {
		b.WriteString(" ")
		b.WriteString(styleProfileBadge.Render("you"))
	}
	b.WriteString("\n")
	if p.User.DisplayName != "" && p.User.DisplayName != handle {
		b.WriteString(styleProfileName.Render(p.User.DisplayName))
		b.WriteString("\n")
	}
	b.WriteString(styleProfileLabel.Render(strings.Repeat("─", max(8, width-6))))
	b.WriteString("\n")

	if p.User.Bio != "" {
		bio := strings.ReplaceAll(p.User.Bio, "\n", " ")
		b.WriteString(wrapPlain(bio, width-6))
		b.WriteString("\n")
	}

	var meta []string
	if p.User.Location != "" {
		meta = append(meta, p.User.Location)
	}
	if p.User.FriendCount > 0 {
		meta = append(meta, fmt.Sprintf("%d friends", p.User.FriendCount))
	}
	if p.ViewerContext.AreFriends {
		meta = append(meta, "friends")
	}
	if len(meta) > 0 {
		b.WriteString(styleProfileLabel.Render(strings.Join(meta, " · ")))
		b.WriteString("\n")
	}

	if m.showingOwnProfile() {
		if lines := m.ownStatsSection(); len(lines) > 0 {
			b.WriteString(styleProfileLabel.Render(strings.Repeat("─", max(8, width-6))))
			b.WriteString("\n")
			for _, line := range lines {
				b.WriteString(styleProfileLabel.Render(line))
				b.WriteString("\n")
			}
		}
	}

	if m.profile.status != "" {
		b.WriteString("\n")
		b.WriteString(styleProfileStatus.Render(m.profile.status))
		b.WriteString("\n")
	} else if m.profile.err != "" {
		b.WriteString("\n")
		b.WriteString(styleErr.Render(m.profile.err))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styleProfileLabel.Render(profileFooterHint(p)))
	return styleProfileCard.Width(width).Render(b.String())
}

func (m Model) showingOwnProfile() bool {
	if m.profile.profile.IsOwnProfile {
		return true
	}
	if m.profile.handle == "me" {
		return true
	}
	return m.meHandle != "" && m.profile.handle == m.meHandle
}

func (m Model) ownStatsSection() []string {
	if m.profile.statsHidden {
		return nil
	}
	if m.profile.statsLoading {
		return []string{"stats…"}
	}
	if m.profile.statsErr != "" {
		return []string{m.profile.statsErr}
	}
	return formatStatsLines(m.profile.stats)
}

// formatStatsLines returns a short numeric summary. At most five
// aggregate scores (sorted, zeros omitted) plus documented ethereum
// leaves. Platform blobs are never dumped.
func formatStatsLines(s api.MeStats) []string {
	keys := make([]string, 0, len(s.AggregateScores))
	for k, v := range s.AggregateScores {
		if v == 0 {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 5 {
		keys = keys[:5]
	}
	var lines []string
	for _, k := range keys {
		v := s.AggregateScores[k]
		label := strings.ReplaceAll(k, "_", " ")
		if v == float64(int64(v)) {
			lines = append(lines, fmt.Sprintf("%s %d", label, int64(v)))
		} else {
			lines = append(lines, fmt.Sprintf("%s %g", label, v))
		}
	}
	if t := strings.TrimSpace(s.Stats.Ethereum.CultTier); t != "" {
		lines = append(lines, "cult "+t)
	}
	if n := s.Stats.Ethereum.TotalOwned; n > 0 {
		lines = append(lines, fmt.Sprintf("owned %d", n))
	}
	return lines
}

func profileFooterHint(p api.Profile) string {
	if p.IsOwnProfile {
		return "esc close · o open web"
	}
	return "esc close · P poke · o open web"
}

// wrapPlain soft-wraps text without ANSI awareness (card body is unstyled text).
func wrapPlain(s string, width int) string {
	if width <= 0 || len(s) <= width {
		return s
	}
	var lines []string
	for len(s) > width {
		// break at last space in window when possible
		cut := width
		if i := strings.LastIndex(s[:width], " "); i > width/3 {
			cut = i
		}
		lines = append(lines, strings.TrimRight(s[:cut], " "))
		s = strings.TrimLeft(s[cut:], " ")
	}
	if s != "" {
		lines = append(lines, s)
	}
	return strings.Join(lines, "\n")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
