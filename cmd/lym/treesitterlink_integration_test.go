// treesitterlink_integration_test.go runs `go list` over the module under every test tag.
// It holds the allowed list of the tree-sitter link guard.

package main

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// treeSitterAllowedLinks are the packages whose test binary may link tree-sitter, each with the reason it does.
var treeSitterAllowedLinks = []scankit.Entry{
	{Key: "internal/planglyph", Why: "implements the code index over quarry"},
	{Key: "internal/quarrycli", Why: "the quarry verbs call quarry directly"},
	{Key: "internal/webstercli", Why: "wires planglyph.NewIndex into webster's geometry"},
	{Key: "internal/loomcli", Why: "wires planglyph.NewIndex into the plan gates and webster's geometry"},
	{Key: "internal/shedcli", Why: "mounts the loom CLI, which links planglyph"},
	{Key: "cmd/lym", Why: "mounts every CLI, including the ones that link planglyph"},
	{Key: "internal/websterengine", Why: "its tests drive the real index through the seam"},
	{Key: "internal/loomshed", Why: "its gate tests assert resolve-backed findings through planglyph.NewIndex"},
	{Key: "internal/failuretrace", Why: "traces a failed verify through planglyph's EncloseAt"},
}

// treeSitterLinkMinTestBinaries is the vacuous-scan floor for the test binaries `go list` reports.
const treeSitterLinkMinTestBinaries = 100

// TestTreeSitterLink_OnlyAllowedTestBinariesLinkIt fails when a test binary outside the allowed list links tree-sitter.
// It also fails when an entry of the allowed list no longer links tree-sitter.
// The scan runs under every tier's tag in both spellings.
// A binary that links tree-sitter only through a tier test file counts.
//
//lym:guard
func TestTreeSitterLink_OnlyAllowedTestBinariesLinkIt(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	cmd := exec.Command("go", "list", "-test", "-deps", "-tags", "tier0,tier1,tier2,tier3,integration,tmux,llm",
		"-f", goListImportsFormat, "./...")
	cmd.Dir = scankit.Root(t)
	logger.Info("cmd/lym test: spawning go list for the tree-sitter link guard", "dir", cmd.Dir)
	output, err := cmd.Output()
	logger.Info("cmd/lym test: go list for the tree-sitter link guard exited", "err", err)
	if err != nil {
		t.Fatalf("go list failed: %v\n%s", err, exitStderr(err))
	}

	deps := parseGoListImports(string(output))
	binaries := 0
	for importPath := range deps {
		if strings.HasSuffix(importPath, ".test") {
			binaries++
		}
	}
	scankit.RequireFloor(t, binaries, treeSitterLinkMinTestBinaries, "tree-sitter link guard")

	for _, finding := range treeSitterLinkFindings(treeSitterLinkedTestPackages(deps), treeSitterAllowedLinks) {
		t.Error(finding)
	}
}

// exitStderr returns the stderr a failed command captured, or the empty string for any other error.
func exitStderr(err error) string {
	if exitErr, ok := err.(*exec.ExitError); ok {
		return string(exitErr.Stderr)
	}
	return ""
}
