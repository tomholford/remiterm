package tui

import (
	"fmt"
	"net/url"
	"strings"

	"remiterm/internal/api"
)

// mediaLabel is a compact, optionally hyperlinked attachment badge.
// Example visible text: "[image ↗]". Empty href skips the OSC 8 link.
func mediaLabel(href, kind string) string {
	label := mediaBadge(kind)
	if href == "" {
		return styleMedia.Render(label)
	}
	return styleMedia.Hyperlink(href).Render(label)
}

// externalLinkMark is the common "opens elsewhere" glyph (north-east arrow).
const externalLinkMark = "↗"

// siteOrigin returns scheme://host from an API base like
// https://www.remilia.net/api/v1. Media paths from the API are site-relative
// (/imgproxy/...), not under /api/v1.
func siteOrigin(apiBase string) string {
	u, err := url.Parse(strings.TrimSpace(apiBase))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "https://www.remilia.net"
	}
	return u.Scheme + "://" + u.Host
}

// absoluteURL resolves a media or bare URL against the site origin.
// Absolute http(s) refs are returned unchanged; relative paths (e.g.
// /imgproxy/…) become https://host/imgproxy/….
func absoluteURL(apiBase, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "http://") {
		return ref
	}
	base, err := url.Parse(siteOrigin(apiBase) + "/")
	if err != nil {
		return ref
	}
	rel, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return base.ResolveReference(rel).String()
}

const (
	previewHintVideo    = "video — o to open in browser"
	previewHintAnimated = "animated · first frame"
)

// previewTarget picks the first still-previewable attachment.
// Video is skipped when a later still exists; a video-only message returns
// the video URL with ok=false so the overlay can show a hint.
func previewTarget(apiBase string, msg api.Message) (href, hint string, ok bool) {
	var videoHref string
	for _, med := range msg.Media {
		u := absoluteURL(apiBase, med.ThumbnailURL)
		if u == "" {
			u = absoluteURL(apiBase, med.URL)
		}
		if u == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(med.Kind), "video") {
			if videoHref == "" {
				videoHref = u
			}
			continue
		}
		if med.IsAnimated {
			return u, previewHintAnimated, true
		}
		return u, "", true
	}
	if videoHref != "" {
		return videoHref, previewHintVideo, false
	}
	return "", "", false
}

// firstOpenURL returns the best absolute URL to open for a message (media first,
// then a bare http(s) token in the text).
func firstOpenURL(apiBase string, msg api.Message) string {
	for _, med := range msg.Media {
		if u := absoluteURL(apiBase, med.URL); u != "" {
			return u
		}
		if u := absoluteURL(apiBase, med.ThumbnailURL); u != "" {
			return u
		}
	}
	for _, part := range strings.Fields(msg.Text) {
		if strings.HasPrefix(part, "https://") || strings.HasPrefix(part, "http://") {
			return strings.TrimRight(part, ".,);]>\"'")
		}
	}
	return ""
}

// mediaBadge is the compact on-screen label for an attachment.
// Example: "[image ↗]". The URL is not shown; open with `o` or click the link.
func mediaBadge(kind string) string {
	if kind == "" {
		kind = "media"
	}
	return fmt.Sprintf("[%s %s]", kind, externalLinkMark)
}
