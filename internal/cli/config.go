package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Configure API credentials",
	}

	cmd.AddCommand(newConfigFlickrCmd())
	cmd.AddCommand(newConfigGoogleCmd())
	return cmd
}

func newConfigFlickrCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "flickr",
		Short: "Set up Flickr API credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			reader := bufio.NewReader(os.Stdin)

			fmt.Println("Flickr API Setup")
			fmt.Println("Get your API key at: https://www.flickr.com/services/apps/create/")
			fmt.Println()

			fmt.Print("API Key: ")
			apiKey, _ := reader.ReadString('\n')
			apiKey = strings.TrimSpace(apiKey)

			fmt.Print("API Secret: ")
			apiSecret, _ := reader.ReadString('\n')
			apiSecret = strings.TrimSpace(apiSecret)

			if apiKey == "" || apiSecret == "" {
				return fmt.Errorf("API key and secret are required")
			}

			cfg := &config.FlickrConfig{
				APIKey:    apiKey,
				APISecret: apiSecret,
			}

			configDir := config.DefaultDir()
			if err := config.SaveFlickrConfig(configDir, cfg); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Printf("\nFlickr credentials saved to %s\n", configDir)
			return nil
		},
	}
}

func newConfigGoogleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "google",
		Short: "Set up Google OAuth credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			reader := bufio.NewReader(os.Stdin)

			fmt.Println("Google OAuth Setup")
			fmt.Println("Create credentials at: https://console.cloud.google.com/apis/credentials")
			fmt.Println("Enable the Photos Library API at: https://console.cloud.google.com/apis/library/photoslibrary.googleapis.com")
			fmt.Println()

			fmt.Print("Client ID: ")
			clientID, _ := reader.ReadString('\n')
			clientID = strings.TrimSpace(clientID)

			fmt.Print("Client Secret: ")
			clientSecret, _ := reader.ReadString('\n')
			clientSecret = strings.TrimSpace(clientSecret)

			if clientID == "" || clientSecret == "" {
				return fmt.Errorf("client ID and secret are required")
			}

			cfg := &config.GoogleConfig{
				ClientID:     clientID,
				ClientSecret: clientSecret,
			}

			configDir := config.DefaultDir()
			if err := config.SaveGoogleConfig(configDir, cfg); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Printf("\nGoogle credentials saved to %s\n", configDir)
			return nil
		},
	}
}
