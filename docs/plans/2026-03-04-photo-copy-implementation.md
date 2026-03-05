# Photo Copy Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Set up a repo with a Python CLI (`photo-copy`) for Flickr and Google Photos operations, plus rclone for S3, enabling easy photo/video copying between all four locations.

**Architecture:** Two tools — rclone (external) for S3 ↔ local, and a Python CLI for Flickr (upload/download via API) and Google Photos (upload via API + Takeout import). Cross-service copies go through a local intermediate directory.

**Tech Stack:** Python 3.10+, click, flickrapi, google-auth, google-api-python-client, tqdm, rclone

---

### Task 1: Python project scaffolding

**Files:**
- Create: `pyproject.toml`
- Create: `src/photo_copy/__init__.py`
- Create: `src/photo_copy/cli.py`

**Step 1: Create `pyproject.toml`**

```toml
[build-system]
requires = ["setuptools>=68.0"]
build-backend = "setuptools.backends._legacy:_Backend"

[project]
name = "photo-copy"
version = "0.1.0"
description = "Copy photos and videos between Flickr, Google Photos, S3, and local directories"
requires-python = ">=3.10"
dependencies = [
    "click>=8.0",
    "flickrapi>=2.4",
    "google-auth>=2.0",
    "google-auth-oauthlib>=1.0",
    "google-api-python-client>=2.0",
    "tqdm>=4.0",
    "requests>=2.28",
]

[project.scripts]
photo-copy = "photo_copy.cli:main"

[tool.setuptools.packages.find]
where = ["src"]

[tool.pytest.ini_options]
testpaths = ["tests"]
```

**Step 2: Create `src/photo_copy/__init__.py`**

```python
"""Photo Copy - copy photos and videos between Flickr, Google Photos, S3, and local directories."""
```

**Step 3: Create minimal `src/photo_copy/cli.py`**

```python
import click


@click.group()
def main():
    """Copy photos and videos between Flickr, Google Photos, S3, and local directories."""
    pass


@main.group()
def flickr():
    """Flickr upload and download commands."""
    pass


@main.group(name="google-photos")
def google_photos():
    """Google Photos upload and Takeout import commands."""
    pass


@main.group()
def config():
    """Configure API credentials."""
    pass
```

**Step 4: Install in development mode and verify CLI works**

Run: `cd /Users/briandeitte/photo-copy && pip install -e .`
Run: `photo-copy --help`
Expected: Shows help with `flickr`, `google-photos`, and `config` subcommands.

**Step 5: Commit**

```bash
git add pyproject.toml src/
git commit -m "feat: scaffold Python project with click CLI skeleton"
```

---

### Task 2: Config module — credential management

**Files:**
- Create: `src/photo_copy/config.py`
- Create: `tests/test_config.py`

**Step 1: Write the failing test**

```python
import json
from pathlib import Path

from photo_copy.config import get_config_dir, save_flickr_config, load_flickr_config


def test_get_config_dir(tmp_path, monkeypatch):
    monkeypatch.setenv("PHOTO_COPY_CONFIG_DIR", str(tmp_path / "custom"))
    config_dir = get_config_dir()
    assert config_dir == tmp_path / "custom"


def test_save_and_load_flickr_config(tmp_path, monkeypatch):
    monkeypatch.setenv("PHOTO_COPY_CONFIG_DIR", str(tmp_path))
    save_flickr_config("test_key", "test_secret")
    loaded = load_flickr_config()
    assert loaded["api_key"] == "test_key"
    assert loaded["api_secret"] == "test_secret"


def test_load_flickr_config_missing(tmp_path, monkeypatch):
    monkeypatch.setenv("PHOTO_COPY_CONFIG_DIR", str(tmp_path))
    loaded = load_flickr_config()
    assert loaded is None
```

**Step 2: Run test to verify it fails**

Run: `pip install pytest && pytest tests/test_config.py -v`
Expected: FAIL — module not found

**Step 3: Write implementation**

