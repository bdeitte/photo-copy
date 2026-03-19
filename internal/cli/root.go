package cli

import (
	"fmt"
	"os"
	"strings"

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
		Short: "Copy photos and videos between Flickr, Google Photos, S3, and local directories",
	}

	rootCmd.PersistentFlags().BoolVar(&opts.debug, "debug", false, "Enable verbose debug logging to stderr")
	rootCmd.PersistentFlags().IntVar(&opts.limit, "limit", 0, "Maximum number of files to upload/download (0 = no limit)")
	rootCmd.PersistentFlags().BoolVar(&opts.noMetadata, "no-metadata", false, "Skip metadata embedding during Flickr downloads (XMP, MP4 creation time, timestamps)")
	rootCmd.PersistentFlags().StringVar(&opts.dateRangeStr, "date-range", "", "Filter by date range (YYYY-MM-DD:YYYY-MM-DD, either side optional). For S3, filters by file modification time via rclone, not embedded metadata dates.")

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		path := cmd.CommandPath()

		if opts.dateRangeStr != "" {
			dr, err := daterange.Parse(opts.dateRangeStr)
			if err != nil {
				return fmt.Errorf("invalid --date-range: %w", err)
			}
			opts.parsedDateRange = dr
		}

		// No-op warnings
		if opts.noMetadata {
			if !strings.Contains(path, "flickr download") {
				fmt.Fprintln(os.Stderr, "Warning: --no-metadata has no effect on "+cmd.Name()+"; metadata embedding only occurs during Flickr downloads")
			}
		}

		if opts.parsedDateRange != nil {
			if strings.Contains(path, "import-takeout") {
				fmt.Fprintln(os.Stderr, "Warning: --date-range has no effect on import-takeout")
				opts.parsedDateRange = nil
			} else if strings.Contains(path, "config") {
				fmt.Fprintln(os.Stderr, "Warning: --date-range has no effect on config commands")
				opts.parsedDateRange = nil
			}
		}

		return nil
	}

	rootCmd.CompletionOptions.DisableDefaultCmd = true

	cobra.EnableCommandSorting = false
	rootCmd.AddCommand(newConfigCmd())
	rootCmd.AddCommand(newFlickrCmd(opts))
	rootCmd.AddCommand(newGooglePhotosCmd(opts))
	rootCmd.AddCommand(newS3Cmd(opts))
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
	rootCmd := NewRootCmd()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
