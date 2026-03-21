# Bundle Tools Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Consolidate rclone, icloudpd, and osxphotos binaries into a shared `tools-bin/` directory with per-tool update scripts and bundled binary resolution at runtime.

**Architecture:** Move `rclone-bin/` to `tools-bin/rclone/`, add `tools-bin/icloudpd/` and `tools-bin/osxphotos/` with update scripts that download pre-built binaries from upstream releases. Runtime resolution checks `tools-bin/{tool}/` next to executable, then cwd, then system PATH as fallback.

**Tech Stack:** Go, Bash, Git LFS

**Spec:** `plans/2026-03-21-bundle-tools-design.md`

---

### Task 1: Update .gitattributes and .golangci.yml before moving files

**Files:**
- Modify: `.gitattributes`
- Modify: `.golangci.yml:16`

These must be updated BEFORE moving binaries to avoid LFS tracking gaps and lint failures on intermediate commits.

- [ ] **Step 1: Update .gitattributes**

Add the new LFS patterns alongside the old one (keep the old pattern until binaries are moved):
```
rclone-bin/rclone-* filter=lfs diff=lfs merge=lfs -text
tools-bin/rclone/rclone-* filter=lfs diff=lfs merge=lfs -text
tools-bin/icloudpd/icloudpd-* filter=lfs diff=lfs merge=lfs -text
tools-bin/osxphotos/osxphotos-* filter=lfs diff=lfs merge=lfs -text
```

- [ ] **Step 2: Update .golangci.yml exclusion**

Change line 16 from:
```yaml
      - rclone-bin
```
to:
```yaml
      - tools-bin
```

- [ ] **Step 3: Commit**

```bash
git add .gitattributes .golangci.yml
git commit -m "Prepare .gitattributes and .golangci.yml for tools-bin/ migration"
```

---

### Task 2: Move rclone-bin/ to tools-bin/rclone/

**Files:**
- Move: `rclone-bin/*` → `tools-bin/rclone/`
- Modify: `rclone-bin/update-rclone.sh` → `tools-bin/rclone/update.sh`

- [ ] **Step 1: Create tools-bin/rclone/ directory and move binaries**

```bash
mkdir -p tools-bin/rclone
git mv rclone-bin/rclone-* tools-bin/rclone/
```

- [ ] **Step 2: Create tools-bin/rclone/update.sh adapted from rclone-bin/update-rclone.sh**

Copy `rclone-bin/update-rclone.sh` to `tools-bin/rclone/update.sh` with one change: the old script has `BIN_DIR="$SCRIPT_DIR/../rclone-bin"` (navigating up to a sibling directory). Change this to `BIN_DIR="$SCRIPT_DIR"` since the script now lives in the same directory as the binaries.

```bash
#!/usr/bin/env bash
set -e

RCLONE_VERSION="${1:-v1.73.2}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="$SCRIPT_DIR"
```

The rest of the script is identical to the existing `rclone-bin/update-rclone.sh`.

- [ ] **Step 3: Remove old rclone-bin/ directory**

```bash
git rm rclone-bin/update-rclone.sh
# rclone-bin/ should now be empty and can be removed
rmdir rclone-bin 2>/dev/null || true
```

- [ ] **Step 4: Remove old LFS pattern from .gitattributes**

Remove the `rclone-bin/rclone-*` line from `.gitattributes` (the new `tools-bin/rclone/rclone-*` pattern was already added in Task 1).

- [ ] **Step 5: Commit**

```bash
git add tools-bin/rclone/ .gitattributes
git commit -m "Move rclone-bin/ to tools-bin/rclone/"
```

---

### Task 3: Create icloudpd update script

**Files:**
- Create: `tools-bin/icloudpd/update.sh`

- [ ] **Step 1: Write the update script**

