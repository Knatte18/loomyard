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
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/weftname"
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

	gitkit.MustRun(t, bare, "git", "init", "--bare")

	tempWork := filepath.Join(dir, "temp-work-"+name)
	if err := os.Mkdir(tempWork, 0o755); err != nil {
		t.Fatalf("mkdir temp work: %v", err)
	}

	gitkit.MustRun(t, tempWork, "git", "init", "-b", "main")
	gitkit.MustRun(t, tempWork, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, tempWork, "git", "config", "user.name", "Test")

	bareURL := filepath.ToSlash(bare)
	gitkit.MustRun(t, tempWork, "git", "remote", "add", "origin", bareURL)

	readmePath := filepath.Join(tempWork, "README.md")
	if err := os.WriteFile(readmePath, []byte("# "+name), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}

	gitkit.MustRun(t, tempWork, "git", "add", "README.md")
	gitkit.MustRun(t, tempWork, "git", "commit", "-m", "init")
	gitkit.MustRun(t, tempWork, "git", "push", "-u", "origin", "main")

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
// the test on any error — the capture-variant sibling of gitkit.MustRun,
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

// makeBareRemoteWithSubdir behaves exactly like makeBareRemote, but seeds an
// additional committed file at <subdir>/marker.txt so the warp repo carries a
// real subdirectory a lyx-anchor subpath test can point at.
func makeBareRemoteWithSubdir(t *testing.T, dir, name, subdir string) string {
	t.Helper()

	bare := filepath.Join(dir, name+".git")
	if err := os.Mkdir(bare, 0o755); err != nil {
		t.Fatalf("mkdir bare: %v", err)
	}

	gitkit.MustRun(t, bare, "git", "init", "--bare")

	tempWork := filepath.Join(dir, "temp-work-"+name)
	if err := os.Mkdir(tempWork, 0o755); err != nil {
		t.Fatalf("mkdir temp work: %v", err)
	}

	gitkit.MustRun(t, tempWork, "git", "init", "-b", "main")
	gitkit.MustRun(t, tempWork, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, tempWork, "git", "config", "user.name", "Test")

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

	gitkit.MustRun(t, tempWork, "git", "add", "README.md", subdir)
	gitkit.MustRun(t, tempWork, "git", "commit", "-m", "init")
	gitkit.MustRun(t, tempWork, "git", "push", "-u", "origin", "main")

	if err := os.RemoveAll(tempWork); err != nil {
		t.Fatalf("remove temp work: %v", err)
	}

	return bare
}

// commitFileOnBranch clones bareRemote into a scratch dir, checks out branch
// (creating it if absent, seeded from the remote's default branch tip),
// writes relPath with contents, commits, and pushes it back — used to seed a
// weft remote fixture with a pre-committed .lyx-anchor marker for the
// adopt-path tests, mirroring how a real prior clone would have left it.
func commitFileOnBranch(t *testing.T, dir, bareRemote, branch, relPath, contents string) {
	t.Helper()

	scratch := filepath.Join(dir, "scratch-"+branch+"-"+filepath.Base(relPath))
	gitkit.MustRun(t, dir, "git", "clone", filepath.ToSlash(bareRemote), filepath.Base(scratch))
	gitkit.MustRun(t, scratch, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, scratch, "git", "config", "user.name", "Test")
	gitkit.MustRun(t, scratch, "git", "checkout", branch)

	target := filepath.Join(scratch, relPath)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", relPath, err)
	}
	if err := os.WriteFile(target, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
	gitkit.MustRun(t, scratch, "git", "add", relPath)
	gitkit.MustRun(t, scratch, "git", "commit", "-m", "seed "+relPath)
	gitkit.MustRun(t, scratch, "git", "push", "origin", branch)
}

