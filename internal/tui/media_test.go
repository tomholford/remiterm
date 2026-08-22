package tui

import (
	"strings"
	"testing"

	"remiterm/internal/api"
)

func TestSiteOrigin(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"https://www.remilia.net/api/v1", "https://www.remilia.net"},
		{"https://www.remilia.net/api/v1/", "https://www.remilia.net"},
		{"http://localhost:8080/api/v1", "http://localhost:8080"},
		{"", "https://www.remilia.net"},
		{"not-a-url", "https://www.remilia.net"},
	}
	for _, tc := range cases {
		if got := siteOrigin(tc.in); got != tc.want {
			t.Fatalf("siteOrigin(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestAbsoluteURL(t *testing.T) {
	t.Parallel()
	base := "https://www.remilia.net/api/v1"
	rel := "/imgproxy/abc/preview/plain/local:///data/blobs/perm/x@webp"
	want := "https://www.remilia.net" + rel
	if got := absoluteURL(base, rel); got != want {
		t.Fatalf("relative: got %q want %q", got, want)
	}
	abs := "https://cdn.example.com/a.png"
	if got := absoluteURL(base, abs); got != abs {
		t.Fatalf("absolute: got %q", got)
	}
	if got := absoluteURL(base, ""); got != "" {
		t.Fatalf("empty: %q", got)
	}
}

func TestPreviewTarget(t *testing.T) {
	t.Parallel()
	base := "https://www.remilia.net/api/v1"

	href, hint, ok := previewTarget(base, api.Message{
		Media: []api.Media{
			{Kind: "image", ThumbnailURL: "/imgproxy/thumb", URL: "/imgproxy/full"},
		},
	})
	if !ok || href != "https://www.remilia.net/imgproxy/thumb" || hint != "" {
		t.Fatalf("thumbnail: href=%q hint=%q ok=%v", href, hint, ok)
	}

	href, hint, ok = previewTarget(base, api.Message{
		Media: []api.Media{
			{Kind: "video", URL: "/vid.mp4"},
			{Kind: "image", URL: "/imgproxy/x"},
		},
	})
	if !ok || href != "https://www.remilia.net/imgproxy/x" || hint != "" {
		t.Fatalf("skip video for still: href=%q hint=%q ok=%v", href, hint, ok)
	}

	href, hint, ok = previewTarget(base, api.Message{
		Media: []api.Media{{Kind: "video", URL: "/vid.mp4"}},
	})
	if ok || href != "https://www.remilia.net/vid.mp4" || hint != previewHintVideo {
		t.Fatalf("video only: href=%q hint=%q ok=%v", href, hint, ok)
	}

	href, hint, ok = previewTarget(base, api.Message{
		Media: []api.Media{{Kind: "image", URL: "/imgproxy/g", IsAnimated: true}},
	})
	if !ok || hint != previewHintAnimated {
		t.Fatalf("animated: href=%q hint=%q ok=%v", href, hint, ok)
	}

	if _, _, ok := previewTarget(base, api.Message{Text: "nope"}); ok {
		t.Fatal("empty should not be ok")
	}
}

func TestFirstOpenURL(t *testing.T) {
	t.Parallel()
	base := "https://www.remilia.net/api/v1"
	msg := api.Message{
		Text: "see this",
		Media: []api.Media{
			{Kind: "image", URL: "/imgproxy/x"},
		},
	}
	got := firstOpenURL(base, msg)
	if got != "https://www.remilia.net/imgproxy/x" {
		t.Fatalf("media: %q", got)
	}

	textOnly := api.Message{Text: "check https://example.com/pic.png please"}
	if got := firstOpenURL(base, textOnly); got != "https://example.com/pic.png" {
		t.Fatalf("text url: %q", got)
	}

	if got := firstOpenURL(base, api.Message{Text: "nope"}); got != "" {
		t.Fatalf("empty: %q", got)
	}
}

func TestMediaBadge(t *testing.T) {
	t.Parallel()
	if got := mediaBadge("image"); got != "[image ↗]" {
		t.Fatalf("%q", got)
	}
	if got := mediaBadge(""); got != "[media ↗]" {
		t.Fatalf("%q", got)
	}
}

func TestMediaLabel(t *testing.T) {
	t.Parallel()
	out := mediaLabel("https://example.com/a", "image")
	if !strings.Contains(out, "https://example.com/a") {
		t.Fatalf("missing href: %q", out)
	}
	if !strings.Contains(out, "[image ↗]") {
		t.Fatalf("missing label: %q", out)
	}
	// Lip Gloss Hyperlink emits OSC 8 (ESC ] 8 ;).
	if !strings.Contains(out, "\x1b]8;") {
		t.Fatalf("missing OSC 8 sequence: %q", out)
	}
	plain := mediaLabel("", "image")
	if plain != styleMedia.Render("[image ↗]") {
		t.Fatalf("empty href: %q", plain)
	}
	if strings.Contains(plain, "\x1b]8;") {
		t.Fatalf("empty href should not hyperlink: %q", plain)
	}
}
