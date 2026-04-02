package xmp

import (
	"strings"
	"testing"
)

func TestBuildDublinCorePacket_AllFields(t *testing.T) {
	meta := Metadata{
		Title:       "My Photo",
		Description: "A nice sunset",
		Tags:        []string{"sunset", "nature"},
	}
	pkt := BuildDublinCorePacket(meta)

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
			t.Errorf("BuildDublinCorePacket missing %q", want)
		}
	}
}

func TestBuildDublinCorePacket_EmptyFields(t *testing.T) {
	meta := Metadata{
		Title: "Only Title",
	}
	pkt := BuildDublinCorePacket(meta)

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

func TestBuildDublinCorePacket_AllEmpty(t *testing.T) {
	meta := Metadata{}
	pkt := BuildDublinCorePacket(meta)

	if strings.Contains(pkt, `dc:title`) {
		t.Error("dc:title should be omitted when empty")
	}
	if strings.Contains(pkt, `dc:description`) {
		t.Error("dc:description should be omitted when empty")
	}
	if strings.Contains(pkt, `dc:subject`) {
		t.Error("dc:subject should be omitted when empty")
	}
	// Should still have xpacket wrapper
	if !strings.Contains(pkt, `<?xpacket begin=`) {
		t.Error("expected xpacket begin marker")
	}
	if !strings.Contains(pkt, `<?xpacket end="w"?>`) {
		t.Error("expected xpacket end marker")
	}
}

func TestBuildDublinCorePacket_XMLEscaping(t *testing.T) {
	meta := Metadata{
		Title:       `Photo & "Friends"`,
		Description: "",
		Tags:        []string{`<not a tag>`},
	}
	pkt := BuildDublinCorePacket(meta)

	if !strings.Contains(pkt, `Photo &amp; &#34;Friends&#34;`) {
		t.Errorf("title not properly XML-escaped, got packet:\n%s", pkt)
	}
	if !strings.Contains(pkt, `&lt;not a tag&gt;`) {
		t.Errorf("tag not properly XML-escaped, got packet:\n%s", pkt)
	}
}

func TestEscapeXML(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"plain text", "plain text"},
		{"a & b", "a &amp; b"},
		{`"quoted"`, "&#34;quoted&#34;"},
		{"<tag>", "&lt;tag&gt;"},
		{"it's", "it&#39;s"},
		{"", ""},
	}
	for _, tt := range tests {
		got := EscapeXML(tt.input)
		if got != tt.want {
			t.Errorf("EscapeXML(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsEmpty(t *testing.T) {
	tests := []struct {
		name string
		meta Metadata
		want bool
	}{
		{"all empty", Metadata{}, true},
		{"title only", Metadata{Title: "t"}, false},
		{"description only", Metadata{Description: "d"}, false},
		{"tags only", Metadata{Tags: []string{"t"}}, false},
		{"all populated", Metadata{Title: "t", Description: "d", Tags: []string{"t"}}, false},
		{"nil tags", Metadata{Tags: nil}, true},
		{"empty tags slice", Metadata{Tags: []string{}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.meta.IsEmpty(); got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}
