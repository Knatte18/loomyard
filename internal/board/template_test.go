// template_test.go — tests for the board ConfigTemplate generator.
//
// Covers: ConfigTemplate returns the exact expected commented string (regression baseline).

package board

import (
	"testing"
)

// TestConfigTemplate asserts ConfigTemplate returns the exact expected commented string.
// This is the regression baseline that proves the relocation preserved content.
func TestConfigTemplate(t *testing.T) {
	expected := "# path: $env:LYX_BOARD_PATH ? ../_board   # board dir (tasks.json + rendered output); relative to cwd or absolute\n" +
		"# home: $env:LYX_HOME ? Home.md           # home page file name; relative to board dir\n" +
		"# sidebar: $env:LYX_SIDEBAR ? _Sidebar.md   # sidebar file name; relative to board dir\n" +
		"# proposal_prefix: $env:LYX_PROPOSAL_PREFIX ? proposal-   # prefix for proposal files\n"

	got := ConfigTemplate()

	if got != expected {
		t.Errorf("ConfigTemplate() = %q; want %q", got, expected)
	}
}
