// summary_test.go exercises the package's external interface.
// It covers Parse's and ParseData's accept/reject table, Path's join, and CommitMessage's subject/body composition.

package summaryparser_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeSummaryFile writes raw content to path, creating its parent directory first and failing the test on any error.
func writeSummaryFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

// TestParse asserts a well-formed summary.md, read from its file or given as bytes, parses into its Title and Body.
// The heading line is excluded from Body, and Body's leading newline is preserved.
// Every malformed or missing file is rejected loud, wrapping its own sentinel where the failure has one.
func TestParse(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	missing := "\x00missing"
	tests := []struct {
		name      string
		content   string
		wantTitle string
		wantBody  string
		wantErr   error
		// wantAnyErr marks a failure with no sentinel of its own (a missing file).
		wantAnyErr bool
	}{
		{
			name:      "title and body",
			content:   "# Added the frobnicator\n\nThe frobnicator now handles widgets.\nIt deviates from the plan by also handling gadgets.\n",
			wantTitle: "Added the frobnicator",
			wantBody:  "\nThe frobnicator now handles widgets.\nIt deviates from the plan by also handling gadgets.\n",
		},
		{
			// The first non-blank line must be the heading, not necessarily the file's first line.
			name:      "leading blank lines skipped",
			content:   "\n\n# Title after blank lines\nBody text.\n",
			wantTitle: "Title after blank lines",
			wantBody:  "Body text.\n",
		},
		{name: "missing file", content: missing, wantAnyErr: true},
		{name: "zero bytes", content: "", wantErr: summaryparser.ErrEmptyFileForTest},
		{name: "blank lines only", content: "\n\n   \n", wantErr: summaryparser.ErrEmptyFileForTest},
		{name: "no heading", content: "Just some narrative with no heading at all.\n", wantErr: summaryparser.ErrNoHeadingForTest},
		// A blank title trims to a bare "#", so Parse reports it as a missing heading;
		// the empty-title sentinel guards a branch that Parse's own trimming makes unreachable today.
		{name: "bare hash", content: "# \n", wantErr: summaryparser.ErrNoHeadingForTest},
		{name: "blank title", content: "#    \nBody text.\n", wantErr: summaryparser.ErrNoHeadingForTest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), summaryparser.FileName)
			// Every row with content parses the same from the file and from the bytes.
			parses := map[string]func() (*summaryparser.Summary, error){
				"Parse": func() (*summaryparser.Summary, error) { return summaryparser.Parse(path) },
			}
			if tt.content != missing {
				writeSummaryFile(t, path, tt.content)
				parses["ParseData"] = func() (*summaryparser.Summary, error) {
					return summaryparser.ParseData([]byte(tt.content), "archived")
				}
			}

			for name, parse := range parses {
				got, err := parse()
				if tt.wantAnyErr {
					if err == nil {
						t.Errorf("%s() error = nil; want an error", name)
					}
					continue
				}
				if tt.wantErr != nil {
					if !errors.Is(err, tt.wantErr) {
						t.Errorf("%s() error = %v; want errors.Is %v", name, err, tt.wantErr)
					}
					continue
				}
				if err != nil {
					t.Errorf("%s() error = %v; want nil", name, err)
					continue
				}
				if got.Title != tt.wantTitle {
					t.Errorf("%s() Title = %q; want %q", name, got.Title, tt.wantTitle)
				}
				if got.Body != tt.wantBody {
					t.Errorf("%s() Body = %q; want %q", name, got.Body, tt.wantBody)
				}
			}
		})
	}
}

// TestPath asserts Path joins the told directory with FileName.
func TestPath(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	got := summaryparser.Path("/some/told/dir")
	want := filepath.Join("/some/told/dir", "summary.md")
	if got != want {
		t.Errorf("Path(%q) = %q; want %q", "/some/told/dir", got, want)
	}
}

// TestCommitMessage covers the cases of the commit message body trim.
//
//testtiming:keep pins the CommitMessage trim cases that LandingMessage's test does not
func TestCommitMessage(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
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
			t.Parallel()
			s := &summaryparser.Summary{Title: tt.title, Body: tt.body}
			got := s.CommitMessage()
			if got != tt.want {
				t.Errorf("CommitMessage() = %q; want %q", got, tt.want)
			}
		})
	}
}
