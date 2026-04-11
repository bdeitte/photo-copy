# Contributing to photo-copy

All contributions welcome!

## Architecture

See [CLAUDE.md](CLAUDE.md#architecture) for architecture details.

## Build

Requires Go 1.25+.

```bash
go build -o photo-copy ./cmd/photo-copy
```

## Linting and testing

Install golangci-lint ([installation options](https://golangci-lint.run/welcome/install/)):

```bash
# macOS
brew install golangci-lint

# or cross-platform
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

Run linting and unit tests:

```bash
golangci-lint run ./...
go test ./...
```

## Integration tests

Integration tests exercise CLI commands end-to-end against mock HTTP servers for Flickr and Google Photos. They use a build tag and don't run with `go test ./...`:

```bash
go test ./internal/cli/ -tags integration
```

S3 integration testing is not included — S3 operations delegate to rclone, and rclone's own test coverage handles that layer. S3 unit tests cover command arg building, config generation, and binary resolution.

## Updating bundled tools

Update all bundled tool binaries:

```bash
./tools-bin/update.sh
```

Update a specific tool:

```bash
./tools-bin/update.sh rclone v1.73.2
./tools-bin/update.sh icloudpd 1.32.2
./tools-bin/update.sh osxphotos 0.75.6    # macOS only
```

### Supported file types

**Uploads and S3 downloads:** JPEG, PNG, TIFF, GIF, HEIC, WebP, MP4, MOV, AVI, MKV

**Flickr and iCloud downloads:** All file types downloaded as-is from the service.

## License

photo-copy is licensed under the MIT license