// makeEmptyBareRemote creates a bare git repository with no commits at all —
// a genuinely empty remote, unlike makeBareRemote's seed-and-push. It exists
// to exercise ensureBoardWorktree's orphan path, which only fires when clone
// leaves no local warpBranch ref to adopt.
func makeEmptyBareRemote(t *testing.T, dir, name string) string {
	t.Helper()

	bare := filepath.Join(dir, name+".git")
	gitkit.MustRun(t, dir, "git", "init", "--bare", "-b", "main", bare)

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
// fabricengine.BoardDir(hubPath)) shares its git-common-dir with weftPrime —
// proving _board is a linked worktree of the same weft repo, not a separate
// clone — and that _board is checked out on wantBranch.
func assertBoardIsWeftWorktree(t *testing.T, hubPath, weftPrime, wantBranch string) {
	t.Helper()
	boardPath := fabricengine.BoardDir(hubPath)

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

// TestCloneHub_AdoptsExistingRemoteWeftPrimaryBranch re-clones against a weft remote whose
// main-weft branch is ahead of main and asserts the fresh weft primary adopted it: checked out on
// main-weft, at the remote branch's tip (the synced marker file present), with origin/main-weft as
// its upstream.
func TestCloneHub_AdoptsExistingRemoteWeftPrimaryBranch(t *testing.T) {
	fixtures := t.TempDir()

	warpBare := makeBareRemote(t, fixtures, "adopt-warp")
	weftBare := makeBareRemote(t, fixtures, "adopt-weft")

	// Advance the weft remote's main-weft past main, the way a previously
	// active hub's synced weft history would have: one extra commit carrying a
	// marker file that only exists on main-weft.
	seedWork := filepath.Join(fixtures, "seed-weft")
	gitkit.MustRun(t, fixtures, "git", "clone", filepath.ToSlash(weftBare), "seed-weft")
	gitkit.MustRun(t, seedWork, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, seedWork, "git", "config", "user.name", "Test")
	gitkit.MustRun(t, seedWork, "git", "checkout", "-b", "main-weft")
	markerName := "synced-weft-state.txt"
	if err := os.WriteFile(filepath.Join(seedWork, markerName), []byte("weft history"), 0o644); err != nil {
		t.Fatalf("write weft marker: %v", err)
	}
	gitkit.MustRun(t, seedWork, "git", "add", markerName)
	gitkit.MustRun(t, seedWork, "git", "commit", "-m", "weft sync")
	gitkit.MustRun(t, seedWork, "git", "push", "-u", "origin", "main-weft")

	remoteTip := gitOutput(t, seedWork, "rev-parse", "main-weft")

	// Clone the hub fresh, as a second machine (or clone --reset) would.
	cloneParent := t.TempDir()
	// ForceBootstrap: true — weftBare is an ordinary seeded bare remote standing in for a weft,
	// not a repo that has ever been one, so it carries no .lyx-anchor.
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(weftBare),
		WarpURL:        filepath.ToSlash(warpBare),
		Subpath:        ".",
		ForceBootstrap: true,
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	hubPath := res.HubPath
	t.Cleanup(func() { _ = os.RemoveAll(hubPath) })

	// The weft prime directory is the warp name's weft sibling — resolved via
	// lyxcwd so the assertion cannot rot against clone's own geometry.
	weftPrime := weftname.SiblingPath(hubPath, "adopt-warp")

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
	// the unsuffixed warp branch "main" — adopted from the local ref
	// cloneRepo already created, proving _board is not a separate clone.
	assertBoardIsWeftWorktree(t, hubPath, weftPrime, "main")
}

// TestCloneHub_CreatesFreshWeftPrimaryBranch asserts that when the weft remote carries no existing
// WeftBranchName-suffixed branch (a genuinely new hub — the non-adopt path), CloneHub creates the
// weft primary's suffixed branch fresh at the cloned HEAD rather than requiring a pre-existing
// remote ref to adopt.
func TestCloneHub_CreatesFreshWeftPrimaryBranch(t *testing.T) {
	fixtures := t.TempDir()

	warpBare := makeBareRemote(t, fixtures, "fresh-warp")
	weftBare := makeBareRemote(t, fixtures, "fresh-weft")

	cloneParent := t.TempDir()
	// ForceBootstrap: true — weftBare is an ordinary seeded bare remote standing in for a weft,
	// not a repo that has ever been one, so it carries no .lyx-anchor.
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(weftBare),
		WarpURL:        filepath.ToSlash(warpBare),
		Subpath:        ".",
		ForceBootstrap: true,
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
	want := fabricengine.WeftBranchName("main")
	if got := currentBranch(t, weftPrime); got != want {
		t.Fatalf("weft prime branch = %q; want %q (freshly created, no remote suffixed branch to adopt)", got, want)
	}

	// _board must be a second worktree of the same weft repo, checked out on
	// the unsuffixed warp branch "main" — a linked worktree, not a separate
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

// TestCloneHub_StrictAbortRemovesHubOnFailure covers teardownHub's cleanup-on-failure behaviour: a
// failing warp clone leaves no residual Hub directory behind, torn down through fabricengine's own
// RemoveAll teardown seam (clone.go's teardownHub). The weft side must be a valid fixture so the
// pre-hub probe succeeds and the hub directory is actually created — pointing the weft at a
// nonexistent path instead would fail at the probe, before teardownHub is ever reached, which is now
// covered as a probe-taxonomy hard error rather than this residual-hub path.
func TestCloneHub_StrictAbortRemovesHubOnFailure(t *testing.T) {
	fixtures := t.TempDir()

	weftBare := makeBareRemote(t, fixtures, "abort-weft")
	nonExistentWarp := filepath.Join(fixtures, "nonexistent-warp.git")

	cloneParent := t.TempDir()
	expectedHubPath := fabricengine.HubPath(cloneParent, fabricengine.DeriveWarpName(filepath.ToSlash(nonExistentWarp)))

	// ForceBootstrap: true — weftBare is an ordinary seeded bare remote standing in for a weft,
	// not a repo that has ever been one, so it carries no .lyx-anchor; without this the old-order
	// guard would fire before the warp clone is even attempted.
	_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(weftBare),
		WarpURL:        filepath.ToSlash(nonExistentWarp),
		Subpath:        ".",
		ForceBootstrap: true,
	})
	if err == nil {
		t.Fatalf("CloneHub should have failed with a non-existent warp remote")
	}
	if _, statErr := os.Stat(expectedHubPath); statErr == nil {
		t.Errorf("hub directory %s should have been removed by teardownHub after clone failure", expectedHubPath)
	}
}

// TestCloneHub_BoardWorktreeOrphanBranchOnEmptyWeftRemote asserts that when the weft remote is
// genuinely empty (no commits, so cloneRepo leaves no local warpBranch ref to adopt),
// ensureBoardWorktree's orphan path fires: _board is materialized as a second weft worktree on a
// freshly orphan- created "main" branch that shares no history with the weft primary's "main-weft"
// branch — both end up with no commits at all.
func TestCloneHub_BoardWorktreeOrphanBranchOnEmptyWeftRemote(t *testing.T) {
	fixtures := t.TempDir()

	// The warp remote needs a real commit for suffixWeftPrimaryBranch to read
	// a checked-out branch; the weft remote is genuinely empty.
	warpBare := makeBareRemote(t, fixtures, "orphan-warp")
	weftBare := makeEmptyBareRemote(t, fixtures, "orphan-weft")

	cloneParent := t.TempDir()
	// weftBare is a genuinely empty remote (makeEmptyBareRemote): the probe's unborn-HEAD check
	// sets WeftLooksLikeWeft, so the old-order guard never fires and no ForceBootstrap is needed.
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL: filepath.ToSlash(weftBare),
		WarpURL: filepath.ToSlash(warpBare),
		Subpath: ".",
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	hubPath := res.HubPath
	t.Cleanup(func() { _ = os.RemoveAll(hubPath) })

	weftPrime := weftname.SiblingPath(hubPath, "orphan-warp")
	if got := currentBranch(t, weftPrime); got != "main-weft" {
		t.Fatalf("weft prime branch = %q; want %q", got, "main-weft")
	}
	// The weft prime's suffixed branch must be BORN — a real ref, not merely the checked-out name.
	// This assertion is inverted from what it was: it used to require an unborn HEAD here, which
	// pinned a defect rather than a contract. `git checkout -b` on an unborn HEAD writes no ref, so
	// refs/heads/main-weft did not exist, and every verb that forks a new pair from the primary died
	// on `fatal: invalid reference: main-weft` — `lyx fabric add` included, which is the example both
	// the parent command and `add` document. CloneHub now lands an initialising empty commit on that
	// branch (bornWeftPrimaryBranch, clone.go).
	if hasNoCommits(t, weftPrime) {
		t.Errorf("weft prime at %s has an unborn HEAD; want its suffixed branch born so a pair can fork from it", weftPrime)
	}

	// _board must still be a linked worktree of the same weft repo, checked
	// out on "main", and itself carry no commits — it is orphan-created, so it
	// gains nothing from the weft primary's initialising commit.
	assertBoardIsWeftWorktree(t, hubPath, weftPrime, "main")
	boardPath := fabricengine.BoardDir(hubPath)
	if !hasNoCommits(t, boardPath) {
		t.Errorf("_board at %s has commits; want an unborn HEAD (fresh orphan branch)", boardPath)
	}
}

// TestCloneHub_AnchorCreatePath asserts the create path: a first-ever clone with an existing
// "backend" subdirectory in the warp writes the marker to disk and returns a fully-populated
// CloneResult naming it.
func TestCloneHub_AnchorCreatePath(t *testing.T) {
	fixtures := t.TempDir()

	warpBare := makeBareRemoteWithSubdir(t, fixtures, "anchor-create-warp", "backend")
	weftBare := makeBareRemote(t, fixtures, "anchor-create-weft")

	cloneParent := t.TempDir()
	// ForceBootstrap: true — weftBare is an ordinary seeded bare remote standing in for a weft,
	// not a repo that has ever been one, so it carries no .lyx-anchor.
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(weftBare),
		WarpURL:        filepath.ToSlash(warpBare),
		Subpath:        "backend",
		ForceBootstrap: true,
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })

	if res.Anchor != "backend" {
		t.Errorf("res.Anchor = %q; want %q", res.Anchor, "backend")
	}
	if filepath.Base(res.BoardDir) != "_board" {
		t.Errorf("res.BoardDir = %q; want it to end in _board", res.BoardDir)
	}
	if filepath.Base(res.PrimeCwd) != "backend" {
		t.Errorf("res.PrimeCwd = %q; want it to end in .../backend", res.PrimeCwd)
	}
	if res.WeftBase == "" {
		t.Errorf("res.WeftBase is empty; want a resolved weft-side base directory")
	}

	markerPath := filepath.Join(res.BoardDir, lyxcwd.AnchorFileName)
	data, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("read marker %s: %v", markerPath, err)
	}
	if got := strings.TrimSpace(string(data)); got != "backend" {
		t.Errorf(".lyx-anchor content = %q; want %q", got, "backend")
	}
}

