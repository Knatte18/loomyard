package discussionparser

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestCheckRelevant asserts the relevant list's format check: a missing list is clean, an unreadable one is an error, and a present one yields the exact check names, in line order, for each defect the contract names.
func TestCheckRelevant(t *testing.T) {
	t.Parallel()

	const (
		missing = "\x00missing"
		dir     = "\x00dir"
	)
	var (
		noEntry   = []string{checkRelevantNoEntry}
		stray     = []string{checkRelevantStrayLine}
		badPath   = []string{checkRelevantPathMalformed}
		noReason  = []string{checkRelevantReasonMissing}
		entryTail = " — why\n"
	)

	tests := []struct {
		name      string
		content   string
		want      []string
		wantLines []int
		wantErr   bool
	}{
		{name: "title and two entries", content: "# Relevant\n\n- `internal/a.go` — why\n- `internal/b.go` — why\n"},
		{name: "entry with members", content: "- `internal/x/y.go` `PlanTable`, `Config.Validate`" + entryTail},
		{name: "duplicate path", content: "- `a.go`" + entryTail + "- `a.go`" + entryTail},
		{name: "blank lines between entries", content: "\n- `a.go`" + entryTail + "\n\n- `b.go`" + entryTail},
		{name: "dot-dot prefix inside a segment", content: "- `..foo/a.go`" + entryTail},
		{name: "no title line", content: "- `a.go`" + entryTail},
		{name: "missing file", content: missing},
		{name: "path is a directory", content: dir, wantErr: true},
		{name: "empty file", content: "", want: noEntry},
		{name: "only a title", content: "# Relevant\n", want: noEntry},
		{name: "paragraph line", content: "- `a.go`" + entryTail + "some prose\n", want: stray},
		{name: "nested bullet", content: "- `a.go`" + entryTail + "  - nested\n", want: stray},
		{name: "star bullet", content: "- `a.go`" + entryTail + "* `b.go`" + entryTail, want: stray},
		{name: "H2", content: "## Files\n- `a.go`" + entryTail, want: stray},
		{name: "second H1 after an entry", content: "- `a.go`" + entryTail + "# Again\n", want: stray},
		{name: "second H1 after the title", content: "# One\n# Two\n- `a.go`" + entryTail, want: stray},
		{name: "bare hash", content: "#\n- `a.go`" + entryTail, want: stray},
		{name: "hash and space", content: "# \n- `a.go`" + entryTail, want: stray},
		{name: "bare bullet", content: "-\n", want: badPath},
		{name: "no backtick", content: "- a.go" + entryTail, want: badPath},
		{name: "unclosed backtick", content: "- `a.go" + entryTail, want: badPath},
		{name: "empty path", content: "- ``" + entryTail, want: badPath},
		{name: "leading slash", content: "- `/a.go`" + entryTail, want: badPath},
		{name: "leading backslash", content: "- `\\a.go`" + entryTail, want: badPath},
		{name: "leading tilde", content: "- `~/a.go`" + entryTail, want: badPath},
		{name: "drive letter", content: "- `C:/x`" + entryTail, want: badPath},
		{name: "dot-dot segment", content: "- `a/../b`" + entryTail, want: badPath},
		{name: "backslash dot-dot segment", content: "- `a\\..\\b`" + entryTail, want: badPath},
		{name: "no separator", content: "- `a.go` why\n", want: noReason},
		{name: "blank reason", content: "- `a.go` —   \n", want: noReason},
		{
			name:      "several defects in line order, each naming its line",
			content:   "- a.go\nprose\n- `b.go`\n",
			want:      []string{checkRelevantPathMalformed, checkRelevantStrayLine, checkRelevantReasonMissing},
			wantLines: []int{1, 2, 3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "relevant.md")
			switch tt.content {
			case missing:
			case dir:
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
			default:
				if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
					t.Fatalf("write relevant list: %v", err)
				}
			}

			findings, err := CheckRelevant(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CheckRelevant() error = %v; want error %t", err, tt.wantErr)
			}
			var got []string
			for _, f := range findings {
				got = append(got, f.Check)
				if f.Path != path {
					t.Errorf("finding %+v Path = %q; want %q", f, f.Path, path)
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("CheckRelevant() checks = %v; want %v", got, tt.want)
			}
			for i, line := range tt.wantLines {
				if want := fmt.Sprintf("line %d:", line); !strings.Contains(findings[i].Detail, want) {
					t.Errorf("finding %d detail = %q; want it to name %q", i, findings[i].Detail, want)
				}
			}
		})
	}
}
