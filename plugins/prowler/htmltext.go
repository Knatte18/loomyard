// htmltext.go normalizes HTML fragments and full documents down to plain,
// whitespace-tidy text. It backs the "readability failed but the body still
// has usable text" fallback step of the fetch cascade.

package main

import (
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Whitespace-normalization patterns applied, in order, by htmlToText. Grouped
// here so the collapse rules are easy to read together rather than scattered
// across the function body.
var (
	horizontalWhitespaceRun = regexp.MustCompile(`[ \t]+`)
	leadingLineWhitespace   = regexp.MustCompile(`\n[ \t]+`)
	excessBlankLines        = regexp.MustCompile(`\n{3,}`)
)

// htmlToText extracts the visible text of an HTML fragment and normalizes its
// whitespace, so downstream formatting (markdown output, comment bodies) is
// not littered with the raw indentation/line-noise of the source markup.
// script/style/noscript elements are removed first since their contents are
// never meant to be read as page text.
func htmlToText(fragment string) string {
	// Wrap the fragment in a synthetic <div> so goquery can parse it as a
	// document even though it may not be a complete, valid HTML document
	// itself (e.g. a single <p> or a bare text run).
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<div>" + fragment + "</div>"))
	if err != nil {
		return ""
	}
	doc.Find("script, style, noscript").Remove()

	text := doc.Find("div").First().Text()
	return normalizeWhitespace(text)
}

// stripToBodyText extracts the body text of a full HTML document, after
// removing elements that are never part of the readable content (scripts,
// styles, and the page chrome: nav/header/footer). It backs the fallback
// path used when Readability fails to identify an article but the page's
// body still contains extractable prose.
func stripToBodyText(fullHTML string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(fullHTML))
	if err != nil {
		return ""
	}
	doc.Find("script, style, noscript, nav, header, footer").Remove()

	body := doc.Find("body")
	if body.Length() == 0 {
		return ""
	}

	bodyHTML, err := body.Html()
	if err != nil {
		return ""
	}
	return htmlToText(bodyHTML)
}

// normalizeWhitespace collapses the noisy whitespace patterns typical of
// pretty-printed HTML source (runs of spaces/tabs, indented line starts, and
// long blank-line stretches) into a compact, readable plain-text shape.
func normalizeWhitespace(text string) string {
	text = horizontalWhitespaceRun.ReplaceAllString(text, " ")
	text = leadingLineWhitespace.ReplaceAllString(text, "\n")
	text = excessBlankLines.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}
