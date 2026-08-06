// longlist_test.go asserts that root.Long names every module that is registered in the cobra root, so the --help prose never silently drifts from the live command tree.
// The module set is derived from the live tree — no hardcoded list.

package main

import (
	"strings"
	"testing"
)

// TestLongList_NamesEveryRegisteredModule asserts every registered child command appears in root.Long.
func TestLongList_NamesEveryRegisteredModule(t *testing.T) {
	root := newRoot()

	for _, child := range root.Commands() {
		// Skip cobra's infrastructure subtrees; they are not domain modules.
		name := child.Name()
		if name == "help" || name == "completion" {
			continue
		}

		if !strings.Contains(root.Long, name) {
			t.Errorf("root.Long does not name registered module %q; add it to the Available modules list in newRoot()", name)
		}
	}
}
