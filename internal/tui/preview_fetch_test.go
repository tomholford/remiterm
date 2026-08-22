package tui

import (
	"context"
	"image/color"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchPreviewOK(t *testing.T) {
	t.Parallel()
	raw := solidPNG(t, color.White)
	var sawAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			sawAuth = true
		}
		if r.Header.Get("User-Agent") != previewUserAgent {
			t.Errorf("ua %q", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(raw)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got, err := fetchPreview(ctx, srv.URL+"/img.png", previewMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	if sawAuth {
		t.Fatal("must not send Authorization")
	}
	if len(got) != len(raw) {
		t.Fatalf("len %d want %d", len(got), len(raw))
	}
}

func TestFetchPreviewHTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := fetchPreview(ctx, srv.URL+"/missing", previewMaxBytes)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("err=%v", err)
	}
}

func TestFetchPreviewOversize(t *testing.T) {
	t.Parallel()
	raw := solidPNG(t, color.White)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(raw)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := fetchPreview(ctx, srv.URL, 8)
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("err=%v", err)
	}
}

func TestFetchPreviewRejectsNonHTTP(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, u := range []string{"file:///etc/passwd", "not-a-url", "", "/imgproxy/x"} {
		if _, err := fetchPreview(ctx, u, previewMaxBytes); err == nil {
			t.Fatalf("expected reject %q", u)
		}
	}
}
