//go:build integration

// pathtracked_integration_test.go pins PathTracked's index-only answer on a real repo.
// It is integration-tagged because the query spawns real git.

package fabricengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

func commitFileForPathTrackedTest(t *testing.T, repoDir, rel string) {
	t.Helper()
	abs := filepath.Join(repoDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(abs, []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	for _, args := range [][]string{
		{"add", "--", rel},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-m", "add " + rel},
	} {
		if _, stderr, exitCode, err := gitexec.RunGit(args, repoDir); err != nil || exitCode != 0 {
			t.Fatalf("git %v: err=%v exit=%d stderr=%s", args, err, exitCode, stderr)
		}
	}
}

func TestPathTracked(t *testing.T) {
	repoDir := newGitRepoForExcludeTest(t)
	commitFileForPathTrackedTest(t, repoDir, "tracked.txt")
	commitFileForPathTrackedTest(t, repoDir, "sub/dir/nested.json")
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
