package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"remiterm/internal/api"
)

// detailView is the full-viewport message detail (reply chain + full body).
// Unlike profile, it replaces the message list in the viewport rather than
// floating a card.
type detailView struct {
	open     bool
	focusID  string
	selected int // index into chain
	chain    []chainItem
}

// openDetail opens detail on msgID from the local store. No-op if unknown.
func (m Model) openDetail(msgID string) (tea.Model, tea.Cmd) {
	if msgID == "" || m.store == nil {
		return m, nil
	}
	chain := m.store.replyChain(msgID)
	if len(chain) == 0 {
		return m, nil
	}
	fi := focusIndex(chain)
	if fi < 0 {
		fi = 0
	}
	m.detail = detailView{
		open:     true,
		focusID:  msgID,
		selected: fi,
		chain:    chain,
	}
	m.showHelp = false
	m.redraw()
	// Prefer showing the focus row near the top of the viewport.
	m.viewport.GotoTop()
	return m, nil
}

// closeDetail leaves detail and selects the detail focus id in the list.
func (m Model) closeDetail() (tea.Model, tea.Cmd) {
	focusID := m.detail.focusID
	m.detail = detailView{}
	if idx := m.store.indexOf(focusID); idx >= 0 {
		m.selected = idx
	}
	m.focus = focusMessages
	m.textarea.Blur()
	m.redraw()
	m.ensureSelectedVisible()
	return m, nil
}

// openSelectedDetail opens detail for the list selection.
func (m Model) openSelectedDetail() (tea.Model, tea.Cmd) {
	if m.store == nil || m.store.len() == 0 {
		return m, nil
	}
	idx := m.selected
	if idx < 0 {
		idx = m.store.len() - 1
	}
	id := m.store.idAt(idx)
	if id == "" {
		return m, nil
	}
	return m.openDetail(id)
}

func (m Model) handleDetailKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		return m.closeDetail()
	case "enter":
		// Commit 3: re-root. Shell: no-op on focus; re-root wired next.
		return m.detailReRoot()
	case "up", "k":
		if m.detail.selected > 0 {
			m.detail.selected--
			m.redraw()
			m.ensureDetailSelectedVisible()
		}
		return m, nil
	case "down", "j":
		if m.detail.selected < len(m.detail.chain)-1 {
			m.detail.selected++
			m.redraw()
			m.ensureDetailSelectedVisible()
		}
		return m, nil
	case "r":
		return m.detailReply()
	case "o":
		return m.detailOpenMedia()
	case "i":
		return m.detailOpenPreview()
	case "p":
		return m.detailOpenProfile()
	case "?", "ctrl+g":
		m.showHelp = true
		return m, nil
	}
	return m, nil
}

// detailReRoot sets the selected chain message as the new detail focus.
func (m Model) detailReRoot() (tea.Model, tea.Cmd) {
	it, ok := m.detailSelectedItem()
	if !ok || it.Kind == chainMissingParent || it.Msg.ID == "" {
		return m, nil
	}
	if it.Msg.ID == m.detail.focusID {
		return m, nil
	}
	return m.openDetail(it.Msg.ID)
}

func (m Model) detailReply() (tea.Model, tea.Cmd) {
	it, ok := m.detailSelectedItem()
	if !ok || it.Kind == chainMissingParent || it.Msg.ID == "" {
		return m, nil
	}
	msg := it.Msg
	m.detail = detailView{}
	return m.startReplyTo(msg)
}

func (m Model) detailOpenMedia() (tea.Model, tea.Cmd) {
	it, ok := m.detailSelectedItem()
	if !ok || it.Kind == chainMissingParent {
		m.errMsg = "no message selected"
		return m, nil
	}
	return m.openMediaFor(it.Msg)
}

func (m Model) detailOpenProfile() (tea.Model, tea.Cmd) {
	it, ok := m.detailSelectedItem()
	if !ok || it.Kind == chainMissingParent {
		m.errMsg = "no message selected"
		return m, nil
	}
	h := authorHandle(it.Msg.Author)
	if h == "" || h == "?" {
		m.errMsg = "no handle"
		return m, nil
	}
	return m.openProfile(h, false)
}

