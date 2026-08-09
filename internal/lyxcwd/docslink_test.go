// docslink_test.go guards markdown link and anchor integrity under manifest/ and docs/: every
// inline markdown link's file part and #anchor must resolve somewhere in the repo. Its placement in
// internal/lyxcwd is a file-layout convenience reusing repoRootForEnforcement and
// walkEnforcementRoots from enforcement_test.go, not an ownership claim on markdown links by that
// package — see CONSTRAINTS.md's Markdown Link Integrity invariant.

package lyxcwd

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// inlineLinkPattern matches an inline markdown link "[text](target)". Reference-style links
// ("[text][ref]") and angle-bracket autolinks ("<https://...>") do not match this pattern and are
// therefore out of grammar, per the Link grammar decision recorded in _mill/discussion.md.
var inlineLinkPattern = regexp.MustCompile(`\[([^\]]*)\]\(([^)\s]+)\)`)

// atxHeadingPattern matches an ATX heading line ("#" through "######" followed by a space) after
// leading whitespace has been trimmed.
var atxHeadingPattern = regexp.MustCompile(`^#{1,6} `)

// docsLinkSlug implements GitHub's heading-slug rule: strip a leading run of "#" characters and the
// single following space, delete every backtick, lowercase the remainder, delete every rune that is
// not a Unicode letter, a Unicode digit, "_", "-", or a space, then replace each remaining space
// with "-". The deletion step is a deletion, not a replacement — an em-dash surrounded by spaces
// leaves the two spaces behind, which become a double hyphen once spaces are replaced.
func docsLinkSlug(heading string) string {
	s := heading

	i := 0
	for i < len(s) && s[i] == '#' {
		i++
	}
	s = s[i:]
	s = strings.TrimPrefix(s, " ")

	s = strings.ReplaceAll(s, "`", "")
	s = strings.ToLower(s)

	var kept strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == ' ' {
			kept.WriteRune(r)
		}
	}

	return strings.ReplaceAll(kept.String(), " ", "-")
}

// docsLink is one inline markdown link found in a document, carrying its 1-based source line and
// its raw, unresolved target string.
type docsLink struct {
	Line   int
	Target string
}

// docsLinkExtract returns every inline markdown link in data, in document order, with its 1-based
// line number. It tracks fenced code blocks (both ``` and ~~~ fences, opened and closed at line
// start allowing up to three leading spaces) and skips every link-shaped match found inside a
// fence, since a fenced example of a broken link is documentation, not a broken link.
func docsLinkExtract(data []byte) []docsLink {
	var links []docsLink

	inFence := false
	var fenceChar byte

	for i, line := range strings.Split(string(data), "\n") {
		if marker, isFence := fenceMarker(line); isFence {
			if !inFence {
				inFence = true
				fenceChar = marker
			} else if marker == fenceChar {
				inFence = false
			}
			continue
		}
		if inFence {
			continue
		}

		for _, match := range inlineLinkPattern.FindAllStringSubmatch(line, -1) {
			links = append(links, docsLink{Line: i + 1, Target: match[2]})
		}
	}

	return links
}

// fenceMarker reports whether line opens or closes a fenced code block (allowing up to three
// leading spaces before the fence), returning the fence character ('`' or '~') when it does.
func fenceMarker(line string) (byte, bool) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return 0, false
	}
	if strings.HasPrefix(trimmed, "```") {
		return '`', true
	}
	if strings.HasPrefix(trimmed, "~~~") {
		return '~', true
	}
	return 0, false
}

// docsLinkHeadingAnchors returns the set of anchors for every ATX heading in data, in document
// order, skipping headings inside fenced code blocks. Each heading's text is passed through
// docsLinkSlug, and GitHub's duplicate-disambiguation suffixes are applied: the first occurrence of
// a slug is bare, the second gets "-1", the third "-2", and so on.
func docsLinkHeadingAnchors(data []byte) map[string]bool {
	anchors := make(map[string]bool)
	seen := make(map[string]int)

	inFence := false
	var fenceChar byte

	for _, line := range strings.Split(string(data), "\n") {
		if marker, isFence := fenceMarker(line); isFence {
			if !inFence {
				inFence = true
				fenceChar = marker
			} else if marker == fenceChar {
				inFence = false
			}
			continue
		}
		if inFence {
			continue
		}

		trimmed := strings.TrimLeft(line, " ")
		if len(line)-len(trimmed) > 3 || !atxHeadingPattern.MatchString(trimmed) {
			continue
		}

		base := docsLinkSlug(trimmed)
		occurrence := seen[base]
		seen[base] = occurrence + 1
		if occurrence == 0 {
			anchors[base] = true
			continue
		}
		anchors[base+"-"+strconv.Itoa(occurrence)] = true
	}

	return anchors
}

