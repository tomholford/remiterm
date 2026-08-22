package tui

import (
	"bytes"
	"image/color"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"remiterm/internal/api"
)

func previewTestModel(msgs ...api.Message) Model {
	m := New(nil, time.Hour)
	m.focus = focusMessages
	m.width = 80
	m.height = 40
	m.ready = true
	for _, msg := range msgs {
		m.store.upsert(msg)
	}
	m.layout()
	return m
}

func TestOpenPreviewNoMedia(t *testing.T) {
	t.Parallel()
	m := previewTestModel(api.Message{
		ID: "1", Text: "nope", Author: api.Author{Handle: "alice"},
	})
	m.selected = 0
	got, _ := m.handleKey(keyMsg("i"))
	m = got.(Model)
	if m.preview.open {
		t.Fatal("preview should stay closed")
	}
	if m.errMsg != "no media on selected message" {
		t.Fatalf("errMsg=%q", m.errMsg)
	}
}

func TestOpenPreviewVideoHint(t *testing.T) {
	t.Parallel()
	m := previewTestModel(api.Message{
		ID: "1", Text: "clip", Author: api.Author{Handle: "alice"},
		Media: []api.Media{{Kind: "video", URL: "https://example.com/v.mp4"}},
	})
	m.selected = 0
	got, cmd := m.handleKey(keyMsg("i"))
	m = got.(Model)
	if cmd != nil {
		t.Fatal("video should not fetch")
	}
	if !m.preview.open {
		t.Fatal("preview should open")
	}
	if m.preview.loading {
		t.Fatal("video should not load")
	}
	card := m.renderPreviewCard(40, 16)
	if !strings.Contains(card, "video") {
		t.Fatalf("%q", card)
	}
}

func TestPreviewKeys(t *testing.T) {
	t.Parallel()
	m := previewTestModel(api.Message{
		ID: "1", Text: "pic", Author: api.Author{Handle: "alice"},
		Media: []api.Media{{Kind: "image", URL: "https://example.com/a.png"}},
	})
	m.selected = 0
	m.preview = previewModal{open: true, url: "https://example.com/a.png"}

	got, _ := m.handleKey(keyMsg("esc"))
	m = got.(Model)
	if m.preview.open {
		t.Fatal("esc should close")
	}

	m.preview = previewModal{open: true}
	got, _ = m.handleKey(keyMsg("q"))
	m = got.(Model)
	if m.preview.open {
		t.Fatal("q should close")
	}

	m.focus = focusCompose
	m.textarea.Focus()
	m.preview = previewModal{}
	got, _ = m.handleKey(keyMsg("i"))
	m = got.(Model)
	if m.preview.open {
		t.Fatal("i in compose should type, not preview")
	}
}

