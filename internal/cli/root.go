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

	rootCmd.AddCommand(newConfigCmd())

	return rootCmd
}

func Execute() {
	rootCmd := NewRootCmd()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
