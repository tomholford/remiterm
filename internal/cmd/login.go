package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"remiterm/internal/auth"
	"remiterm/internal/config"
)

func newLoginCmd(cfgFn func() *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Sign in with RemiliaNET (OAuth Authorization Code + PKCE)",
		Long: `Opens a browser to sign in with RemiliaNET, then listens on the
loopback redirect URI for the authorization code.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := cfgFn()
			oc := oauthCfg(cfg)
			fmt.Printf("client_id:     %s\n", oc.ClientID)
			fmt.Printf("redirect_uri:  %s\n", oc.RedirectURI())
			iss := oc.Issuer
			if iss == "" {
				iss = auth.DefaultIssuer
			}
			fmt.Printf("issuer:        %s\n\n", iss)

			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()

			tok, err := auth.Login(ctx, oc)
			if err != nil {
				return fmt.Errorf("login: %w\n\nFallback: remiterm auth set-token", err)
			}

			store := auth.Store{FilePath: cfg.TokensPath()}
			if err := store.Save(tok); err != nil {
				return fmt.Errorf("store tokens: %w", err)
			}
			fmt.Printf("logged in (%s)\n", store.Backend())
			if tok.Scope != "" {
				fmt.Printf("scope: %s\n", tok.Scope)
			}

			// Best-effort whoami.
			client := newAPIClient(cfg.APIBase, tok.AccessToken)
			meCtx, meCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer meCancel()
			if me, err := client.Me(meCtx); err == nil && me.Handle() != "" {
				if me.User.DisplayName != "" && me.User.DisplayName != me.Handle() {
					fmt.Printf("user:  %s (%s)\n", me.Handle(), me.User.DisplayName)
				} else {
					fmt.Printf("user:  %s\n", me.Handle())
				}
			}
			return nil
		},
	}
}

func newLogoutCmd(cfgFn func() *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear local tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			return newAuthClearCmd(cfgFn).RunE(cmd, args)
		},
	}
}
