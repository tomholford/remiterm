package cmd

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/oauth2"

	"remiterm/internal/api"
	"remiterm/internal/auth"
	"remiterm/internal/config"
)

func oauthCfg(cfg *config.Config) auth.OAuthConfig {
	return auth.OAuthConfig{
		ClientID:     cfg.ClientID,
		Issuer:       cfg.Issuer,
		CallbackPort: cfg.CallbackPort,
	}
}

// loadAPIClient loads tokens (refreshing when possible) and builds an API client.
func loadAPIClient(ctx context.Context, cfg *config.Config) (*api.Client, error) {
	src, _, err := loadTokenSource(ctx, cfg)
	if err != nil {
		return nil, err
	}
	// Fail fast if the source cannot mint a token.
	if _, err := auth.AccessToken(ctx, src); err != nil {
		return nil, fmt.Errorf("resolve access token: %w (try `remiterm login`)", err)
	}
	client := newAPIClient(cfg.APIBase, "")
	client.HTTPClient = oauth2.NewClient(ctx, src)
	client.Token = ""
	return client, nil
}

func newAPIClient(baseURL, token string) *api.Client {
	c := api.New(baseURL, token)
	c.UserAgent = config.AppName + "/" + Version
	return c
}

func loadTokenSource(ctx context.Context, cfg *config.Config) (oauth2.TokenSource, auth.Store, error) {
	store := auth.Store{FilePath: cfg.TokensPath()}
	tok, err := store.Load()
	if err != nil {
		if errors.Is(err, auth.ErrNotFound) {
			return nil, store, fmt.Errorf("no token: run `remiterm login` or `remiterm auth set-token`")
		}
		return nil, store, err
	}
	src, err := store.TokenSource(ctx, oauthCfg(cfg), tok)
	if err != nil {
		return nil, store, err
	}
	return src, store, nil
}
