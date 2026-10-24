//go:build tier1

// patternbase_integration_test.go covers PatternBaseReader against a real git repository, so it carries the integration tag.

package loomshed

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

func TestPatternBaseReader(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	gitkit.Git(t, worktree, "init", "-b", "main")
	gitkit.Git(t, worktree, "config", "user.email", "test@test.com")
	gitkit.Git(t, worktree, "config", "user.name", "Test")
	withoutPattern := gitkit.CommitFile(t, worktree, "a.txt", "a\n", "no overview yet")
	const firstOverview = "- `PATTERN-a` — a.\n"
	const grownOverview = firstOverview + "- `PATTERN-b` — b.\n"
	first := gitkit.CommitFile(t, worktree, "PATTERN.md", firstOverview, "overview")
	gitkit.CommitFile(t, worktree, "PATTERN.md", grownOverview, "grown overview")

	mergeBaseFault := errors.New("no merge base")
	tests := []struct {
		name      string
		mergeBase func() (string, error)
		want      string
		// wantErr is a fragment the returned error holds; empty means no error.
		wantErr string
	}{
		{"nil merge base reads HEAD", nil, grownOverview, ""},
		{"merge base naming the first commit reads its bytes", func() (string, error) { return first, nil }, firstOverview, ""},
		{"revision without the overview reads nil", func() (string, error) { return withoutPattern, nil }, "", ""},
		{"merge base error is returned", func() (string, error) { return "", mergeBaseFault }, "", mergeBaseFault.Error()},
		{"empty merge base is refused", func() (string, error) { return "", nil }, "", "no revision"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := PatternBaseReader(worktree, tt.mergeBase)()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("PatternBaseReader()() error = %v; want one holding %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("PatternBaseReader()() error = %v; want nil", err)
			}
			if string(got) != tt.want || (tt.want == "" && got != nil) {
				t.Errorf("PatternBaseReader()() = %q; want %q", got, tt.want)
			}
		})
	}
}
