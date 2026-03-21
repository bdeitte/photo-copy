package icloud

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

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

// FindTool locates a tool binary using this search order:
// 1. Environment variable override (for testing)
// 2. Bundled binary in tools-bin/{name}/ next to executable
// 3. Bundled binary in tools-bin/{name}/ in cwd
// 4. System PATH fallback
//
// On darwin/arm64, if the exact binary isn't found in tools-bin,
// tries the darwin-amd64 variant (runs via Rosetta 2).
func FindTool(name, envVar string, extraSearchDirs ...string) (string, error) {
	// 1. Env var override
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

	// 2-3. Check tools-bin/{name}/ in search dirs
	searchDirs := make([]string, 0, len(extraSearchDirs)+2)

	if exe, err := os.Executable(); err == nil {
		searchDirs = append(searchDirs, filepath.Dir(exe))
	}
	if cwd, err := os.Getwd(); err == nil {
		searchDirs = append(searchDirs, cwd)
	}
	searchDirs = append(searchDirs, extraSearchDirs...)

	binName := toolBinaryName(name, runtime.GOOS, runtime.GOARCH)
	for _, dir := range searchDirs {
		path := filepath.Join(dir, "tools-bin", name, binName)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}

		// Rosetta fallback: on darwin/arm64, try darwin-amd64
		if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
			fallback := toolBinaryName(name, "darwin", "amd64")
			path = filepath.Join(dir, "tools-bin", name, fallback)
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
	}

	// 4. System PATH fallback
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("%s not found in tools-bin/ or system PATH. Run ./tools-bin/%s/update.sh to download, or install manually: pipx install %s", name, name, name)
}

func toolBinaryName(name, goos, goarch string) string {
	binName := fmt.Sprintf("%s-%s-%s", name, goos, goarch)
	if goos == "windows" {
		binName += ".exe"
	}
	return binName
}
