package main

import (
	"testing"
)

func TestReflowText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "two_sentences_split",
			input: "The script renames the column. The migration touched three files.\n",
			want:  "The script renames the column.\nThe migration touched three files.\n",
		},
		{
			name:  "clause_boundary_comma_and_pronoun",
			input: "The migration touched three files, and the review took three rounds to catch a stale reference.\n",
			want:  "The migration touched three files,\nand the review took three rounds to catch a stale reference.\n",
		},
		{
			name:  "compound_predicate_not_split",
			input: "The script renames the column and updates the model.\n",
			want:  "The script renames the column and updates the model.\n",
		},
		{
			name:  "semicolon_split",
			input: "Use a plain newline; never a backslash.\n",
			want:  "Use a plain newline;\nnever a backslash.\n",
		},
		{
			name:  "abbreviation_not_split",
			input: "This applies to headings, e.g. this one, and to lists.\n",
			want:  "This applies to headings, e.g. this one, and to lists.\n",
		},
		{
			name:  "code_span_period_preserved",
			input: "Run `go test ./...` to check. It should pass.\n",
			want:  "Run `go test ./...` to check.\nIt should pass.\n",
		},
		{
			name:  "code_span_with_embedded_backtick",
			input: "The value `` `quoted` `` is a code span. It has one backtick inside.\n",
			want:  "The value `` `quoted` `` is a code span.\nIt has one backtick inside.\n",
		},
		{
			name:  "unmatched_backtick_left_alone",
			input: "Don't use `sed here. It prompts for permission.\n",
			want:  "Don't use `sed here.\nIt prompts for permission.\n",
		},
		{
			name:  "markdown_link_preserved",
			input: "See [the doc](https://example.com/a.b.c). It has details.\n",
			want:  "See [the doc](https://example.com/a.b.c).\nIt has details.\n",
		},
		{
			name:  "bare_url_preserved",
			input: "Visit https://example.com/path.ext for more. Then continue.\n",
			want:  "Visit https://example.com/path.ext for more.\nThen continue.\n",
		},
		{
			name: "fenced_code_untouched",
			input: "Some text. More text.\n" +
				"```go\n" +
				"func f() { return }. Not a sentence.\n" +
				"```\n" +
				"After. Fence.\n",
			want: "Some text.\nMore text.\n" +
				"```go\n" +
				"func f() { return }. Not a sentence.\n" +
				"```\n" +
				"After.\nFence.\n",
		},
		{
			name: "nested_fence_different_length",
			input: "Outer text here.\n" +
				"````markdown\n" +
				"```go\n" +
				"code\n" +
				"```\n" +
				"````\n" +
				"Trailing text.\n",
			want: "Outer text here.\n" +
				"````markdown\n" +
				"```go\n" +
				"code\n" +
				"```\n" +
				"````\n" +
				"Trailing text.\n",
		},
		{
			name:  "list_item_split_preserves_marker_and_indent",
			input: "- The script renames the column. It also updates the model.\n",
			want:  "- The script renames the column.\n  It also updates the model.\n",
		},
		{
			name:  "numbered_list_item",
			input: "1. First sentence here. Second sentence here.\n",
			want:  "1. First sentence here.\n   Second sentence here.\n",
		},
		{
			name:  "heading_untouched",
			input: "# A heading. With a period.\n",
			want:  "# A heading. With a period.\n",
		},
		{
			name:  "blockquote_untouched",
			input: "> Quoted text. Stays on one line.\n",
			want:  "> Quoted text. Stays on one line.\n",
		},
		{
			name:  "table_row_untouched",
			input: "| A. B | C. D |\n|---|---|\n",
			want:  "| A. B | C. D |\n|---|---|\n",
		},
		{
			name:  "thematic_break_untouched",
			input: "---\n",
			want:  "---\n",
		},
		{
			name:  "linkdef_untouched",
			input: "[ref]: https://example.com \"Title. With period.\"\n",
			want:  "[ref]: https://example.com \"Title. With period.\"\n",
		},
		{
			name:  "yaml_frontmatter_untouched",
			input: "---\nname: foo\ndescription: One. Two.\n---\nBody one. Body two.\n",
			want:  "---\nname: foo\ndescription: One. Two.\n---\nBody one.\nBody two.\n",
		},
		{
			name:  "no_trailing_newline_preserved",
			input: "One. Two.",
			want:  "One.\nTwo.",
		},
		{
			name:  "quoted_closer_after_period",
			input: "She said \"stop.\" Then left.\n",
			want:  "She said \"stop.\"\nThen left.\n",
		},
		{
			name:  "lowercase_after_period_not_split",
			input: "e.g. this continues lowercase after the period.\n",
			want:  "e.g. this continues lowercase after the period.\n",
		},
		{
			// Regression: the em-dash guard's lookahead window must be
			// rune-based, not byte-based -- a byte-length window can
			// truncate a multi-byte em dash right at the window edge,
			// silently disabling the guard and over-splitting an
			// appositive as if it were a new independent clause.
			name:  "em_dash_appositive_not_split_near_window_edge",
			input: "Pane-shell command strings — argument quoting, the call operator, and the prompt-file read idiom — are built ONLY via `internal/shell`.\n",
			want:  "Pane-shell command strings — argument quoting, the call operator, and the prompt-file read idiom — are built ONLY via `internal/shell`.\n",
		},
		{
			// Regression: a code span inside a markdown link's text (a
			// common pattern for linking to a term's own definition) must
			// not leave a nested, unresolved mask placeholder in the output.
			name:  "code_span_inside_link_text_preserved",
			input: "See the [`hardener`](../manifest/designs/hardener.md) concept. It runs a round loop.\n",
			want:  "See the [`hardener`](../manifest/designs/hardener.md) concept.\nIt runs a round loop.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reflowText(tt.input)
			if got != tt.want {
				t.Errorf("reflowText(%q) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestReflowText_CollapsedInvariant enforces the safety invariant relied on
// by the repo sweep: reflowing must never change content, only line breaks.
func TestReflowText_CollapsedInvariant(t *testing.T) {
	samples := []string{
		"Plain paragraph. With two sentences, and a clause; plus a semicolon.\n",
		"- List item one. With two sentences.\n- List item two, and more text here.\n",
		"# Heading\n\nParagraph after heading. Second sentence follows here.\n\n```go\ncode; here. not prose\n```\n",
		"Mixed `code span. with period` and a [link](http://x.com/a.b). Done.\n",
		"No sentence boundary at all just one long line of words\n",
		"Trailing unmatched ` backtick stays put. Next sentence.\n",
	}
	for _, s := range samples {
		got := reflowText(s)
		if collapsed(got) != collapsed(s) {
			t.Errorf("reflowText(%q) broke collapsed invariant:\n got collapsed: %q\nwant collapsed: %q", s, collapsed(got), collapsed(s))
		}
	}
}

func TestCollapsed(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"single_space", "a b", "a b"},
		{"multi_space", "a   b", "a b"},
		{"newlines", "a\nb\n\nc", "a b c"},
		{"leading_trailing", "  a b  ", "a b"},
		{"tabs", "a\tb", "a b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := collapsed(tt.input); got != tt.want {
				t.Errorf("collapsed(%q) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMaskCodeSpans(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"simple", "before `code` after"},
		{"double_backtick_with_single_inside", "before ``foo ` bar`` after"},
		{"unmatched", "before `unmatched text"},
		{"multiple_spans", "one `a` two `b` three"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var spans []string
			masked := maskCodeSpans(tt.input, &spans)
			got := unmaskText(masked, spans)
			if got != tt.input {
				t.Errorf("mask/unmask round trip: got %q; want %q (masked=%q)", got, tt.input, masked)
			}
		})
	}
}
