# Test Coverage Improvement Plan

## Current State

**91 test functions** across 21 test files:
- 64 unit tests across 14 packages
- 27 integration tests in `internal/cli/integration_test.go`

### Well-Covered Areas
- Flickr OAuth signing, HTTP retry/backoff, transfer log I/O
- JPEG XMP metadata writing/reading, MP4 timestamp manipulation (V0/V1)
- Date range parsing and boundary checking
- Config file persistence and error handling
- Transfer result tracking, validation, and reporting
- Integration: Flickr download/upload happy paths, retries, resume, pagination, metadata embedding
- Integration: Google upload happy paths, resume, retries, partial failure
- Integration: Google Takeout import with media filtering
- Integration: Date range filtering on both download and upload
- Integration: `--limit` and `--no-metadata` flags

---

## Priority 1: High-Impact Gaps

### 1.1 S3 Integration Tests (0 tests today)

S3 is the only service with **zero integration tests**. The unit tests cover argument building but never exercise the actual rclone subprocess flow.

**Tests to add** (in `integration_test.go` or a new `s3_integration_test.go`):
- Since S3 uses a real rclone binary subprocess, integration tests require a different approach than Flickr/Google (which use mock HTTP servers). Options:
  - Mock rclone by placing a shell script that mimics rclone's behavior in the test PATH
  - Use a local S3-compatible server (e.g., MinIO in a container) if practical
  - At minimum, test `buildFilesFrom`, `countFiles`, and `runRcloneWithProgress` with a real rclone binary against a temp directory using the `--dry-run` flag
- Test `s3 upload` with `--limit`, `--date-range`, and media-only flags
- Test `s3 download` with prefix filtering
- Test error case: missing rclone binary

### 1.2 Video Metadata in Integration Tests

Integration tests only verify XMP embedding in **JPEGs**. No integration test exercises the MP4/MOV metadata path during Flickr download.

**Tests to add:**
- `TestFlickrDownload_VideoMetadata` — download an MP4, verify:
  - MP4 creation time set from `date_taken`
  - XMP UUID box written with title/description/tags
  - File modification time set correctly
- `TestFlickrDownload_VideoNoMetadata` — download MP4 with `--no-metadata`, verify no container changes

### 1.3 Flickr Upload Error Handling

Upload error paths are barely tested — only one test for HTTP 500, and it doesn't verify the result struct thoroughly.

**Tests to add:**
- `TestFlickrUpload_ConsecutiveFailureAbort` — verify the 10-consecutive-failure abort threshold triggers and reports correctly
- `TestFlickrUpload_PartialFailure` — mix of successes and failures, verify result counts and error list
- `TestFlickrUpload_EmptyDirectory` — verify graceful handling of no media files

### 1.4 Google Upload Daily Limit

The 10,000 upload/day limit is enforced in code but never tested.

**Tests to add:**
- `TestGoogleUpload_DailyLimitEnforced` — set up scenario where limit would be exceeded, verify it stops at 10,000 (or mock the limit to a smaller number for testing)

---

## Priority 2: Edge Cases and Error Conditions

### 2.1 Flickr Download Edge Cases

- `TestFlickrDownload_EmptyPhotoList` — API returns 0 photos, verify graceful exit with empty result
- `TestFlickrDownload_AllPhotosFilteredByDateRange` — all photos outside range, verify 0 downloads and correct skip count
- `TestFlickrDownload_PermanentDownloadFailure` — photo URL always returns 404 (no fallback available), verify error recorded and other photos continue
- `TestFlickrDownload_ContextCancellation` — cancel context mid-download, verify partial result returned cleanly

### 2.2 Google Takeout Edge Cases

- `TestGoogleImportTakeout_DuplicateFilenames` — zip contains two files with the same name (different directories), verify `_1` suffix applied
- `TestGoogleImportTakeout_NestedDirectories` — verify files from nested `Google Photos/Album/SubDir/` extracted correctly
- `TestGoogleImportTakeout_EmptyZip` — zip with no media files, verify graceful handling
- `TestGoogleImportTakeout_MixedZips` — one zip has media, another is empty

### 2.3 Transfer Log Edge Cases

- `TestTransferLog_ConcurrentAppends` — multiple goroutines appending simultaneously (race detection)
- `TestTransferLog_CorruptedFile` — log with invalid/binary content, verify graceful handling
- `TestTransferLog_VeryLargeLog` — log with 100k+ entries, verify performance is acceptable

### 2.4 Config Error Handling

- `TestLoadConfig_PartialJSON` — truncated JSON file (simulating interrupted write), verify clear error
- `TestSaveConfig_PermissionDenied` — read-only directory, verify error propagated
- `TestConfigDir_EnvOverride_InvalidPath` — env var points to non-existent path, verify dir is created

---

## Priority 3: Unit Test Gaps

### 3.1 Logging Package