// TestCloneHub_AnchorTypoPathHardErrors asserts that a create-path clone against a subpath that
// does not exist in the warp worktree (a typo like "backedn") is a hard error,
// and that teardownHub removes the hub — mirroring TestCloneHub_StrictAbortRemovesHubOnFailure's
// coverage for the anchor guard.
func TestCloneHub_AnchorTypoPathHardErrors(t *testing.T) {
	fixtures := t.TempDir()

	warpBare := makeBareRemoteWithSubdir(t, fixtures, "anchor-typo-warp", "backend")
	weftBare := makeBareRemote(t, fixtures, "anchor-typo-weft")

	cloneParent := t.TempDir()
	expectedHubPath := fabricengine.HubPath(cloneParent, fabricengine.DeriveWarpName(filepath.ToSlash(warpBare)))

	// ForceBootstrap: true — weftBare is an ordinary seeded bare remote standing in for a weft,
	// not a repo that has ever been one, so it carries no .lyx-anchor.
	_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(weftBare),
		WarpURL:        filepath.ToSlash(warpBare),
		Subpath:        "backedn",
		ForceBootstrap: true,
	})
	if err == nil {
		t.Fatalf("CloneHub() with a nonexistent subpath should have failed")
	}
	if _, statErr := os.Stat(expectedHubPath); statErr == nil {
		t.Errorf("hub directory %s should have been removed by teardownHub after the anchor guard failure", expectedHubPath)
	}
}

