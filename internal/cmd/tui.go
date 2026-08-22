package cmd

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/oauth2"

	"remiterm/internal/auth"
	"remiterm/internal/cache"
	"remiterm/internal/config"
	"remiterm/internal/tui"
)

func runTUI(cfg *config.Config) error {
	ctx := context.Background()
	src, _, err := loadTokenSource(ctx, cfg)
	if err != nil {
		return err
	}

	// Fail fast before alt-screen if the token is unusable.
	if _, err := auth.AccessToken(ctx, src); err != nil {
		return fmt.Errorf("resolve access token: %w (try `remiterm login`)", err)
	}

	client := newAPIClient(cfg.APIBase, "")
	client.HTTPClient = oauth2.NewClient(ctx, src)
	// oauth2 transport injects Authorization; leave Token empty.
	client.Token = ""

	model := tui.New(client, cfg.PollInterval)
	model.AttachSettings(settingsFromConfig(cfg), persistSettings(cfg))
	if db, err := cache.Open(cfg.CachePath()); err == nil {
		defer func() { _ = db.Close() }()
		model.AttachCache(db)
	}

	// Alt screen + mouse mode are declared on Model.View (Charm v2).
	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}

func settingsFromConfig(cfg *config.Config) tui.Settings {
	s := cfg.Settings()
	return tui.Settings{
		AuthorLabel:     s.AuthorLabel,
		TimestampFormat: s.TimestampFormat,
		Theme:           s.Theme,
		PollInterval:    s.PollInterval,
	}
}

func persistSettings(cfg *config.Config) tui.PersistFunc {
	return func(s tui.Settings) error {
		return cfg.SaveSettings(config.Settings{
			AuthorLabel:     s.AuthorLabel,
			TimestampFormat: s.TimestampFormat,
			Theme:           s.Theme,
			PollInterval:    s.PollInterval,
		})
	}
}
