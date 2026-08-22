package tui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// previewHTTPClient is a plain client on purpose: the API client's oauth2
// transport would attach a Bearer token to attacker-controlled media URLs.
var previewHTTPClient = &http.Client{Timeout: 15 * time.Second}

const previewUserAgent = "remiterm/dev"

// fetchPreview GETs url with a size cap. Only http(s) is allowed; no
// Authorization header is sent.
func fetchPreview(ctx context.Context, rawURL string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = previewMaxBytes
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("unsupported media url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", previewUserAgent)

	res, err := previewHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch media: HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("image exceeds %d byte limit", maxBytes)
	}
	return data, nil
}
