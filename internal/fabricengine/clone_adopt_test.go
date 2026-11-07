// clone_adopt_test.go proves CloneHub's weft-primary-branch handling on three paths.
// The adopt path checks out an existing remote main-weft branch as a tracking local branch.
// The local branch inherits the accumulated weft content and a working upstream.
// The alternative would fork a new, untracked branch that silently disowns that history.
// The fresh path covers no existing remote main-weft branch.
// The suffixed branch is then created new at the cloned HEAD.
// The strict-abort path covers a failing warp clone, which must leave no residual Hub directory.
// Teardown runs through teardownHub in clone.go.
//
// Package fabricengine_test keeps its real end-to-end git fixtures apart from the internal unit tests.
// It shares the TestMain in testmain_test.go.
// This file defines its own bare-remote fixture helpers, makeBareRemote and makeBareRemoteWithSubdir.

package fabricengine_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// makeBareRemote creates a bare git repository at dir/name.git and returns its path.
// The repository's main branch holds a single commit.
func makeBareRemote(t *testing.T, dir, name string) string {
	t.Helper()

	bare := filepath.Join(dir, name+".git")
	if err := os.Mkdir(bare, 0o755); err != nil {
		t.Fatalf("mkdir bare: %v", err)
	}

	gitkit.Git(t, bare, "init", "--bare")

	tempWork := filepath.Join(dir, "temp-work-"+name)
	if err := os.Mkdir(tempWork, 0o755); err != nil {
		t.Fatalf("mkdir temp work: %v", err)
	}

	gitkit.Git(t, tempWork, "init", "-b", "main")
	gitkit.Git(t, tempWork, "config", "user.email", "test@test.com")
	gitkit.Git(t, tempWork, "config", "user.name", "Test")

	bareURL := filepath.ToSlash(bare)
	gitkit.MustRun(t, tempWork, "git", "remote", "add", "origin", bareURL)

	gitkit.CommitFile(t, tempWork, "README.md", "# "+name, "init")
	gitkit.MustRun(t, tempWork, "git", "push", "-u", "origin", "main")

	if err := os.RemoveAll(tempWork); err != nil {
		t.Fatalf("remove temp work: %v", err)
	}

	return bare
}

// gitOutput runs a git command in dir and returns its trimmed stdout, failing the test on any error.
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

// makeBareRemoteWithSubdir creates a bare git repository at dir/name.git whose main branch holds a README.
// It also commits a file at subdir/marker.txt, so a subpath test has a real directory to point at.
func makeBareRemoteWithSubdir(t *testing.T, dir, name, subdir string) string {
	t.Helper()

	bare := filepath.Join(dir, name+".git")
	if err := os.Mkdir(bare, 0o755); err != nil {
		t.Fatalf("mkdir bare: %v", err)
	}

	gitkit.Git(t, bare, "init", "--bare")

	tempWork := filepath.Join(dir, "temp-work-"+name)
	if err := os.Mkdir(tempWork, 0o755); err != nil {
		t.Fatalf("mkdir temp work: %v", err)
	}

	gitkit.Git(t, tempWork, "init", "-b", "main")
	gitkit.Git(t, tempWork, "config", "user.email", "test@test.com")
	gitkit.Git(t, tempWork, "config", "user.name", "Test")

	bareURL := filepath.ToSlash(bare)
	gitkit.MustRun(t, tempWork, "git", "remote", "add", "origin", bareURL)

	readmePath := filepath.Join(tempWork, "README.md")
	if err := os.WriteFile(readmePath, []byte("# "+name), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	subdirPath := filepath.Join(tempWork, subdir)
	if err := os.MkdirAll(subdirPath, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdirPath, "marker.txt"), []byte("subdir content"), 0o644); err != nil {
		t.Fatalf("write subdir marker: %v", err)
	}

	gitkit.Git(t, tempWork, "add", "README.md", subdir)
	gitkit.Git(t, tempWork, "commit", "-m", "init")
	gitkit.MustRun(t, tempWork, "git", "push", "-u", "origin", "main")

	if err := os.RemoveAll(tempWork); err != nil {
		t.Fatalf("remove temp work: %v", err)
	}

	return bare
}

