package flickr

import (
	"encoding/json"
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

func TestFlickrDescriptionUnmarshal(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"with content", `{"_content": "hello world"}`, "hello world"},
		{"empty content", `{"_content": ""}`, ""},
		{"raw html not stripped", `{"_content": "<b>bold</b>"}`, "<b>bold</b>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d flickrDescription
			if err := json.Unmarshal([]byte(tt.input), &d); err != nil {
				t.Fatalf("json.Unmarshal(%q) error: %v", tt.input, err)
			}
			if d.Content != tt.want {
				t.Errorf("flickrDescription.Content = %q, want %q", d.Content, tt.want)
			}
		})
	}
}

func TestBuildPhotoMeta(t *testing.T) {
	tests := []struct {
		name        string
		title       string
		descHTML    string
		tagsStr     string
		wantTitle   string
		wantDesc    string
		wantTags    []string
		wantIsEmpty bool
	}{
		{
			name:      "all fields populated",
			title:     "My Photo",
			descHTML:  "<p>A great photo</p>",
			tagsStr:   "vacation beach summer",
			wantTitle: "My Photo",
			wantDesc:  "A great photo",
			wantTags:  []string{"vacation", "beach", "summer"},
		},
		{
			name:        "empty fields",
			title:       "",
			descHTML:    "",
			tagsStr:     "",
			wantTitle:   "",
			wantDesc:    "",
			wantTags:    nil,
			wantIsEmpty: true,
		},
		{
			name:        "title only",
			title:       "Just a title",
			descHTML:    "",
			tagsStr:     "",
			wantTitle:   "Just a title",
			wantDesc:    "",
			wantTags:    nil,
			wantIsEmpty: false,
		},
		{
			name:      "tags with extra spaces",
			title:     "",
			descHTML:  "",
			tagsStr:   "  tag1   tag2  tag3  ",
			wantTitle: "",
			wantDesc:  "",
			wantTags:  []string{"tag1", "tag2", "tag3"},
		},
		{
			name:      "description with html",
			title:     "",
			descHTML:  "<b>bold</b> and <i>italic</i>",
			tagsStr:   "",
			wantTitle: "",
			wantDesc:  "bold and italic",
			wantTags:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPhotoMeta(tt.title, tt.descHTML, tt.tagsStr)

			if got.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", got.Title, tt.wantTitle)
			}
			if got.Description != tt.wantDesc {
				t.Errorf("Description = %q, want %q", got.Description, tt.wantDesc)
			}
			if len(got.Tags) != len(tt.wantTags) {
				t.Errorf("Tags = %v, want %v", got.Tags, tt.wantTags)
			} else {
				for i := range got.Tags {
					if got.Tags[i] != tt.wantTags[i] {
						t.Errorf("Tags[%d] = %q, want %q", i, got.Tags[i], tt.wantTags[i])
					}
				}
			}
			if got.isEmpty() != tt.wantIsEmpty {
				t.Errorf("isEmpty() = %v, want %v", got.isEmpty(), tt.wantIsEmpty)
			}
		})
	}
}
