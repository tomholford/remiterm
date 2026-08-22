package tui

import (
	"bytes"
	"context"
	"image"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"remiterm/internal/api"
)

const (
	previewCardMinWidth  = 24
	previewCardMinHeight = 8
	// styleProfileCard: rounded border + Padding(1, 2). Width/Height on the
	// style include that chrome, so the mosaic budget subtracts it.
	previewCardHPad    = 6 // left/right border + padding
	previewCardVPad    = 4 // top/bottom border + padding
	previewTitleLines  = 1
	previewFooterLines = 2 // blank row + key hints
)

// previewModal is a Canvas overlay for in-TUI stills.
type previewModal struct {
	open    bool
	loading bool
	url     string
	hint    string
	err     string
	gen     uint64
	img     image.Image
	art     string
}

type previewMsg struct {
	gen uint64
	url string
	img image.Image
	err error
}

func (m Model) openSelectedPreview() (tea.Model, tea.Cmd) {
	if m.store == nil || m.store.len() == 0 {
		m.errMsg = "no messages"
		return m, nil
	}
	idx := m.selected
	if idx < 0 {
		idx = m.store.len() - 1
	}
	list := m.store.list()
	if idx < 0 || idx >= len(list) {
		return m, nil
	}
	return m.openPreviewFor(list[idx])
}

func (m Model) openPreviewFor(msg api.Message) (tea.Model, tea.Cmd) {
	href, hint, ok := previewTarget(m.apiBase, msg)
	if href == "" && !ok {
		m.errMsg = "no media on selected message"
		return m, nil
	}
	m.preview.gen++
	m.preview.open = true
	m.preview.url = href
	m.preview.hint = hint
	m.preview.err = ""
	m.preview.img = nil
	m.preview.art = ""
	m.showHelp = false
	if !ok {
		m.preview.loading = false
		return m, nil
	}
	m.preview.loading = true
	return m, m.fetchPreviewCmd(href, m.preview.gen)
}

func (m Model) closePreview() (tea.Model, tea.Cmd) {
	m.preview = previewModal{}
	return m, nil
}

func (m Model) fetchPreviewCmd(rawURL string, gen uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		raw, err := fetchPreview(ctx, rawURL, previewMaxBytes)
		if err != nil {
			return previewMsg{gen: gen, url: rawURL, err: err}
		}
		img, _, err := decodeStill(bytes.NewReader(raw), previewMaxBytes)
		if err != nil {
			return previewMsg{gen: gen, url: rawURL, err: err}
		}
		return previewMsg{gen: gen, url: rawURL, img: img}
	}
}

func (m Model) applyPreviewMsg(msg previewMsg) (tea.Model, tea.Cmd) {
	if !m.preview.open || msg.gen != m.preview.gen {
		return m, nil
	}
	m.preview.loading = false
	if msg.err != nil {
		m.preview.err = msg.err.Error()
		return m, nil
	}
	m.preview.img = msg.img
	m.preview.art = m.renderPreviewArt()
	return m, nil
}

func (m Model) handlePreviewKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		return m.closePreview()
	case "ctrl+c":
		return m, tea.Quit
	case "o":
		if m.preview.url == "" {
			m.preview.err = "no url to open"
			return m, nil
		}
		if err := openURL(m.preview.url); err != nil {
			m.preview.err = "open: " + err.Error()
			return m, nil
		}
		m.preview.err = ""
		return m, m.setTransientStatus("opened in browser")
	}
	return m, nil
}

func (m Model) renderPreviewOverlay(base string) string {
	w, h := m.width, m.height
	if w < 20 {
		w = 20
	}
	if h < 8 {
		h = 8
	}
	card := m.renderPreviewCard(previewCardWidth(w), previewCardHeight(h))
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

func (m Model) renderPreviewCard(width, height int) string {
	if width < previewCardMinWidth {
		width = previewCardMinWidth
	}
	if height < previewCardMinHeight {
		height = previewCardMinHeight
	}
	var b strings.Builder
	b.WriteString(styleProfileTitle.Render("media preview"))
	b.WriteString("\n")
	if m.preview.hint != "" {
		b.WriteString(styleProfileLabel.Render(m.preview.hint))
		b.WriteString("\n")
	}

	switch {
	case m.preview.loading:
		b.WriteString(styleProfileLabel.Render("loading…"))
		b.WriteString("\n")
	case m.preview.err != "":
		b.WriteString(styleErr.Render(m.preview.err))
		b.WriteString("\n")
	case m.preview.art != "":
		b.WriteString(strings.TrimRight(m.preview.art, "\n"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styleProfileLabel.Render("esc close · o open original"))
	return styleProfileCard.Width(width).Height(height).Render(b.String())
}

func previewCardWidth(termW int) int {
	if termW < previewCardMinWidth {
		return previewCardMinWidth
	}
	return termW
}

func previewCardHeight(termH int) int {
	if termH < previewCardMinHeight {
		return previewCardMinHeight
	}
	return termH
}

func previewMosaicBudget(termW, termH int, hasHint bool) (innerW, innerH int) {
	innerW = previewCardWidth(termW) - previewCardHPad
	innerH = previewCardHeight(termH) - previewCardVPad - previewTitleLines - previewFooterLines
	if hasHint {
		innerH--
	}
	if innerW < 1 {
		innerW = 1
	}
	if innerH < 1 {
		innerH = 1
	}
	return innerW, innerH
}

func (m Model) renderPreviewArt() string {
	if m.preview.img == nil {
		return ""
	}
	innerW, innerH := previewMosaicBudget(m.width, m.height, m.preview.hint != "")
	return renderMosaic(m.preview.img, innerW, innerH)
}

func (m Model) detailOpenPreview() (tea.Model, tea.Cmd) {
	it, ok := m.detailSelectedItem()
	if !ok || it.Kind == chainMissingParent {
		m.errMsg = "no message selected"
		return m, nil
	}
	return m.openPreviewFor(it.Msg)
}