// commitFileOnBranch commits relPath with contents on branch in bareRemote and pushes it back.
// It creates the branch from the remote's default branch tip when the branch is absent.
// Tests use it to seed a weft remote with a .lym-anchor marker, as a prior clone would have left it.
func commitFileOnBranch(t *testing.T, dir, bareRemote, branch, relPath, contents string) {
	t.Helper()

	scratch := filepath.Join(dir, "scratch-"+branch+"-"+filepath.Base(relPath))
	gitkit.Git(t, dir, "clone", filepath.ToSlash(bareRemote), filepath.Base(scratch))
	gitkit.Git(t, scratch, "config", "user.email", "test@test.com")
	gitkit.Git(t, scratch, "config", "user.name", "Test")
	gitkit.CommitFileOnBranch(t, scratch, branch, relPath, contents, "seed "+relPath)
	gitkit.Git(t, scratch, "push", "origin", branch)
}

// makeEmptyBareRemote creates a bare git repository with no commits at all.
// A clone of it leaves no local branch to adopt, which exercises the orphan-branch path.
func makeEmptyBareRemote(t *testing.T, dir, name string) string {
	t.Helper()

	bare := filepath.Join(dir, name+".git")
	gitkit.MustRun(t, dir, "git", "init", "--bare", "-b", "main", bare)

	return bare
}

// resolveGitCommonDir returns the absolute, cleaned git common directory of the repo at repoDir.
// A relative result, such as `.git`, resolves against repoDir, not the test process's working directory.
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

// assertBoardIsWeftWorktree asserts that the Bolt checkout of hubPath is a linked worktree of the weft repo.
// The checkout is located with fabricengine.BoltDir and must share weftPrime's git common directory.
// That shared directory proves the checkout is not a separate clone.
// It also asserts the checkout is on wantBranch.
func assertBoardIsWeftWorktree(t *testing.T, hubPath, weftPrime, wantBranch string) {
	t.Helper()
	boardPath := fabricengine.BoltDir(hubPath)

	boardCommonDir := resolveGitCommonDir(t, boardPath)
	weftCommonDir := resolveGitCommonDir(t, weftPrime)
	if boardCommonDir != weftCommonDir {
		t.Errorf("Bolt checkout git-common-dir = %q; want %q (same weft repo as weft prime)", boardCommonDir, weftCommonDir)
	}

	if got := gitkit.Git(t, boardPath, "branch", "--show-current"); got != wantBranch {
		t.Errorf("Bolt checkout branch = %q; want %q", got, wantBranch)
	}
}

// hasNoCommits reports whether `git log` fails or prints nothing in dir.
// That is the unborn-HEAD state of a freshly orphan-created branch.
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

// TestCloneHub_AdoptsExistingRemoteWeftPrimaryBranch asserts the weft primary adopts the remote main-weft branch.
// The remote branch is ahead of main.
// The adopted branch is checked out at the remote tip, with origin/main-weft as its upstream.
func TestCloneHub_AdoptsExistingRemoteWeftPrimaryBranch(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixtures := t.TempDir()

	warpBare := makeBareRemote(t, fixtures, "adopt-warp")
	weftBare := makeBareRemote(t, fixtures, "adopt-weft")

	// Advance the weft remote's main-weft past main, as a previously active hub's synced weft history would.
	// That is one extra commit carrying a marker file that exists only on main-weft.
	seedWork := filepath.Join(fixtures, "seed-weft")
	gitkit.MustRun(t, fixtures, "git", "clone", filepath.ToSlash(weftBare), "seed-weft")
	gitkit.Git(t, seedWork, "config", "user.email", "test@test.com")
	gitkit.Git(t, seedWork, "config", "user.name", "Test")
	gitkit.MustRun(t, seedWork, "git", "checkout", "-b", "main-weft")
	markerName := "synced-weft-state.txt"
	gitkit.CommitFile(t, seedWork, markerName, "weft history", "weft sync")
	gitkit.MustRun(t, seedWork, "git", "push", "-u", "origin", "main-weft")

	remoteTip := gitOutput(t, seedWork, "rev-parse", "main-weft")

	// Clone the hub fresh, as a second machine (or clone --reset) would.
	cloneParent := t.TempDir()
	// ForceBootstrap is needed because weftBare is a plain seeded remote, not a weft that has ever been one.
	// It carries no .lym-anchor.
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(weftBare),
		WarpURL:        filepath.ToSlash(warpBare),
		Subpath:        ".",
		ForceBootstrap: true,
		Shortname:      "tst",
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	hubPath := res.HubPath
	t.Cleanup(func() { _ = os.RemoveAll(hubPath) })

	// The weft prime directory is the warp's weft sibling.
	// The test resolves it with weftname.SiblingPath rather than deriving it from clone's own layout.
	weftPrime := weftname.SiblingPath(hubPath, "adopt-warp")

	if got := gitkit.CurrentBranch(t, weftPrime); got != "main-weft" {
		t.Fatalf("weft prime branch = %q; want %q", got, "main-weft")
	}

	// The adopted branch sits on the remote main-weft tip, the synced history, and not on main's HEAD.
	// The anchor commit that this clone adds is the one commit above that tip.
	// The marker file's presence is the user-visible form of the same assertion.
	localParent := gitOutput(t, weftPrime, "rev-parse", "HEAD^")
	if localParent != remoteTip {
		t.Errorf("weft prime HEAD^ = %s; want remote main-weft tip %s (existing weft history adopted)", localParent, remoteTip)
	}
	if _, err := os.Stat(filepath.Join(weftPrime, markerName)); err != nil {
		t.Errorf("weft prime marker file %s missing: %v; want the synced weft content checked out", markerName, err)
	}

	// The adopted branch tracks origin/main-weft, so the first push and any rebase recovery have an upstream.
	upstream := gitOutput(t, weftPrime, "rev-parse", "--abbrev-ref", "main-weft@{u}")
	if upstream != "origin/main-weft" {
		t.Errorf("weft prime upstream = %q; want %q", upstream, "origin/main-weft")
	}

	// The Bolt checkout must be a second worktree of the same weft repo, not a separate clone.
	// It is checked out on the unsuffixed warp branch `main`, adopted from the local ref that the clone created.
	assertBoardIsWeftWorktree(t, hubPath, weftPrime, "main")
}

