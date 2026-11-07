// lint_test.go drives the break classification and the line-length check over base and new text pairs.

package commentlint

import (
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// source wraps comment lines in a Go file, one `//` line each, above a declaration.
func source(commentLines ...string) string {
	return "package p\n\n" + strings.Join(commentLines, "\n") + "\nfunc F() {}\n"
}

// flaggedLines returns the line numbers of newText that rule flags.
func flaggedLines(rule Rule, baseText, newText string) []int {
	var lines []int
	for _, finding := range findFileFindings("p.go", baseText, newText) {
		if finding.Rule == rule {
			lines = append(lines, finding.Line)
		}
	}
	return lines
}

// proseOfLength returns a sentence of n characters made of short words; n is at least five.
func proseOfLength(n int) string {
	return strings.Repeat("y", n%5) + strings.Repeat(" word", n/5)[1:] + "."
}

func TestFindNewWrappedBreaks(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	wrapped := source("// the first line of a", "// fixed-column wrapped comment", "// that goes on.")
	flaggedWrapped := []int{3, 4}

	tests := []struct {
		name string
		base string
		new  string
		want []int
	}{
		{"a fixed-column-wrapped new comment is flagged", "", wrapped, flaggedWrapped},
		{"an untouched wrapped block is not flagged", wrapped, wrapped, nil},
		{
			"an untouched wrapped line is not flagged when an edit adds a break elsewhere in the file",
			wrapped,
			wrapped + "\n// one more short line.\n// and another.\nfunc G() {}\n",
			nil,
		},
		{
			"a break an edit creates in a legacy wrapped block is flagged",
			wrapped,
			source("// the first line of a", "// fixed-column wrapped", "// comment that goes on."),
			[]int{4},
		},
		{
			"fixing one flagged break leaves the block's other breaks unflagged",
			wrapped,
			source("// the first line of a fixed-column wrapped comment", "// that goes on."),
			nil,
		},
		{
			"a rename of a legacy wrapped line's last word is not flagged",
			wrapped,
			source("// the first line of one", "// fixed-column wrapped comment", "// that goes on."),
			nil,
		},
		{
			"a rename of a legacy wrapped line's first word is not flagged",
			wrapped,
			source("// the first line of a", "// column-limited wrapped comment", "// that goes on."),
			nil,
		},
		{
			"a break an edit creates beside a rename in a legacy wrapped block is flagged",
			wrapped,
			source("// the first line of a", "// fixed-column folded", "// comment that goes on."),
			[]int{4},
		},
		{
			"a legacy wrapped line pair rewritten in most of its words is flagged",
			wrapped,
			source("// one second row in the", "// fixed-column wrapped comment", "// that goes on."),
			[]int{3},
		},
		{
			"a legacy wrapped line pair with exactly half its words unchanged is flagged",
			source("// a b", "// c d"),
			source("// a x", "// y d"),
			[]int{3},
		},
		{"a sentence end passes", "", source("// ends a sentence.", "// starts the next."), nil},
		{"a sentence end before a closing parenthesis passes", "", source("// ends a sentence (like this.)", "// starts the next."), nil},
		{"a sentence end before a closing quote passes", "", source(`// ends with "a quote."`, "// starts the next."), nil},
		{"a sentence end before a closing backtick passes", "", source("// ends with `code.`", "// starts the next."), nil},
		{"a question mark passes", "", source("// is it a sentence?", "// yes."), nil},
		{"a colon passes", "", source("// the cases are:", "// one, two."), nil},
		{"a semicolon passes", "", source("// first clause;", "// second clause."), nil},
		{"a comma before a conjunction and a clause passes", "", source("// one thing happens,", "// but it does not."), nil},
		{"a comma before yet and a clause passes", "", source("// one thing happens,", "// yet the other does not."), nil},
		{"a comma before so and a clause passes", "", source("// one thing happens,", "// so it does too."), nil},
		{"a comma before and and a pronoun subject passes", "", source("// the gate runs,", "// and it returns the value."), nil},
		{"a comma before but and a determiner subject passes", "", source("// the gate runs,", "// but the gate refuses it."), nil},
		{"a comma before and and a backticked subject passes", "", source("// the gate runs,", "// and `Foo` returns the value."), nil},
		{"a comma before a conjunction and a compound predicate is flagged", "", source("// the gate runs,", "// and returns the value."), []int{3}},
		{"a comma before a conjunction and a lone backticked identifier is flagged", "", source("// the gate runs,", "// and `Foo`."), []int{3}},
		{"a comma before a conjunction and a subject with no verb is flagged", "", source("// the gate runs,", "// and the value."), []int{3}},
		{"a comma before a conjunction and a subject the opener set misses is flagged", "", source("// the gate runs,", "// and nothing runs."), []int{3}},
		{"an untouched legacy comma-conjunction break is not checked", source("// the gate runs,", "// and returns the value."), source("// the gate runs,", "// and returns the value."), nil},
		{"a comma before anything else is flagged", "", source("// one thing, then", "// another."), []int{3}},
		{"a block's blank separator line ends the paragraph", "", source("// first paragraph without a stop", "//", "// second paragraph."), nil},
		{"a directive line ends its block, so the prose above it is still checked", "", source("// a wrapped", "// line", "//lym:guard"), []int{3}},
		{"an indented code paragraph is never flagged", "", source("// a wrapped", "// line", "//\tcode"), nil},
		{"a list item paragraph is never flagged", "", source("// a wrapped", "// line", "// - item"), nil},
		{"a heading paragraph is never flagged", "", source("// a wrapped", "// line", "// # Heading"), nil},
		{"a wrapped paragraph beside an indented code paragraph is flagged", "", source("//\tcode", "//", "// a wrapped", "// line."), []int{5}},
		{"a wrapped paragraph beside a heading paragraph is flagged", "", source("// # Heading", "//", "// a wrapped", "// line."), []int{5}},
		{"a wrapped paragraph beside a list paragraph is flagged", "", source("// - item", "//", "// a wrapped", "// line."), []int{5}},
		{"an end-of-line comment is no block", "", "package p\n\nvar x = 1 // wrapped\n// line.\n", nil},
		{"a generated file is never flagged", "", "// Code generated by tool. DO NOT EDIT.\n\n" + wrapped, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := flaggedLines(RuleWrappedBreak, tt.base, tt.new); !slices.Equal(got, tt.want) {
				t.Errorf("flagged lines = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestFindNewLongLines(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	atLimit := "// " + proseOfLength(LineLengthLimit)
	longURL := "https://example.com/" + strings.Repeat("a", LineLengthLimit)
	longSpan := "`" + proseOfLength(LineLengthLimit) + "`"

	tests := []struct {
		name string
		base string
		new  string
		want []int
	}{
		{"a new line one character under the limit, the space after the slashes uncounted, passes", "", source("// " + proseOfLength(LineLengthLimit-1)), nil},
		{"a new line at the limit is flagged", "", source(atLimit), []int{3}},
		{"indentation and a list marker are not counted", "", source("//   - " + proseOfLength(LineLengthLimit-1)), nil},
		{"a list item at the limit is flagged", "", source("//   - " + proseOfLength(LineLengthLimit)), []int{3}},
		{"an indented code line at the limit is flagged", "", source("//\t" + proseOfLength(LineLengthLimit)), []int{3}},
		{"characters are counted, not bytes", "", source("// " + strings.Repeat("é", LineLengthLimit-1)), nil},
		{"a line carried over unchanged is not flagged", source(atLimit), source("// a new first line.", atLimit), nil},
		{"a line an edit changes is flagged", source(atLimit), source(atLimit + "!"), []int{3}},
		{"a second copy of a long line the base holds once is flagged", source(atLimit), source(atLimit, atLimit), []int{4}},
		{"a long line moved within the file is not flagged", source(atLimit, "// a."), source("// a.", atLimit), nil},
		{"a heading is exempt", "", source("// # " + proseOfLength(LineLengthLimit)), nil},
		{"a level-two heading is exempt", "", source("// ## " + proseOfLength(LineLengthLimit)), nil},
		{"a blockquote is exempt", "", source("// > " + proseOfLength(LineLengthLimit)), nil},
		{"a table row is exempt", "", source("// | " + proseOfLength(LineLengthLimit) + " |"), nil},
		{"a URL that alone reaches the limit is exempt", "", source("// See " + longURL + " for it."), nil},
		{"a code span that alone reaches the limit is exempt", "", source("// Run " + longSpan + " once."), nil},
		{"a directive line is not checked", "", source("//lym:" + proseOfLength(LineLengthLimit)), nil},
		{"an end-of-line comment is not checked", "", "package p\n\nvar x = 1 // " + proseOfLength(LineLengthLimit) + "\n", nil},
		{"a generated file is never flagged", "", "// Code generated by tool. DO NOT EDIT.\n\n" + source(atLimit), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := flaggedLines(RuleLineLength, tt.base, tt.new); !slices.Equal(got, tt.want) {
				t.Errorf("flagged lines = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestFindFileFindingsOrdersByLine(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	atLimit := "// " + proseOfLength(LineLengthLimit)
	tests := []struct {
		name string
		new  string
		want []Finding
	}{
		{
			"a long line before a wrapped break comes first",
			source(atLimit, "// a wrapped", "// line."),
			[]Finding{
				{File: "p.go", Line: 3, Rule: RuleLineLength, Text: atLimit},
				{File: "p.go", Line: 4, Rule: RuleWrappedBreak, Text: "// a wrapped"},
			},
		},
		{
			"a wrapped break before a long line comes first",
			source("// a wrapped", "// line.", atLimit),
			[]Finding{
				{File: "p.go", Line: 3, Rule: RuleWrappedBreak, Text: "// a wrapped"},
				{File: "p.go", Line: 5, Rule: RuleLineLength, Text: atLimit},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := findFileFindings("p.go", "", tt.new); !slices.Equal(got, tt.want) {
				t.Errorf("findFileFindings() = %+v; want %+v", got, tt.want)
			}
		})
	}
}

func TestFindingString(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	finding := Finding{File: "a/b.go", Line: 7, Rule: RuleLineLength, Text: "// text"}
	if got, want := finding.String(), "a/b.go:7: line length: // text"; got != want {
		t.Errorf("Finding.String() = %q; want %q", got, want)
	}
}

func TestParseNameStatus(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	got := parseNameStatus("M\x00a.go\x00A\x00b.go\x00D\x00c.go\x00R100\x00old.go\x00new.go\x00")
	want := []fileChange{
		{path: "a.go", basePath: "a.go"},
		{path: "b.go"},
		{path: "new.go", basePath: "old.go"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("parseNameStatus() = %+v; want %+v", got, want)
	}
}
