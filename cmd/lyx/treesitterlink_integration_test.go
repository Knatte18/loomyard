//go:build integration

// treesitterlink_integration_test.go runs `go list` over the module under every test tag and holds the allowed list of the tree-sitter link guard.
// The chain search and the comparison live in treesitterlink_test.go.

package main

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// treeSitterAllowedLinks are the packages whose test binary may link tree-sitter, each with the reason it still does.
var treeSitterAllowedLinks = []scankit.Entry{
	{Key: "internal/planglyph", Why: "implements the code index over quarry"},
	{Key: "internal/quarrycli", Why: "the quarry verbs call quarry directly"},
	{Key: "internal/webstercli", Why: "wires planglyph.NewIndex into webster's geometry"},
	{Key: "internal/loomcli", Why: "wires planglyph.NewIndex into the plan gates and webster's geometry"},
	{Key: "internal/shedcli", Why: "mounts the loom CLI, which links planglyph"},
	{Key: "cmd/lyx", Why: "mounts every CLI, including the ones that link planglyph"},
	{Key: "internal/websterengine", Why: "its tests drive the real index through the seam"},
	{Key: "internal/loomshed", Why: "its gate tests assert resolve-backed findings through planglyph.NewIndex"},
}

// treeSitterLinkMinTestBinaries is the vacuous-scan floor for the test binaries `go list` reports.
const treeSitterLinkMinTestBinaries = 100

// TestTreeSitterLink_OnlyAllowedTestBinariesLinkIt fails when a test binary outside treeSitterAllowedLinks links tree-sitter, or when an allowed one no longer does.
// The tag set is every tier's, so a binary that links it only through a `tmux` or `llm` test file counts.
//
//lyx:guard
func TestTreeSitterLink_OnlyAllowedTestBinariesLinkIt(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("go", "list", "-test", "-deps", "-tags", "integration,tmux,llm",
		"-f", goListImportsFormat, "./...")
	cmd.Dir = scankit.Root(t)
	logger.Info("cmd/lyx test: spawning go list for the tree-sitter link guard", "dir", cmd.Dir)
	output, err := cmd.Output()
	logger.Info("cmd/lyx test: go list for the tree-sitter link guard exited", "err", err)
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