// TestCloneHub_CreatesFreshWeftPrimaryBranch asserts that CloneHub creates the weft primary branch fresh.
// The branch, named by `fabricengine.RecordsBranchName`, starts at the cloned HEAD.
// It is created only when the weft remote has no such branch to adopt.
func TestCloneHub_CreatesFreshWeftPrimaryBranch(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixtures := t.TempDir()

	warpBare := makeBareRemote(t, fixtures, "fresh-warp")
	weftBare := makeBareRemote(t, fixtures, "fresh-weft")

	cloneParent := t.TempDir()
	// ForceBootstrap is needed because weftBare is a plain seeded remote, not a weft that has ever been one.
	// It carries no .lym-anchor.
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(weftBare),
		WarpURL:        filepath.ToSlash(warpBare),
		Subpath:        ".",
		ForceBootstrap: true,
		Shortname:      "tst",
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	hubPath := res.HubPath
	t.Cleanup(func() { _ = os.RemoveAll(hubPath) })

	if _, err := os.Stat(filepath.Join(hubPath, "fresh-warp", ".git")); err != nil {
		t.Fatalf("warp clone missing .git: %v", err)
	}

	weftPrime := weftname.SiblingPath(hubPath, "fresh-warp")
	want := fabricengine.RecordsBranchName("main")
	if got := gitkit.CurrentBranch(t, weftPrime); got != want {
		t.Fatalf("weft prime branch = %q; want %q (freshly created, no remote suffixed branch to adopt)", got, want)
	}

	// The Bolt checkout must be a second worktree of the same weft repo, not a separate clone.
	// It is checked out on the unsuffixed warp branch `main`.
	assertBoardIsWeftWorktree(t, hubPath, weftPrime, "main")

	// The freshly created branch has no upstream, which is left to the first push.
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", want+"@{u}")
	cmd.Dir = weftPrime
	if err := cmd.Run(); err == nil {
		t.Errorf("weft prime branch %q has an upstream; want none (fresh-created, not adopted)", want)
	}
}

// TestCloneHub_StrictAbortRemovesHubOnFailure asserts that a failing warp clone leaves no hub directory behind.
// The weft remote is a valid fixture, so the probe passes and the hub directory is created before the failure.
// A nonexistent weft path would fail at the probe instead, before teardown runs, so this test does not use one.
func TestCloneHub_StrictAbortRemovesHubOnFailure(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixtures := t.TempDir()

	weftBare := makeBareRemote(t, fixtures, "abort-weft")
	nonExistentWarp := filepath.Join(fixtures, "nonexistent-warp.git")

	cloneParent := t.TempDir()
	expectedHubPath := fabricengine.HubPath(cloneParent, fabricengine.DeriveWarpName(filepath.ToSlash(nonExistentWarp)))

	// ForceBootstrap is needed because weftBare is a plain seeded remote, not a weft that has ever been one.
	// It carries no .lym-anchor, so without ForceBootstrap the guard fails before the warp clone is attempted.
	_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(weftBare),
		WarpURL:        filepath.ToSlash(nonExistentWarp),
		Subpath:        ".",
		ForceBootstrap: true,
		Shortname:      "tst",
	})
	if err == nil {
		t.Fatalf("CloneHub should have failed with a non-existent warp remote")
	}
	if _, statErr := os.Stat(expectedHubPath); statErr == nil {
		t.Errorf("hub directory %s should have been removed by teardownHub after clone failure", expectedHubPath)
	}
}

