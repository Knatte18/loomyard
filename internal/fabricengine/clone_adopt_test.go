//go:build integration

// clone_adopt_test.go proves CloneHub's weft-primary adopt path: when the weft
// remote already carries the suffixed primary branch (a re-clone of a hub with
// synced weft history), the fresh clone checks out a tracking local branch of
// origin/<branch>-weft — inheriting the accumulated weft content and a working
// upstream — instead of forking a new, untracked branch at the default branch's
// HEAD that silently disowns that history.
//
// Package fabricengine_test to reuse clone_differential_test.go's
// makeBareRemote and currentBranch helpers; shares the TestMain in
// testmain_test.go.

package fabricengine_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// gitOutput runs a git command in dir and returns its trimmed stdout, failing
// the test on any error — the capture-variant sibling of lyxtest.MustRun,
// which discards output.
func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s in %s: %v", strings.Join(args, " "), dir, err)
	}
	return strings.TrimSpace(string(out))
}

// TestCloneHub_AdoptsExistingRemoteWeftPrimaryBranch re-clones against a weft
// remote whose main-weft branch is ahead of main and asserts the fresh weft
// primary adopted it: checked out on main-weft, at the remote branch's tip
// (the synced marker file present), with origin/main-weft as its upstream.
func TestCloneHub_AdoptsExistingRemoteWeftPrimaryBranch(t *testing.T) {
	fixtures := t.TempDir()

	hostBare := makeBareRemote(t, fixtures, "adopt-host")
	weftBare := makeBareRemote(t, fixtures, "adopt-weft")
	boardBare := makeBareRemote(t, fixtures, "adopt-board")

	// Advance the weft remote's main-weft past main, the way a previously
	// active hub's synced weft history would have: one extra commit carrying a
	// marker file that only exists on main-weft.
	seedWork := filepath.Join(fixtures, "seed-weft")
	lyxtest.MustRun(t, fixtures, "git", "clone", filepath.ToSlash(weftBare), "seed-weft")
	lyxtest.MustRun(t, seedWork, "git", "config", "user.email", "test@test.com")
	lyxtest.MustRun(t, seedWork, "git", "config", "user.name", "Test")
	lyxtest.MustRun(t, seedWork, "git", "checkout", "-b", "main-weft")
	markerName := "synced-weft-state.txt"
	if err := os.WriteFile(filepath.Join(seedWork, markerName), []byte("weft history"), 0o644); err != nil {
		t.Fatalf("write weft marker: %v", err)
	}
	lyxtest.MustRun(t, seedWork, "git", "add", markerName)
	lyxtest.MustRun(t, seedWork, "git", "commit", "-m", "weft sync")
	lyxtest.MustRun(t, seedWork, "git", "push", "-u", "origin", "main-weft")

	remoteTip := gitOutput(t, seedWork, "rev-parse", "main-weft")

	// Clone the hub fresh, as a second machine (or clone --reset) would.
	cloneParent := t.TempDir()
	hubPath, _, err := fabricengine.CloneHub(
		cloneParent,
		filepath.ToSlash(hostBare),
		filepath.ToSlash(weftBare),
		filepath.ToSlash(boardBare),
	)
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(hubPath) })

	// The weft prime directory is the host name's weft sibling — resolved via
	// hubgeometry so the assertion cannot rot against clone's own geometry.
	weftPrime := hubgeometry.WeftSiblingPath(hubPath, "adopt-host")

	if got := currentBranch(t, weftPrime); got != "main-weft" {
		t.Fatalf("weft prime branch = %q; want %q", got, "main-weft")
	}

	// The adopted branch must sit at the remote main-weft tip — the synced
	// history — not at main's HEAD. The marker file existing is the
	// user-visible form of the same assertion.
	localTip := gitOutput(t, weftPrime, "rev-parse", "HEAD")
	if localTip != remoteTip {
		t.Errorf("weft prime HEAD = %s; want remote main-weft tip %s (existing weft history adopted)", localTip, remoteTip)
	}
	if _, err := os.Stat(filepath.Join(weftPrime, markerName)); err != nil {
		t.Errorf("weft prime marker file %s missing: %v; want the synced weft content checked out", markerName, err)
	}

	// The adopted branch must track origin/main-weft so the first push (and any
	// rebase-recovery) has an upstream to work against.
	upstream := gitOutput(t, weftPrime, "rev-parse", "--abbrev-ref", "main-weft@{u}")
	if upstream != "origin/main-weft" {
		t.Errorf("weft prime upstream = %q; want %q", upstream, "origin/main-weft")
	}
}
