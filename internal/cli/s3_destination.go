package cli

import (
	"fmt"
	"net/url"
	"strings"
)

// parseS3Destination parses an S3 destination string into bucket, prefix, and
// optional region. It accepts two formats:
//   - Plain bucket: "my-bucket" or "my-bucket/prefix/path/"
//   - S3 URL: "https://<bucket>.s3.<region>.amazonaws.com/<prefix>"
func parseS3Destination(dest string) (bucket, prefix, region string, err error) {
	if dest == "" {
		return "", "", "", fmt.Errorf("S3 destination cannot be empty")
	}

	if strings.HasPrefix(dest, "https://") || strings.HasPrefix(dest, "http://") {
		return parseS3URL(dest)
	}

	// s3://bucket/prefix format — strip scheme and parse as plain bucket
	dest = strings.TrimPrefix(dest, "s3://")

	// Plain bucket format: "bucket" or "bucket/prefix/path/"
	bucket, prefix, _ = strings.Cut(dest, "/")
	if bucket == "" {
		return "", "", "", fmt.Errorf("S3 bucket name cannot be empty")
	}
	return bucket, prefix, "", nil
}

// parseS3URL parses https://<bucket>.s3.<region>.amazonaws.com/<prefix>
func parseS3URL(rawURL string) (bucket, prefix, region string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid S3 URL: %w", err)
	}

	host := u.Hostname()

	// Expected formats:
	//   <bucket>.s3.<region>.amazonaws.com  (regional)
	//   <bucket>.s3.amazonaws.com           (us-east-1 default)
	s3Idx := strings.Index(host, ".s3.")
	if s3Idx < 0 || !strings.HasSuffix(host, ".amazonaws.com") {
		return "", "", "", fmt.Errorf("not a valid S3 URL (expected <bucket>.s3.<region>.amazonaws.com): %s", rawURL)
	}

	bucket = host[:s3Idx]
	if bucket == "" {
		return "", "", "", fmt.Errorf("S3 URL has empty bucket name: %s", rawURL)
	}
	// Extract region from between ".s3." and ".amazonaws.com".
	// Regionless URLs like <bucket>.s3.amazonaws.com imply us-east-1.
	afterS3 := host[s3Idx+4:] // e.g. "us-west-2.amazonaws.com" or "amazonaws.com"
	if afterS3 == "amazonaws.com" {
		region = "us-east-1"
	} else {
		region = strings.TrimSuffix(afterS3, ".amazonaws.com")
	}

	prefix = strings.TrimPrefix(u.Path, "/")

	return bucket, prefix, region, nil
}
