// repo_test.go covers openRepo, resolveTargets, and the four exported query wrappers (TOC, Glyphs,
// Resolve, Expand), and declares writeFixtureRepo, the small on-disk Go fixture builder every test
// file in this package shares.

package planglyph

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeFixtureRepo writes files (keyed by repository-relative path) under a fresh t.TempDir() and
// returns that directory's absolute path, ready to hand to openRepo.
func writeFixtureRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) failed: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) failed: %v", full, err)
		}
	}
	return root
}

// TestOpenRepo_Success asserts openRepo succeeds against a real directory.
func TestOpenRepo_Success(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n"})

	repo, err := openRepo(root)
	if err != nil {
		t.Fatalf("openRepo(%q) returned error: %v", root, err)
	}
	if repo == nil {
		t.Fatalf("openRepo(%q) returned nil *quarry.Repo with a nil error", root)
	}
}

// TestOpenRepo_NonRepository asserts openRepo's error against a directory that does not exist
// satisfies errors.Is(err, ErrQuarryUnavailable), so a caller can distinguish an infrastructure
// failure without string matching.
func TestOpenRepo_NonRepository(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	_, err := openRepo(missing)
	if !errors.Is(err, ErrQuarryUnavailable) {
		t.Errorf("openRepo(%q) error = %v; want errors.Is(err, ErrQuarryUnavailable)", missing, err)
	}
}

// TestResolveTargets_Success asserts resolveTargets returns Resolve's own positional results
// unchanged.
func TestResolveTargets_Success(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
	repo, err := openRepo(root)
	if err != nil {
		t.Fatalf("openRepo(%q) returned error: %v", root, err)
	}

	results, err := resolveTargets(repo, []string{"sub#Foo"})
	if err != nil {
		t.Fatalf("resolveTargets(...) returned error: %v", err)
	}
	if len(results) != 1 || results[0].Target != "sub#Foo" {
		t.Errorf("resolveTargets(...) = %+v; want one result for %q", results, "sub#Foo")
	}
}

// TestTOC_Success asserts TOC delegates to the underlying quarry.Repo.TOC and returns its answer
// unchanged.
func TestTOC_Success(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})

	answer, err := TOC(root, "sub")
	if err != nil {
		t.Fatalf("TOC(%q, %q) returned error: %v", root, "sub", err)
	}
	if answer.Dir != "sub" {
		t.Errorf("TOC(%q, %q).Dir = %q; want %q", root, "sub", answer.Dir, "sub")
	}
}

// TestTOC_QuarryUnavailable asserts TOC's error satisfies errors.Is(err, ErrQuarryUnavailable)
// when the repository itself cannot be opened.
func TestTOC_QuarryUnavailable(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	_, err := TOC(missing, "sub")
	if !errors.Is(err, ErrQuarryUnavailable) {
		t.Errorf("TOC(%q, ...) error = %v; want errors.Is(err, ErrQuarryUnavailable)", missing, err)
	}
}

// TestGlyphs_Success asserts Glyphs delegates to the underlying quarry.Repo.Glyphs and returns its
// answer unchanged.
func TestGlyphs_Success(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})

	answer, err := Glyphs(root, "sub")
	if err != nil {
		t.Fatalf("Glyphs(%q, %q) returned error: %v", root, "sub", err)
	}
	if len(answer.Symbols) != 1 || answer.Symbols[0].ID != "sub#Foo" {
		t.Errorf("Glyphs(%q, %q).Symbols = %+v; want one symbol %q", root, "sub", answer.Symbols, "sub#Foo")
	}
}

// TestResolve_Success asserts Resolve delegates to the underlying quarry.Repo.Resolve and returns
// its result slice unchanged.
func TestResolve_Success(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})

	results, err := Resolve(root, []string{"sub#Foo"})
	if err != nil {
		t.Fatalf("Resolve(%q, ...) returned error: %v", root, err)
	}
	if len(results) != 1 || results[0].Target != "sub#Foo" {
		t.Errorf("Resolve(%q, ...) = %+v; want one result for %q", root, results, "sub#Foo")
	}
}

// TestExpand_Success asserts Expand delegates to the underlying quarry.Repo.Expand and returns its
// answer unchanged.
func TestExpand_Success(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\ntype T struct{}\n"})

	answer, err := Expand(root, "sub#T")
	if err != nil {
		t.Fatalf("Expand(%q, %q) returned error: %v", root, "sub#T", err)
	}
	if answer.ID != "sub#T" {
		t.Errorf("Expand(%q, %q).ID = %q; want %q", root, "sub#T", answer.ID, "sub#T")
	}
}
