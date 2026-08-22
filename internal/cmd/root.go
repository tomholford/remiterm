package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"remiterm/internal/config"
)

// Version is set at build time via -ldflags when desired.
var Version = "dev"

// Execute runs the root command.
func Execute() {
	if err := NewRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// NewRoot builds the cobra command tree.
func NewRoot() *cobra.Command {
	var cfg *config.Config

	root := &cobra.Command{
		Use:           config.AppName,
		Short:         "RemiliaNET global chat in your terminal",
		Long:          "remiterm is a TUI client for RemiliaNET global chat over the public API.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Skip heavy load for bare help/version.
			if cmd.Name() == "help" || cmd.Name() == "completion" || cmd.Name() == "version" {
				return nil
			}
			c, err := config.Load()
			if err != nil {
				return err
			}
			cfg = c
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(cfg)
		},
	}

	root.Version = Version
	root.SetVersionTemplate("{{.Version}}\n")

	root.AddCommand(newVersionCmd())
	root.AddCommand(newAuthCmd(func() *config.Config { return cfg }))
	root.AddCommand(newWhoamiCmd(func() *config.Config { return cfg }))
	root.AddCommand(newLoginCmd(func() *config.Config { return cfg }))
	root.AddCommand(newLogoutCmd(func() *config.Config { return cfg }))
	root.AddCommand(newDemoCmd())

	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), Version)
			return err
		},
	}
}
