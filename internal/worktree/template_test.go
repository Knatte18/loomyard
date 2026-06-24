// template_test.go — tests for the worktree ConfigTemplate generator.
//
// Covers: ConfigTemplate returns the exact expected commented string (regression baseline).

package worktree

import (
	"testing"
)

// TestConfigTemplate asserts ConfigTemplate returns the exact expected commented string.
// This is the regression baseline that proves the relocation preserved content.
func TestConfigTemplate(t *testing.T) {
	expected := "# branch_prefix: $env:LYX_BRANCH_PREFIX ?    # prefix prepended to the slug to form the branch name (e.g. \"hanf/\"); empty = branch == slug\n"

	got := ConfigTemplate()

	if got != expected {
		t.Errorf("ConfigTemplate() = %q; want %q", got, expected)
	}
}