// TestCloneHub_BoardWorktreeOrphanBranchOnEmptyWeftRemote asserts the orphan path for an empty weft remote.
// The Bolt checkout becomes a second weft worktree on a new orphan `main` branch.
// That branch shares no history with the weft primary's `main-weft` and has no commits.
func TestCloneHub_BoardWorktreeOrphanBranchOnEmptyWeftRemote(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixtures := t.TempDir()

	// The warp remote needs a real commit, because the weft primary is named after the warp's branch.
	// The weft remote is empty.
	warpBare := makeBareRemote(t, fixtures, "orphan-warp")
	weftBare := makeEmptyBareRemote(t, fixtures, "orphan-weft")

	cloneParent := t.TempDir()
	// weftBare is empty, so the probe finds an unborn HEAD and sets `WeftLooksLikeWeft`.
	// That clears the guard that requires ForceBootstrap, so none is needed.
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:   filepath.ToSlash(weftBare),
		WarpURL:   filepath.ToSlash(warpBare),
		Subpath:   ".",
		Shortname: "tst",
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	hubPath := res.HubPath
	t.Cleanup(func() { _ = os.RemoveAll(hubPath) })

	weftPrime := weftname.SiblingPath(hubPath, "orphan-warp")
	if got := gitkit.Git(t, weftPrime, "branch", "--show-current"); got != "main-weft" {
		t.Fatalf("weft prime branch = %q; want %q", got, "main-weft")
	}
	// The weft prime's `main-weft` branch must be born: a real ref, not only the checked-out name.
	// An unborn HEAD writes no ref, so every verb that forks a pair from the primary fails with `invalid reference`.
	// `lym fabric add` is one such verb.
	// CloneHub lands an initialising empty commit on that branch (bornWeftPrimaryBranch, clone.go).
	if hasNoCommits(t, weftPrime) {
		t.Errorf("weft prime at %s has an unborn HEAD; want its suffixed branch born so a pair can fork from it", weftPrime)
	}

	// The Bolt checkout must still be a linked worktree of the same weft repo, checked
	// out on "main", and itself carry no commits — it is orphan-created, so it
	// gains nothing from the weft primary's initialising commit.
	assertBoardIsWeftWorktree(t, hubPath, weftPrime, "main")
	boardPath := fabricengine.BoltDir(hubPath)
	if !hasNoCommits(t, boardPath) {
		t.Errorf("Bolt checkout at %s has commits; want an unborn HEAD (fresh orphan branch)", boardPath)
	}
}

// TestCloneHub_EmptyWeftRemoteWithForeignDefaultBranch asserts the naming of the weft primary and the Bolt checkout.
// They are named after the warp prime's branch, not after the empty weft remote's own unborn HEAD.
// A weft bare created by `git init --bare` on a host whose default branch is "master" must still yield
// "main-weft" and a Bolt checkout on "main" for a warp on "main"; otherwise `lym fabric add` can never fork a pair.
func TestCloneHub_EmptyWeftRemoteWithForeignDefaultBranch(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixtures := t.TempDir()

	warpBare := makeBareRemote(t, fixtures, "foreign-head-warp")
	weftBare := filepath.Join(fixtures, "foreign-head-weft.git")
	gitkit.MustRun(t, fixtures, "git", "init", "--bare", "-b", "master", weftBare)

	cloneParent := t.TempDir()
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:   filepath.ToSlash(weftBare),
		WarpURL:   filepath.ToSlash(warpBare),
		Subpath:   ".",
		Shortname: "tst",
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })

	weftPrime := weftname.SiblingPath(res.HubPath, "foreign-head-warp")
	if got := gitkit.Git(t, weftPrime, "branch", "--show-current"); got != "main-weft" {
		t.Errorf("weft prime branch = %q; want %q", got, "main-weft")
	}
	if hasNoCommits(t, weftPrime) {
		t.Errorf("weft prime at %s has an unborn HEAD; want main-weft born so a pair can fork from it", weftPrime)
	}
	assertBoardIsWeftWorktree(t, res.HubPath, weftPrime, "main")
}

