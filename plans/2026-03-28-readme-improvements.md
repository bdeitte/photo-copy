# README Improvements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Improve the README with clarifications, missing feature documentation, and a better development section.

**Architecture:** All changes are to `README.md`. No code changes. Each task is an independent edit to a specific section.

**Tech Stack:** Markdown

---

### Task 1: Clarify Rosetta wording (item 3)

**Files:**
- Modify: `README.md:107`

- [ ] **Step 1: Edit the iCloud download note**

Change line 107 from:
```
- icloudpd for downloads bundled for Linux amd64/arm64, macOS amd64 (runs via Rosetta on Apple Silicon), and Windows amd64. Other platforms: `pipx install icloudpd`.
```
to:
```
- icloudpd for downloads bundled for Linux amd64/arm64, macOS amd64 (Apple Silicon runs via Rosetta 2), and Windows amd64. Other platforms: `pipx install icloudpd`.
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "Clarify Rosetta 2 wording in iCloud download notes"
```

---

### Task 2: Add cross-service transfer overview (item 5)

**Files:**
- Modify: `README.md` — add a new section after line 6 (closing `</p>` tag), before `## Setup`

- [ ] **Step 1: Add overview paragraph**

Insert after the centered header block and before `## Setup`:

```markdown
## Overview

photo-copy copies photos and videos between cloud services and local directories. Each service has its own `download` and `upload` command that transfers between a local directory and that service. To copy between two services (e.g., Flickr to S3), download to a local directory first, then upload from that directory — there is no direct service-to-service transfer.
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "Add overview section explaining cross-service transfer model"
```

---

### Task 3: Add --limit to Features section (item 6)

**Files:**
- Modify: `README.md:135` — in the Filtering Options subsection

- [ ] **Step 1: Add --limit documentation**

After the `--date-range` paragraph (line 135), add:

```markdown
- `--limit N` — Only process the first N files. Useful for testing a transfer before running the full batch, or for processing files incrementally. For iCloud downloads, this maps to icloudpd's `--recent` flag, which selects the N most recently uploaded photos.
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "Document --limit flag in Features section"
```

---

### Task 4: Add Flickr upload rate limiting info (item 7)

**Files:**
- Modify: `README.md:130-131` — in the Rate limiting & retry subsection

- [ ] **Step 1: Expand Flickr rate limiting to cover uploads**

After the existing Flickr bullet (line 130), add a second bullet:

```markdown
- **Flickr uploads** — Uploads continue past individual file failures (logging each error) rather than failing fast. If 10 uploads fail consecutively, the transfer aborts to avoid wasting time on a systemic issue (e.g., expired auth token).
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "Document Flickr upload consecutive failure abort in rate limiting section"
```

---

### Task 5: Clarify Google Photos rate limiting UX (item 8)

**Files:**
- Modify: `README.md:131` — the Google Photos bullet in Rate limiting & retry

- [ ] **Step 1: Expand Google Photos bullet**

Change the Google Photos bullet from:
```markdown
- **Google Photos** — Subject to a 10,000 uploads/day limit, enforced in code.
```
to:
```markdown
- **Google Photos** — Subject to a 10,000 uploads/day limit. If more files are queued, the upload is automatically capped at 10,000 with a log message — re-run the next day to continue.
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "Clarify Google Photos daily limit behavior"
```

---

### Task 6: Add duplicate handling info (item 9)

**Files:**
- Modify: `README.md` — add a new paragraph at the end of the "Resumable transfers" subsection (after line 126)

- [ ] **Step 1: Add duplicate handling note**

After the paragraph ending "...skipping files that already succeeded." (line 126), add:

```markdown
**Note on duplicates:** Resumable transfers skip files already completed in the current transfer run. However, Flickr and Google Photos uploads do not check whether a file already exists in the service — re-uploading the same files to a new directory will create duplicates. S3 avoids this via rclone's file comparison. iCloud uploads rely on Photos.app's built-in deduplication. Google Takeout import renames files on filename collision (e.g., `photo_1.jpg`).
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "Document duplicate handling behavior across services"
```

---

### Task 7: Clarify supported file types for upload vs download (item 10)

**Files:**
- Modify: `README.md:179-180` — the Supported file types subsection

- [ ] **Step 1: Expand supported file types section**

Change from:
```markdown
### Supported file types

JPEG, PNG, TIFF, GIF, HEIC, WebP, MP4, MOV, AVI, MKV
```
to:
```markdown
### Supported file types

**Uploads and S3 downloads:** JPEG, PNG, TIFF, GIF, HEIC, WebP, MP4, MOV, AVI, MKV — files with other extensions are skipped.

**Flickr and iCloud downloads:** All file types are downloaded as-is from the service, regardless of extension.
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "Clarify supported file types apply to uploads only"
```

---

### Task 8: Add AGENTS.md symlink and update Development section (item 11)

**Files:**
- Create: `AGENTS.md` (symlink to `CLAUDE.md`)
- Modify: `README.md:185`

- [ ] **Step 1: Create AGENTS.md symlink**

```bash
ln -s CLAUDE.md AGENTS.md
```

- [ ] **Step 2: Update README to point to AGENTS.md**

Change from:
```markdown
See [CLAUDE.md](CLAUDE.md#architecture) for some details on the project.
```
to:
```markdown
See [AGENTS.md](AGENTS.md#architecture) for architecture details on the project.
```

- [ ] **Step 3: Commit**

```bash
git add AGENTS.md README.md
git commit -m "Add AGENTS.md symlink and point README to it for architecture details"
```