```bash
#!/usr/bin/env bash
set -e

ICLOUDPD_VERSION="${1:-1.32.2}"
ICLOUDPD_VERSION="${ICLOUDPD_VERSION#v}"  # Strip leading 'v' if present
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="$SCRIPT_DIR"

mkdir -p "$BIN_DIR"

# Detect current version from an existing binary
CURRENT_VERSION=""
for bin in "$BIN_DIR"/icloudpd-*; do
    [[ -x "$bin" ]] || continue
    ver=$("$bin" --version 2>/dev/null | head -1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+') || continue
    CURRENT_VERSION="$ver"
    break
done
CURRENT_VERSION="${CURRENT_VERSION:-unknown}"

if [ "$CURRENT_VERSION" = "$ICLOUDPD_VERSION" ]; then
    echo "Already at icloudpd $ICLOUDPD_VERSION — nothing to do."
    exit 0
fi

echo "=== icloudpd Update: $CURRENT_VERSION -> $ICLOUDPD_VERSION ==="
echo ""
echo "Release: https://github.com/icloud-photos-downloader/icloud_photos_downloader/releases/tag/v${ICLOUDPD_VERSION}"
echo ""

WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

BASE_URL="https://github.com/icloud-photos-downloader/icloud_photos_downloader/releases/download/v${ICLOUDPD_VERSION}"

download_icloudpd() {
    local upstream_name="$1"
    local binary_name="$2"

    echo "Downloading icloudpd $ICLOUDPD_VERSION ($upstream_name)..."
    curl -sL "${BASE_URL}/icloudpd-${ICLOUDPD_VERSION}-${upstream_name}" -o "$BIN_DIR/$binary_name"
    chmod +x "$BIN_DIR/$binary_name"
    echo "  -> $binary_name"
}

# Upstream uses "macos" not "darwin", and no arm64 macOS binary exists
download_icloudpd "linux-amd64"     "icloudpd-linux-amd64"
download_icloudpd "linux-arm64"     "icloudpd-linux-arm64"
download_icloudpd "macos-amd64"     "icloudpd-darwin-amd64"

# Windows binary has .exe suffix upstream
echo "Downloading icloudpd $ICLOUDPD_VERSION (windows-amd64)..."
curl -sL "${BASE_URL}/icloudpd-${ICLOUDPD_VERSION}-windows-amd64.exe" -o "$BIN_DIR/icloudpd-windows-amd64.exe"
echo "  -> icloudpd-windows-amd64.exe"

echo ""
echo "icloudpd $ICLOUDPD_VERSION downloaded."
echo "Files in $BIN_DIR:"
ls -lh "$BIN_DIR"/icloudpd-*
```

- [ ] **Step 2: Make executable and commit**

```bash
chmod +x tools-bin/icloudpd/update.sh
git add tools-bin/icloudpd/update.sh
git commit -m "Add icloudpd update script"
```

---

### Task 4: Create osxphotos update script

**Files:**
- Create: `tools-bin/osxphotos/update.sh`

- [ ] **Step 1: Write the update script**

The upstream asset name is `osxphotos_MacOS_exe_darwin_arm64_v{VER}.zip`. Only one platform (darwin/arm64).

```bash
#!/usr/bin/env bash
set -e

OSXPHOTOS_VERSION="${1:-0.75.6}"
OSXPHOTOS_VERSION="${OSXPHOTOS_VERSION#v}"  # Strip leading 'v' if present
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="$SCRIPT_DIR"

mkdir -p "$BIN_DIR"

# Detect current version from an existing binary
CURRENT_VERSION=""
if [[ -x "$BIN_DIR/osxphotos-darwin-arm64" ]]; then
    ver=$("$BIN_DIR/osxphotos-darwin-arm64" version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1) || true
    CURRENT_VERSION="${ver:-unknown}"
else
    CURRENT_VERSION="unknown"
fi

if [ "$CURRENT_VERSION" = "$OSXPHOTOS_VERSION" ]; then
    echo "Already at osxphotos $OSXPHOTOS_VERSION — nothing to do."
    exit 0
fi

echo "=== osxphotos Update: $CURRENT_VERSION -> $OSXPHOTOS_VERSION ==="
echo ""
echo "Release: https://github.com/RhetTbull/osxphotos/releases/tag/v${OSXPHOTOS_VERSION}"
echo ""

WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

ZIP_NAME="osxphotos_MacOS_exe_darwin_arm64_v${OSXPHOTOS_VERSION}.zip"
URL="https://github.com/RhetTbull/osxphotos/releases/download/v${OSXPHOTOS_VERSION}/${ZIP_NAME}"

echo "Downloading osxphotos $OSXPHOTOS_VERSION (darwin-arm64)..."
curl -sL "$URL" -o "$WORK_DIR/$ZIP_NAME"
unzip -q -o "$WORK_DIR/$ZIP_NAME" -d "$WORK_DIR/osxphotos"

# Find the osxphotos binary in the extracted zip
EXTRACTED_BIN=$(find "$WORK_DIR/osxphotos" -name "osxphotos" -type f | head -1)
if [ -z "$EXTRACTED_BIN" ]; then
    echo "Error: could not find osxphotos binary in zip"
    exit 1
fi

cp "$EXTRACTED_BIN" "$BIN_DIR/osxphotos-darwin-arm64"
chmod +x "$BIN_DIR/osxphotos-darwin-arm64"
echo "  -> osxphotos-darwin-arm64"

echo ""
echo "osxphotos $OSXPHOTOS_VERSION downloaded."
ls -lh "$BIN_DIR"/osxphotos-*
```

