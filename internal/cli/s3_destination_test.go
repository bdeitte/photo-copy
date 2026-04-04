package cli

import "testing"

func TestParseS3Destination_PlainBucket(t *testing.T) {
	bucket, prefix, region, err := parseS3Destination("my-bucket")
	if err != nil {
		t.Fatal(err)
	}
	if bucket != "my-bucket" {
		t.Errorf("bucket = %q, want %q", bucket, "my-bucket")
	}
	if prefix != "" {
		t.Errorf("prefix = %q, want empty", prefix)
	}
	if region != "" {
		t.Errorf("region = %q, want empty", region)
	}
}

func TestParseS3Destination_PlainBucketWithPath(t *testing.T) {
	bucket, prefix, region, err := parseS3Destination("my-bucket/photos/2024/")
	if err != nil {
		t.Fatal(err)
	}
	if bucket != "my-bucket" {
		t.Errorf("bucket = %q, want %q", bucket, "my-bucket")
	}
	if prefix != "photos/2024/" {
		t.Errorf("prefix = %q, want %q", prefix, "photos/2024/")
	}
	if region != "" {
		t.Errorf("region = %q, want empty", region)
	}
}

func TestParseS3Destination_URL(t *testing.T) {
	bucket, prefix, region, err := parseS3Destination("https://deitte-backup-things.s3.us-west-2.amazonaws.com/deitte-com/")
	if err != nil {
		t.Fatal(err)
	}
	if bucket != "deitte-backup-things" {
		t.Errorf("bucket = %q, want %q", bucket, "deitte-backup-things")
	}
	if prefix != "deitte-com/" {
		t.Errorf("prefix = %q, want %q", prefix, "deitte-com/")
	}
	if region != "us-west-2" {
		t.Errorf("region = %q, want %q", region, "us-west-2")
	}
}

func TestParseS3Destination_URLNoPath(t *testing.T) {
	bucket, prefix, region, err := parseS3Destination("https://my-bucket.s3.eu-west-1.amazonaws.com/")
	if err != nil {
		t.Fatal(err)
	}
	if bucket != "my-bucket" {
		t.Errorf("bucket = %q, want %q", bucket, "my-bucket")
	}
	if prefix != "" {
		t.Errorf("prefix = %q, want empty", prefix)
	}
	if region != "eu-west-1" {
		t.Errorf("region = %q, want %q", region, "eu-west-1")
	}
}

func TestParseS3Destination_URLDeepPath(t *testing.T) {
	bucket, prefix, region, err := parseS3Destination("https://my-bucket.s3.us-east-1.amazonaws.com/a/b/c/")
	if err != nil {
		t.Fatal(err)
	}
	if bucket != "my-bucket" {
		t.Errorf("bucket = %q, want %q", bucket, "my-bucket")
	}
	if prefix != "a/b/c/" {
		t.Errorf("prefix = %q, want %q", prefix, "a/b/c/")
	}
	if region != "us-east-1" {
		t.Errorf("region = %q, want %q", region, "us-east-1")
	}
}

func TestParseS3Destination_InvalidURL(t *testing.T) {
	_, _, _, err := parseS3Destination("https://not-an-s3-url.example.com/path")
	if err == nil {
		t.Fatal("expected error for non-S3 URL")
	}
}

func TestParseS3Destination_EmptyString(t *testing.T) {
	_, _, _, err := parseS3Destination("")
	if err == nil {
		t.Fatal("expected error for empty string")
	}
}
