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
// branch.
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

// makeEmptyBareRemote creates a bare git repository with no commits at all —
// a genuinely empty remote, unlike makeBareRemote's seed-and-push. It exists
// to exercise ensureBoardWorktree's orphan path, which only fires when clone
// leaves no local hostBranch ref to adopt.
func makeEmptyBareRemote(t *testing.T, dir, name string) string {
	t.Helper()

	bare := filepath.Join(dir, name+".git")
	lyxtest.MustRun(t, dir, "git", "init", "--bare", "-b", "main", bare)

	return bare
}

// resolveGitCommonDir returns the absolute, cleaned git-common-dir for the
// repo at repoDir, resolving a relative result (e.g. ".git") against repoDir
// itself rather than the test process's own cwd.
func resolveGitCommonDir(t *testing.T, repoDir string) string {
	t.Helper()
	common := gitOutput(t, repoDir, "rev-parse", "--git-common-dir")
	if !filepath.IsAbs(common) {
		common = filepath.Join(repoDir, common)
	}
	abs, err := filepath.Abs(common)
	if err != nil {
		t.Fatalf("filepath.Abs(%q): %v", common, err)
	}
	return filepath.Clean(abs)
}

// assertBoardIsWeftWorktree asserts that _board (resolved via
// hubgeometry.BoardDir(hubPath)) shares its git-common-dir with weftPrime —
// proving _board is a linked worktree of the same weft repo, not a separate
// clone — and that _board is checked out on wantBranch.
func assertBoardIsWeftWorktree(t *testing.T, hubPath, weftPrime, wantBranch string) {
	t.Helper()
	boardPath := hubgeometry.BoardDir(hubPath)

	boardCommonDir := resolveGitCommonDir(t, boardPath)
	weftCommonDir := resolveGitCommonDir(t, weftPrime)
	if boardCommonDir != weftCommonDir {
		t.Errorf("_board git-common-dir = %q; want %q (same weft repo as weft prime)", boardCommonDir, weftCommonDir)
	}

	if got := currentBranch(t, boardPath); got != wantBranch {
		t.Errorf("_board branch = %q; want %q", got, wantBranch)
	}
}

// hasNoCommits reports whether `git log` fails or returns no output in dir —
// the unborn-HEAD signature of a freshly orphan-created branch with no
// commits yet.
func hasNoCommits(t *testing.T, dir string) bool {
	t.Helper()
	cmd := exec.Command("git", "log")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return true
	}
	return strings.TrimSpace(string(out)) == ""
}

