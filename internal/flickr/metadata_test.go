package flickr

import (
	"testing"
)

func TestStripHTML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"plain text", "plain text", "plain text"},
		{"simple tag", "<b>bold</b>", "bold"},
		{"link tag", `<a href="http://example.com">click here</a>`, "click here"},
		{"br tags", "line1<br>line2<br/>line3", "line1 line2 line3"},
		{"nested tags", "<div><p>nested <b>bold</b></p></div>", "nested bold"},
		{"html entities", "rock &amp; roll", "rock & roll"},
		{"numeric entity", "&#39;quoted&#39;", "'quoted'"},
		{"heart expression", "I <3 this", "I <3 this"},
		{"multiple spaces", "hello   world", "hello world"},
		{"newlines and tabs", "hello\n\t  world", "hello world"},
		{"leading trailing space", "  hello  ", "hello"},
		{"mixed html and entities", "<p>Tom &amp; Jerry</p>", "Tom & Jerry"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripHTML(tt.input)
			if got != tt.want {
				t.Errorf("stripHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