```python
import json
import os
from pathlib import Path


def get_config_dir() -> Path:
    """Return the config directory, creating it if needed."""
    config_dir = Path(os.environ.get("PHOTO_COPY_CONFIG_DIR", Path.home() / ".config" / "photo-copy"))
    config_dir.mkdir(parents=True, exist_ok=True)
    return config_dir


def save_flickr_config(api_key: str, api_secret: str) -> None:
    """Save Flickr API credentials."""
    config_file = get_config_dir() / "flickr.json"
    config_file.write_text(json.dumps({"api_key": api_key, "api_secret": api_secret}))


def load_flickr_config() -> dict | None:
    """Load Flickr API credentials, or None if not configured."""
    config_file = get_config_dir() / "flickr.json"
    if not config_file.exists():
        return None
    return json.loads(config_file.read_text())


def save_google_config(client_secrets_path: str) -> None:
    """Copy Google OAuth client secrets file to config dir."""
    import shutil
    dest = get_config_dir() / "google_client_secrets.json"
    shutil.copy2(client_secrets_path, dest)


def load_google_config() -> Path | None:
    """Return path to Google client secrets, or None if not configured."""
    config_file = get_config_dir() / "google_client_secrets.json"
    if not config_file.exists():
        return None
    return config_file


def get_log_dir() -> Path:
    """Return the log directory, creating it if needed."""
    log_dir = get_config_dir() / "logs"
    log_dir.mkdir(parents=True, exist_ok=True)
    return log_dir
```

**Step 4: Run tests to verify they pass**

Run: `pytest tests/test_config.py -v`
Expected: 3 passed

**Step 5: Commit**

```bash
git add src/photo_copy/config.py tests/test_config.py
git commit -m "feat: add config module for credential management"
```

---

### Task 3: Config CLI commands

**Files:**
- Modify: `src/photo_copy/cli.py`

**Step 1: Add config subcommands to `cli.py`**

Add these commands under the `config` group:

```python
from photo_copy.config import save_flickr_config, load_flickr_config, save_google_config, load_google_config


@config.command()
def flickr():
    """Configure Flickr API credentials."""
    click.echo("Set up Flickr API credentials.")
    click.echo("Get your API key at: https://www.flickr.com/services/apps/create/")
    api_key = click.prompt("Flickr API key")
    api_secret = click.prompt("Flickr API secret")
    save_flickr_config(api_key, api_secret)
    click.echo("Flickr credentials saved.")


@config.command()
def google():
    """Configure Google Photos OAuth credentials."""
    click.echo("Set up Google Photos API credentials.")
    click.echo("1. Go to https://console.cloud.google.com/")
    click.echo("2. Create a project and enable the Photos Library API")
    click.echo("3. Create OAuth 2.0 credentials (Desktop app)")
    click.echo("4. Download the client secrets JSON file")
    secrets_path = click.prompt("Path to client secrets JSON file")
    save_google_config(secrets_path)
    click.echo("Google credentials saved.")
```

Note: The `flickr` name conflicts with the existing `flickr` command group. Rename the config subcommands or nest differently. The config group commands should be named `flickr_creds` and `google_creds`, or better: keep the CLI structure as `photo-copy config flickr` and `photo-copy config google` by making the config group's commands use unique function names:

```python
@config.command("flickr")
def config_flickr():
    ...

@config.command("google")
def config_google():
    ...
```

**Step 2: Test manually**

Run: `photo-copy config --help`
Expected: Shows `flickr` and `google` subcommands.

**Step 3: Commit**

```bash
git add src/photo_copy/cli.py
git commit -m "feat: add config CLI commands for Flickr and Google credentials"
```

---

### Task 4: Media file utilities

**Files:**
- Create: `src/photo_copy/media.py`
- Create: `tests/test_media.py`

**Step 1: Write the failing test**

```python
from pathlib import Path

from photo_copy.media import is_media_file, list_media_files


def test_is_media_file_jpg():
    assert is_media_file(Path("photo.jpg")) is True


def test_is_media_file_mp4():
    assert is_media_file(Path("video.mp4")) is True


def test_is_media_file_txt():
    assert is_media_file(Path("notes.txt")) is False


def test_is_media_file_json():
    assert is_media_file(Path("metadata.json")) is False


def test_is_media_file_case_insensitive():
    assert is_media_file(Path("PHOTO.JPG")) is True
    assert is_media_file(Path("video.MOV")) is True


def test_list_media_files(tmp_path):
    (tmp_path / "a.jpg").touch()
    (tmp_path / "b.png").touch()
    (tmp_path / "c.txt").touch()
    (tmp_path / "d.mp4").touch()
    result = list_media_files(tmp_path)
    names = {f.name for f in result}
    assert names == {"a.jpg", "b.png", "d.mp4"}
```

