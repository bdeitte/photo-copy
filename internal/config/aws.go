package config

import (
	"fmt"
	"os"
	"strings"
)

// ReadAWSCredentials reads the [default] profile from an AWS credentials file.
func ReadAWSCredentials(path string) (*S3Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &S3Config{}
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