// TestCloneHub_AnchorFileNotDirectoryHardErrors asserts that a create-path clone against a subpath
// naming an existing FILE is refused with a message that says so — "does not exist" alone misled,
// since the path plainly exists — and that teardownHub removes the hub.
func TestCloneHub_AnchorFileNotDirectoryHardErrors(t *testing.T) {
	fixtures := t.TempDir()

	warpBare := makeBareRemote(t, fixtures, "anchor-file-warp")
	weftBare := makeBareRemote(t, fixtures, "anchor-file-weft")

	cloneParent := t.TempDir()
	expectedHubPath := fabricengine.HubPath(cloneParent, fabricengine.DeriveWarpName(filepath.ToSlash(warpBare)))

	// ForceBootstrap: true — weftBare is an ordinary seeded bare remote standing in for a weft,
	// not a repo that has ever been one, so it carries no .lyx-anchor.
	_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(weftBare),
		WarpURL:        filepath.ToSlash(warpBare),
		Subpath:        "README.md",
		ForceBootstrap: true,
	})
	if err == nil {
		t.Fatalf("CloneHub() with a file-valued subpath should have failed")
	}
	if !strings.Contains(err.Error(), "as a directory") {
		t.Errorf("CloneHub() error = %q; want it to say the subpath is not a directory, not that it does not exist", err)
	}
	if _, statErr := os.Stat(expectedHubPath); statErr == nil {
		t.Errorf("hub directory %s should have been removed by teardownHub after the anchor guard failure", expectedHubPath)
	}
}

