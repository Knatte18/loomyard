// summary_test.go exercises Parse's accept/reject table, Path's join, and CommitMessage's
// subject/body composition -- the read-side coverage this package owns as summaryparser-leaf's
// external interface.

package summaryparser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// writeSummaryFile writes raw content to path, creating its parent
// directory first, failing the test on any error.
func writeSummaryFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

// TestParse_ValidParsesTitleAndBody asserts a well-formed summary.md (a "# <title>" heading followed
// by free-form narrative) parses into its Title and Body exactly, with the heading line itself
// excluded from Body -- and Body's leading newline preserved.
func TestParse_ValidParsesTitleAndBody(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, summaryparser.FileName)
	writeSummaryFile(t, path, "# Added the frobnicator\n\nThe frobnicator now handles widgets.\nIt deviates from the plan by also handling gadgets.\n")

	got, err := summaryparser.Parse(path)
	if err != nil {
		t.Fatalf("Parse() error = %v; want nil", err)
	}
	if got.Title != "Added the frobnicator" {
		t.Errorf("Parse() Title = %q; want %q", got.Title, "Added the frobnicator")
	}
	wantBody := "\nThe frobnicator now handles widgets.\nIt deviates from the plan by also handling gadgets.\n"
	if got.Body != wantBody {
		t.Errorf("Parse() Body = %q; want %q", got.Body, wantBody)
	}
}

// TestParse_LeadingBlankLinesSkipped asserts a heading preceded by blank lines still parses -- the
// first NON-BLANK line is what must be the heading, not necessarily the file's first line.
func TestParse_LeadingBlankLinesSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, summaryparser.FileName)
	writeSummaryFile(t, path, "\n\n# Title after blank lines\nBody text.\n")

	got, err := summaryparser.Parse(path)
	if err != nil {
		t.Fatalf("Parse() error = %v; want nil", err)
	}
	if got.Title != "Title after blank lines" {
		t.Errorf("Parse() Title = %q; want %q", got.Title, "Title after blank lines")
	}
}

// TestParse_MissingFile asserts a missing summary.md is a wrapped error, never a guessed nil result.
func TestParse_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, summaryparser.FileName)

	if _, err := summaryparser.Parse(path); err == nil {
		t.Fatalf("Parse() error = nil; want an error for a missing file")
	}
}

// TestParse_EmptyFile asserts a present-but-empty (or blank-only) summary.md is rejected loud rather
// than parsed as a title-less summary.
func TestParse_EmptyFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"zero bytes", ""},
		{"blank lines only", "\n\n   \n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, summaryparser.FileName)
			writeSummaryFile(t, path, tt.content)

			if _, err := summaryparser.Parse(path); err == nil {
				t.Fatalf("Parse() error = nil; want an error for %q", tt.name)
			}
		})
	}
}

// TestParse_NoHeadingFirstLine asserts a file whose first non-blank line is not a "# " heading is
// rejected loud rather than silently treating the whole file as an untitled body.
func TestParse_NoHeadingFirstLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, summaryparser.FileName)
	writeSummaryFile(t, path, "Just some narrative with no heading at all.\n")

	if _, err := summaryparser.Parse(path); err == nil {
		t.Fatalf("Parse() error = nil; want an error for a missing heading")
	}
}

// TestParse_EmptyTitle asserts a "# " heading whose title is blank (or whitespace-only) is rejected
// loud.
func TestParse_EmptyTitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, summaryparser.FileName)
	writeSummaryFile(t, path, "#    \nBody text.\n")

	if _, err := summaryparser.Parse(path); err == nil {
		t.Fatalf("Parse() error = nil; want an error for an empty title")
	}
}

// TestPath asserts Path joins the told directory with FileName.
func TestPath(t *testing.T) {
	got := summaryparser.Path("/some/told/dir")
	want := filepath.Join("/some/told/dir", "summary.md")
	if got != want {
		t.Errorf("Path(%q) = %q; want %q", "/some/told/dir", got, want)
	}
}

// TestCommitMessage covers the commitmessage-body-trim Shared Decision's named cases.
func TestCommitMessage(t *testing.T) {
	tests := []struct {
		name  string
		title string
		body  string
		want  string
	}{
		{
			name:  "body starts with newline yields exactly one blank line",
			title: "Added the frobnicator",
			body:  "\nThe frobnicator now handles widgets.\n",
			want:  "Added the frobnicator\n\nThe frobnicator now handles widgets.\n",
		},
		{
			name:  "empty body yields bare title with no trailing blank line",
			title: "Added the frobnicator",
			body:  "",
			want:  "Added the frobnicator",
		},
		{
			name:  "whitespace-only body yields bare title with no trailing blank line",
			title: "Added the frobnicator",
			body:  "   \n\t\n",
			want:  "Added the frobnicator",
		},
		{
			name:  "body with no leading blank line is unchanged by the trim",
			title: "Added the frobnicator",
			body:  "The frobnicator now handles widgets.\n",
			want:  "Added the frobnicator\n\nThe frobnicator now handles widgets.\n",
		},
		{
			name:  "trailing whitespace survives untouched",
			title: "Added the frobnicator",
			body:  "\nThe frobnicator now handles widgets.\n\n  ",
			want:  "Added the frobnicator\n\nThe frobnicator now handles widgets.\n\n  ",
		},
		{
			name:  "integration suite failure section reaches the composed message intact",
			title: "Added the frobnicator",
			body:  "\nThe frobnicator now handles widgets.\n\n## Integration suite failed\n\nThe plan-level `## verify:` suite failed. SHA-bisect localized the failure to card `card-3` (commit `abc123`).\n",
			want:  "Added the frobnicator\n\nThe frobnicator now handles widgets.\n\n## Integration suite failed\n\nThe plan-level `## verify:` suite failed. SHA-bisect localized the failure to card `card-3` (commit `abc123`).\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &summaryparser.Summary{Title: tt.title, Body: tt.body}
			got := s.CommitMessage()
			if got != tt.want {
				t.Errorf("CommitMessage() = %q; want %q", got, tt.want)
			}
		})
	}
}
