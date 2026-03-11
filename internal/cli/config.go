package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/flickr"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Configure API credentials",
	}

	cmd.AddCommand(newConfigFlickrCmd())
	cmd.AddCommand(newConfigGoogleCmd())
	cmd.AddCommand(newConfigS3Cmd())
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

			fmt.Println("\nStarting OAuth authorization...")
			reqToken, reqSecret, authURL, err := flickr.GetRequestToken(cfg)
			if err != nil {
				return fmt.Errorf("getting request token: %w", err)
			}

			fmt.Printf("\nOpen this URL in your browser:\n%s\n\n", authURL)
			fmt.Print("Enter the verification code: ")
			verifier, _ := reader.ReadString('\n')
			verifier = strings.TrimSpace(verifier)

			accessToken, accessSecret, err := flickr.ExchangeToken(cfg, reqToken, reqSecret, verifier)
			if err != nil {
				return fmt.Errorf("exchanging token: %w", err)
			}

			cfg.OAuthToken = accessToken
			cfg.OAuthTokenSecret = accessSecret

			if err := config.SaveFlickrConfig(configDir, cfg); err != nil {
				return fmt.Errorf("saving config with tokens: %w", err)
			}

			fmt.Println("Flickr OAuth complete! Credentials saved.")
			return nil
		},
	}
}

func newConfigS3Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "s3",
		Short: "Set up S3 credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			reader := bufio.NewReader(os.Stdin)
			configDir := config.DefaultDir()

			fmt.Println("S3 Credential Setup")
			fmt.Println()

			home, _ := os.UserHomeDir()
			awsCredsPath := filepath.Join(home, ".aws", "credentials")
			if _, err := os.Stat(awsCredsPath); err == nil {
				fmt.Print("Found existing AWS credentials at ~/.aws/credentials. Use these? (y/n): ")
				answer, _ := reader.ReadString('\n')
				answer = strings.TrimSpace(strings.ToLower(answer))

				if answer == "y" || answer == "yes" {
					cfg, err := readAWSCredentials(awsCredsPath)
					if err != nil {
						fmt.Printf("Warning: could not read AWS credentials: %v\n", err)
						fmt.Println("Falling back to manual entry.")
					} else {
						fmt.Print("AWS Region (e.g., us-east-1): ")
						region, _ := reader.ReadString('\n')
						region = strings.TrimSpace(region)
						if region == "" {
							region = "us-east-1"
						}
						cfg.Region = region

						if err := config.SaveS3Config(configDir, cfg); err != nil {
							return fmt.Errorf("saving config: %w", err)
						}
						fmt.Printf("\nS3 credentials saved to %s\n", configDir)
						return nil
					}
				}
			}

			fmt.Print("AWS Access Key ID: ")
			accessKey, _ := reader.ReadString('\n')
			accessKey = strings.TrimSpace(accessKey)

			fmt.Print("AWS Secret Access Key: ")
			secretKey, _ := reader.ReadString('\n')
			secretKey = strings.TrimSpace(secretKey)

			fmt.Print("AWS Region (e.g., us-east-1): ")
			region, _ := reader.ReadString('\n')
			region = strings.TrimSpace(region)
			if region == "" {
				region = "us-east-1"
			}

			if accessKey == "" || secretKey == "" {
				return fmt.Errorf("access key and secret key are required")
			}

			cfg := &config.S3Config{
				AccessKeyID:     accessKey,
				SecretAccessKey: secretKey,
				Region:          region,
			}

			if err := config.SaveS3Config(configDir, cfg); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Printf("\nS3 credentials saved to %s\n", configDir)
			return nil
		},
	}
}

func readAWSCredentials(path string) (*config.S3Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &config.S3Config{}
	lines := strings.Split(string(data), "\n")
	inDefault := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "[default]" {
			inDefault = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			inDefault = false
			continue
		}
		if !inDefault {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "aws_access_key_id":
			cfg.AccessKeyID = val
		case "aws_secret_access_key":
			cfg.SecretAccessKey = val
		}
	}

	if cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, fmt.Errorf("could not find access key and secret in [default] profile")
	}

	return cfg, nil
}

func newConfigGoogleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "google",
		Short: "Set up Google OAuth credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			reader := bufio.NewReader(os.Stdin)

			fmt.Println("Google OAuth Setup")
			fmt.Println()
			fmt.Println("Steps:")
			fmt.Println("  1. Go to https://console.cloud.google.com/apis/credentials")
			fmt.Println("  2. Create a project if you don't have one")
			fmt.Println("  3. If needed, click to configure the OAuth consent screen")
			fmt.Println("  4. If you see 'Google Auth Platform not configured yet', click 'Get Started':")
			fmt.Println("     - Enter an app name (e.g. 'photo-copy') and your email")
			fmt.Println("     - For Audience, select 'External'")
			fmt.Println("     - Finish, then go to 'Audience' and add your Google email as a test user")
			fmt.Println("     - Go back to the Credentials page")
			fmt.Println("  5. Click '+ CREATE CREDENTIALS' > 'OAuth client ID'")
			fmt.Println("  6. Select 'Desktop app' as the application type, then click Create")
			fmt.Println("  7. Copy the Client ID and Client Secret shown in the dialog")
			fmt.Println("  8. Enable the Photos Library API at:")
			fmt.Println("     https://console.cloud.google.com/apis/library/photoslibrary.googleapis.com")
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