// TestCloneHub_AnchorRootDefaultPath asserts the create path with an explicit "."
// subpath writes "."
// to the marker and returns Anchor == ".".
func TestCloneHub_AnchorRootDefaultPath(t *testing.T) {
	fixtures := t.TempDir()

	warpBare := makeBareRemote(t, fixtures, "anchor-root-warp")
	weftBare := makeBareRemote(t, fixtures, "anchor-root-weft")

	cloneParent := t.TempDir()
	// ForceBootstrap: true — weftBare is an ordinary seeded bare remote standing in for a weft,
	// not a repo that has ever been one, so it carries no .lyx-anchor.
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(weftBare),
		WarpURL:        filepath.ToSlash(warpBare),
		Subpath:        ".",
		ForceBootstrap: true,
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })

	if res.Anchor != "." {
		t.Errorf("res.Anchor = %q; want %q", res.Anchor, ".")
	}

	markerPath := filepath.Join(res.BoardDir, lyxcwd.AnchorFileName)
	data, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("read marker %s: %v", markerPath, err)
	}
	if got := strings.TrimSpace(string(data)); got != "." {
		t.Errorf(".lyx-anchor content = %q; want %q", got, ".")
	}
}

// TestCloneHub_AnchorAdoptPath covers the adopt path against a weft remote already carrying a
// committed .lyx-anchor="backend" on its default branch (mirroring what CloneHub's own create path
// would have left, had the CLI layer committed it onto weft:main): a re-clone with no --subpath
// reads the recorded value;
// a conflicting non-default --subpath hard-errors;
// a matching --subpath succeeds.
func TestCloneHub_AnchorAdoptPath(t *testing.T) {
	fixtures := t.TempDir()

	warpBare := makeBareRemoteWithSubdir(t, fixtures, "anchor-adopt-warp", "backend")
	weftBare := makeBareRemote(t, fixtures, "anchor-adopt-weft")

	// Seed the weft remote's main branch with a committed .fabric-anchor, the
	// same file _board's checkout will already carry once cloned — this is
	// what "adopt" means: the marker arrives via git history, not a write.
	commitFileOnBranch(t, fixtures, weftBare, "main", lyxcwd.AnchorFileName, "backend\n")

	// No --subpath: the recorded anchor is read and returned as-is. No ForceBootstrap needed: the
	// weft already carries the .lyx-anchor marker seeded above, so the guard admits it regardless.
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

	// A conflicting non-default --subpath must hard-error rather than
	// silently re-anchor: the record is authoritative on adopt.
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
}

