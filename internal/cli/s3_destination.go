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

	// Plain bucket format: "bucket" or "bucket/prefix/path/"
	bucket, prefix, _ = strings.Cut(dest, "/")
	return bucket, prefix, "", nil
}

// parseS3URL parses https://<bucket>.s3.<region>.amazonaws.com/<prefix>
func parseS3URL(rawURL string) (bucket, prefix, region string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid S3 URL: %w", err)
	}

	host := u.Hostname()

	// Expected format: <bucket>.s3.<region>.amazonaws.com
	s3Idx := strings.Index(host, ".s3.")
	if s3Idx < 0 || !strings.HasSuffix(host, ".amazonaws.com") {
		return "", "", "", fmt.Errorf("not a valid S3 URL (expected <bucket>.s3.<region>.amazonaws.com): %s", rawURL)
	}

	bucket = host[:s3Idx]
	// Between ".s3." and ".amazonaws.com"
	regionPart := host[s3Idx+4 : len(host)-len(".amazonaws.com")]
	region = regionPart

	prefix = strings.TrimPrefix(u.Path, "/")

	return bucket, prefix, region, nil
}
