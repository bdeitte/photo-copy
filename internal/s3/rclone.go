package s3

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func rcloneBinaryName(goos, goarch string) string {
	name := fmt.Sprintf("rclone-%s-%s", goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

func findRcloneBinary(binDir string) (string, error) {
	name := rcloneBinaryName(runtime.GOOS, runtime.GOARCH)
	path := filepath.Join(binDir, name)

	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("rclone binary not found at %s: %w", path, err)
	}

	return path, nil
}

func rcloneBinDir() (string, error) {
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Join(filepath.Dir(exe), "tools-bin", "rclone")
		if _, err := os.Stat(dir); err == nil {
			return dir, nil
		}
	}

	cwd, err := os.Getwd()
	if err == nil {
		dir := filepath.Join(cwd, "tools-bin", "rclone")
		if _, err := os.Stat(dir); err == nil {
			return dir, nil
		}
	}

	return "", fmt.Errorf("tools-bin/rclone directory not found (checked next to executable and current directory). Run ./tools-bin/rclone/update.sh to download rclone binaries")
}

func writeRcloneConfig(accessKeyID, secretAccessKey, region string) (string, error) {
	content := fmt.Sprintf("[s3]\ntype = s3\nprovider = AWS\naccess_key_id = %s\nsecret_access_key = %s\nregion = %s\n",
		accessKeyID, secretAccessKey, region)

	f, err := os.CreateTemp("", "rclone-config-*.conf")
	if err != nil {
		return "", fmt.Errorf("creating temp config: %w", err)
	}

	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("writing config: %w", err)
	}

	_ = f.Close()
	return f.Name(), nil
}