func (m Model) detailSelectedItem() (chainItem, bool) {
	if !m.detail.open || m.detail.selected < 0 || m.detail.selected >= len(m.detail.chain) {
		return chainItem{}, false
	}
	return m.detail.chain[m.detail.selected], true
}

func (m *Model) ensureDetailSelectedVisible() {
	if m.detail.selected < 0 || m.detail.selected >= len(m.itemSpans) {
		return
	}
	revealSpan(&m.viewport, m.itemSpans[m.detail.selected])
}

// renderDetail draws the reply chain into the viewport.
func (m *Model) renderDetail() string {
	if len(m.detail.chain) == 0 {
		m.itemSpans = nil
		return styleStatus.Render("message not loaded")
	}
	title := styleStatus.Render("message detail · esc back")
	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n\n")
	// title + the extra blank line from "\n\n".
	prefix := lipgloss.Height(title) + 1
	blocks := make([]string, len(m.detail.chain))
	for i, it := range m.detail.chain {
		blocks[i] = m.renderChainItem(it, i == m.detail.selected)
		b.WriteString(blocks[i])
		if i < len(m.detail.chain)-1 {
			b.WriteString("\n\n")
		}
	}
	m.itemSpans = lineSpans(blocks, prefix, 1)
	return b.String()
}

func (m Model) renderChainItem(it chainItem, selected bool) string {
	if it.Kind == chainMissingParent {
		line := styleReply.Render("↳ parent not loaded")
		if selected {
			return styleSelected.Width(max(0, m.contentWidth())).Render(line)
		}
		return line
	}

	msg := it.Msg
	label := authorLabel(msg.Author, m.prefs.AuthorLabel)
	identity := authorHandle(msg.Author)
	hStyle := styleHandle
	if m.meHandle != "" && identity == m.meHandle {
		hStyle = styleHandleOwn
	}

	ts := ""
	if t := messageTime(msg.CreatedAt); !t.IsZero() {
		if it.Kind == chainFocus {
			ts = t.Local().Format("2006-01-02 15:04:05")
		} else {
			ts = formatTimestamp(t, time.Now(), m.prefs.TimestampFormat)
		}
	}

	var head strings.Builder
	switch it.Kind {
	case chainAncestor:
		head.WriteString(styleReply.Render("↑ "))
	case chainChild:
		head.WriteString(styleReply.Render("↳ "))
	case chainFocus:
		head.WriteString(styleBanner.Render("● "))
	}
	head.WriteString(styleTime.Render(ts))
	head.WriteString(" ")
	head.WriteString(hStyle.Render(label))

	// Body: full text for focus; one-line snippet for ancestors/children.
	var body strings.Builder
	if it.Kind == chainFocus {
		body.WriteString(msg.Text)
	} else {
		body.WriteString(truncateSnippet(msg.Text, replySnippetMax))
	}
	for _, med := range msg.Media {
		href := absoluteURL(m.apiBase, med.URL)
		if href == "" {
			href = absoluteURL(m.apiBase, med.ThumbnailURL)
		}
		body.WriteString(" ")
		body.WriteString(mediaLabel(href, med.Kind))
	}
	if len(msg.Reactions) > 0 {
		var parts []string
		for _, r := range msg.Reactions {
			parts = append(parts, fmt.Sprintf("%s%d", r.Emoji, r.Count))
		}
		body.WriteString("\n")
		body.WriteString(styleTime.Render(strings.Join(parts, " ")))
	}
	if isEdited(msg) {
		body.WriteString(styleTime.Render(" (edited)"))
	}

	block := head.String() + "\n" + body.String()
	cw := m.contentWidth()
	if selected {
		return styleSelected.Width(max(0, cw)).Render(block)
	}
	if cw > 4 {
		return lipgloss.NewStyle().Width(cw).Render(block)
	}
	return block
}

// openMediaFor opens the first media/URL on msg in the browser.
func (m Model) openMediaFor(msg api.Message) (tea.Model, tea.Cmd) {
	u := firstOpenURL(m.apiBase, msg)
	if u == "" {
		m.errMsg = "no media/url on selected message"
		return m, nil
	}
	if err := openURL(u); err != nil {
		m.errMsg = "open: " + err.Error()
		return m, nil
	}
	m.errMsg = ""
	return m, m.setTransientStatus("opened in browser")
}
