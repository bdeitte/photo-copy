// Package jpegmeta provides utilities for writing XMP metadata into JPEG files.
package jpegmeta

import (
	"html"
	"strings"
)

// Metadata holds XMP metadata fields to embed in a JPEG file.
type Metadata struct {
	Title       string
	Description string
	Tags        []string
}

// buildXMPPacket constructs a complete XMP packet as an XML string using Dublin Core (dc) namespace.
// Empty fields are omitted from the output.
func buildXMPPacket(meta Metadata) string {
	var sb strings.Builder

	sb.WriteString(`<?xpacket begin="` + "\xef\xbb\xbf" + `" id="W5M0MpCehiHzreSzNTczkc9d"?>`)
	sb.WriteString(`<x:xmpmeta xmlns:x="adobe:ns:meta/">`)
	sb.WriteString(`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">`)
	sb.WriteString(`<rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/">`)

	if meta.Title != "" {
		sb.WriteString(`<dc:title><rdf:Alt><rdf:li xml:lang="x-default">`)
		sb.WriteString(html.EscapeString(meta.Title))
		sb.WriteString(`</rdf:li></rdf:Alt></dc:title>`)
	}

	if meta.Description != "" {
		sb.WriteString(`<dc:description><rdf:Alt><rdf:li xml:lang="x-default">`)
		sb.WriteString(html.EscapeString(meta.Description))
		sb.WriteString(`</rdf:li></rdf:Alt></dc:description>`)
	}

	if len(meta.Tags) > 0 {
		sb.WriteString(`<dc:subject><rdf:Bag>`)
		for _, tag := range meta.Tags {
			sb.WriteString(`<rdf:li>`)
			sb.WriteString(html.EscapeString(tag))
			sb.WriteString(`</rdf:li>`)
		}
		sb.WriteString(`</rdf:Bag></dc:subject>`)
	}

	sb.WriteString(`</rdf:Description>`)
	sb.WriteString(`</rdf:RDF>`)
	sb.WriteString(`</x:xmpmeta>`)
	sb.WriteString(`<?xpacket end="w"?>`)

	return sb.String()
}
