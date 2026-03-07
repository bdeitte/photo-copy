package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var debug bool

func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "photo-copy",
		Short: "Copy photos and videos between Flickr, Google Photos, S3, and local directories",
	}

	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable verbose debug logging to stderr")
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	cobra.EnableCommandSorting = false
	rootCmd.AddCommand(newConfigCmd())
	rootCmd.AddCommand(newFlickrCmd())
	rootCmd.AddCommand(newGooglePhotosCmd())
	rootCmd.AddCommand(newS3Cmd())
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