// TestCloneHub_AdoptsExistingRemoteWeftPrimaryBranch re-clones against a weft
// remote whose main-weft branch is ahead of main and asserts the fresh weft
// primary adopted it: checked out on main-weft, at the remote branch's tip
// (the synced marker file present), with origin/main-weft as its upstream.
func TestCloneHub_AdoptsExistingRemoteWeftPrimaryBranch(t *testing.T) {
	fixtures := t.TempDir()

	hostBare := makeBareRemote(t, fixtures, "adopt-host")
	weftBare := makeBareRemote(t, fixtures, "adopt-weft")

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
	hubPath, err := fabricengine.CloneHub(
		cloneParent,
		filepath.ToSlash(hostBare),
		filepath.ToSlash(weftBare),
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

	// _board must be a second worktree of the same weft repo, checked out on
	// the unsuffixed host branch "main" — adopted from the local ref
	// cloneRepo already created, proving _board is not a separate clone.
	assertBoardIsWeftWorktree(t, hubPath, weftPrime, "main")
}

// TestCloneHub_CreatesFreshWeftPrimaryBranch asserts that when the weft
// remote carries no existing WeftBranchName-suffixed branch (a genuinely new
// hub — the non-adopt path), CloneHub creates the weft primary's suffixed
// branch fresh at the cloned HEAD rather than requiring a pre-existing
// remote ref to adopt.
func TestCloneHub_CreatesFreshWeftPrimaryBranch(t *testing.T) {
	fixtures := t.TempDir()

	hostBare := makeBareRemote(t, fixtures, "fresh-host")
	weftBare := makeBareRemote(t, fixtures, "fresh-weft")

	cloneParent := t.TempDir()
	hubPath, err := fabricengine.CloneHub(
		cloneParent,
		filepath.ToSlash(hostBare),
		filepath.ToSlash(weftBare),
	)
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(hubPath) })

	if _, err := os.Stat(filepath.Join(hubPath, "fresh-host", ".git")); err != nil {
		t.Fatalf("host clone missing .git: %v", err)
	}

	weftPrime := hubgeometry.WeftSiblingPath(hubPath, "fresh-host")
	want := fabricengine.WeftBranchName("main")
	if got := currentBranch(t, weftPrime); got != want {
		t.Fatalf("weft prime branch = %q; want %q (freshly created, no remote suffixed branch to adopt)", got, want)
	}

	// _board must be a second worktree of the same weft repo, checked out on
	// the unsuffixed host branch "main" — a linked worktree, not a separate
	// clone (a plain ".git" file existing would no longer distinguish the two).
	assertBoardIsWeftWorktree(t, hubPath, weftPrime, "main")

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
// seam (clone.go's teardownHub).
func TestCloneHub_StrictAbortRemovesHubOnFailure(t *testing.T) {
	fixtures := t.TempDir()

	hostBare := makeBareRemote(t, fixtures, "abort-host")
	nonExistentWeft := filepath.Join(fixtures, "nonexistent-weft.git")

	cloneParent := t.TempDir()
	expectedHubPath := hubgeometry.HubPath(cloneParent, fabricengine.DeriveHostName(filepath.ToSlash(hostBare)))

	_, err := fabricengine.CloneHub(cloneParent, filepath.ToSlash(hostBare), filepath.ToSlash(nonExistentWeft))
	if err == nil {
		t.Fatalf("CloneHub should have failed with a non-existent weft remote")
	}
	if _, statErr := os.Stat(expectedHubPath); statErr == nil {
		t.Errorf("hub directory %s should have been removed by teardownHub after clone failure", expectedHubPath)
	}
}

// TestCloneHub_BoardWorktreeOrphanBranchOnEmptyWeftRemote asserts that when
// the weft remote is genuinely empty (no commits, so cloneRepo leaves no
// local hostBranch ref to adopt), ensureBoardWorktree's orphan path fires:
// _board is materialized as a second weft worktree on a freshly orphan-
// created "main" branch that shares no history with the weft primary's
// "main-weft" branch — both end up with no commits at all.
func TestCloneHub_BoardWorktreeOrphanBranchOnEmptyWeftRemote(t *testing.T) {
	fixtures := t.TempDir()

	// The host remote needs a real commit for suffixWeftPrimaryBranch to read
	// a checked-out branch; the weft remote is genuinely empty.
	hostBare := makeBareRemote(t, fixtures, "orphan-host")
	weftBare := makeEmptyBareRemote(t, fixtures, "orphan-weft")

	cloneParent := t.TempDir()
	hubPath, err := fabricengine.CloneHub(
		cloneParent,
		filepath.ToSlash(hostBare),
		filepath.ToSlash(weftBare),
	)
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(hubPath) })

	weftPrime := hubgeometry.WeftSiblingPath(hubPath, "orphan-host")
	if got := currentBranch(t, weftPrime); got != "main-weft" {
		t.Fatalf("weft prime branch = %q; want %q", got, "main-weft")
	}
	if !hasNoCommits(t, weftPrime) {
		t.Errorf("weft prime at %s has commits; want an unborn HEAD (genuinely empty weft remote)", weftPrime)
	}

	// _board must still be a linked worktree of the same weft repo, checked
	// out on "main", and itself carry no commits — proving the orphan branch
	// shares no history with main-weft.
	assertBoardIsWeftWorktree(t, hubPath, weftPrime, "main")
	boardPath := hubgeometry.BoardDir(hubPath)
	if !hasNoCommits(t, boardPath) {
		t.Errorf("_board at %s has commits; want an unborn HEAD (fresh orphan branch)", boardPath)
	}
}
