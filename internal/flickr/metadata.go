package flickr

import (
	"html"
	"strings"

	xhtml "golang.org/x/net/html"
)

// flickrDescription handles Flickr's {"_content": "text"} description format.
type flickrDescription struct {
	Content string `json:"_content"`
}

// photoMeta holds parsed metadata for a Flickr photo.
type photoMeta struct {
	Title       string
	Description string
	Tags        []string
}

// isEmpty returns true if all fields are empty.
func (m photoMeta) isEmpty() bool {
	return m.Title == "" && m.Description == "" && len(m.Tags) == 0
}

// buildPhotoMeta constructs a photoMeta from raw Flickr API fields.
// descriptionHTML is stripped of HTML tags, tagsStr is split on whitespace.
func buildPhotoMeta(title, descriptionHTML, tagsStr string) photoMeta {
	desc := stripHTML(descriptionHTML)
	tags := strings.Fields(tagsStr)
	return photoMeta{
		Title:       title,
		Description: desc,
		Tags:        tags,
	}
}

// stripHTML extracts plain text from an HTML string using the x/net/html
// tokenizer. It decodes HTML entities, collapses whitespace, and trims.
func stripHTML(s string) string {
	if s == "" {
		return ""
	}

	// Use the x/net/html tokenizer so malformed HTML (like "I <3 this")
	// is handled gracefully — the tokenizer treats "< " sequences as text.
	tokenizer := xhtml.NewTokenizer(strings.NewReader(s))

	var b strings.Builder
	for {
		tt := tokenizer.Next()
		switch tt {
		case xhtml.ErrorToken:
			// EOF or error — we're done.
			result := b.String()
			result = html.UnescapeString(result)
			// Collapse all whitespace runs to a single space.
			fields := strings.Fields(result)
			return strings.Join(fields, " ")
		case xhtml.TextToken:
			b.Write(tokenizer.Raw())
		case xhtml.SelfClosingTagToken, xhtml.StartTagToken, xhtml.EndTagToken:
			// Replace block-like and br tags with a space to separate words.
			name, _ := tokenizer.TagName()
			tagName := string(name)
			if tagName == "br" || tagName == "p" || tagName == "div" ||
				tagName == "li" || tagName == "tr" || tagName == "td" {
				b.WriteByte(' ')
			}
		}
	}
}