- [ ] **Step 2: Make executable and commit**

```bash
chmod +x tools-bin/osxphotos/update.sh
git add tools-bin/osxphotos/update.sh
git commit -m "Add osxphotos update script"
```

---

### Task 5: Create top-level tools-bin/update.sh

**Files:**
- Create: `tools-bin/update.sh`

- [ ] **Step 1: Write the top-level script**

```bash
#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

if [ -n "$1" ]; then
    # Update a specific tool
    TOOL_SCRIPT="$SCRIPT_DIR/$1/update.sh"
    if [ ! -f "$TOOL_SCRIPT" ]; then
        echo "Error: unknown tool '$1'. Available: rclone, icloudpd, osxphotos"
        exit 1
    fi
    shift
    bash "$TOOL_SCRIPT" "$@"
else
    # Update all tools
    echo "=== Updating all tools ==="
    echo ""
    bash "$SCRIPT_DIR/rclone/update.sh"
    echo ""
    bash "$SCRIPT_DIR/icloudpd/update.sh"
    echo ""
    bash "$SCRIPT_DIR/osxphotos/update.sh"
    echo ""
    echo "=== All tools updated ==="
fi
```

- [ ] **Step 2: Make executable and commit**

```bash
chmod +x tools-bin/update.sh
git add tools-bin/update.sh
git commit -m "Add top-level tools-bin/update.sh"
```

---

### Task 6: Update rclone binary resolution

**Files:**
- Modify: `internal/s3/rclone.go:29-47`
- Modify: `internal/s3/rclone_test.go`

- [ ] **Step 1: Write the failing test**

In `internal/s3/rclone_test.go`, update `TestFindRcloneBinary` to use the new `tools-bin/rclone` directory structure:

```go
func TestFindRcloneBinary(t *testing.T) {
	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, "tools-bin", "rclone")
	_ = os.MkdirAll(binDir, 0755)

	name := rcloneBinaryName(runtime.GOOS, runtime.GOARCH)
	fakeBin := filepath.Join(binDir, name)
	_ = os.WriteFile(fakeBin, []byte("fake"), 0755)

	got, err := findRcloneBinary(binDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != fakeBin {
		t.Errorf("got %q, want %q", got, fakeBin)
	}
}
```

Note: `findRcloneBinary()` takes a `binDir` arg and doesn't care about the parent path — this test verifies the directory structure change is consistent but the function itself is unchanged. The real change is in `rcloneBinDir()`.

- [ ] **Step 2: Run test to verify it passes** (this test should still pass since `findRcloneBinary` is path-agnostic)

```bash
go test ./internal/s3/ -run TestFindRcloneBinary -v
```

- [ ] **Step 3: Update rcloneBinDir() in rclone.go**

Change `rclone-bin` to `tools-bin/rclone` in `rcloneBinDir()` at `internal/s3/rclone.go:29-47`:

```go
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
```

- [ ] **Step 4: Run all s3 tests**

```bash
go test ./internal/s3/ -v
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/s3/rclone.go internal/s3/rclone_test.go
git commit -m "Update rclone binary resolution to use tools-bin/rclone/"
```

---

### Task 7: Update icloud binary resolution with bundled binary support

**Files:**
- Modify: `internal/icloud/icloud.go:21-37`
- Modify: `internal/icloud/icloud_test.go`

