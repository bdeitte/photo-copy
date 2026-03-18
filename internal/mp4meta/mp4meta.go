// Package mp4meta provides utilities for editing MP4/MOV container metadata.
package mp4meta

import (
	"encoding/binary"
	"fmt"
	"html"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	gomp4 "github.com/abema/go-mp4"
)

// xmpUUID is the UUID identifying an XMP metadata box in an MP4/MOV container.
var xmpUUID = []byte{0xBE, 0x7A, 0xCF, 0xCB, 0x97, 0xA9, 0x42, 0xE8, 0x9C, 0x71, 0x99, 0x94, 0x91, 0xE3, 0xAF, 0xAC}

// XMPMetadata holds XMP metadata fields to embed in an MP4/MOV file.
type XMPMetadata struct {
	Title       string
	Description string
	Tags        []string
}

// escapeXML escapes s for safe inclusion in XML content.
func escapeXML(s string) string {
	return html.EscapeString(s)
}

// buildMP4XMPPacket constructs a complete XMP packet as bytes using Dublin Core (dc) namespace.
// Empty fields are omitted from the output.
func buildMP4XMPPacket(meta XMPMetadata) []byte {
	var sb strings.Builder

	sb.WriteString(`<?xpacket begin="` + "\xef\xbb\xbf" + `" id="W5M0MpCehiHzreSzNTczkc9d"?>`)
	sb.WriteString(`<x:xmpmeta xmlns:x="adobe:ns:meta/">`)
	sb.WriteString(`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">`)
	sb.WriteString(`<rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/">`)

	if meta.Title != "" {
		sb.WriteString(`<dc:title><rdf:Alt><rdf:li xml:lang="x-default">`)
		sb.WriteString(escapeXML(meta.Title))
		sb.WriteString(`</rdf:li></rdf:Alt></dc:title>`)
	}

	if meta.Description != "" {
		sb.WriteString(`<dc:description><rdf:Alt><rdf:li xml:lang="x-default">`)
		sb.WriteString(escapeXML(meta.Description))
		sb.WriteString(`</rdf:li></rdf:Alt></dc:description>`)
	}

	if len(meta.Tags) > 0 {
		sb.WriteString(`<dc:subject><rdf:Bag>`)
		for _, tag := range meta.Tags {
			sb.WriteString(`<rdf:li>`)
			sb.WriteString(escapeXML(tag))
			sb.WriteString(`</rdf:li>`)
		}
		sb.WriteString(`</rdf:Bag></dc:subject>`)
	}

	sb.WriteString(`</rdf:Description>`)
	sb.WriteString(`</rdf:RDF>`)
	sb.WriteString(`</x:xmpmeta>`)
	sb.WriteString(`<?xpacket end="w"?>`)

	return []byte(sb.String())
}

// buildUUIDBox builds a UUID box containing the given payload with the XMP UUID.
func buildUUIDBox(payload []byte) []byte {
	size := uint32(8 + 16 + len(payload)) // 4 size + 4 "uuid" + 16 UUID + payload
	box := make([]byte, 0, size)
	box = binary.BigEndian.AppendUint32(box, size)
	box = append(box, 'u', 'u', 'i', 'd')
	box = append(box, xmpUUID...)
	box = append(box, payload...)
	return box
}

// insertOrReplaceUUIDBox walks the top-level MP4 boxes in data, skipping any
// existing XMP UUID box, and inserts a new one after the "moov" box (or at EOF).
func insertOrReplaceUUIDBox(data []byte, xmpPayload []byte) ([]byte, error) {
	out := make([]byte, 0, len(data)+8+16+len(xmpPayload))
	pos := 0
	moovFound := false

	for pos+8 <= len(data) {
		boxSize := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		boxType := string(data[pos+4 : pos+8])

		headerSize := 8
		switch boxSize {
		case 0:
			// Extends to EOF.
			boxSize = len(data) - pos
		case 1:
			// 64-bit extended size.
			if pos+16 > len(data) {
				// Malformed: copy remaining and stop.
				out = append(out, data[pos:]...)
				return out, nil
			}
			extSize := binary.BigEndian.Uint64(data[pos+8 : pos+16])
			if extSize > uint64(len(data)) {
				// Invalid: copy remaining and stop.
				out = append(out, data[pos:]...)
				return out, nil
			}
			boxSize = int(extSize)
			headerSize = 16
		}

		if boxSize < headerSize || pos+boxSize > len(data) {
			// Malformed box: copy remaining and stop.
			out = append(out, data[pos:]...)
			return out, nil
		}

		// Skip existing XMP UUID box.
		if boxType == "uuid" && pos+8+16 <= pos+boxSize {
			if string(data[pos+8:pos+24]) == string(xmpUUID) {
				pos += boxSize
				continue
			}
		}

		// Copy this box through.
		out = append(out, data[pos:pos+boxSize]...)
		pos += boxSize

		// After moov, insert the new XMP UUID box.
		if boxType == "moov" {
			out = append(out, buildUUIDBox(xmpPayload)...)
			moovFound = true
		}
	}

	// If no moov found, append at end.
	if !moovFound {
		out = append(out, buildUUIDBox(xmpPayload)...)
	}

	return out, nil
}

