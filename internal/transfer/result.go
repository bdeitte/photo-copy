package transfer

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/briandeitte/photo-copy/internal/logging"
)

// FileError records a per-file failure for the end-of-transfer summary.
type FileError struct {
	File   string
	Reason string
}

// ValidationWarning describes a post-transfer validation issue.
type ValidationWarning struct {
	File    string // empty for non-file-specific warnings
	Message string
}

// Result collects transfer statistics, errors, and validation results.
type Result struct {
	Service   string
	Operation string // "download", "upload", "import"
	Dir       string // output dir (download) or input dir (upload)

	Expected   int // expected total (e.g., API-reported photo count); 0 if unknown
	Succeeded  int
	Failed     int
	Skipped    int
	TotalBytes int64

	Scanned bool // true when counts come from directory scan (S3) rather than per-file tracking

	Errors   []FileError
	Warnings []ValidationWarning

	StartTime time.Time
	EndTime   time.Time
}

// NewResult creates a Result and records the start time.
func NewResult(service, operation, dir string) *Result {
	return &Result{
		Service:   service,
		Operation: operation,
		Dir:       dir,
		StartTime: time.Now(),
	}
}

// RecordSuccess increments the success counter and accumulates bytes.
func (r *Result) RecordSuccess(_ string, sizeBytes int64) {
	r.Succeeded++
	r.TotalBytes += sizeBytes
}

// RecordError records a per-file failure.
func (r *Result) RecordError(filename, reason string) {
	r.Failed++
	r.Errors = append(r.Errors, FileError{File: filename, Reason: reason})
}

// RecordSkip adds to the skipped count (already-transferred files).
func (r *Result) RecordSkip(count int) {
	r.Skipped += count
}

// Finish records the end time.
func (r *Result) Finish() {
	r.EndTime = time.Now()
}

// Duration returns the elapsed transfer time.
func (r *Result) Duration() time.Duration {
	if r.EndTime.IsZero() {
		return time.Since(r.StartTime)
	}
	return r.EndTime.Sub(r.StartTime)
}

// Validate runs post-transfer checks: count verification and zero-size file detection.
func (r *Result) Validate() {
	// Check expected vs actual count — includes failed since those were accounted for
	accounted := r.Succeeded + r.Skipped + r.Failed
	if r.Expected > 0 && accounted != r.Expected {
		r.Warnings = append(r.Warnings, ValidationWarning{
			Message: fmt.Sprintf("expected %d files but processed %d (succeeded=%d, skipped=%d, failed=%d)",
				r.Expected, accounted, r.Succeeded, r.Skipped, r.Failed),
		})
	}

	// Check for zero-size files in the output directory
	if r.Dir != "" && (r.Operation == "download" || r.Operation == "import") {
		entries, err := os.ReadDir(r.Dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.Size() == 0 {
				r.Warnings = append(r.Warnings, ValidationWarning{
					File:    e.Name(),
					Message: "zero-size file",
				})
			}
		}
	}
}

// ValidateTransferLog checks that each entry in the transfer log has a corresponding
// file on disk. The resolve function maps a log entry (e.g., photo ID) to a file path,
// returning "" if no matching file exists.
func (r *Result) ValidateTransferLog(logPath string, resolve func(entry string) string) {
	f, err := os.Open(logPath)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		entry := strings.TrimSpace(scanner.Text())
		if entry == "" {
			continue
		}
		if resolve(entry) == "" {
			r.Warnings = append(r.Warnings, ValidationWarning{
				File:    entry,
				Message: fmt.Sprintf("transfer log entry %q has no matching file on disk", entry),
			})
		}
	}
}

