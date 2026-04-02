// Package jpegmeta provides utilities for writing XMP metadata into JPEG files.
package jpegmeta

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/briandeitte/photo-copy/internal/xmp"
)

// xmpNamespace is the Adobe XMP namespace identifier prepended to XMP APP1 payloads.
const xmpNamespace = "http://ns.adobe.com/xap/1.0/\x00"


// SetMetadata writes XMP metadata into a JPEG file, inserting or replacing any existing
// XMP APP1 segment. The original file is only replaced on success; permissions are preserved.
func SetMetadata(filePath string, meta xmp.Metadata) error {
	// Preserve original file permissions.
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("stat %s: %w", filePath, err)
	}
	perm := info.Mode().Perm()

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filePath, err)
	}

	// Verify SOI marker.
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return fmt.Errorf("%s is not a JPEG file (missing SOI marker)", filePath)
	}

	// Build XMP payload = namespace header + XMP packet.
	xmpPayload := xmpNamespace + xmp.BuildDublinCorePacket(meta)
	if len(xmpPayload)+2 > 65535 {
		return fmt.Errorf("XMP payload too large (%d bytes); max is 65533", len(xmpPayload))
	}

	// Build new JPEG by scanning existing segments.
	// Strategy: copy SOI, then scan segments, skip existing XMP APP1, insert new XMP APP1
	// after APP0/EXIF APP1 segments and before SOS.
	out := make([]byte, 0, len(data)+4+len(xmpPayload))
	out = append(out, 0xFF, 0xD8) // SOI

	xmpInserted := false
	i := 2 // start after SOI

	for i+2 <= len(data) {
		if data[i] != 0xFF {
			// Not a marker byte — this shouldn't happen in valid JPEG; just copy remaining.
			out = append(out, data[i:]...)
			break
		}

		marker := data[i+1]

		// Handle markers with no length field (standalone markers).
		if marker == 0xD8 || marker == 0xD9 {
			// SOI or EOI — copy as-is.
			out = append(out, data[i], data[i+1])
			i += 2
			continue
		}

		// All other markers have a 2-byte length field.
		if i+4 > len(data) {
			break
		}
		segLen := int(data[i+2])<<8 | int(data[i+3])
		if i+2+segLen > len(data) {
			// Truncated segment; copy what we have.
			out = append(out, data[i:]...)
			break
		}
		segEnd := i + 2 + segLen
		payload := data[i+4 : segEnd]

		// SOS: insert XMP before this if not yet inserted.
		if marker == 0xDA {
			if !xmpInserted {
				out = appendXMPSegment(out, xmpPayload)
				xmpInserted = true
			}
			// Copy SOS and everything after it verbatim (image data follows).
			out = append(out, data[i:]...)
			break
		}

		// APP1: check if it's an existing XMP segment to skip.
		if marker == 0xE1 && strings.HasPrefix(string(payload), xmpNamespace) {
			// Skip existing XMP APP1.
			i = segEnd
			continue
		}

		// APP0 or APP1 (EXIF): copy, then check if we should insert XMP after.
		isAPP0 := marker == 0xE0
		isExifAPP1 := marker == 0xE1
		out = append(out, data[i:segEnd]...)
		i = segEnd

		// Insert XMP after APP0 or EXIF APP1 segments.
		if !xmpInserted && (isAPP0 || isExifAPP1) {
			out = appendXMPSegment(out, xmpPayload)
			xmpInserted = true
		}
	}

	// If we never found a SOS or APP0/EXIF, insert XMP at end (before EOI if present).
	if !xmpInserted {
		out = appendXMPSegment(out, xmpPayload)
	}

	// Write to temp file then rename over original.
	tmp, err := os.CreateTemp(filepath.Dir(filePath), "*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	_, writeErr := tmp.Write(out)
	closeErr := tmp.Close()

	if writeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", writeErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", closeErr)
	}

	if err := os.Chmod(tmpPath, perm); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", err)
	}

	return nil
}

// appendXMPSegment appends an APP1 segment containing the given XMP payload to buf.
func appendXMPSegment(buf []byte, payload string) []byte {
	segLen := uint16(len(payload) + 2)
	buf = append(buf, 0xFF, 0xE1)
	buf = binary.BigEndian.AppendUint16(buf, segLen)
	buf = append(buf, []byte(payload)...)
	return buf
}