// TestDocsLinkSlug covers GitHub's heading-slug rules against literal data, including the three
// worked examples from _mill/discussion.md's "Link-checker implementation notes" and a fourth case
// for the Fabric Git Invariant heading, since card 5 links depend on exactly that slug.
func TestDocsLinkSlug(t *testing.T) {
	tests := []struct {
		name    string
		heading string
		want    string
	}{
		{
			name:    "phase machine em-dash heading",
			heading: "## The phase machine — a flat producer list, no predefined slots",
			want:    "the-phase-machine--a-flat-producer-list-no-predefined-slots",
		},
		{
			name:    "summary artifact backtick and slash heading",
			heading: "## The summary artifact — `_lyx/webster/summary.md`",
			want:    "the-summary-artifact--_lyxwebstersummarymd",
		},
		{
			name:    "when it runs colon heading",
			heading: "## When it runs: deferred to merge-time, not mid-task",
			want:    "when-it-runs-deferred-to-merge-time-not-mid-task",
		},
		{
			name:    "fabric git invariant parens and plus heading",
			heading: "## Fabric Git Invariant (warp + weft)",
			want:    "fabric-git-invariant-warp--weft",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := docsLinkSlug(tt.heading)
			if got != tt.want {
				t.Errorf("docsLinkSlug(%q) = %q; want %q", tt.heading, got, tt.want)
			}
		})
	}
}

// TestDocsLinkExtract covers the inline-link grammar over literal data: plain links, links whose
// text carries backticks, multiple links on one line, fence skipping for both fence styles, and
// that reference-style links plus autolinks are silently ignored as out of grammar.
func TestDocsLinkExtract(t *testing.T) {
	tests := []struct {
		name string
		data string
		want []docsLink
	}{
		{
			name: "plain inline link",
			data: "See [loom.md](loom.md) for details.",
			want: []docsLink{{Line: 1, Target: "loom.md"}},
		},
		{
			name: "link text with backticks",
			data: "See [`internal/fabricengine`](../../internal/fabricengine/doc.go).",
			want: []docsLink{{Line: 1, Target: "../../internal/fabricengine/doc.go"}},
		},
		{
			name: "two links on one line",
			data: "[a](a.md) and [b](b.md)",
			want: []docsLink{{Line: 1, Target: "a.md"}, {Line: 1, Target: "b.md"}},
		},
		{
			name: "link inside backtick fence is skipped",
			data: "text\n```\n[a](a.md)\n```\nmore",
			want: nil,
		},
		{
			name: "link inside tilde fence is skipped",
			data: "text\n~~~\n[a](a.md)\n~~~\nmore",
			want: nil,
		},
		{
			name: "reference-style link and autolink are ignored",
			data: "[text][ref] and <https://example.com>\n\n[ref]: target.md",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := docsLinkExtract([]byte(tt.data))
			if len(got) != len(tt.want) {
				t.Fatalf("docsLinkExtract(%q) = %v; want %v", tt.data, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("docsLinkExtract(%q)[%d] = %v; want %v", tt.data, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestDocsLinkHeadingAnchors covers anchor-set construction over literal data: distinct headings,
// duplicate-slug disambiguation up to three occurrences, and that a "#"-prefixed line inside a
// fence never counts as a heading.
func TestDocsLinkHeadingAnchors(t *testing.T) {
	tests := []struct {
		name string
		data string
		want map[string]bool
	}{
		{
			name: "distinct headings",
			data: "# One\n## Two\n### Three\n",
			want: map[string]bool{"one": true, "two": true, "three": true},
		},
		{
			name: "two identically-slugging headings",
			data: "## Foo\n## Foo\n",
			want: map[string]bool{"foo": true, "foo-1": true},
		},
		{
			name: "three identically-slugging headings",
			data: "## Foo\n## Foo\n## Foo\n",
			want: map[string]bool{"foo": true, "foo-1": true, "foo-2": true},
		},
		{
			name: "hash-prefixed line inside fence is not a heading",
			data: "```\n# Not A Heading\n```\n## Real Heading\n",
			want: map[string]bool{"real-heading": true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := docsLinkHeadingAnchors([]byte(tt.data))
			if len(got) != len(tt.want) {
				t.Fatalf("docsLinkHeadingAnchors(%q) = %v; want %v", tt.data, got, tt.want)
			}
			for k := range tt.want {
				if !got[k] {
					t.Errorf("docsLinkHeadingAnchors(%q) missing anchor %q; got %v", tt.data, k, got)
				}
			}
		})
	}
}