**Step 2: Run test to verify it fails**

Run: `pytest tests/test_media.py -v`
Expected: FAIL — module not found

**Step 3: Write implementation**

```python
from pathlib import Path

MEDIA_EXTENSIONS = {
    ".jpg", ".jpeg", ".png", ".tiff", ".tif", ".gif", ".heic", ".webp",
    ".mp4", ".mov", ".avi", ".mkv",
}


def is_media_file(path: Path) -> bool:
    """Check if a file has a recognized media extension."""
    return path.suffix.lower() in MEDIA_EXTENSIONS


def list_media_files(directory: Path) -> list[Path]:
    """List all media files in a directory (non-recursive)."""
    return sorted(f for f in directory.iterdir() if f.is_file() and is_media_file(f))
```

**Step 4: Run tests to verify they pass**

Run: `pytest tests/test_media.py -v`
Expected: 6 passed

**Step 5: Commit**

```bash
git add src/photo_copy/media.py tests/test_media.py
git commit -m "feat: add media file detection utilities"
```

---

### Task 5: Flickr download

**Files:**
- Create: `src/photo_copy/flickr.py`
- Create: `tests/test_flickr.py`

**Step 1: Write the failing test**

```python
from pathlib import Path
from unittest.mock import MagicMock, patch

from photo_copy.flickr import download_photos


def test_download_photos_skips_existing(tmp_path):
    """Already-downloaded files are skipped."""
    # Create a file that simulates already downloaded
    (tmp_path / "12345.jpg").touch()

    mock_flickr = MagicMock()
    # Simulate one photo in the user's stream
    mock_flickr.people.getPhotos.return_value = {
        "photos": {
            "page": 1,
            "pages": 1,
            "photo": [{"id": "12345", "title": "test", "originalformat": "jpg"}],
        }
    }

    with patch("photo_copy.flickr._get_flickr_client", return_value=mock_flickr):
        stats = download_photos(tmp_path)

    assert stats["skipped"] == 1
    assert stats["downloaded"] == 0
```

**Step 2: Run test to verify it fails**

Run: `pytest tests/test_flickr.py -v`
Expected: FAIL — module not found

**Step 3: Write implementation**

```python
import logging
from pathlib import Path

import requests
from tqdm import tqdm

from photo_copy.config import load_flickr_config
from photo_copy.media import is_media_file

logger = logging.getLogger(__name__)


def _get_flickr_client():
    """Create an authenticated Flickr API client."""
    import flickrapi

    config = load_flickr_config()
    if config is None:
        raise click.ClickException("Flickr not configured. Run: photo-copy config flickr")
    return flickrapi.FlickrAPI(config["api_key"], config["api_secret"], format="parsed-json")


def download_photos(output_dir: Path) -> dict:
    """Download all photos from the authenticated user's Flickr account."""
    output_dir.mkdir(parents=True, exist_ok=True)
    flickr = _get_flickr_client()

    # Get user ID
    user_info = flickr.test.login()
    user_id = user_info["user"]["id"]

    stats = {"downloaded": 0, "skipped": 0, "failed": 0}
    page = 1
    pages = 1

    while page <= pages:
        response = flickr.people.getPhotos(
            user_id=user_id, extras="url_o,originalformat", per_page=500, page=page
        )
        photos = response["photos"]
        pages = photos["pages"]

        for photo in tqdm(photos["photo"], desc=f"Page {page}/{pages}"):
            photo_id = photo["id"]
            ext = photo.get("originalformat", "jpg")
            filename = f"{photo_id}.{ext}"
            dest = output_dir / filename

            if dest.exists():
                stats["skipped"] += 1
                continue

            url = photo.get("url_o")
            if not url:
                # Try to get original size URL via photos.getSizes
                try:
                    sizes = flickr.photos.getSizes(photo_id=photo_id)
                    for size in sizes["sizes"]["size"]:
                        if size["label"] == "Original":
                            url = size["source"]
                            break
                except Exception:
                    pass

            if not url:
                logger.warning(f"No original URL for photo {photo_id}, skipping")
                stats["failed"] += 1
                continue

            try:
                resp = requests.get(url, stream=True, timeout=60)
                resp.raise_for_status()
                with open(dest, "wb") as f:
                    for chunk in resp.iter_content(chunk_size=8192):
                        f.write(chunk)
                stats["downloaded"] += 1
            except Exception as e:
                logger.error(f"Failed to download photo {photo_id}: {e}")
                stats["failed"] += 1

        page += 1

    return stats


def upload_photos(input_dir: Path) -> dict:
    """Upload all media files from a directory to Flickr."""
    from photo_copy.media import list_media_files

    flickr = _get_flickr_client()
    files = list_media_files(input_dir)
    stats = {"uploaded": 0, "failed": 0}

    for filepath in tqdm(files, desc="Uploading to Flickr"):
        try:
            flickr.upload(filename=str(filepath), title=filepath.stem)
            stats["uploaded"] += 1
        except Exception as e:
            logger.error(f"Failed to upload {filepath.name}: {e}")
            stats["failed"] += 1

    return stats
```