// PrintSummary writes a human-readable summary to the logger (stderr).
func (r *Result) PrintSummary(log *logging.Logger) {
	log.Info("")
	log.Info("=== %s %s summary ===", r.Service, r.Operation)

	if r.Scanned {
		log.Info("files in directory: %d", r.Succeeded)
	} else {
		parts := []string{fmt.Sprintf("%d succeeded", r.Succeeded)}
		if r.Skipped > 0 {
			parts = append(parts, fmt.Sprintf("%d skipped", r.Skipped))
		}
		if r.Failed > 0 {
			parts = append(parts, fmt.Sprintf("%d failed", r.Failed))
		}
		log.Info("files: %s", strings.Join(parts, ", "))
	}

	if r.Expected > 0 {
		log.Info("expected: %d", r.Expected)
	}
	log.Info("total size: %s", formatBytes(r.TotalBytes))
	log.Info("duration: %s", r.Duration().Truncate(time.Second))

	if len(r.Errors) > 0 {
		log.Info("")
		log.Info("failed files:")
		for _, e := range r.Errors {
			log.Error("  %s: %s", e.File, e.Reason)
		}
	}

	if len(r.Warnings) > 0 {
		log.Info("")
		log.Info("validation warnings:")
		for _, w := range r.Warnings {
			if w.File != "" {
				log.Info("  [%s] %s", w.File, w.Message)
			} else {
				log.Info("  %s", w.Message)
			}
		}
	}

	if len(r.Errors) == 0 && len(r.Warnings) == 0 {
		log.Info("status: OK")
	}
}

// HandleResult runs validation, prints summary, and writes report.
// If reportDir is empty, falls back to result.Dir.
func HandleResult(result *Result, log *logging.Logger, reportDir string) {
	if result == nil {
		return
	}
	if reportDir == "" {
		reportDir = result.Dir
	}
	result.Validate()
	result.PrintSummary(log)
	if reportPath, err := result.WriteReport(reportDir); err != nil {
		log.Error("writing report: %v", err)
	} else {
		log.Info("report written to %s", reportPath)
	}
}

// WriteReport writes a detailed report file and returns its path.
func (r *Result) WriteReport(dir string) (string, error) {
	filename := fmt.Sprintf("photo-copy-report-%s-%s-%s.txt",
		r.Service, r.Operation, r.StartTime.Format("20060102-150405"))
	reportPath := filepath.Join(dir, filename)

	var b strings.Builder
	fmt.Fprintf(&b, "photo-copy %s %s report\n", r.Service, r.Operation)
	fmt.Fprintf(&b, "generated: %s\n", r.EndTime.Format(time.RFC3339))
	fmt.Fprintf(&b, "duration: %s\n", r.Duration().Truncate(time.Second))
	fmt.Fprintf(&b, "directory: %s\n\n", r.Dir)

	fmt.Fprintf(&b, "--- counts ---\n")
	fmt.Fprintf(&b, "succeeded: %d\n", r.Succeeded)
	fmt.Fprintf(&b, "skipped:   %d\n", r.Skipped)
	fmt.Fprintf(&b, "failed:    %d\n", r.Failed)
	if r.Expected > 0 {
		fmt.Fprintf(&b, "expected:  %d\n", r.Expected)
	}
	fmt.Fprintf(&b, "total size: %s\n\n", formatBytes(r.TotalBytes))

	fmt.Fprintf(&b, "--- errors ---\n")
	if len(r.Errors) == 0 {
		fmt.Fprintf(&b, "No errors.\n")
	} else {
		for _, e := range r.Errors {
			fmt.Fprintf(&b, "%s: %s\n", e.File, e.Reason)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintf(&b, "--- validation ---\n")
	if len(r.Warnings) == 0 {
		fmt.Fprintf(&b, "No warnings.\n")
	} else {
		for _, w := range r.Warnings {
			if w.File != "" {
				fmt.Fprintf(&b, "[%s] %s\n", w.File, w.Message)
			} else {
				fmt.Fprintf(&b, "%s\n", w.Message)
			}
		}
	}

	return reportPath, os.WriteFile(reportPath, []byte(b.String()), 0644)
}

// ScanDir counts files and sizes in r.Dir. Used by S3 after rclone completes.
func (r *Result) ScanDir() error {
	r.Scanned = true
	entries, err := os.ReadDir(r.Dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		r.Succeeded++
		r.TotalBytes += info.Size()
	}
	return nil
}

// formatBytes returns a human-readable byte size.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