- [ ] **Step 1: Write failing tests for bundled binary resolution**

Add tests to `internal/icloud/icloud_test.go`:

```go
func TestFindTool_BundledBinary(t *testing.T) {
	// Create a fake tools-bin/{tool}/ directory structure
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "tools-bin", "icloudpd")
	_ = os.MkdirAll(toolDir, 0755)

	goos := runtime.GOOS
	goarch := runtime.GOARCH
	binName := fmt.Sprintf("icloudpd-%s-%s", goos, goarch)
	fakeBin := filepath.Join(toolDir, binName)
	_ = os.WriteFile(fakeBin, []byte("#!/bin/sh"), 0755)

	// findTool should find the bundled binary when given the base dir
	path, err := FindTool("icloudpd", "PHOTO_COPY_ICLOUDPD_PATH", tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != fakeBin {
		t.Errorf("got %q, want %q", path, fakeBin)
	}
}

func TestFindTool_RosettaFallback(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("Rosetta fallback only applies on darwin/arm64")
	}

	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "tools-bin", "icloudpd")
	_ = os.MkdirAll(toolDir, 0755)

	// Only provide darwin-amd64 binary (no arm64)
	fakeBin := filepath.Join(toolDir, "icloudpd-darwin-amd64")
	_ = os.WriteFile(fakeBin, []byte("#!/bin/sh"), 0755)

	path, err := FindTool("icloudpd", "PHOTO_COPY_ICLOUDPD_PATH", tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != fakeBin {
		t.Errorf("got %q, want %q", path, fakeBin)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/icloud/ -run TestFindTool_Bundled -v
go test ./internal/icloud/ -run TestFindTool_Rosetta -v
```

Expected: FAIL — `FindTool` doesn't exist yet (function is lowercase `findTool`), and signature has changed.

- [ ] **Step 3: Implement the new FindTool function**

Replace `findTool` in `internal/icloud/icloud.go` with an exported `FindTool` that adds bundled binary resolution. The function needs additional `searchDirs` to check for bundled binaries:

```go
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
```

- [ ] **Step 4: Update callers of findTool to use FindTool**

In `internal/icloud/download.go`, change:
```go
icloudpdPath, err := findTool("icloudpd", "PHOTO_COPY_ICLOUDPD_PATH")
```
to:
```go
icloudpdPath, err := FindTool("icloudpd", "PHOTO_COPY_ICLOUDPD_PATH")
```

In `internal/icloud/upload.go`, change:
```go
osxphotosPath, err := findTool("osxphotos", "PHOTO_COPY_OSXPHOTOS_PATH")
```
to:
```go
osxphotosPath, err := FindTool("osxphotos", "PHOTO_COPY_OSXPHOTOS_PATH")
```

- [ ] **Step 5: Update existing findTool tests to use FindTool**

Update test function calls from `findTool(...)` to `FindTool(...)` in `internal/icloud/icloud_test.go`. The existing tests (env var override, not found, etc.) should still pass since those code paths are preserved.

- [ ] **Step 6: Run all icloud tests**