**Step 4: Run tests to verify they pass**

Run: `pytest tests/test_flickr.py -v`
Expected: 1 passed

**Step 5: Commit**

```bash
git add src/photo_copy/flickr.py tests/test_flickr.py
git commit -m "feat: add Flickr download and upload functionality"
```

---

### Task 6: Flickr CLI commands

**Files:**
- Modify: `src/photo_copy/cli.py`

**Step 1: Wire up Flickr commands in CLI**

Add to `cli.py` under the `flickr` group:

```python
import click
from pathlib import Path

from photo_copy.flickr import download_photos, upload_photos


@flickr.command()
@click.option("--output-dir", required=True, type=click.Path(path_type=Path), help="Directory to save downloaded photos")
def download(output_dir: Path):
    """Download all photos from your Flickr account."""
    stats = download_photos(output_dir)
    click.echo(f"Done. Downloaded: {stats['downloaded']}, Skipped: {stats['skipped']}, Failed: {stats['failed']}")


@flickr.command()
@click.option("--input-dir", required=True, type=click.Path(exists=True, path_type=Path), help="Directory of photos to upload")
def upload(input_dir: Path):
    """Upload photos from a directory to your Flickr account."""
    stats = upload_photos(input_dir)
    click.echo(f"Done. Uploaded: {stats['uploaded']}, Failed: {stats['failed']}")
```

Note: The `flickr` group in the CLI and the `config flickr` command use different function names to avoid conflicts. Make sure imports are at the top of `cli.py`.

**Step 2: Test manually**

Run: `photo-copy flickr --help`
Expected: Shows `download` and `upload` subcommands.

**Step 3: Commit**

```bash
git add src/photo_copy/cli.py
git commit -m "feat: add Flickr download/upload CLI commands"
```

---

### Task 7: Google Photos upload

**Files:**
- Create: `src/photo_copy/google.py`
- Create: `tests/test_google.py`

**Step 1: Write the failing test**

```python
from pathlib import Path
from unittest.mock import MagicMock, patch

from photo_copy.google import import_takeout


def test_import_takeout_extracts_media(tmp_path):
    """Takeout import extracts media files and skips JSON metadata."""
    import zipfile

    takeout_dir = tmp_path / "takeout"
    takeout_dir.mkdir()
    output_dir = tmp_path / "output"

    # Create a fake takeout zip
    zip_path = takeout_dir / "takeout-001.zip"
    with zipfile.ZipFile(zip_path, "w") as zf:
        zf.writestr("Takeout/Google Photos/Album/photo1.jpg", b"fake jpg data")
        zf.writestr("Takeout/Google Photos/Album/photo1.jpg.json", b'{"title": "photo1"}')
        zf.writestr("Takeout/Google Photos/Album/video.mp4", b"fake mp4 data")
        zf.writestr("Takeout/Google Photos/Album/metadata.json", b'{"album": "test"}')

    stats = import_takeout(takeout_dir, output_dir)

    assert stats["extracted"] == 2
    assert (output_dir / "photo1.jpg").exists()
    assert (output_dir / "video.mp4").exists()
    assert not (output_dir / "photo1.jpg.json").exists()
    assert not (output_dir / "metadata.json").exists()
```

**Step 2: Run test to verify it fails**

Run: `pytest tests/test_google.py -v`
Expected: FAIL — module not found

**Step 3: Write implementation**