// TestCloneHub_AnchorCreatePath asserts the create path:
// a first-ever clone with an existing "backend" subdirectory in the warp commits the marker at the prime weft's root;
// it commits nothing to the board, and it returns a fully-populated CloneResult naming it.
func TestCloneHub_AnchorCreatePath(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixtures := t.TempDir()

	warpBare := makeBareRemoteWithSubdir(t, fixtures, "anchor-create-warp", "backend")
	weftBare := makeBareRemote(t, fixtures, "anchor-create-weft")

	cloneParent := t.TempDir()
	// ForceBootstrap: true, because weftBare is an ordinary seeded bare remote standing in for a weft;
	// it has never been a repo, so it carries no .lym-anchor.
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(weftBare),
		WarpURL:        filepath.ToSlash(warpBare),
		Subpath:        "backend",
		ForceBootstrap: true,
		Shortname:      "tst",
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })

	if res.Anchor != "backend" {
		t.Errorf("res.Anchor = %q; want %q", res.Anchor, "backend")
	}
	if filepath.Base(res.BoardDir) != fabricengine.BoltDirName {
		t.Errorf("res.BoardDir = %q; want it to end in %s", res.BoardDir, fabricengine.BoltDirName)
	}
	if filepath.Base(res.PrimeCwd) != "backend" {
		t.Errorf("res.PrimeCwd = %q; want it to end in .../backend", res.PrimeCwd)
	}
	if res.WeftBase == "" {
		t.Errorf("res.WeftBase is empty; want a resolved weft-side base directory")
	}

	assertMarkerCommittedOnPrimeWeftNotBoard(t, res, "anchor-create-warp", "backend")
}

// assertMarkerCommittedOnPrimeWeftNotBoard asserts that the hub's prime weft HEAD holds want as its .lym-anchor,
// and that the board worktree holds no such file.
func assertMarkerCommittedOnPrimeWeftNotBoard(t *testing.T, res fabricengine.CloneResult, warpName, want string) {
	t.Helper()

	weftPrime := weftname.SiblingPath(res.HubPath, warpName)
	if got := gitkit.Git(t, weftPrime, "show", "HEAD:"+lymcwd.AnchorFileName); strings.TrimSpace(got) != want {
		t.Errorf("git show HEAD:%s in the prime weft = %q; want %q", lymcwd.AnchorFileName, got, want)
	}
	if _, err := os.Stat(filepath.Join(res.BoardDir, lymcwd.AnchorFileName)); !os.IsNotExist(err) {
		t.Errorf("board holds a %s (stat err = %v); want none", lymcwd.AnchorFileName, err)
	}
}

// TestCloneHub_AnchorGuardHardErrors asserts that a create-path clone against an unusable subpath is a hard error,
// and that teardownHub removes the hub.
// Two cases are refused: a subpath missing from the warp worktree (a typo like "backedn");
// and a subpath naming an existing file, refused with a message saying it is not a directory.
func TestCloneHub_AnchorGuardHardErrors(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name         string
		warpSubdir   string
		subpath      string
		wantErrorHas string
	}{
		{"typo_path", "backend", "backedn", ""},
		{"file_not_directory", "", "README.md", "as a directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fixtures := t.TempDir()

			var warpBare string
			if tt.warpSubdir != "" {
				warpBare = makeBareRemoteWithSubdir(t, fixtures, "anchor-guard-warp", tt.warpSubdir)
			} else {
				warpBare = makeBareRemote(t, fixtures, "anchor-guard-warp")
			}
			weftBare := makeBareRemote(t, fixtures, "anchor-guard-weft")

			cloneParent := t.TempDir()
			expectedHubPath := fabricengine.HubPath(cloneParent, fabricengine.DeriveWarpName(filepath.ToSlash(warpBare)))

			// ForceBootstrap: true, because weftBare is an ordinary seeded bare remote standing in for a weft;
			// it has never been a repo, so it carries no .lym-anchor.
			_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
				WeftURL:        filepath.ToSlash(weftBare),
				WarpURL:        filepath.ToSlash(warpBare),
				Subpath:        tt.subpath,
				ForceBootstrap: true,
				Shortname:      "tst",
			})
			if err == nil {
				t.Fatalf("CloneHub() with subpath %q should have failed", tt.subpath)
			}
			if tt.wantErrorHas != "" && !strings.Contains(err.Error(), tt.wantErrorHas) {
				t.Errorf("CloneHub() error = %q; want it to say the subpath is not a directory, not that it does not exist", err)
			}
			if _, statErr := os.Stat(expectedHubPath); statErr == nil {
				t.Errorf("hub directory %s should have been removed by teardownHub after the anchor guard failure", expectedHubPath)
			}
		})
	}
}

