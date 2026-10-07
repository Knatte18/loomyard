//go:build integration

// lint_integration_test.go drives Lint over a real repository.

package commentlint

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

const (
	legacyWrapped = "package p\n\n// a legacy comment that wraps in the\n// middle of a sentence.\nfunc Legacy() {}\n"
	freshWrapped  = "package p\n\n// a fresh comment that wraps in the\n// middle of a sentence.\nfunc Fresh() {}\n"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", name, err)
	}
}

func commitAll(t *testing.T, dir, message string) string {
	t.Helper()
	gitkit.Git(t, dir, "add", "-A")
	gitkit.Git(t, dir, "commit", "-q", "-m", message)
	return gitkit.Git(t, dir, "rev-parse", "HEAD")
}

func findingKeys(findings []Finding) []string {
	var keys []string
	for _, f := range findings {
		keys = append(keys, f.File+":"+strconv.Itoa(f.Line))
	}
	return keys
}

// TestLint_OverARepository walks one repository through a pure rename, a committed edit and an uncommitted edit.
// The steps share the repository and run in order.
func TestLint_OverARepository(t *testing.T) {
	dir := t.TempDir()
	gitkit.Git(t, dir, "init", "-q")
	writeFile(t, dir, "legacy.go", legacyWrapped)
	base := commitAll(t, dir, "legacy wrapped comment")

	gitkit.Git(t, dir, "mv", "legacy.go", "renamed.go")
	afterRename := commitAll(t, dir, "rename")
	t.Run("a pure rename flags nothing", func(t *testing.T) {
		findings, err := Lint(dir, base)
		if err != nil {
			t.Fatalf("Lint() error = %v; want nil", err)
		}
		if len(findings) != 0 {
			t.Errorf("Lint() = %+v; want no findings", findings)
		}
	})

	writeFile(t, dir, "added.go", freshWrapped)
	commitAll(t, dir, "add a wrapped comment")
	t.Run("the base form sees committed changes", func(t *testing.T) {
		findings, err := Lint(dir, afterRename)
		if err != nil {
			t.Fatalf("Lint() error = %v; want nil", err)
		}
		if got, want := findingKeys(findings), []string{"added.go:3"}; !slices.Equal(got, want) {
			t.Errorf("Lint() findings = %v; want %v", got, want)
		}
	})

	writeFile(t, dir, "renamed.go", legacyWrapped+"\n// an uncommitted comment that wraps in the\n// middle.\nfunc Edited() {}\n")
	writeFile(t, dir, "untracked.go", freshWrapped)
	t.Run("the worktree form sees an edit and an untracked file but not legacy lines", func(t *testing.T) {
		findings, err := Lint(dir, "")
		if err != nil {
			t.Fatalf("Lint() error = %v; want nil", err)
		}
		if got, want := findingKeys(findings), []string{"renamed.go:7", "untracked.go:3"}; !slices.Equal(got, want) {
			t.Errorf("Lint() findings = %v; want %v", got, want)
		}
	})
}
