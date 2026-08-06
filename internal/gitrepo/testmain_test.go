//go:build integration

// testmain_test.go wires the package's test binary into the hermetic git test
// environment: lyxtest.HermeticGitEnv() runs once before any test, so
// gitrepo's git-spawning fixtures never inherit the operator's global
// gitconfig (see CONSTRAINTS.md's Hermetic Git Test Environment Invariant).
// Only the TestMain function body below is identical to
// internal/gitexec/testmain_test.go's; that file carries no //go:build tag at
// all (untagged, runs in Tier 1) while this file is //go:build
// integration-tagged (Tier 2-only), because gitrepo's one untagged test file
// (keyvalidation_test.go) does no git spawning and so needs no
// HermeticGitEnv() call, unlike every one of gitrepo's other (tagged) test
// files.

package gitrepo_test

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestMain runs lyxtest.HermeticGitEnv() before any test in this package spawns git, then delegates
// to the normal test run.
func TestMain(m *testing.M) {
	lyxtest.HermeticGitEnv()
	os.Exit(m.Run())
}
