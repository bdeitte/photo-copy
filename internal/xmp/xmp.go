// Package xmp provides shared XMP metadata types and Dublin Core packet building.
package xmp

import (
	"html"
	"strings"
	"time"
)

// Metadata holds XMP metadata fields for embedding in media files.
type Metadata struct {
	Title       string
	Description string
	Tags        []string
	CreateDate  time.Time
}

// IsEmpty returns true if all fields are empty.
func (m Metadata) IsEmpty() bool {
	return m.Title == "" && m.Description == "" && len(m.Tags) == 0 && m.CreateDate.IsZero()
}

// EscapeXML escapes s for safe inclusion in XML content.
func EscapeXML(s string) string {
	return html.EscapeString(s)
}

// BuildDublinCorePacket constructs a complete XMP packet as an XML string using
// the Dublin Core (dc) namespace. Empty fields are omitted from the output.
func BuildDublinCorePacket(meta Metadata) string {
	var sb strings.Builder

	sb.WriteString(`<?xpacket begin="` + "\xef\xbb\xbf" + `" id="W5M0MpCehiHzreSzNTczkc9d"?>`)
	sb.WriteString(`<x:xmpmeta xmlns:x="adobe:ns:meta/">`)
	sb.WriteString(`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">`)
	sb.WriteString(`<rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:xmp="http://ns.adobe.com/xap/1.0/">`)

	if meta.Title != "" {
		sb.WriteString(`<dc:title><rdf:Alt><rdf:li xml:lang="x-default">`)
		sb.WriteString(EscapeXML(meta.Title))
		sb.WriteString(`</rdf:li></rdf:Alt></dc:title>`)
	}

	if meta.Description != "" {
		sb.WriteString(`<dc:description><rdf:Alt><rdf:li xml:lang="x-default">`)
		sb.WriteString(EscapeXML(meta.Description))
		sb.WriteString(`</rdf:li></rdf:Alt></dc:description>`)
	}

	if len(meta.Tags) > 0 {
		sb.WriteString(`<dc:subject><rdf:Bag>`)
		for _, tag := range meta.Tags {
			sb.WriteString(`<rdf:li>`)
			sb.WriteString(EscapeXML(tag))
			sb.WriteString(`</rdf:li>`)
		}
		sb.WriteString(`</rdf:Bag></dc:subject>`)
	}

	if !meta.CreateDate.IsZero() {
		sb.WriteString(`<xmp:CreateDate>`)
		sb.WriteString(meta.CreateDate.UTC().Format("2006-01-02T15:04:05Z"))
		sb.WriteString(`</xmp:CreateDate>`)
	}

	sb.WriteString(`</rdf:Description>`)
	sb.WriteString(`</rdf:RDF>`)
	sb.WriteString(`</x:xmpmeta>`)
	sb.WriteString(`<?xpacket end="w"?>`)

	return sb.String()
}
