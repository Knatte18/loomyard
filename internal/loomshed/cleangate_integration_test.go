// cleangate_integration_test.go exercises NewCleanGate against a real scratch repository, at the integration tier.

package loomshed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestCleanGate(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name         string
		dirty        func(t *testing.T, worktree string)
		wantPassed   bool
		wantFindings []string
	}{
		{
			name:       "a clean tree passes",
			dirty:      func(t *testing.T, worktree string) {},
			wantPassed: true,
		},
		{
			name: "an ignored file passes",
			dirty: func(t *testing.T, worktree string) {
				writeTestFile(t, filepath.Join(worktree, ".scratch", "notes.md"), "scratch\n")
			},
			wantPassed: true,
		},
		{
			name: "an untracked file fails naming it",
			dirty: func(t *testing.T, worktree string) {
				writeTestFile(t, filepath.Join(worktree, "200k"), "cautious base est\n")
			},
			wantFindings: []string{"200k", "Commit each one that belongs to the change, delete the rest"},
		},
		{
			name:         "a modified tracked file fails naming it",
			dirty:        func(t *testing.T, worktree string) { writeTestFile(t, filepath.Join(worktree, "a.txt"), "changed\n") },
			wantFindings: []string{"a.txt"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			worktree := t.TempDir()
			gitkit.Git(t, worktree, "init", "-b", "main")
			gitkit.Git(t, worktree, "config", "user.email", "test@test.com")
			gitkit.Git(t, worktree, "config", "user.name", "Test")
			gitkit.CommitFile(t, worktree, ".gitignore", ".scratch/\n", "ignore scratch")
			gitkit.CommitFile(t, worktree, "a.txt", "a\n", "init")
			tt.dirty(t, worktree)

			got, err := NewCleanGate(worktree)()
			if err != nil {
				t.Fatalf("NewCleanGate()() error = %v; want nil", err)
			}
			if got.Passed != tt.wantPassed {
				t.Errorf("NewCleanGate()() Passed = %v; want %v (findings %q)", got.Passed, tt.wantPassed, got.Findings)
			}
			for _, want := range tt.wantFindings {
				if !strings.Contains(got.Findings, want) {
					t.Errorf("NewCleanGate()() Findings = %q; want it to contain %q", got.Findings, want)
				}
			}
		})
	}
}

// writeTestFile writes content at path, creating its directory.
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
