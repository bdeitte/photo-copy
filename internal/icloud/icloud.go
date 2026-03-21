package icloud

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/logging"
)

type Client struct {
	cfg *config.ICloudConfig
	log *logging.Logger
}

func NewClient(cfg *config.ICloudConfig, log *logging.Logger) *Client {
	return &Client{cfg: cfg, log: log}
}

func findTool(name, envVar string) (string, error) {
	if path := os.Getenv(envVar); path != "" {
		info, err := os.Stat(path)
		if err != nil {
			return "", fmt.Errorf("%s path from %s not found: %s", name, envVar, path)
		}
		if info.IsDir() || info.Mode()&0111 == 0 {
			return "", fmt.Errorf("%s path from %s is not executable: %s", name, envVar, path)
		}
		return path, nil
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not found. Install it with: pipx install %s", name, name)
	}
	return path, nil
}
