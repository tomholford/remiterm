package cmd

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"remiterm/internal/demo"
	"remiterm/internal/tui"
)

func newDemoCmd() *cobra.Command {
	var (
		latency  time.Duration
		pages    int
		pageSize int
		poll     time.Duration
		verbose  bool
	)

	cmd := &cobra.Command{
		Use:   "demo",
		Short: "Run the TUI against a local fake API (no login, no network)",
		Long: `Start an in-process synthetic RemiliaNET API and open the chat TUI against it.

Useful for iterating on scroll/history UX without hitting production or OAuth.

Request logs are off by default (they corrupt the TUI). Use --verbose to print
each ListGlobalChat/Post to stderr.

Example:
  remiterm demo --latency 400ms
  remiterm demo --latency 200ms --pages 20 --page-size 50 --poll 1h
  remiterm demo --verbose 2>demo.log`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := demo.Options{
				Pages:    pages,
				PageSize: pageSize,
				Latency:  latency,
			}
			srv := demo.NewServer(opts)
			if !verbose {
				srv.SetLogger(nil)
			}
			if err := srv.Listen(); err != nil {
				return fmt.Errorf("demo server: %w", err)
			}
			defer func() { _ = srv.Close() }()

			// Flag zeros mean "use package defaults" after normalize.
			totalPages, totalSize := pages, pageSize
			if totalPages <= 0 {
				totalPages = 100
			}
			if totalSize <= 0 {
				totalSize = 100
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
				"demo API %s  pages=%d×%d latency=%s poll=%s (ctrl+c quit)\n",
				srv.URL(), totalPages, totalSize, latency, poll)

			client := newAPIClient(srv.URL(), "")
			model := tui.New(client, poll)
			p := tea.NewProgram(model)
			if _, err := p.Run(); err != nil {
				return fmt.Errorf("tui: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().DurationVar(&latency, "latency", 300*time.Millisecond, "artificial delay per API request")
	cmd.Flags().IntVar(&pages, "pages", 100, "number of synthetic history pages")
	cmd.Flags().IntVar(&pageSize, "page-size", 100, "messages per page (max 100)")
	cmd.Flags().DurationVar(&poll, "poll", time.Hour, "TUI live-poll interval (high = quiet while testing backscroll)")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "log each demo API request to stderr (corrupts TUI; redirect 2>file)")

	return cmd
}