- `TestErrorLogger_AlwaysOutputs` — verify Error() outputs regardless of debug flag
- `TestErrorLogger_HasPrefix` — verify "ERROR: " prefix in output
- `TestLogger_NilWriter` — verify defaults to stderr without panic

### 3.2 MediaDate Package

Currently only 2 tests. The EXIF and MP4 reading are tested in their own packages, but the fallback chain in `ResolveDate` deserves more coverage.

- `TestResolveDate_JPEG_WithExif` — JPEG with EXIF date returns EXIF date (not mtime)
- `TestResolveDate_MP4_WithCreationTime` — MP4 with creation time returns it
- `TestResolveDate_JPEG_NoExif_FallsBackToMtime` — JPEG without EXIF returns file mtime
- `TestResolveDate_UnknownExtension_UsesMtime` — `.webp` file uses mtime
- `TestResolveDate_PermissionError` — unreadable file returns zero time

### 3.3 Media Package

- `TestIsSupportedFile_CaseSensitivity` — verify `.JPG`, `.Mp4`, `.HEIC` all match
- `TestSupportedExtensions_Complete` — verify SupportedExtensions() returns all expected extensions
- `TestIsSupportedFile_NoExtension` — file with no extension returns false
- `TestIsSupportedFile_DotOnly` — file named `.jpg` (no base) returns true

### 3.4 JPEG/MP4 Metadata Edge Cases

- `TestSetMetadata_UnicodeContent` — XMP with CJK characters, emoji
- `TestSetMetadata_VeryLargeMetadata` — metadata approaching 65533-byte APP1 limit
- `TestSetMetadata_MetadataExceedsLimit` — metadata over 65533 bytes returns error
- `TestSetXMPMetadata_LargeMP4` — verify XMP insertion doesn't corrupt large files (use a more realistic test MP4)
- `TestReadDate_MalformedExif` — JPEG with invalid EXIF structure returns zero time gracefully
- `TestReadCreationTime_MissingMvhd` — MP4 without mvhd box returns zero time

### 3.5 Flickr OAuth Edge Cases

- `TestOAuthSign_SpecialCharactersInParams` — URL encoding of `+`, `=`, `&`, spaces
- `TestOAuthSign_EmptyParams` — signing with no extra params
- `TestOAuthSign_PostMethod` — signature changes with HTTP method

### 3.6 S3 Unit Tests

- `TestBuildUploadArgs_SpecialCharsInPrefix` — prefix with spaces or unicode
- `TestFindRcloneBinary_PermissionDenied` — binary exists but not executable
- `TestWriteRcloneConfig_EmptyCredentials` — verify config still written (rclone will error later)

---

## Priority 4: Structural Improvements

### 4.1 CLI Config Command Tests

The `config` subcommands (`config flickr`, `config google`, `config s3`) have zero test coverage. These involve interactive stdin reading which is harder to test, but the `readAWSCredentials` helper is testable.

- `TestReadAWSCredentials_ValidFile` — parses `[default]` profile correctly
- `TestReadAWSCredentials_MissingFile` — returns error
- `TestReadAWSCredentials_NoDefaultProfile` — returns error or empty config
- `TestReadAWSCredentials_MalformedFile` — graceful handling

### 4.2 Transfer Validation Enhancement

- `TestValidate_ZeroSizeFiles_UploadOperation` — verify zero-size check skipped for uploads (only runs for downloads)
- `TestValidate_SymlinksInDirectory` — ScanDir with symlinks
- `TestWriteReport_LongErrorList` — report with many errors formats correctly

### 4.3 Retry Logic Completeness

Unit tests cover 429 and 500, but not other 5xx codes.

- `TestRetryableGet_RetriesOn502` — Bad Gateway
- `TestRetryableGet_RetriesOn503` — Service Unavailable
- `TestRetryableGet_NoRetryOn400` — Client errors don't retry
- `TestRetryableGet_NoRetryOn401` — Auth errors don't retry
- `TestRetryableGet_NoRetryOn403` — Forbidden doesn't retry

---

## Implementation Order

1. **S3 integration tests** (P1.1) — biggest gap, only untested service
2. **Video metadata integration** (P1.2) — validates an important feature path
3. **Flickr upload error handling** (P1.3) — validates the abort threshold
4. **Retry logic completeness** (P4.3) — quick wins, verifies important behavior
5. **MediaDate fallback chain** (P3.2) — verifies date resolution priority
6. **Google Takeout edge cases** (P2.2) — duplicate filename handling is tricky
7. **Remaining P2 edge cases** — error resilience
8. **Remaining P3 unit tests** — filling gaps
9. **CLI config tests** (P4.1) — interactive commands are harder to test
10. **Google daily limit** (P1.4) — may require refactoring to make testable

Estimated new tests: ~60-70 test functions across priorities 1-4.