```bash
go test ./internal/icloud/ -v
```

Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add internal/icloud/icloud.go internal/icloud/icloud_test.go internal/icloud/download.go internal/icloud/upload.go
git commit -m "Add bundled binary resolution for icloudpd and osxphotos"
```

---

### Task 8: Update config icloud to use FindTool

**Files:**
- Modify: `internal/cli/config.go:224-237`

- [ ] **Step 1: Update icloudpd lookup in config.go**

At `internal/cli/config.go:224-229`, replace:
```go
// Check icloudpd is installed
icloudpdPath, err := exec.LookPath("icloudpd")
if err != nil {
    return fmt.Errorf("icloudpd not found. Install it with: pipx install icloudpd")
}
```

With:
```go
// Check icloudpd is installed
icloudpdPath, err := icloud.FindTool("icloudpd", "PHOTO_COPY_ICLOUDPD_PATH")
if err != nil {
    return err
}
```

- [ ] **Step 2: Update osxphotos lookup in config.go**

At `internal/cli/config.go:232-237`, replace:
```go
// Check osxphotos (optional)
if osxphotosPath, err := exec.LookPath("osxphotos"); err == nil {
    fmt.Printf("Found osxphotos at: %s\n", osxphotosPath)
} else {
    fmt.Println("Warning: osxphotos not found. Upload to iCloud will not be available.")
    fmt.Println("Install with: pipx install osxphotos")
}
```

With:
```go
// Check osxphotos (optional)
if osxphotosPath, err := icloud.FindTool("osxphotos", "PHOTO_COPY_OSXPHOTOS_PATH"); err == nil {
    fmt.Printf("Found osxphotos at: %s\n", osxphotosPath)
} else {
    fmt.Println("Warning: osxphotos not found. Upload to iCloud will not be available.")
    fmt.Println("Run ./tools-bin/osxphotos/update.sh to download (macOS ARM64 only), or install manually: pipx install osxphotos")
}
```

- [ ] **Step 3: Update import to include icloud package**

Add `"github.com/briandeitte/photo-copy/internal/icloud"` to imports in `config.go`. Do NOT remove `"os/exec"` — it is still used at line 267 for `exec.Command` in the auth flow.

- [ ] **Step 4: Run lint and tests**

```bash
golangci-lint run ./internal/cli/...
go test ./internal/cli/ -v
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/config.go
git commit -m "Use FindTool for icloudpd/osxphotos lookup in config icloud"
```

---

### Task 9: Update setup.sh and setup.bat

**Files:**
- Modify: `setup.sh`
- Modify: `setup.bat`

- [ ] **Step 1: Update setup.sh**

Replace the rclone check section (lines 16-22) with checks for all three tools:

```bash
# Verify tool binaries are present
if [ ! -d "tools-bin/rclone" ] || [ -z "$(ls tools-bin/rclone/rclone-* 2>/dev/null)" ]; then
    echo ""
    echo "Warning: rclone binaries not found in tools-bin/rclone/"
    echo "(S3 commands will not work without rclone binaries)"
fi

if [ ! -d "tools-bin/icloudpd" ] || [ -z "$(ls tools-bin/icloudpd/icloudpd-* 2>/dev/null)" ]; then
    echo ""
    echo "Warning: icloudpd binaries not found in tools-bin/icloudpd/"
    echo "(iCloud download will fall back to system-installed icloudpd)"
fi

if [ ! -d "tools-bin/osxphotos" ] || [ -z "$(ls tools-bin/osxphotos/osxphotos-* 2>/dev/null)" ]; then
    echo ""
    echo "Warning: osxphotos binary not found in tools-bin/osxphotos/"
    echo "(iCloud upload will fall back to system-installed osxphotos)"
fi

echo ""
echo "To download all tool binaries: ./tools-bin/update.sh"
```

- [ ] **Step 2: Update setup.bat**

Replace the rclone check section (lines 18-31) with checks for all three tools and updated guidance:

```bat
if not exist "tools-bin\rclone" (
    echo.
    echo Warning: rclone binaries not found in tools-bin\rclone\
    echo ^(S3 commands will not work without rclone binaries^)
) else (
    dir /b tools-bin\rclone\rclone-* >nul 2>nul
    if %errorlevel% neq 0 (
        echo.
        echo Warning: rclone binaries not found in tools-bin\rclone\
        echo ^(S3 commands will not work without rclone binaries^)
    )
)

if not exist "tools-bin\icloudpd" (
    echo.
    echo Warning: icloudpd binaries not found in tools-bin\icloudpd\
    echo ^(iCloud download will fall back to system-installed icloudpd^)
) else (
    dir /b tools-bin\icloudpd\icloudpd-* >nul 2>nul
    if %errorlevel% neq 0 (
        echo.
        echo Warning: icloudpd binaries not found in tools-bin\icloudpd\
        echo ^(iCloud download will fall back to system-installed icloudpd^)
    )
)

if not exist "tools-bin\osxphotos" (
    echo.
    echo Warning: osxphotos binary not found in tools-bin\osxphotos\
    echo ^(iCloud upload will fall back to system-installed osxphotos^)
) else (
    dir /b tools-bin\osxphotos\osxphotos-* >nul 2>nul
    if %errorlevel% neq 0 (
        echo.
        echo Warning: osxphotos binary not found in tools-bin\osxphotos\
        echo ^(iCloud upload will fall back to system-installed osxphotos^)
    )
)

