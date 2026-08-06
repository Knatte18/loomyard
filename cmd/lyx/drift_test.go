// drift_test.go asserts that every command in the lyx cobra tree has a non-empty
// Short description. This is the structural self-documentation invariant: if a
// developer adds a subcommand without a Short, this test fails at CI time rather
// than silently producing a help tree with blank entries.

package main

import (
	"fmt"
	"testing"

	"github.com/spf13/cobra"
)

// TestDriftGuard_AllCommandsHaveShort asserts every command carries a non-empty Short, skipping cobra infrastructure.
func TestDriftGuard_AllCommandsHaveShort(t *testing.T) {
	root := newRoot()
	// Traverse the tree recursively and collect violations.
	violations := collectMissingShorts(root)
	for _, v := range violations {
		t.Errorf("command %q has no Short description", v)
	}
}

// collectMissingShorts performs a depth-first walk collecting commands with empty Short.
func collectMissingShorts(cmd *cobra.Command) []string {
	// Skip cobra's infrastructure subtrees; they are not domain commands.
	name := cmd.Name()
	if name == "help" || name == "completion" {
		return nil
	}

	var violations []string

	if cmd.Short == "" {
		violations = append(violations, fmt.Sprintf("%s", cmd.CommandPath()))
	}

	for _, child := range cmd.Commands() {
		violations = append(violations, collectMissingShorts(child)...)
	}

	return violations
}
