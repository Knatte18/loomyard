// longlist_test.go asserts that root.Long names every module that is registered
// in the cobra root, so the --help prose never silently drifts from the live
// command tree. The module set is derived from the live tree — no hardcoded list.

package main

import (
	"strings"
	"testing"
)

// TestLongList_NamesEveryRegisteredModule builds the live cobra root via
// newRoot() and asserts that every registered child command's name appears in
// root.Long. Cobra's infrastructure commands ("help" and "completion") are
// skipped because they are auto-generated and not part of the module list.
//
// This guard enforces the "registered => in --help prose" half of the
// CLI/Cobra Invariant: if a module is wired into root.AddCommand it must also
// be named in the Long description so operators and agents can discover it.
func TestLongList_NamesEveryRegisteredModule(t *testing.T) {
	root := newRoot()

	for _, child := range root.Commands() {
		// Skip cobra's infrastructure subtrees; they are not domain modules and
		// are not part of the human-readable module list in root.Long.
		// This mirrors the skip logic in drift_test.go's collectMissingShorts.
		name := child.Name()
		if name == "help" || name == "completion" {
			continue
		}

		if !strings.Contains(root.Long, name) {
			t.Errorf("root.Long does not name registered module %q; add it to the Available modules list in newRoot()", name)
		}
	}
}