```python
import logging
import zipfile
from pathlib import Path

import requests
from tqdm import tqdm

from photo_copy.config import load_google_config, get_config_dir
from photo_copy.media import is_media_file

logger = logging.getLogger(__name__)


def _get_google_photos_service():
    """Create an authenticated Google Photos API service."""
    from google.auth.transport.requests import Request
    from google.oauth2.credentials import Credentials
    from google_auth_oauthlib.flow import InstalledAppFlow
    from googleapiclient.discovery import build

    SCOPES = ["https://www.googleapis.com/auth/photoslibrary.appendonly"]
    token_path = get_config_dir() / "google_token.json"
    client_secrets = load_google_config()

    if client_secrets is None:
        raise RuntimeError("Google not configured. Run: photo-copy config google")

    creds = None
    if token_path.exists():
        creds = Credentials.from_authorized_user_file(str(token_path), SCOPES)

    if not creds or not creds.valid:
        if creds and creds.expired and creds.refresh_token:
            creds.refresh(Request())
        else:
            flow = InstalledAppFlow.from_client_secrets_file(str(client_secrets), SCOPES)
            creds = flow.run_local_server(port=0)
        token_path.write_text(creds.to_json())

    return build("photoslibrary", "v1", credentials=creds, static_discovery=False)


def upload_photos(input_dir: Path) -> dict:
    """Upload all media files from a directory to Google Photos."""
    from photo_copy.media import list_media_files

    service = _get_google_photos_service()
    files = list_media_files(input_dir)
    stats = {"uploaded": 0, "skipped": 0, "failed": 0}

    # Track uploaded files for resumability
    uploaded_log = get_config_dir() / "google_uploaded.txt"
    already_uploaded = set()
    if uploaded_log.exists():
        already_uploaded = set(uploaded_log.read_text().splitlines())

    for filepath in tqdm(files, desc="Uploading to Google Photos"):
        if filepath.name in already_uploaded:
            stats["skipped"] += 1
            continue

        try:
            # Step 1: Upload bytes
            headers = {
                "Authorization": f"Bearer {service._http.credentials.token}",
                "Content-Type": "application/octet-stream",
                "X-Goog-Upload-File-Name": filepath.name,
                "X-Goog-Upload-Protocol": "raw",
            }
            with open(filepath, "rb") as f:
                resp = requests.post(
                    "https://photoslibrary.googleapis.com/v1/uploads",
                    headers=headers,
                    data=f,
                    timeout=300,
                )
            resp.raise_for_status()
            upload_token = resp.text

            # Step 2: Create media item
            service.mediaItems().batchCreate(
                body={
                    "newMediaItems": [
                        {
                            "simpleMediaItem": {"uploadToken": upload_token, "fileName": filepath.name}
                        }
                    ]
                }
            ).execute()

            stats["uploaded"] += 1
            with open(uploaded_log, "a") as f:
                f.write(filepath.name + "\n")

        except Exception as e:
            logger.error(f"Failed to upload {filepath.name}: {e}")
            stats["failed"] += 1

    return stats


def import_takeout(takeout_dir: Path, output_dir: Path) -> dict:
    """Extract media files from Google Takeout zip files."""
    output_dir.mkdir(parents=True, exist_ok=True)
    stats = {"extracted": 0, "skipped": 0}

    zip_files = sorted(takeout_dir.glob("*.zip"))
    if not zip_files:
        logger.warning(f"No zip files found in {takeout_dir}")
        return stats

    for zip_path in tqdm(zip_files, desc="Processing Takeout zips"):
        with zipfile.ZipFile(zip_path, "r") as zf:
            for name in zf.namelist():
                member_path = Path(name)
                if not is_media_file(member_path):
                    continue

                dest = output_dir / member_path.name
                if dest.exists():
                    stats["skipped"] += 1
                    continue

                # Handle duplicate filenames by appending a number
                if dest.exists():
                    stem = dest.stem
                    suffix = dest.suffix
                    counter = 1
                    while dest.exists():
                        dest = output_dir / f"{stem}_{counter}{suffix}"
                        counter += 1

                data = zf.read(name)
                dest.write_bytes(data)
                stats["extracted"] += 1

    return stats
```

**Step 4: Run tests to verify they pass**

Run: `pytest tests/test_google.py -v`
Expected: 1 passed

**Step 5: Commit**

```bash
git add src/photo_copy/google.py tests/test_google.py
git commit -m "feat: add Google Photos upload and Takeout import"
```

---

### Task 8: Google Photos CLI commands

**Files:**
- Modify: `src/photo_copy/cli.py`

**Step 1: Wire up Google Photos commands in CLI**

