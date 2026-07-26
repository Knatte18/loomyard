//go:build integration

// clone_adopt_test.go proves CloneHub's weft-primary-branch handling: the
// adopt path (an existing remote main-weft branch is checked out as a
// tracking local branch, inheriting the accumulated weft content and a
// working upstream, instead of forking a new, untracked branch that silently
// disowns that history), the fresh path (no existing remote main-weft branch,
// so the suffixed branch is created new at the cloned HEAD — a genuinely new
// hub, formerly covered by clone_differential_test.go's "fresh" subtest
// before its deletion), and the strict-abort teardown path (a failing weft
// clone leaves no residual Hub directory, torn down through fabricengine's
// own RemoveAll teardown seam in clone.go's teardownHub — formerly covered by
// clone_differential_test.go's TestCloneHub_DifferentialStrictAbort).
//
// Package fabricengine_test to isolate real end-to-end git fixtures from the
// package's internal unit tests; shares the TestMain in testmain_test.go.
// makeBareRemote and currentBranch (below) are this file's own bare-remote
// fixture helpers, relocated here from clone_differential_test.go before its
// deletion (this file was already their only other consumer).

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

// makeBareRemote creates a bare git repository with a single commit on the main
// branch, mirroring warpengine's clone_integration_test.go helper of the same
// name (duplicated here, not imported, since that helper is unexported in a
// different package).
//
// It initializes a bare repo at <dir>/<name>.git, then seeds it by initializing
// a working repository, creating and committing a README, and pushing back to
// the bare repo. Returns the path to the bare repository.
func makeBareRemote(t *testing.T, dir, name string) string {
	t.Helper()

	bare := filepath.Join(dir, name+".git")
	if err := os.Mkdir(bare, 0o755); err != nil {
		t.Fatalf("mkdir bare: %v", err)
	}

	lyxtest.MustRun(t, bare, "git", "init", "--bare")

	tempWork := filepath.Join(dir, "temp-work-"+name)
	if err := os.Mkdir(tempWork, 0o755); err != nil {
		t.Fatalf("mkdir temp work: %v", err)
	}

	lyxtest.MustRun(t, tempWork, "git", "init", "-b", "main")
	lyxtest.MustRun(t, tempWork, "git", "config", "user.email", "test@test.com")
	lyxtest.MustRun(t, tempWork, "git", "config", "user.name", "Test")

	bareURL := filepath.ToSlash(bare)
	lyxtest.MustRun(t, tempWork, "git", "remote", "add", "origin", bareURL)

	readmePath := filepath.Join(tempWork, "README.md")
	if err := os.WriteFile(readmePath, []byte("# "+name), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}

	lyxtest.MustRun(t, tempWork, "git", "add", "README.md")
	lyxtest.MustRun(t, tempWork, "git", "commit", "-m", "init")
	lyxtest.MustRun(t, tempWork, "git", "push", "-u", "origin", "main")

	if err := os.RemoveAll(tempWork); err != nil {
		t.Fatalf("remove temp work: %v", err)
	}

	return bare
}

// currentBranch returns the branch checked out at repoPath via `git branch
// --show-current`, failing the test on any git error.
func currentBranch(t *testing.T, repoPath string) string {
	t.Helper()
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git branch --show-current in %s: %v", repoPath, err)
	}
	return strings.TrimSpace(string(out))
}

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

// TestCloneHub_CreatesFreshWeftPrimaryBranch asserts that when the weft
// remote carries no existing WeftBranchName-suffixed branch (a genuinely new
// hub — the non-adopt path), CloneHub creates the weft primary's suffixed
// branch fresh at the cloned HEAD rather than requiring a pre-existing
// remote ref to adopt. Formerly covered, alongside a warpengine comparison,
// by clone_differential_test.go's TestCloneHub_DifferentialEquivalence
// subtests before that file's deletion.
func TestCloneHub_CreatesFreshWeftPrimaryBranch(t *testing.T) {
	fixtures := t.TempDir()

	hostBare := makeBareRemote(t, fixtures, "fresh-host")
	weftBare := makeBareRemote(t, fixtures, "fresh-weft")
	boardBare := makeBareRemote(t, fixtures, "fresh-board")

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

	if _, err := os.Stat(filepath.Join(hubPath, "fresh-host", ".git")); err != nil {
		t.Fatalf("host clone missing .git: %v", err)
	}
	if _, err := os.Stat(filepath.Join(hubgeometry.BoardDir(hubPath), ".git")); err != nil {
		t.Fatalf("board clone missing .git: %v", err)
	}

	weftPrime := hubgeometry.WeftSiblingPath(hubPath, "fresh-host")
	want := fabricengine.WeftBranchName("main")
	if got := currentBranch(t, weftPrime); got != want {
		t.Fatalf("weft prime branch = %q; want %q (freshly created, no remote suffixed branch to adopt)", got, want)
	}

	// The freshly-created branch carries no upstream — that is deliberately
	// left to the first push, distinguishing it from the adopt path's
	// origin-tracking branch (TestCloneHub_AdoptsExistingRemoteWeftPrimaryBranch).
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", want+"@{u}")
	cmd.Dir = weftPrime
	if err := cmd.Run(); err == nil {
		t.Errorf("weft prime branch %q has an upstream; want none (fresh-created, not adopted)", want)
	}
}

// TestCloneHub_StrictAbortRemovesHubOnFailure covers teardownHub's
// cleanup-on-failure behaviour: a failing weft clone leaves no residual Hub
// directory behind, torn down through fabricengine's own RemoveAll teardown
// seam (clone.go's teardownHub). Formerly covered, alongside a warpengine
// comparison, by clone_differential_test.go's
// TestCloneHub_DifferentialStrictAbort before that file's deletion.
func TestCloneHub_StrictAbortRemovesHubOnFailure(t *testing.T) {
	fixtures := t.TempDir()

	hostBare := makeBareRemote(t, fixtures, "abort-host")
	nonExistentWeft := filepath.Join(fixtures, "nonexistent-weft.git")

	cloneParent := t.TempDir()
	expectedHubPath := hubgeometry.HubPath(cloneParent, fabricengine.DeriveHostName(filepath.ToSlash(hostBare)))

	_, _, err := fabricengine.CloneHub(cloneParent, filepath.ToSlash(hostBare), filepath.ToSlash(nonExistentWeft), "")
	if err == nil {
		t.Fatalf("CloneHub should have failed with a non-existent weft remote")
	}
	if _, statErr := os.Stat(expectedHubPath); statErr == nil {
		t.Errorf("hub directory %s should have been removed by teardownHub after clone failure", expectedHubPath)
	}
}
