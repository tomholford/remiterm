package auth

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/oauth2"
)

// TokenSource returns an oauth2.TokenSource that refreshes via the issuer and
// persists rotated tokens back to the store.
func (s Store) TokenSource(ctx context.Context, cfg OAuthConfig, tok Tokens) (oauth2.TokenSource, error) {
	if !tok.Valid() {
		return nil, ErrNotFound
	}
	// Env-only tokens (no refresh) — static.
	if tok.RefreshToken == "" {
		return oauth2.StaticTokenSource(tok.ToOAuth2()), nil
	}
	if cfg.ClientID == "" {
		// Can still use access token until it expires; no refresh without client_id.
		return oauth2.StaticTokenSource(tok.ToOAuth2()), nil
	}
	o2, err := cfg.oauth2Config(ctx)
	if err != nil {
		return nil, err
	}
	base := o2.TokenSource(ctx, tok.ToOAuth2())
	return &persistingSource{
		inner: base,
		store: s,
	}, nil
}

// persistingSource wraps a TokenSource and writes back when the access token changes.
type persistingSource struct {
	inner oauth2.TokenSource
	store Store

	mu   sync.Mutex
	last string
}

func (p *persistingSource) Token() (*oauth2.Token, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	t, err := p.inner.Token()
	if err != nil {
		return nil, fmt.Errorf("refresh token: %w", err)
	}
	if t.AccessToken != "" && t.AccessToken != p.last {
		p.last = t.AccessToken
		if err := p.store.Save(FromOAuth2(t)); err != nil {
			// Still return the token; persistence failure is non-fatal for this request.
			_ = err
		}
	}
	return t, nil
}

// AccessToken resolves a fresh access token string.
func AccessToken(ctx context.Context, src oauth2.TokenSource) (string, error) {
	t, err := src.Token()
	if err != nil {
		return "", err
	}
	if t.AccessToken == "" {
		return "", ErrNotFound
	}
	return t.AccessToken, nil
}