Add to `cli.py` under the `google_photos` group:

```python
from photo_copy.google import upload_photos as google_upload, import_takeout


@google_photos.command()
@click.option("--input-dir", required=True, type=click.Path(exists=True, path_type=Path), help="Directory of photos to upload")
def upload(input_dir: Path):
    """Upload photos from a directory to Google Photos."""
    stats = google_upload(input_dir)
    click.echo(f"Done. Uploaded: {stats['uploaded']}, Skipped: {stats['skipped']}, Failed: {stats['failed']}")


@google_photos.command("import-takeout")
@click.option("--takeout-dir", required=True, type=click.Path(exists=True, path_type=Path), help="Directory containing Takeout zip files")
@click.option("--output-dir", required=True, type=click.Path(path_type=Path), help="Directory to extract media files to")
def takeout(takeout_dir: Path, output_dir: Path):
    """Extract photos/videos from Google Takeout zip files."""
    stats = import_takeout(takeout_dir, output_dir)
    click.echo(f"Done. Extracted: {stats['extracted']}, Skipped: {stats['skipped']}")
```

**Step 2: Test manually**

Run: `photo-copy google-photos --help`
Expected: Shows `upload` and `import-takeout` subcommands.

**Step 3: Commit**

```bash
git add src/photo_copy/cli.py
git commit -m "feat: add Google Photos upload and import-takeout CLI commands"
```

---

### Task 9: Setup script

**Files:**
- Create: `setup.sh`

**Step 1: Write setup.sh**

```bash
#!/usr/bin/env bash
set -euo pipefail

echo "=== Photo Copy Setup ==="

# Install rclone if not present
if command -v rclone &> /dev/null; then
    echo "rclone is already installed: $(rclone version | head -1)"
else
    echo "Installing rclone..."
    curl https://rclone.org/install.sh | sudo bash
    echo "rclone installed: $(rclone version | head -1)"
fi

# Install Python CLI
echo ""
echo "Installing photo-copy CLI..."
pip install -e "$(dirname "$0")"
echo ""
echo "photo-copy CLI installed. Run 'photo-copy --help' to get started."
echo ""
echo "=== Next steps ==="
echo "1. Configure rclone for S3:  rclone config"
echo "2. Configure Flickr:         photo-copy config flickr"
echo "3. Configure Google Photos:  photo-copy config google"
```

**Step 2: Make executable and test**

Run: `chmod +x setup.sh && bash -n setup.sh`
Expected: No syntax errors.

**Step 3: Commit**

```bash
git add setup.sh
git commit -m "feat: add setup script for rclone and Python CLI installation"
```

---

### Task 10: README

**Files:**
- Create: `README.md`

**Step 1: Write README.md**

```markdown
# photo-copy

Copy photos and videos between Flickr, Google Photos, S3, and local directories.

## Quick Start

### Linux / macOS

```bash
./setup.sh
```

This installs [rclone](https://rclone.org/) (for S3 operations) and the `photo-copy` CLI (for Flickr and Google Photos).

### Windows

1. Install rclone: `winget install Rclone.Rclone` or download from https://rclone.org/downloads/
2. Install the CLI: `pip install -e .`

## Configuration

### S3 (via rclone)

```bash
rclone config
# Create a new remote of type "s3", enter your AWS credentials
```

### Flickr

```bash
photo-copy config flickr
# Enter your API key and secret from https://www.flickr.com/services/apps/create/
```

### Google Photos

```bash
photo-copy config google
# Provide path to OAuth client secrets JSON from Google Cloud Console
# (Enable the Photos Library API first)
```

## Usage

### Flickr

```bash
# Download all your Flickr photos to a local directory
photo-copy flickr download --output-dir ./flickr-photos

# Upload a directory of photos to Flickr
photo-copy flickr upload --input-dir ./my-photos
```

### Google Photos

```bash
# Upload a directory of photos to Google Photos
photo-copy google-photos upload --input-dir ./my-photos

# Extract media from Google Takeout zip files
photo-copy google-photos import-takeout --takeout-dir ./takeout-zips --output-dir ./google-photos
```

### S3 (via rclone)

```bash
# Copy local photos to S3
rclone copy ./my-photos myremote:my-bucket/photos/ --progress

