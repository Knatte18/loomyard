//go:build tier1

// derive_integration_test.go drives DeriveTargets over a real repository holding a small Go module.

package impactset

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

// deriveTargets returns DeriveTargets of dir from base, failing the test on an error.
func deriveTargets(t *testing.T, dir, base string) Targets {
	t.Helper()
	targets, err := DeriveTargets(context.Background(), dir, base)
	if err != nil {
		t.Fatalf("DeriveTargets(%q) error = %v; want nil", base, err)
	}
	return targets
}

// packagesOf joins the impacted packages of every module of targets with spaces.
func packagesOf(targets Targets) string {
	var packages []string
	for _, module := range targets.Modules {
		packages = append(packages, module.Packages...)
	}
	return strings.Join(packages, " ")
}

// TestDerive_OverARepository walks one repository through each fallback, a one-package change and the working-tree forms.
// The steps share the repository and run in order, each against the commit the step before left, so the test is parallel as a whole and no step is.
func TestDerive_OverARepository(t *testing.T) {
	t.Parallel()

	t.Run("no go.mod falls back", func(t *testing.T) {
		dir := t.TempDir()
		gitkit.Git(t, dir, "init", "-q")
		writeRepoFile(t, dir, "a/a.go", "package a\n")
		base := commitAll(t, dir, "base")
		if got := deriveTargets(t, dir, base); !got.Whole || got.Fallback != "no go.mod at the worktree root" {
			t.Errorf("DeriveTargets() = %+v; want the no-go.mod fallback", got)
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
		if got := deriveTargets(t, dir, ""); !got.Whole || got.Fallback != "no base commit to diff from" || got.Base != "" {
			t.Errorf("DeriveTargets() = %+v; want the empty-base fallback with no base", got)
		}
	})

	t.Run("a base off HEAD's ancestry falls back", func(t *testing.T) {
		gitkit.Git(t, dir, "checkout", "-q", "-b", "side")
		writeRepoFile(t, dir, "side.txt", "side\n")
		side := commitAll(t, dir, "side")
		gitkit.Git(t, dir, "checkout", "-q", branch)
		got := deriveTargets(t, dir, side)
		if !got.Whole || got.Fallback == "" || got.Base != "" {
			t.Errorf("DeriveTargets() = %+v; want a not-an-ancestor fallback with no base", got)
		}
	})

	writeRepoFile(t, dir, "a/a.go", "package a\n\nfunc F() {}\n")
	commitAll(t, dir, "change a")
	t.Run("a one-package change derives that package's set and the outside guard", func(t *testing.T) {
		got := deriveTargets(t, dir, base)
		if got.Whole || got.Base != base || packagesOf(got) != "./a ./b" || !reflect.DeepEqual(got.Guards, map[string][]GuardTest{"g": {{Name: "TestGuard", Tier: 0}}}) {
			t.Errorf("DeriveTargets() = %+v; want packages ./a ./b on base %s and the guard of g", got, base)
		}
	})

	writeRepoFile(t, dir, "go.mod", "module example.com/m\n\ngo 1.26\n\n// changed\n")
	commitAll(t, dir, "change go.mod")
	t.Run("a go.mod change falls back", func(t *testing.T) {
		if got := deriveTargets(t, dir, base); !got.Whole || got.Fallback != "go.mod changed" {
			t.Errorf("DeriveTargets() = %+v; want the go.mod fallback", got)
		}
	})

	afterGoMod := gitkit.Git(t, dir, "rev-parse", "HEAD")
	if err := os.RemoveAll(filepath.Join(dir, "c")); err != nil {
		t.Fatalf("RemoveAll(c): %v", err)
	}
	commitAll(t, dir, "delete c")
	t.Run("a deleted file whose directory is no longer a package falls back", func(t *testing.T) {
		got := deriveTargets(t, dir, afterGoMod)
		if !got.Whole || got.Fallback != "c/c.go is neither a Markdown file nor under a Go package directory" {
			t.Errorf("DeriveTargets() = %+v; want the deleted-package fallback", got)
		}
	})

	writeRepoFile(t, dir, ".gitignore", "*.txt\n")
	head := commitAll(t, dir, "ignore text files")
	targetsOf := func(t *testing.T) Targets {
		t.Helper()
		return deriveTargets(t, dir, head)
	}
	remove := func(t *testing.T, names ...string) {
		t.Helper()
		for _, name := range names {
			if err := os.RemoveAll(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
				t.Fatalf("RemoveAll(%s): %v", name, err)
			}
		}
	}

	t.Run("an uncommitted edit selects its package", func(t *testing.T) {
		writeRepoFile(t, dir, "a/a.go", "package a\n\nfunc G() {}\n")
		defer gitkit.Git(t, dir, "checkout", "--", "a/a.go")
		if got := targetsOf(t); got.Whole || packagesOf(got) != "./a ./b" || got.Base != head {
			t.Errorf("DeriveTargets() = %+v; want packages ./a ./b on base %s", got, head)
		}
	})

	t.Run("a package reached only through a tier0 test file joins the set of the imported package's change", func(t *testing.T) {
		tiered := t.TempDir()
		gitkit.Git(t, tiered, "init", "-q")
		writeRepoFile(t, tiered, "go.mod", "module example.com/m\n\ngo 1.26\n")
		writeRepoFile(t, tiered, "a/a.go", "package a\n")
		writeRepoFile(t, tiered, "d/d.go", "package d\n")
		writeRepoFile(t, tiered, "d/d_test.go", "//go:build tier0\n\npackage d\n\nimport _ \"example.com/m/a\"\n")
		tierBase := commitAll(t, tiered, "base")
		writeRepoFile(t, tiered, "a/a.go", "package a\n\nfunc H() {}\n")
		if got := deriveTargets(t, tiered, tierBase); got.Whole || packagesOf(got) != "./a ./d" {
			t.Errorf("DeriveTargets() = %+v; want packages ./a ./d, the importer through its tier0 test file included", got)
		}
	})

	t.Run("an untracked new file selects its package", func(t *testing.T) {
		writeRepoFile(t, dir, "n/n.go", "package n\n")
		defer remove(t, "n")
		if got := targetsOf(t); got.Whole || packagesOf(got) != "./n" {
			t.Errorf("DeriveTargets() = %+v; want package ./n", got)
		}
	})

	t.Run("an ignored file selects nothing", func(t *testing.T) {
		writeRepoFile(t, dir, "notes.txt", "x\n")
		defer remove(t, "notes.txt")
		got := targetsOf(t)
		if got.Whole || len(got.Modules) != 0 || !reflect.DeepEqual(got.Guards, map[string][]GuardTest{"g": {{Name: "TestGuard", Tier: 0}}}) {
			t.Errorf("DeriveTargets() = %+v; want no module and the guard of g", got)
		}
	})

	t.Run("a Markdown-only working-tree change yields the empty set", func(t *testing.T) {
		writeRepoFile(t, dir, "notes.md", "x\n")
		defer remove(t, "notes.md")
		if got := targetsOf(t); got.Whole || len(got.Modules) != 0 {
			t.Errorf("DeriveTargets() = %+v; want no module", got)
		}
	})

	t.Run("a file of a nested module falls back to whole", func(t *testing.T) {
		writeRepoFile(t, dir, "tools/go.mod", "module example.com/tools\n\ngo 1.26\n")
		writeRepoFile(t, dir, "tools/x/x.go", "package x\n")
		defer remove(t, "tools")
		if got := targetsOf(t); !got.Whole || got.Fallback == "" {
			t.Errorf("DeriveTargets() = %+v; want a whole fallback", got)
		}
	})

	t.Run("a go.sum edit falls back to whole", func(t *testing.T) {
		writeRepoFile(t, dir, "go.sum", "\n")
		defer remove(t, "go.sum")
		if got := targetsOf(t); !got.Whole || got.Fallback != "go.sum changed" {
			t.Errorf("DeriveTargets() = %+v; want the go.sum fallback", got)
		}
	})

	t.Run("a misplaced guard marker returns the scan error", func(t *testing.T) {
		writeRepoFile(t, dir, "g/h_test.go", "package g\n\n//lyx:guard\nvar misplaced = 1\n")
		defer remove(t, "g/h_test.go")
		if _, err := DeriveTargets(context.Background(), dir, head); !IsGuardScanError(err) {
			t.Errorf("DeriveTargets() error = %v; want a *GuardScanError", err)
		}
	})
}
