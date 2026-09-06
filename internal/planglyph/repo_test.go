// repo_test.go covers openRepo and resolveTargets, and declares writeFixtureRepo, the small
// on-disk Go fixture builder every test file in this package shares.

package planglyph

import (
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

// TestOpenRepo_NonRepository asserts openRepo wraps quarry.Open's own error with a "planglyph:"
// prefix when the directory does not exist.
func TestOpenRepo_NonRepository(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	_, err := openRepo(missing)
	if err == nil {
		t.Fatalf("openRepo(%q) returned nil error; want an error", missing)
	}
	const wantPrefix = "planglyph:"
	if got := err.Error(); len(got) < len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
		t.Errorf("openRepo(%q) error = %q; want it to begin with %q", missing, got, wantPrefix)
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