// SetXMPMetadata writes XMP metadata into an MP4/MOV file as a UUID box.
// It inserts or replaces any existing XMP UUID box. The original file is only
// replaced on success; permissions are preserved.
func SetXMPMetadata(filePath string, meta XMPMetadata) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("stat %s: %w", filePath, err)
	}
	perm := info.Mode().Perm()

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filePath, err)
	}

	if len(data) < 8 {
		return fmt.Errorf("%s is too small to be a valid MP4 file", filePath)
	}

	// Basic validation: check for a known MP4 box type in the first box.
	firstType := string(data[4:8])
	validTypes := map[string]bool{"ftyp": true, "moov": true, "mdat": true, "free": true, "skip": true, "wide": true}
	if !validTypes[firstType] {
		return fmt.Errorf("%s does not appear to be a valid MP4 file (first box type: %q)", filePath, firstType)
	}

	xmpPayload := buildMP4XMPPacket(meta)

	result, err := insertOrReplaceUUIDBox(data, xmpPayload)
	if err != nil {
		return fmt.Errorf("processing %s: %w", filePath, err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(filePath), "*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	_, writeErr := tmp.Write(result)
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

// mp4Epoch is the offset in seconds between Unix epoch (1970-01-01) and
// the MP4/QuickTime epoch (1904-01-01).
const mp4Epoch = 2082844800

// SetCreationTime sets the creation and modification timestamps in the
// mvhd, tkhd, and mdhd boxes of an MP4 or MOV file. It writes to a temp
// file and renames over the original on success.
func SetCreationTime(filePath string, t time.Time) error {
	mp4Seconds := t.Unix() + mp4Epoch
	if mp4Seconds < 0 {
		return fmt.Errorf("date %v is before MP4 epoch (1904-01-01)", t)
	}
	mp4Time := uint64(mp4Seconds)

	in, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening %s: %w", filePath, err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.CreateTemp(filepath.Dir(filePath), "*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := out.Name()

	w := gomp4.NewWriter(out)

	_, err = gomp4.ReadBoxStructure(in, func(h *gomp4.ReadHandle) (any, error) {
		switch h.BoxInfo.Type {
		case gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia():
			_, err := w.StartBox(&h.BoxInfo)
			if err != nil {
				return nil, err
			}
			val, err := h.Expand()
			if err != nil {
				return nil, err
			}
			_, err = w.EndBox()
			return val, err

		case gomp4.BoxTypeMvhd():
			return nil, rewriteMvhd(h, w, mp4Time)

		case gomp4.BoxTypeTkhd():
			return nil, rewriteTkhd(h, w, mp4Time)

		case gomp4.BoxTypeMdhd():
			return nil, rewriteMdhd(h, w, mp4Time)

		default:
			return nil, w.CopyBox(in, &h.BoxInfo)
		}
	})

	closeErr := out.Close()
	_ = in.Close()

	if err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("processing MP4: %w", err)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", closeErr)
	}

	if renameErr := os.Rename(tmpPath, filePath); renameErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", renameErr)
	}
	return nil
}

func rewriteMvhd(h *gomp4.ReadHandle, w *gomp4.Writer, mp4Time uint64) error {
	box, _, err := h.ReadPayload()
	if err != nil {
		return err
	}
	mvhd := box.(*gomp4.Mvhd)

	if mvhd.GetVersion() == 0 {
		if mp4Time > math.MaxUint32 {
			return fmt.Errorf("timestamp overflows version 0 mvhd (32-bit)")
		}
		mvhd.CreationTimeV0 = uint32(mp4Time)
		mvhd.ModificationTimeV0 = uint32(mp4Time)
	} else {
		mvhd.CreationTimeV1 = mp4Time
		mvhd.ModificationTimeV1 = mp4Time
	}

	bi, err := w.StartBox(&h.BoxInfo)
	if err != nil {
		return err
	}
	if _, err := gomp4.Marshal(w, mvhd, bi.Context); err != nil {
		return err
	}
	_, err = w.EndBox()
	return err
}

func rewriteTkhd(h *gomp4.ReadHandle, w *gomp4.Writer, mp4Time uint64) error {
	box, _, err := h.ReadPayload()
	if err != nil {
		return err
	}
	tkhd := box.(*gomp4.Tkhd)

	if tkhd.GetVersion() == 0 {
		if mp4Time > math.MaxUint32 {
			return fmt.Errorf("timestamp overflows version 0 tkhd (32-bit)")
		}
		tkhd.CreationTimeV0 = uint32(mp4Time)
		tkhd.ModificationTimeV0 = uint32(mp4Time)
	} else {
		tkhd.CreationTimeV1 = mp4Time
		tkhd.ModificationTimeV1 = mp4Time
	}

	bi, err := w.StartBox(&h.BoxInfo)
	if err != nil {
		return err
	}
	if _, err := gomp4.Marshal(w, tkhd, bi.Context); err != nil {
		return err
	}
	_, err = w.EndBox()
	return err
}

func rewriteMdhd(h *gomp4.ReadHandle, w *gomp4.Writer, mp4Time uint64) error {
	box, _, err := h.ReadPayload()
	if err != nil {
		return err
	}
	mdhd := box.(*gomp4.Mdhd)

	if mdhd.GetVersion() == 0 {
		if mp4Time > math.MaxUint32 {
			return fmt.Errorf("timestamp overflows version 0 mdhd (32-bit)")
		}
		mdhd.CreationTimeV0 = uint32(mp4Time)
		mdhd.ModificationTimeV0 = uint32(mp4Time)
	} else {
		mdhd.CreationTimeV1 = mp4Time
		mdhd.ModificationTimeV1 = mp4Time
	}

	bi, err := w.StartBox(&h.BoxInfo)
	if err != nil {
		return err
	}
	if _, err := gomp4.Marshal(w, mdhd, bi.Context); err != nil {
		return err
	}
	_, err = w.EndBox()
	return err
}
