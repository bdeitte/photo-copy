package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

type rootOpts struct {
	debug bool
	limit int
}

func NewRootCmd() *cobra.Command {
	opts := &rootOpts{}

	rootCmd := &cobra.Command{
		Use:   "photo-copy",
		Short: "Copy photos and videos between Flickr, Google Photos, S3, and local directories",
	}

	rootCmd.PersistentFlags().BoolVar(&opts.debug, "debug", false, "Enable verbose debug logging to stderr")
	rootCmd.PersistentFlags().IntVar(&opts.limit, "limit", 0, "Maximum number of files to upload/download (0 = no limit)")
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