// TestCloneHub_AnchorRootDefaultPath asserts the create path with an explicit "." subpath:
// it commits "." as the prime weft's marker and returns Anchor == ".".
func TestCloneHub_AnchorRootDefaultPath(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixtures := t.TempDir()

	warpBare := makeBareRemote(t, fixtures, "anchor-root-warp")
	weftBare := makeBareRemote(t, fixtures, "anchor-root-weft")

	cloneParent := t.TempDir()
	// ForceBootstrap: true, because weftBare is an ordinary seeded bare remote standing in for a weft;
	// it has never been a repo, so it carries no .lym-anchor.
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(weftBare),
		WarpURL:        filepath.ToSlash(warpBare),
		Subpath:        ".",
		ForceBootstrap: true,
		Shortname:      "tst",
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })

	if res.Anchor != "." {
		t.Errorf("res.Anchor = %q; want %q", res.Anchor, ".")
	}

	assertMarkerCommittedOnPrimeWeftNotBoard(t, res, "anchor-root-warp", ".")
}

// seedWeftBranchFromMain pushes a branch named branch at the main tip of bareRemote.
// A commit later made on either branch leaves the other one markerless.
func seedWeftBranchFromMain(t *testing.T, dir, bareRemote, branch string) {
	t.Helper()

	scratch := filepath.Join(dir, "scratch-seed-"+branch)
	gitkit.Git(t, dir, "clone", filepath.ToSlash(bareRemote), filepath.Base(scratch))
	gitkit.Git(t, scratch, "push", "origin", "main:"+branch)
}

// TestCloneHub_AnchorAdoptPath covers the adopt path against a weft remote with .lym-anchor="backend" committed.
// The marker sits on main-weft, which the prime's weft worktree checks out and which wins;
// or it sits only on the Bolt legacy branch, the fallback.
// A re-clone with no --subpath reads the recorded value; a conflicting non-default --subpath hard-errors;
// a matching --subpath succeeds; and no re-clone writes a Bolt marker the weft's own branch did not already carry.
func TestCloneHub_AnchorAdoptPath(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// seed commits the marker on the branch under test of the weft remote.
		seed func(t *testing.T, fixtures, weftBare string)
		// wantBoardMarker reports whether the board is expected to hold the marker on the weft remote's main.
		wantBoardMarker bool
	}{
		{"MainWeftMarker", func(t *testing.T, fixtures, weftBare string) {
			seedWeftBranchFromMain(t, fixtures, weftBare, "main-weft")
			commitFileOnBranch(t, fixtures, weftBare, "main-weft", lymcwd.AnchorFileName, "backend\n")
		}, false},
		{"BoltMarker", func(t *testing.T, fixtures, weftBare string) {
			seedWeftBranchFromMain(t, fixtures, weftBare, "main-weft")
			commitFileOnBranch(t, fixtures, weftBare, "main", lymcwd.AnchorFileName, "backend\n")
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fixtures := t.TempDir()

			warpBare := makeBareRemoteWithSubdir(t, fixtures, "anchor-adopt-warp", "backend")
			weftBare := makeBareRemote(t, fixtures, "anchor-adopt-weft")
			tt.seed(t, fixtures, weftBare)

			// With no --subpath, the recorded anchor is read and returned as-is.
			// ForceBootstrap is not needed; the weft already carries the .lym-anchor marker seeded above,
			// so the guard admits it regardless.
			cloneParent := t.TempDir()
			res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
				WeftURL: filepath.ToSlash(weftBare),
				WarpURL: filepath.ToSlash(warpBare),
				Subpath: "",
			})
			if err != nil {
				t.Fatalf("CloneHub() error = %v; want nil", err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })
			if res.Anchor != "backend" {
				t.Errorf("res.Anchor = %q; want %q (adopted from the recorded marker)", res.Anchor, "backend")
			}
			_, boardStatErr := os.Stat(filepath.Join(res.BoardDir, lymcwd.AnchorFileName))
			if gotBoardMarker := boardStatErr == nil; gotBoardMarker != tt.wantBoardMarker {
				t.Errorf("board holds a %s = %v; want %v (the adopt writes nothing)", lymcwd.AnchorFileName, gotBoardMarker, tt.wantBoardMarker)
			}

			// A conflicting non-default --subpath must hard-error rather than silently re-anchor:
			// the recorded anchor is authoritative on adopt.
			conflictParent := t.TempDir()
			_, err = fabricengine.CloneHub(conflictParent, fabricengine.CloneOptions{
				WeftURL: filepath.ToSlash(weftBare),
				WarpURL: filepath.ToSlash(warpBare),
				Subpath: "frontend",
			})
			if err == nil {
				t.Fatalf("CloneHub() with --subpath frontend against a recorded backend anchor should have failed")
			}

			// A matching --subpath succeeds identically to the no-flag case.
			matchParent := t.TempDir()
			matchRes, err := fabricengine.CloneHub(matchParent, fabricengine.CloneOptions{
				WeftURL: filepath.ToSlash(weftBare),
				WarpURL: filepath.ToSlash(warpBare),
				Subpath: "backend",
			})
			if err != nil {
				t.Fatalf("CloneHub() with matching --subpath backend error = %v; want nil", err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(matchRes.HubPath) })
			if matchRes.Anchor != "backend" {
				t.Errorf("matchRes.Anchor = %q; want %q", matchRes.Anchor, "backend")
			}
		})
	}
}

