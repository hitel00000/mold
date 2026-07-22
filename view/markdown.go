package view

import (
	"bytes"
	"html/template"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"
)

var (
	markdownParser = goldmark.New(
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
		),
	)
	sanitizer = bluemonday.UGCPolicy()
)

// RenderMarkdown parses raw markdown text into HTML and applies bluemonday UGCPolicy sanitization
// to prevent stored XSS attacks (<script>, onerror handlers, javascript: URIs, etc.).
func RenderMarkdown(raw string) template.HTML {
	if raw == "" {
		return ""
	}

	var buf bytes.Buffer
	if err := markdownParser.Convert([]byte(raw), &buf); err != nil {
		// Fallback to sanitized plain text on parsing error
		sanitized := sanitizer.Sanitize(raw)
		return template.HTML(sanitized)
	}

	sanitizedBytes := sanitizer.SanitizeBytes(buf.Bytes())
	return template.HTML(sanitizedBytes)
}
