//go:build integration

// derive_integration_test.go drives Derive over a real repository holding a small Go module.

package impactset

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

func writeRepoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", name, err)
	}
}

func commitAll(t *testing.T, dir, message string) string {
	t.Helper()
	gitkit.Git(t, dir, "add", "-A")
	gitkit.Git(t, dir, "commit", "-q", "-m", message)
	return gitkit.Git(t, dir, "rev-parse", "HEAD")
}

func derive(t *testing.T, dir, base string) Derivation {
	t.Helper()
	derivation, err := Derive(dir, base)
	if err != nil {
		t.Fatalf("Derive(%q) error = %v; want nil", base, err)
	}
	return derivation
}

// TestDerive_OverARepository walks one repository through each fallback and a one-package change.
// The steps share the repository and run in order, each against the commit the step before left.
func TestDerive_OverARepository(t *testing.T) {
	t.Run("no go.mod falls back", func(t *testing.T) {
		dir := t.TempDir()
		gitkit.Git(t, dir, "init", "-q")
		writeRepoFile(t, dir, "a/a.go", "package a\n")
		base := commitAll(t, dir, "base")
		if got := derive(t, dir, base); got.Command != "" || got.Fallback != "no go.mod at the worktree root" {
			t.Errorf("Derive() = %+v; want the no-go.mod fallback", got)
		}
	})

	dir := t.TempDir()
	gitkit.Git(t, dir, "init", "-q")
	branch := gitkit.Git(t, dir, "symbolic-ref", "--short", "HEAD")
	writeRepoFile(t, dir, "go.mod", "module example.com/m\n\ngo 1.26\n")
	writeRepoFile(t, dir, "a/a.go", "package a\n")
	writeRepoFile(t, dir, "b/b.go", "package b\n\nimport _ \"example.com/m/a\"\n")
	writeRepoFile(t, dir, "c/c.go", "package c\n")
	writeRepoFile(t, dir, "g/g.go", "package g\n")
	writeRepoFile(t, dir, "g/g_test.go", "package g\n\nimport \"testing\"\n\n//lyx:guard\nfunc TestGuard(t *testing.T) {}\n")
	base := commitAll(t, dir, "base")

	t.Run("an empty base falls back", func(t *testing.T) {
		if got := derive(t, dir, ""); got.Command != "" || got.Fallback != "no base commit to diff from" || got.Base != "" {
			t.Errorf("Derive() = %+v; want the empty-base fallback with no base", got)
		}
	})

	t.Run("a base off HEAD's ancestry falls back", func(t *testing.T) {
		gitkit.Git(t, dir, "checkout", "-q", "-b", "side")
		writeRepoFile(t, dir, "side.txt", "side\n")
		side := commitAll(t, dir, "side")
		gitkit.Git(t, dir, "checkout", "-q", branch)
		got := derive(t, dir, side)
		if got.Command != "" || got.Fallback == "" || got.Base != "" {
			t.Errorf("Derive() = %+v; want a not-an-ancestor fallback with no base", got)
		}
	})

	writeRepoFile(t, dir, "a/a.go", "package a\n\nfunc F() {}\n")
	commitAll(t, dir, "change a")
	t.Run("a one-package change derives that package's command", func(t *testing.T) {
		got := derive(t, dir, base)
		want := gateCommand("./a ./b", "go test -tags integration -run '^(TestGuard)$' ./g")
		if got.Command != want || got.Fallback != "" || got.Base != base {
			t.Errorf("Derive() = %+v; want command %q on base %s", got, want, base)
		}
	})

	writeRepoFile(t, dir, "go.mod", "module example.com/m\n\ngo 1.26\n\n// changed\n")
	commitAll(t, dir, "change go.mod")
	t.Run("a go.mod change falls back", func(t *testing.T) {
		if got := derive(t, dir, base); got.Command != "" || got.Fallback != "go.mod changed" || got.Base != base {
			t.Errorf("Derive() = %+v; want the go.mod fallback on base %s", got, base)
		}
	})

	afterGoMod := gitkit.Git(t, dir, "rev-parse", "HEAD")
	if err := os.RemoveAll(filepath.Join(dir, "c")); err != nil {
		t.Fatalf("RemoveAll(c): %v", err)
	}
	commitAll(t, dir, "delete c")
	t.Run("a deleted file whose directory is no longer a package falls back", func(t *testing.T) {
		got := derive(t, dir, afterGoMod)
		if got.Command != "" || got.Fallback != "c/c.go is neither a Markdown file nor under a Go package directory" {
			t.Errorf("Derive() = %+v; want the deleted-package fallback", got)
		}
	})
}
