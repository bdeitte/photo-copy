package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/briandeitte/photo-copy/internal/daterange"
	"github.com/spf13/cobra"
)

type rootOpts struct {
	debug           bool
	limit           int
	noMetadata      bool
	dateRangeStr    string
	parsedDateRange *daterange.DateRange
}

func NewRootCmd() *cobra.Command {
	opts := &rootOpts{}

	rootCmd := &cobra.Command{
		Use:   "photo-copy",
		Short: "Copy photos and videos between iCloud Photos, Flickr, Google Photos, S3, and local directories",
	}

	rootCmd.PersistentFlags().BoolVar(&opts.debug, "debug", false, "Enable verbose debug logging to stderr")
	rootCmd.PersistentFlags().IntVar(&opts.limit, "limit", 0, "Maximum number of files to upload/download (0 = no limit)")
	rootCmd.PersistentFlags().BoolVar(&opts.noMetadata, "no-metadata", false, "Skip metadata embedding during downloads (XMP, MP4 creation time, timestamps)")
	rootCmd.PersistentFlags().StringVar(&opts.dateRangeStr, "date-range", "", "Filter by date range (YYYY-MM-DD:YYYY-MM-DD, either side optional). For S3, filters by file modification time. For all others, filters by embedded metadata dates.")

	// NOTE: Cobra silently overrides a parent's PersistentPreRunE if a child
	// defines its own. If a child command needs PersistentPreRunE, it must
	// call this parent hook explicitly to preserve date-range parsing and
	// no-op warning behavior.
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if opts.dateRangeStr != "" {
			dr, err := daterange.Parse(opts.dateRangeStr)
			if err != nil {
				return fmt.Errorf("invalid --date-range: %w", err)
			}
			opts.parsedDateRange = dr
		}

		// No-op warnings — use command annotations instead of name matching
		errW := cmd.ErrOrStderr()
		if opts.noMetadata && cmd.Annotations["supportsMetadata"] != "true" {
			_, _ = fmt.Fprintln(errW, "Warning: --no-metadata has no effect on "+cmd.CommandPath()+"; metadata embedding only occurs during downloads")
		}

		if opts.parsedDateRange != nil && cmd.Annotations["supportsDateRange"] != "true" {
			_, _ = fmt.Fprintf(errW, "Warning: --date-range has no effect on %s\n", cmd.CommandPath())
			opts.parsedDateRange = nil
		}

		return nil
	}

	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	cobra.EnableCommandSorting = false
	rootCmd.AddCommand(newConfigCmd())
	rootCmd.AddCommand(newFlickrCmd(opts))
	rootCmd.AddCommand(newGooglePhotosCmd(opts))
	rootCmd.AddCommand(newS3Cmd(opts))
	rootCmd.AddCommand(newICloudCmd(opts))
	rootCmd.InitDefaultHelpCmd()
	// Move help command to end by removing and re-adding it
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "help" {
			rootCmd.RemoveCommand(cmd)
			rootCmd.AddCommand(cmd)
			break
		}
	}

	return rootCmd
}

func Execute() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	rootCmd := NewRootCmd()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		stop()
		// NOTE: Depends on cobra's internal error message format for unknown commands.
		if strings.Contains(err.Error(), "unknown command") {
			printAvailableCommands(os.Stderr, rootCmd)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	stop()
}

// exactArgs returns a cobra.PositionalArgs that requires exactly n arguments,
// with a user-friendly error message that shows usage.
func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < n {
			return fmt.Errorf("requires %d argument(s)\n\nUsage:\n  %s\n\nRun '%s --help' for more information",
				n, cmd.UseLine(), cmd.CommandPath())
		}
		if len(args) > n {
			return fmt.Errorf("accepts %d argument(s), received %d\n\nUsage:\n  %s\n\nRun '%s --help' for more information",
				n, len(args), cmd.UseLine(), cmd.CommandPath())
		}
		return nil
	}
}

// printAvailableCommands writes the list of available commands to w.
func printAvailableCommands(w io.Writer, cmd *cobra.Command) {
	_, _ = fmt.Fprintln(w, "Available commands:")
	for _, c := range cmd.Commands() {
		if c.IsAvailableCommand() {
			_, _ = fmt.Fprintf(w, "  %-20s %s\n", c.Name(), c.Short)
		}
	}
	_, _ = fmt.Fprintln(w)
}
