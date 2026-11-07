// repo_test.go tests the exported query wrappers TOC, Glyphs, Resolve and Expand.
// It also declares writeFixtureRepo, the on-disk Go fixture builder that the package's test files share.

package planglyph

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/quarry/quarry"
)

// writeFixtureRepo writes files, keyed by repository-relative path, under a fresh t.TempDir().
// It returns that directory's absolute path, which serves as the repository root.
func writeFixtureRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	return plankit.Repo(t, files)
}

// TestTOC_Success asserts TOC delegates to quarry.Repo.TOC and returns its answer unchanged.
func TestTOC_Success(t *testing.T) {
	tiergate.Require(t, 0)
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})

	answer, err := TOC(root, "sub")
	if err != nil {
		t.Fatalf("TOC(%q, %q) returned error: %v", root, "sub", err)
	}
	if answer.Dir != "sub" {
		t.Errorf("TOC(%q, %q).Dir = %q; want %q", root, "sub", answer.Dir, "sub")
	}
}

// TestTOC_QuarryUnavailable asserts that TOC's error satisfies errors.Is(err, ErrQuarryUnavailable).
// The check applies when the repository itself cannot be opened.
func TestTOC_QuarryUnavailable(t *testing.T) {
	tiergate.Require(t, 0)
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	_, err := TOC(missing, "sub")
	if !errors.Is(err, ErrQuarryUnavailable) {
		t.Errorf("TOC(%q, ...) error = %v; want errors.Is(err, ErrQuarryUnavailable)", missing, err)
	}
}

// TestGlyphs_Success asserts Glyphs delegates to quarry.Repo.Glyphs and returns its answer unchanged.
func TestGlyphs_Success(t *testing.T) {
	tiergate.Require(t, 0)
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})

	answer, err := Glyphs(root, "sub")
	if err != nil {
		t.Fatalf("Glyphs(%q, %q) returned error: %v", root, "sub", err)
	}
	if len(answer.Symbols) != 1 || answer.Symbols[0].ID != "sub#Foo" {
		t.Errorf("Glyphs(%q, %q).Symbols = %+v; want one symbol %q", root, "sub", answer.Symbols, "sub#Foo")
	}
}

// TestResolve_Success asserts Resolve delegates to quarry.Repo.Resolve and returns its result slice unchanged.
func TestResolve_Success(t *testing.T) {
	tiergate.Require(t, 0)
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})

	results, err := Resolve(root, []string{"sub#Foo"})
	if err != nil {
		t.Fatalf("Resolve(%q, ...) returned error: %v", root, err)
	}
	if len(results) != 1 || results[0].Target != "sub#Foo" {
		t.Errorf("Resolve(%q, ...) = %+v; want one result for %q", root, results, "sub#Foo")
	}
}

// TestExpand_Success asserts that Expand returns the underlying quarry's answer unchanged.
func TestExpand_Success(t *testing.T) {
	tiergate.Require(t, 0)
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\ntype T struct{}\n"})

	answer, err := Expand(root, "sub#T")
	if err != nil {
		t.Fatalf("Expand(%q, %q) returned error: %v", root, "sub#T", err)
	}
	if answer.ID != "sub#T" {
		t.Errorf("Expand(%q, %q).ID = %q; want %q", root, "sub#T", answer.ID, "sub#T")
	}
}

// TestEnsureResolveCoverage pins the coverage guard on a batched Resolve answer.
// An answer covering fewer or more targets than asked is ErrQuarryUnavailable.
// The uncovered targets are never silently exempted from the resolve-backed validation pass.
func TestEnsureResolveCoverage(t *testing.T) {
	tiergate.Require(t, 0)
	targets := []string{"sub#A", "sub#B"}

	if err := ensureResolveCoverage(targets, make([]quarry.ResolveResult, 2)); err != nil {
		t.Errorf("ensureResolveCoverage() with a covering answer = %v; want nil", err)
	}

	err := ensureResolveCoverage(targets, make([]quarry.ResolveResult, 1))
	if err == nil {
		t.Fatalf("ensureResolveCoverage() with a short answer = nil; want a wrapped ErrQuarryUnavailable — the uncovered target would otherwise silently skip validation")
	}
	if !errors.Is(err, ErrQuarryUnavailable) {
		t.Errorf("ensureResolveCoverage() error = %v; want it to wrap ErrQuarryUnavailable", err)
	}
}
