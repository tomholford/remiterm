package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"remiterm/internal/config"
)

func newWhoamiCmd(cfgFn func() *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Print the authenticated user (GET /me)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := cfgFn()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			client, err := loadAPIClient(ctx, cfg)
			if err != nil {
				return err
			}

			me, err := client.Me(ctx)
			if err != nil {
				return err
			}
			handle := me.Handle()
			if handle == "" {
				fmt.Println("authenticated (profile fields empty or unexpected shape)")
				return nil
			}
			if me.User.DisplayName != "" && me.User.DisplayName != handle {
				fmt.Printf("%s (%s)\n", handle, me.User.DisplayName)
			} else {
				fmt.Println(handle)
			}
			return nil
		},
	}
}
