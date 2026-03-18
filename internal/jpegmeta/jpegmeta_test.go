package jpegmeta

import (
	"strings"
	"testing"
)

// --- Tests for buildXMPPacket ---

func TestBuildXMPPacket_AllFields(t *testing.T) {
	meta := Metadata{
		Title:       "My Photo",
		Description: "A nice sunset",
		Tags:        []string{"sunset", "nature"},
	}
	pkt := buildXMPPacket(meta)

	checks := []string{
		`<?xpacket begin=`,
		`id="W5M0MpCehiHzreSzNTczkc9d"`,
		`<?xpacket end="w"?>`,
		`xmlns:dc="http://purl.org/dc/elements/1.1/"`,
		`<dc:title>`,
		`<rdf:Alt>`,
		`xml:lang="x-default"`,
		`My Photo`,
		`</dc:title>`,
		`<dc:description>`,
		`A nice sunset`,
		`</dc:description>`,
		`<dc:subject>`,
		`<rdf:Bag>`,
		`<rdf:li>sunset</rdf:li>`,
		`<rdf:li>nature</rdf:li>`,
		`</rdf:Bag>`,
		`</dc:subject>`,
	}
	for _, want := range checks {
		if !strings.Contains(pkt, want) {
			t.Errorf("buildXMPPacket missing %q", want)
		}
	}
}

func TestBuildXMPPacket_EmptyFields(t *testing.T) {
	meta := Metadata{
		Title: "Only Title",
	}
	pkt := buildXMPPacket(meta)

	if !strings.Contains(pkt, `<dc:title>`) {
		t.Error("expected dc:title to be present")
	}
	if strings.Contains(pkt, `dc:description`) {
		t.Error("dc:description should be omitted when empty")
	}
	if strings.Contains(pkt, `dc:subject`) {
		t.Error("dc:subject should be omitted when empty")
	}
}

func TestBuildXMPPacket_XMLEscaping(t *testing.T) {
	meta := Metadata{
		Title:       `Photo & "Friends"`,
		Description: "",
		Tags:        []string{`<not a tag>`},
	}
	pkt := buildXMPPacket(meta)

	if !strings.Contains(pkt, `Photo &amp; &#34;Friends&#34;`) {
		t.Errorf("title not properly XML-escaped, got packet:\n%s", pkt)
	}
	if !strings.Contains(pkt, `&lt;not a tag&gt;`) {
		t.Errorf("tag not properly XML-escaped, got packet:\n%s", pkt)
	}
}