# Copy S3 photos to local
rclone copy myremote:my-bucket/photos/ ./my-photos --progress
```

### Cross-service copies

For copies between services (e.g., Flickr to S3), use a local directory as an intermediate:

```bash
# Flickr -> S3
photo-copy flickr download --output-dir ./tmp-photos
rclone copy ./tmp-photos myremote:my-bucket/flickr-backup/ --progress

# S3 -> Google Photos
rclone copy myremote:my-bucket/photos/ ./tmp-photos --progress
photo-copy google-photos upload --input-dir ./tmp-photos

# Google Takeout -> S3
photo-copy google-photos import-takeout --takeout-dir ./takeout-zips --output-dir ./tmp-photos
rclone copy ./tmp-photos myremote:my-bucket/google-backup/ --progress
```

## Supported file types

Images: JPEG, PNG, TIFF, GIF, HEIC, WebP
Videos: MP4, MOV, AVI, MKV

## Notes

- **Google Photos downloads:** The Google Photos API only allows downloading photos uploaded by this app. To export your full library, use [Google Takeout](https://takeout.google.com/) and then `import-takeout`.
- **Rate limits:** Google Photos allows 10,000 uploads per day.
- **Resumability:** Flickr download and Google Photos upload skip already-processed files on re-runs.
```

**Step 2: Commit**

```bash
git add README.md
git commit -m "docs: add README with setup and usage instructions"
```

---

### Task 11: CLAUDE.md

**Files:**
- Create: `CLAUDE.md`

**Step 1: Write CLAUDE.md**

```markdown
# Photo Copy - Claude Code Instructions

## Project Overview

This repo provides tools to copy photos and videos between four locations:
- **Local directories** on the filesystem
- **Amazon S3** buckets
- **Flickr** (user's account)
- **Google Photos** (upload via API, download via Google Takeout)

## Architecture

Two tools are used:
- **rclone** — for S3 <-> local copies. Called directly as a shell command.
- **photo-copy** Python CLI — for Flickr and Google Photos operations. Installed from this repo.

Cross-service copies (e.g., Flickr -> S3) go through a local intermediate directory.

## Key Commands

### S3 operations (rclone)
```bash
rclone copy /local/path remote:bucket/path --progress  # local -> S3
rclone copy remote:bucket/path /local/path --progress  # S3 -> local
```

### Flickr operations
```bash
photo-copy flickr download --output-dir /path    # download all user's photos
photo-copy flickr upload --input-dir /path        # upload directory to Flickr
```

### Google Photos operations
```bash
photo-copy google-photos upload --input-dir /path                                    # upload to Google Photos
photo-copy google-photos import-takeout --takeout-dir /path/to/zips --output-dir /path  # extract from Takeout
```

### Configuration
```bash
rclone config                # set up S3 remote
photo-copy config flickr     # set up Flickr API credentials
photo-copy config google     # set up Google OAuth credentials
```

## Development

```bash
pip install -e .             # install in dev mode
pytest                       # run tests
```

## Copy Patterns

When asked to copy between locations, use this logic:
- **Local <-> S3:** Use rclone directly
- **Local <-> Flickr:** Use photo-copy flickr commands
- **Local -> Google Photos:** Use photo-copy google-photos upload
- **Google Takeout -> Local:** Use photo-copy google-photos import-takeout
- **Any other combination:** Chain through a local temp directory

## Credentials

Stored in `~/.config/photo-copy/`. Config dir can be overridden with `PHOTO_COPY_CONFIG_DIR` env var.

## Constraints

- Google Photos API cannot download your full library (only photos uploaded by this app). Use Google Takeout for full exports.
- Google Photos has a 10,000 uploads/day rate limit.
- Albums and comments are not preserved — only raw media files are copied.

## Supported Media Types

Images: .jpg, .jpeg, .png, .tiff, .tif, .gif, .heic, .webp
Videos: .mp4, .mov, .avi, .mkv
```

**Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: add CLAUDE.md for Claude Code session context"
```

---

### Task 12: Final integration test

**Step 1: Run full test suite**

Run: `pytest tests/ -v`
Expected: All tests pass.

**Step 2: Verify CLI end-to-end**

Run: `photo-copy --help`
Run: `photo-copy flickr --help`
Run: `photo-copy google-photos --help`
Run: `photo-copy config --help`
Expected: All show correct subcommands and options.

**Step 3: Final commit if any fixes were needed**

```bash
git status
# If clean, nothing to commit. If fixes were made, commit them.
```