func TestRenderPreviewCard(t *testing.T) {
	t.Parallel()
	raw := solidPNG(t, color.RGBA{G: 200, A: 255})
	img, _, err := decodeStill(bytes.NewReader(raw), previewMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	m := previewTestModel()
	m.preview = previewModal{open: true, img: img, hint: previewHintAnimated}
	m.preview.art = m.renderPreviewArt()
	card := m.renderPreviewCard(40, 16)
	if !strings.Contains(card, "media preview") {
		t.Fatalf("title: %q", card)
	}
	if !strings.Contains(card, "animated") {
		t.Fatalf("hint: %q", card)
	}
	if !strings.Contains(card, "esc close") {
		t.Fatalf("footer: %q", card)
	}
	if m.preview.art == "" {
		t.Fatal("expected mosaic art")
	}
}

func TestPreviewArtFillsTerminal(t *testing.T) {
	t.Parallel()
	raw := solidPNG(t, color.RGBA{B: 200, A: 255})
	img, _, err := decodeStill(bytes.NewReader(raw), previewMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	m := previewTestModel()
	m.preview = previewModal{open: true, img: img}

	m.width, m.height = 40, 24
	small := m.renderPreviewArt()
	m.width, m.height = 120, 60
	large := m.renderPreviewArt()
	if large == "" || small == "" {
		t.Fatal("empty mosaic")
	}
	if lipgloss.Width(large) <= lipgloss.Width(small) {
		t.Fatalf("width did not grow: small=%d large=%d", lipgloss.Width(small), lipgloss.Width(large))
	}
	if lipgloss.Height(large) <= lipgloss.Height(small) {
		t.Fatalf("height did not grow: small=%d large=%d", lipgloss.Height(small), lipgloss.Height(large))
	}

	m.width, m.height = 100, 40
	card := m.renderPreviewCard(previewCardWidth(m.width), previewCardHeight(m.height))
	if got, want := lipgloss.Width(card), m.width; got != want {
		t.Fatalf("card width %d want %d", got, want)
	}
	if got, want := lipgloss.Height(card), m.height; got != want {
		t.Fatalf("card height %d want %d", got, want)
	}
}

func TestPreviewOverlayFillsTerminal(t *testing.T) {
	t.Parallel()
	raw := solidPNG(t, color.RGBA{B: 200, A: 255})
	img, _, err := decodeStill(bytes.NewReader(raw), previewMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	m := previewTestModel()
	m.width, m.height = 80, 24
	m.preview = previewModal{open: true, img: img}
	m.preview.art = m.renderPreviewArt()

	card := m.renderPreviewCard(previewCardWidth(m.width), previewCardHeight(m.height))
	if got, want := lipgloss.Width(card), m.width; got != want {
		t.Fatalf("card width %d want %d", got, want)
	}
	if got, want := lipgloss.Height(card), m.height; got != want {
		t.Fatalf("card height %d want %d (gutter below overlay)", got, want)
	}

	artH := lipgloss.Height(m.preview.art)
	_, innerH := previewMosaicBudget(m.width, m.height, false)
	if artH < innerH-1 {
		t.Fatalf("art height %d, inner budget %d", artH, innerH)
	}
	overlay := m.renderPreviewOverlay("base")
	if got := lipgloss.Height(overlay); got != m.height {
		t.Fatalf("overlay height %d want %d", got, m.height)
	}
}

func TestPreviewMosaicBudget(t *testing.T) {
	t.Parallel()
	w, h := previewMosaicBudget(80, 24, false)
	if w != 80-previewCardHPad {
		t.Fatalf("innerW %d", w)
	}
	wantH := 24 - previewCardVPad - previewTitleLines - previewFooterLines
	if h != wantH {
		t.Fatalf("innerH %d want %d", h, wantH)
	}
	_, hintH := previewMosaicBudget(80, 24, true)
	if hintH != wantH-1 {
		t.Fatalf("hint innerH %d want %d", hintH, wantH-1)
	}
}

func TestPreviewOverlayLoading(t *testing.T) {
	t.Parallel()
	m := previewTestModel()
	m.preview = previewModal{open: true, loading: true}
	card := m.renderPreviewCard(40, 16)
	if !strings.Contains(card, "loading") {
		t.Fatalf("%q", card)
	}
	out := m.renderPreviewOverlay("base")
	if !strings.Contains(out, "media preview") {
		t.Fatalf("overlay: %q", out)
	}
}

func TestPreviewFetchApplies(t *testing.T) {
	t.Parallel()
	raw := solidPNG(t, color.White)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(raw)
	}))
	t.Cleanup(srv.Close)

	m := previewTestModel(api.Message{
		ID: "1", Text: "pic", Author: api.Author{Handle: "alice"},
		Media: []api.Media{{Kind: "image", URL: srv.URL + "/a.png"}},
	})
	m.selected = 0
	got, cmd := m.openSelectedPreview()
	m = got.(Model)
	if !m.preview.open || !m.preview.loading || cmd == nil {
		t.Fatalf("open loading=%v cmd=%v", m.preview.loading, cmd != nil)
	}
	msg := cmd()
	got, _ = m.Update(msg)
	m = got.(Model)
	if m.preview.loading {
		t.Fatal("should finish loading")
	}
	if m.preview.err != "" {
		t.Fatalf("err %q", m.preview.err)
	}
	if m.preview.art == "" || m.preview.img == nil {
		t.Fatal("expected decoded mosaic")
	}
}

func TestPreviewIgnoresStaleFetch(t *testing.T) {
	t.Parallel()
	m := previewTestModel()
	m.preview = previewModal{open: true, gen: 2, loading: true}
	got, _ := m.applyPreviewMsg(previewMsg{gen: 1, img: nil})
	m = got.(Model)
	if !m.preview.loading {
		t.Fatal("stale fetch should be ignored")
	}
}

func TestDetailOpensPreview(t *testing.T) {
	t.Parallel()
	m := previewTestModel(api.Message{
		ID: "1", Text: "pic", Author: api.Author{Handle: "alice"},
		Media: []api.Media{{Kind: "video", URL: "https://example.com/v.mp4"}},
	})
	got, _ := m.openDetail("1")
	m = got.(Model)
	got, _ = m.handleKey(keyMsg("i"))
	m = got.(Model)
	if !m.preview.open {
		t.Fatal("i in detail should preview")
	}
}
