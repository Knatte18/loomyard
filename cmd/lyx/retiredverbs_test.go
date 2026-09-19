// retiredverbs_test.go asserts that the root command tree carries no bare child command named
// "run" -- the retired root-alias name that "start" replaces (see the plan's final-verb-set
// decision). This is a pure *cobra.Command walk with no exec.Command, no gitexec, and no
// hubforge.NewHub, per the Test Tier Purity Invariant.

package main

import "testing"

// TestRoot_NoBareRunChild asserts that newRoot()'s command tree has no bare child command named
// "run". The root alias for loom's bootstrap verb is named "start" after the rename; a surviving
// "run" child would mean the retired root alias is still registered.
func TestRoot_NoBareRunChild(t *testing.T) {
	root := newRoot()
	for _, child := range root.Commands() {
		name := child.Name()
		if name == "help" || name == "completion" {
			continue
		}
		if name == "run" {
			t.Errorf("newRoot() root tree carries a bare child command named %q; the retired root alias must not survive the rename", name)
		}
	}
}