echo.
echo To download all tool binaries: bash tools-bin\update.sh
```

- [ ] **Step 3: Commit**

```bash
git add setup.sh setup.bat
git commit -m "Update setup scripts for tools-bin/ directory"
```

---

### Task 10: Update README.md

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update setup/prerequisites section**

Remove the `pipx install icloudpd` and `pipx install osxphotos` prerequisites. Replace with a note that these tools are bundled. Add platform gap notes:

- icloudpd: bundled for linux amd64/arm64, macOS amd64 (runs via Rosetta on Apple Silicon), Windows amd64. Other platforms: `pipx install icloudpd`.
- osxphotos: bundled for macOS ARM64 only. Intel Macs: `pipx install osxphotos`.

- [ ] **Step 2: Update the "Updating rclone" section** (lines 194-200)

Replace:
```markdown
### Updating rclone

To update the bundled rclone binaries:

\```bash
./rclone-bin/update-rclone.sh v1.68.2
\```
```

With:
```markdown
### Updating bundled tools

To update all bundled tool binaries:

\```bash
./tools-bin/update.sh
\```

To update a specific tool:

\```bash
./tools-bin/update.sh rclone v1.73.2
./tools-bin/update.sh icloudpd 1.32.2
./tools-bin/update.sh osxphotos 0.75.6
\```
```

- [ ] **Step 3: Add Acknowledgments section at end of README.md**

```markdown
## Acknowledgments

photo-copy relies on these excellent open-source tools:

- **[rclone](https://rclone.org/)** — Used for S3 uploads and downloads
- **[icloudpd](https://github.com/icloud-photos-downloader/icloud_photos_downloader)** — Used for iCloud Photos downloads
- **[osxphotos](https://github.com/RhetTbull/osxphotos)** — Used for iCloud Photos uploads on macOS
```

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "Update README for bundled icloudpd and osxphotos"
```

---

### Task 11: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update architecture references**

Update all references to `rclone-bin/` in CLAUDE.md:
- Change `rclone-bin/` to `tools-bin/rclone/` in the S3 package description
- Change `rclone-bin/update-rclone.sh` to `tools-bin/rclone/update.sh`
- Update the iCloud package description to mention bundled binaries instead of `pipx install`
- Update `setup.sh` description to mention `tools-bin/`

- [ ] **Step 2: Run final lint and tests**

```bash
golangci-lint run ./...
go test ./...
```

Expected: all pass.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "Update CLAUDE.md architecture docs for tools-bin/"
```

---

### Task 12: Download icloudpd and osxphotos binaries

**Files:**
- Run: `tools-bin/icloudpd/update.sh`
- Run: `tools-bin/osxphotos/update.sh`

- [ ] **Step 1: Run the icloudpd update script**

```bash
./tools-bin/icloudpd/update.sh
```

Expected: downloads 4 binaries (linux-amd64, linux-arm64, darwin-amd64, windows-amd64.exe).

- [ ] **Step 2: Run the osxphotos update script**

```bash
./tools-bin/osxphotos/update.sh
```

Expected: downloads 1 binary (darwin-arm64).

- [ ] **Step 3: Verify binaries are LFS-tracked**

```bash
git lfs ls-files
```

Should show the new icloudpd and osxphotos binaries alongside the rclone binaries.

- [ ] **Step 4: Commit binaries**

```bash
git add tools-bin/icloudpd/icloudpd-* tools-bin/osxphotos/osxphotos-*
git commit -m "Add bundled icloudpd and osxphotos binaries"
```

---

### Task 13: Final verification

- [ ] **Step 1: Run all tests**

```bash
go test ./...
```

Expected: all pass.

- [ ] **Step 2: Run linter**

```bash
golangci-lint run ./...
```

Expected: all pass.

- [ ] **Step 3: Verify rclone-bin/ is fully removed**

```bash
ls rclone-bin/ 2>/dev/null && echo "ERROR: rclone-bin still exists" || echo "OK: rclone-bin removed"
```

- [ ] **Step 4: Verify tools-bin/ structure**

```bash
ls -la tools-bin/
ls -la tools-bin/rclone/
ls -la tools-bin/icloudpd/
ls -la tools-bin/osxphotos/
```

Expected: all directories exist with their respective binaries and update scripts.
