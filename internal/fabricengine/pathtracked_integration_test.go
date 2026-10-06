//go:build integration

// pathtracked_integration_test.go pins PathTracked's index-only answer on a real repo.
// It is integration-tagged because the query spawns real git.

package fabricengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

func TestPathTracked(t *testing.T) {
	t.Parallel()

	repoDir := newGitRepoForExcludeTest(t)
	gitkit.CommitFile(t, repoDir, "tracked.txt", "x\n", "add tracked.txt")
	gitkit.CommitFile(t, repoDir, "sub/dir/nested.json", "x\n", "add sub/dir/nested.json")
	if err := os.WriteFile(filepath.Join(repoDir, "untracked.txt"), []byte("y\n"), 0o644); err != nil {
		t.Fatalf("write untracked: %v", err)
	}

	cases := []struct {
		name string
		rel  string
		want bool
	}{
		{"committed file", "tracked.txt", true},
		{"untracked file on disk", "untracked.txt", false},
		{"absent path", "missing.txt", false},
		{"committed file in subdirectory", "sub/dir/nested.json", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := PathTracked(repoDir, tc.rel)
			if err != nil {
				t.Fatalf("PathTracked(%q): %v", tc.rel, err)
			}
			if got != tc.want {
				t.Errorf("PathTracked(%q) = %v, want %v", tc.rel, got, tc.want)
			}
		})
	}
}
