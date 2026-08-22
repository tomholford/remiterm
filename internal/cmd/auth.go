package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"remiterm/internal/auth"
	"remiterm/internal/config"
)

func newAuthCmd(cfgFn func() *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage stored credentials",
	}
	cmd.AddCommand(newAuthSetTokenCmd(cfgFn))
	cmd.AddCommand(newAuthStatusCmd(cfgFn))
	cmd.AddCommand(newAuthClearCmd(cfgFn))
	return cmd
}

func newAuthSetTokenCmd(cfgFn func() *config.Config) *cobra.Command {
	var tokenFlag string
	var refreshFlag string

	cmd := &cobra.Command{
		Use:   "set-token",
		Short: "Store an access token (paste or --token)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := cfgFn()
			store := auth.Store{FilePath: cfg.TokensPath()}

			tok := strings.TrimSpace(tokenFlag)
			if tok == "" {
				tok = strings.TrimSpace(os.Getenv("REMILIA_ACCESS_TOKEN"))
			}
			if tok == "" {
				fmt.Fprint(os.Stderr, "Paste access token (input hidden): ")
				b, err := term.ReadPassword(int(syscall.Stdin))
				fmt.Fprintln(os.Stderr)
				if err != nil {
					// Fallback if not a TTY.
					reader := bufio.NewReader(os.Stdin)
					line, err2 := reader.ReadString('\n')
					if err2 != nil {
						return fmt.Errorf("read token: %w", err)
					}
					tok = strings.TrimSpace(line)
				} else {
					tok = strings.TrimSpace(string(b))
				}
			}
			if tok == "" {
				return fmt.Errorf("empty token")
			}

			t := auth.Tokens{
				AccessToken:  tok,
				RefreshToken: strings.TrimSpace(refreshFlag),
				TokenType:    "Bearer",
			}
			if err := store.Save(t); err != nil {
				return err
			}
			fmt.Printf("token stored (%s)\n", store.Backend())
			return nil
		},
	}
	cmd.Flags().StringVar(&tokenFlag, "token", "", "access token (prefer stdin for safety)")
	cmd.Flags().StringVar(&refreshFlag, "refresh-token", "", "optional refresh token")
	return cmd
}

func newAuthStatusCmd(cfgFn func() *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether a token is stored",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := cfgFn()
			store := auth.Store{FilePath: cfg.TokensPath()}
			backend := store.Backend()
			tok, err := store.Load()
			if errors.Is(err, auth.ErrNotFound) {
				fmt.Println("no token stored")
				fmt.Printf("backend: %s\n", backend)
				return nil
			}
			if err != nil {
				return err
			}
			fmt.Printf("token: present (%d chars)\n", len(tok.AccessToken))
			fmt.Printf("backend: %s\n", backend)
			if tok.Scope != "" {
				fmt.Printf("scope: %s\n", tok.Scope)
			}
			if !tok.ExpiresAt.IsZero() {
				fmt.Printf("expires_at: %s\n", tok.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z"))
			}
			if tok.RefreshToken != "" {
				fmt.Println("refresh_token: present")
			}
			return nil
		},
	}
}

func newAuthClearCmd(cfgFn func() *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Remove stored tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := cfgFn()
			store := auth.Store{FilePath: cfg.TokensPath()}
			if err := store.Clear(); err != nil {
				return err
			}
			fmt.Println("tokens cleared")
			return nil
		},
	}
}