// TestCloneHub_StaleFabricAnchorHardErrors asserts that a weft remote carrying a leftover
// pre-rename ".fabric-anchor" marker with no ".lyx-anchor" beside it is a hard error naming
// re-clone as the remedy,
// and that teardownHub removes the hub — an old clone must never silently fall through to the
// create path and re-anchor under the new name.
func TestCloneHub_StaleFabricAnchorHardErrors(t *testing.T) {
	fixtures := t.TempDir()

	warpBare := makeBareRemoteWithSubdir(t, fixtures, "stale-anchor-warp", "backend")
	weftBare := makeBareRemote(t, fixtures, "stale-anchor-weft")

	// Seed the weft remote's main branch with a committed .fabric-anchor
	// under the pre-rename name only — no .lyx-anchor beside it — mirroring
	// what an old clone's _board checkout would carry.
	commitFileOnBranch(t, fixtures, weftBare, "main", ".fabric-anchor", "backend\n")

	cloneParent := t.TempDir()
	expectedHubPath := fabricengine.HubPath(cloneParent, fabricengine.DeriveWarpName(filepath.ToSlash(warpBare)))

	// No ForceBootstrap: the guard admits this fixture on its own (card 3's probe recognises the
	// stale pre-rename marker), and the test asserts on the stale-marker error naming the marker
	// rename as the remedy — a message the bootstrap guard never produces.
	_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL: filepath.ToSlash(weftBare),
		WarpURL: filepath.ToSlash(warpBare),
		Subpath: "",
	})
	if err == nil {
		t.Fatalf("CloneHub() against a stale .fabric-anchor with no .lyx-anchor should have failed")
	}
	// The remedy must be the marker rename, never "re-clone": this error is produced BY a clone, so
	// prescribing another clone is a loop the operator cannot exit.
	if !strings.Contains(err.Error(), "git mv .fabric-anchor .lyx-anchor") {
		t.Errorf("CloneHub() error = %q; want it to name the marker rename as the remedy", err.Error())
	}
	if strings.Contains(err.Error(), "re-clone this hub") {
		t.Errorf("CloneHub() error = %q; want it NOT to prescribe the very action that produced it", err.Error())
	}
	if _, statErr := os.Stat(expectedHubPath); statErr == nil {
		t.Errorf("hub directory %s should have been removed by teardownHub after the stale-marker guard failure", expectedHubPath)
	}
}

// TestCloneHub_RejectsUnusableSubpath asserts a structurally impossible --subpath is refused
// BEFORE anything is created. An absolute subpath used to be accepted and recorded verbatim,
// producing a hub whose every weft commit failed with git's "Invalid path '/backend'" because
// ScopedPathspec built an absolute pathspec from it; an escaping subpath was caught only much
// later, by an unrelated resolver, with a diagnosis blaming a marker that had never been written.
func TestCloneHub_RejectsUnusableSubpath(t *testing.T) {
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
			})
			if err == nil {
				t.Fatalf("CloneHub(--subpath %q) = nil error; want a rejection", tt.subpath)
			}
			if !errors.Is(err, lyxcwd.ErrInvalidAnchor) {
				t.Errorf("CloneHub(--subpath %q) error = %v; want wrapped ErrInvalidAnchor", tt.subpath, err)
			}
			if _, statErr := os.Stat(expectedHubPath); statErr == nil {
				t.Errorf("hub directory %s exists after a rejected --subpath; want nothing created", expectedHubPath)
			}
		})
	}
}