// TestCloneHub_StaleFabricAnchorHardErrors asserts a hard error for a weft remote with a stale marker.
// The stale marker is ".fabric-anchor" with no ".lym-anchor" beside it; the error names the marker rename as the remedy,
// and teardownHub removes the hub.
// An old clone must never silently fall through to the create path and re-anchor under ".lym-anchor".
func TestCloneHub_StaleFabricAnchorHardErrors(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixtures := t.TempDir()

	warpBare := makeBareRemoteWithSubdir(t, fixtures, "stale-anchor-warp", "backend")
	weftBare := makeBareRemote(t, fixtures, "stale-anchor-weft")

	// Seed the weft remote's main branch with a committed .fabric-anchor and no .lym-anchor beside it;
	// that is what an old clone's Bolt checkout would carry.
	commitFileOnBranch(t, fixtures, weftBare, "main", ".fabric-anchor", "backend\n")

	cloneParent := t.TempDir()
	expectedHubPath := fabricengine.HubPath(cloneParent, fabricengine.DeriveWarpName(filepath.ToSlash(warpBare)))

	// No ForceBootstrap: the guard admits this fixture, since its probe recognises a stale .fabric-anchor marker.
	// The test asserts the stale-marker error that names the marker rename as the remedy;
	// the bootstrap guard never produces that message.
	_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL: filepath.ToSlash(weftBare),
		WarpURL: filepath.ToSlash(warpBare),
		Subpath: "",
	})
	if err == nil {
		t.Fatalf("CloneHub() against a stale .fabric-anchor with no .lym-anchor should have failed")
	}
	// The remedy must be the marker rename, never "re-clone": this error is produced by a clone;
	// prescribing another clone is a loop the operator cannot exit.
	if !strings.Contains(err.Error(), "git mv .fabric-anchor .lym-anchor") {
		t.Errorf("CloneHub() error = %q; want it to name the marker rename as the remedy", err.Error())
	}
	if strings.Contains(err.Error(), "re-clone this hub") {
		t.Errorf("CloneHub() error = %q; want it NOT to prescribe the very action that produced it", err.Error())
	}
	if _, statErr := os.Stat(expectedHubPath); statErr == nil {
		t.Errorf("hub directory %s should have been removed by teardownHub after the stale-marker guard failure", expectedHubPath)
	}
}

