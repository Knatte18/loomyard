//go:build tier1

// encloseat_integration_test.go covers EncloseAt against a real git fixture repository, reading files committed at a revision through quarry's git layer.

package planglyph

import (
	"errors"
	"testing"

	"github.com/Knatte18/quarry/quarry"
)

// TestEncloseAt commits one Go file holding an import and a function, and asserts how each kind of location answers at that commit.
func TestEncloseAt(t *testing.T) {
	t.Parallel()
	f := newDeltaFixtureRepo(t)
	rev := f.writeAndCommit("sub/a.go", "package sub\n\nimport \"fmt\"\n\nfunc Foo() { fmt.Println() }\n", "add Foo")
	f.writeAndCommit("sub/notes.md", "# notes\n", "add notes")

	targets := []string{"sub/a.go:5", "sub/a.go:3", "sub/notes.md:1"}
	results, err := EncloseAt(f.root, rev, targets)
	if err != nil {
		t.Fatalf("EncloseAt(%q, %q, %q) returned error: %v", f.root, rev, targets, err)
	}
	if len(results) != len(targets) {
		t.Fatalf("EncloseAt(...) returned %d results; want %d", len(results), len(targets))
	}

	t.Run("a line inside a function answers found with its glyph", func(t *testing.T) {
		got := results[0]
		if got.Status != quarry.StatusFound || len(got.Symbols) != 1 || got.Symbols[0].ID != "sub#Foo" {
			t.Errorf("EncloseAt(...)[%q] = %+v; want found with symbol %q", targets[0], got, "sub#Foo")
		}
	})

	t.Run("an import line answers not_found with the file's self glyph", func(t *testing.T) {
		got := results[1]
		if got.Status != quarry.StatusNotFound || got.Unit != "sub/a.go#" {
			t.Errorf("EncloseAt(...)[%q] = %+v; want not_found with unit %q", targets[1], got, "sub/a.go#")
		}
	})

	t.Run("a markdown location is a rejection", func(t *testing.T) {
		got := results[2]
		if got.Status != "" || got.Reason == "" {
			t.Errorf("EncloseAt(...)[%q] = %+v; want a rejection with a reason and no status", targets[2], got)
		}
	})

	t.Run("an unknown revision is quarry unavailable", func(t *testing.T) {
		_, err := EncloseAt(f.root, "does-not-exist-rev", targets)
		if !errors.Is(err, ErrQuarryUnavailable) {
			t.Errorf("EncloseAt(%q, %q, ...) error = %v; want errors.Is(err, ErrQuarryUnavailable)", f.root, "does-not-exist-rev", err)
		}
	})
}
