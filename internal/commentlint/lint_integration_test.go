// lint_integration_test.go drives Lint and LintCommitRange over a real repository.

package commentlint

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
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

// TestLint_OverARepository walks one repository through a pure rename and a committed edit.
// It then adds an uncommitted edit and an untracked file.
// The steps share the repository and run in order, so the test is parallel as a whole and no step is.
func TestLint_OverARepository(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	dir := t.TempDir()
	gitkit.Git(t, dir, "init", "-q")
	writeFile(t, dir, "legacy.go", legacyWrapped)
	base := commitAll(t, dir, "legacy wrapped comment")

	gitkit.Git(t, dir, "mv", "legacy.go", "renamed.go")
	afterRename := commitAll(t, dir, "rename")
	t.Run("a pure rename flags nothing", func(t *testing.T) {
		for name, lint := range map[string]func(string, string) ([]Finding, error){"Lint": Lint, "LintCommitRange": LintCommitRange} {
			findings, err := lint(dir, base)
			if err != nil {
				t.Fatalf("%s() error = %v; want nil", name, err)
			}
			if len(findings) != 0 {
				t.Errorf("%s() = %+v; want no findings", name, findings)
			}
		}
	})

	writeFile(t, dir, "added.go", freshWrapped)
	commitAll(t, dir, "add a wrapped comment")
	writeFile(t, dir, "renamed.go", legacyWrapped+"\n// an uncommitted comment that wraps in the\n// middle.\nfunc Edited() {}\n")
	writeFile(t, dir, "untracked.go", freshWrapped)
	t.Run("the commit range form sees committed changes but not uncommitted ones", func(t *testing.T) {
		findings, err := LintCommitRange(dir, afterRename)
		if err != nil {
			t.Fatalf("LintCommitRange() error = %v; want nil", err)
		}
		if got, want := findingKeys(findings), []string{"added.go:3"}; !slices.Equal(got, want) {
			t.Errorf("LintCommitRange() findings = %v; want %v", got, want)
		}
	})
	t.Run("the working tree form against HEAD sees an edit and an untracked file but not legacy lines", func(t *testing.T) {
		findings, err := Lint(dir, "")
		if err != nil {
			t.Fatalf("Lint() error = %v; want nil", err)
		}
		if got, want := findingKeys(findings), []string{"renamed.go:7", "untracked.go:3"}; !slices.Equal(got, want) {
			t.Errorf("Lint() findings = %v; want %v", got, want)
		}
	})
	t.Run("the working tree form against a base sees committed and uncommitted changes", func(t *testing.T) {
		findings, err := Lint(dir, afterRename)
		if err != nil {
			t.Fatalf("Lint() error = %v; want nil", err)
		}
		if got, want := findingKeys(findings), []string{"added.go:3", "renamed.go:7", "untracked.go:3"}; !slices.Equal(got, want) {
			t.Errorf("Lint() findings = %v; want %v", got, want)
		}
	})

	writeFile(t, dir, "indexless.go", legacyWrapped)
	gitkit.Git(t, dir, "add", "indexless.go")
	gitkit.Git(t, dir, "commit", "-q", "-m", "add a legacy wrapped comment")
	withIndexless := gitkit.Git(t, dir, "rev-parse", "HEAD")
	gitkit.Git(t, dir, "rm", "-q", "--cached", "indexless.go")
	gitkit.Git(t, dir, "commit", "-q", "-m", "untrack it")
	t.Run("an untracked file the base holds flags only its new lines", func(t *testing.T) {
		findings, err := Lint(dir, withIndexless)
		if err != nil {
			t.Fatalf("Lint() error = %v; want nil", err)
		}
		if got, want := findingKeys(findings), []string{"renamed.go:7", "untracked.go:3"}; !slices.Equal(got, want) {
			t.Errorf("Lint() findings = %v; want %v", got, want)
		}
	})
}