// TestCloneHub_RejectsUnusableSubpath asserts that an impossible --subpath is refused before anything is created.
// An absolute subpath is recorded verbatim, and every weft commit then fails with git's "Invalid path '/backend'";
// ScopedPathspec builds an absolute pathspec from it.
// An escaping subpath must be refused up front as well;
// left to a later resolver, it would be blamed on a marker that was never written.
func TestCloneHub_RejectsUnusableSubpath(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name    string
		subpath string
	}{
		{"absolute", "/backend"},
		{"escapes one level", ".."},
		{"escapes two levels", "../.."},
		{"escapes via a segment", "backend/../.."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixtures := t.TempDir()
			warpBare := makeBareRemoteWithSubdir(t, fixtures, "unusable-subpath-warp", "backend")
			weftBare := makeBareRemote(t, fixtures, "unusable-subpath-weft")

			cloneParent := t.TempDir()
			expectedHubPath := fabricengine.HubPath(cloneParent, fabricengine.DeriveWarpName(filepath.ToSlash(warpBare)))

			_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
				WeftURL:        filepath.ToSlash(weftBare),
				WarpURL:        filepath.ToSlash(warpBare),
				Subpath:        tt.subpath,
				ForceBootstrap: true,
				Shortname:      "tst",
			})
			if err == nil {
				t.Fatalf("CloneHub(--subpath %q) = nil error; want a rejection", tt.subpath)
			}
			if !errors.Is(err, lymcwd.ErrInvalidAnchor) {
				t.Errorf("CloneHub(--subpath %q) error = %v; want wrapped ErrInvalidAnchor", tt.subpath, err)
			}
			if _, statErr := os.Stat(expectedHubPath); statErr == nil {
				t.Errorf("hub directory %s exists after a rejected --subpath; want nothing created", expectedHubPath)
			}
		})
	}
}

// TestCloneHub_RefusesAWarpRemoteWhoseHeadNamesANonexistentBranch pins that CloneHub refuses such a remote.
//
// Such a remote's HEAD names a branch that does not exist, while other branches exist.
// `git clone` exits 0 on it, so the refusal cannot rely on the clone's exit status.
//
// A warp remote with no branches at all is the empty-remote bootstrap and must still clone;
// an unborn warp HEAD is a legitimate documented state on its own.
// Only the combination of an unborn HEAD and existing branches is refused.
func TestCloneHub_RefusesAWarpRemoteWhoseHeadNamesANonexistentBranch(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("UnbornHeadWithBranchesOnTheRemoteIsRefused", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		weftRemote := makeBareRemote(t, dir, "dangling-head-weft")
		warpRemote := makeBareRemote(t, dir, "dangling-head-warp")
		// makeBareRemote pushes `main`; this repoints the bare repo's HEAD at a branch that is never created.
		// That is the state `git init --bare` (no -b) leaves behind.
		gitkit.MustRun(t, warpRemote, "git", "symbolic-ref", "HEAD", "refs/heads/master")

		into := filepath.Join(dir, "into-dangling")
		if err := os.Mkdir(into, 0o755); err != nil {
			t.Fatalf("mkdir into: %v", err)
		}

		_, err := fabricengine.CloneHub(into, fabricengine.CloneOptions{
			WeftURL:        filepath.ToSlash(weftRemote),
			WarpURL:        filepath.ToSlash(warpRemote),
			ForceBootstrap: true,
			Shortname:      "tst",
		})
		if err == nil {
			t.Fatalf("CloneHub against a warp remote whose HEAD names a nonexistent ref = nil; want a refusal")
		}
		if !strings.Contains(err.Error(), "nothing checked out") {
			t.Errorf("CloneHub error = %q; want it to name the unchecked-out clone", err)
		}

		// The strict-abort teardown must leave no residual hub, as every CloneHub failure does.
		entries, readErr := os.ReadDir(into)
		if readErr != nil {
			t.Fatalf("read into dir: %v", readErr)
		}
		if len(entries) != 0 {
			t.Errorf("residual hub left behind after the refusal: %v", entries)
		}
	})

	t.Run("EmptyWarpRemoteStillClones", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		weftRemote := makeBareRemote(t, dir, "empty-warp-weft")
		emptyWarp := filepath.Join(dir, "empty-warp.git")
		if err := os.Mkdir(emptyWarp, 0o755); err != nil {
			t.Fatalf("mkdir empty warp bare: %v", err)
		}
		gitkit.MustRun(t, emptyWarp, "git", "init", "--bare")

		into := filepath.Join(dir, "into-empty")
		if err := os.Mkdir(into, 0o755); err != nil {
			t.Fatalf("mkdir into: %v", err)
		}

		if _, err := fabricengine.CloneHub(into, fabricengine.CloneOptions{
			WeftURL:        filepath.ToSlash(weftRemote),
			WarpURL:        filepath.ToSlash(emptyWarp),
			ForceBootstrap: true,
			Shortname:      "tst",
		}); err != nil {
			t.Fatalf("CloneHub against a genuinely EMPTY warp remote = %v; want nil — an unborn warp HEAD with no remote branches is the documented bootstrap state, not the refused conjunction", err)
		}
	})
}
