// add_rollback_adopt_test.go proves that Add's rollback never deletes a pre-existing (adopted) weft branch.
// When Add adopts an existing <slug>-weft branch and then fails at a later step:
// the rollback tears down only the worktree Add created; the branch, and any unpushed history it carries, survives.
// This test injects a deterministic post-adopt failure, a launcher blocker directory, instead of a network failure.
//
// Package fabricengine_test, reusing the external-test-package fixture idiom of lifecycle_differential_test.go;
// it shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// mustRecordsRepoRoot resolves fabricengine.RecordsRepoRoot(l), failing the test on any error.
// The many call sites across this package's tests use it to get the weft prime worktree path.
func mustRecordsRepoRoot(t *testing.T, l *lymcwd.Location) string {
	t.Helper()

	root, err := fabricengine.RecordsRepoRoot(l)
	if err != nil {
		t.Fatalf("RecordsRepoRoot: %v", err)
	}
	return root
}

// refuseRecordsCommits points l's records repository at a hooks directory whose pre-commit hook refuses every commit;
// so Add fails at its origin-record commit, the last step that can fail after the pair is wired.
func refuseRecordsCommits(t *testing.T, l *lymcwd.Location) {
	t.Helper()

	hooks := t.TempDir()
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\necho refused by test >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write pre-commit hook: %v", err)
	}
	gitkit.MustRun(t, mustRecordsRepoRoot(t, l), "git", "config", "core.hooksPath", hooks)
}

// plantLauncherBlocker plants a directory at the pair's ide launcher script path;
// so Add fails writing the launcher scripts after the pair's worktrees and weft branch exist.
// The rollback's launcher teardown removes the planted directory and the launcher directory.
// It returns the planted path.
func plantLauncherBlocker(tb testing.TB, l *lymcwd.Location, slug string) string {
	tb.Helper()

	scriptName := "ide.sh"
	if runtime.GOOS == "windows" {
		scriptName = "ide.cmd"
	}
	blocker := filepath.Join(fabricengine.LauncherDir(l, slug), scriptName)
	if err := os.MkdirAll(blocker, 0o755); err != nil {
		tb.Fatalf("plant launcher blocker %s: %v", blocker, err)
	}
	return blocker
}

// TestAddRollback_AdoptedWeftBranchSurvives pre-creates a weft branch carrying a unique commit,
// forces Add to fail after adopting it, and asserts the rollback removes only the weft worktree.
// The branch stays at that unique commit, and no warp-side residue is left.
func TestAddRollback_AdoptedWeftBranchSurvives(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "adopt-rollback-keep"
	// The hub from hubforge.CopyHub already has the real-hub shape this test needs:
	// the weft primary is checked out on the suffixed sibling of the warp's branch;
	// a real Bolt checkout worktree is in place for the gate's ownedManagedBranch/primaryWeftBranch reads;
	// and the repo-wide fabric.yaml is present for Add's eager RepoWiredNames load.
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftBranch := fabricengine.RecordsBranchName(slug)

	// Pre-create the weft branch with a unique commit that predates the Add;
	// the rollback must not destroy that history.
	// The seeding worktree is removed again, so Add can adopt the branch.
	seedDir := filepath.Join(t.TempDir(), "seed")
	gitkit.MustRun(t, mustRecordsRepoRoot(t, l), "git", "worktree", "add", "-b", weftBranch, seedDir, fabricengine.RecordsBranchName("main"))
	preciousSHA := gitkit.CommitFile(t, seedDir, "precious.txt", "pre-existing weft work\n", "precious pre-existing weft work")
	gitkit.MustRun(t, mustRecordsRepoRoot(t, l), "git", "worktree", "remove", seedDir)

	// Inject a deterministic failure after the adopt:
	// a directory at the ide launcher script path makes writeLaunchers fail, triggering rollback.
	plantLauncherBlocker(t, l, slug)

	const branchPrefix = "task/"
	warpBranch := branchPrefix + slug
	topology := fabricengine.NewTopology(fabricengine.Config{BranchPrefix: branchPrefix})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err == nil {
		t.Fatalf("Add should have failed (launcher blocker)")
	}

	// The adopted branch survives the rollback, still at its unique commit.
	if !gitkit.BranchExists(t, mustRecordsRepoRoot(t, l), weftBranch) {
		t.Fatalf("adopted weft branch %q was deleted by Add's rollback; want it preserved", weftBranch)
	}
	branchSHA := gitkit.RevParse(t, mustRecordsRepoRoot(t, l), "refs/heads/"+weftBranch)
	if branchSHA != preciousSHA {
		t.Errorf("adopted weft branch %q = %s; want the pre-existing commit %s", weftBranch, branchSHA, preciousSHA)
	}

	// Everything Add itself created is rolled back:
	// no weft worktree dir, no warp worktree dir, no warp branch.
	if _, err := os.Stat(fabricengine.RecordsWorktreePath(l, slug)); !os.IsNotExist(err) {
		t.Errorf("weft worktree dir still exists at %s", fabricengine.RecordsWorktreePath(l, slug))
	}
	if _, err := os.Stat(fabricengine.WorktreePath(l, slug)); !os.IsNotExist(err) {
		t.Errorf("warp worktree dir still exists at %s", fabricengine.WorktreePath(l, slug))
	}
	if gitkit.BranchExists(t, l.WorktreePath(), warpBranch) {
		t.Errorf("warp branch %q still exists", warpBranch)
	}
}

// TestAddRollback_LiveWeftFromOrigin forces Add to fail after it took a live pair's weft branch from origin.
// It asserts that the rollback leaves origin untouched:
// a local branch Add created from origin is deleted,
// and a pre-existing local branch Add fast-forwarded stays at origin's tip, not rewound.
func TestAddRollback_LiveWeftFromOrigin(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	for _, tc := range []struct {
		name           string
		localBehind    bool
		wantBranchGone bool
	}{
		{name: "origin-only branch created locally is deleted", wantBranchGone: true},
		{name: "fast-forwarded local branch stays at origin's tip", localBehind: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			slug := "live-rollback-origin"
			if tc.localBehind {
				slug = "live-rollback-ff"
			}
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			l := h.Location
			weftBranch := fabricengine.RecordsBranchName(slug)
			weftRoot := mustRecordsRepoRoot(t, l)

			clone := t.TempDir()
			gitkit.MustRun(t, clone, "git", "clone", "--quiet", h.RecordsBare, ".")
			gitkit.MustRun(t, clone, "git", "checkout", "--quiet", "-b", weftBranch)
			// The branch records its anchor, so adopting it commits nothing the rollback would leave on top of origin's tip.
			gitkit.CommitFile(t, clone, lymcwd.AnchorFileName, l.AnchorRel+"\n", "record the anchor")
			gitkit.CommitFile(t, clone, "first.txt", "first\n", "first origin work")
			gitkit.MustRun(t, clone, "git", "push", "--quiet", "origin", weftBranch)
			if tc.localBehind {
				gitkit.MustRun(t, weftRoot, "git", "fetch", "--quiet", "origin", weftBranch)
				gitkit.MustRun(t, weftRoot, "git", "branch", weftBranch, "FETCH_HEAD")
				gitkit.CommitFile(t, clone, "second.txt", "second\n", "second origin work")
				gitkit.MustRun(t, clone, "git", "push", "--quiet", "origin", weftBranch)
			}
			originTip := gitkit.RevParse(t, h.RecordsBare, weftBranch)

			// A blocker at the ide launcher script path fails Add at writeLaunchers, after the weft branch is taken from origin.
			plantLauncherBlocker(t, l, slug)

			if _, err := h.Topology.Add(l, slug, fabricengine.AddOptions{}); err == nil {
				t.Fatalf("Add should have failed (launcher blocker)")
			}

			if got := gitkit.RevParse(t, h.RecordsBare, weftBranch); got != originTip {
				t.Errorf("origin weft branch = %s; want unchanged %s", got, originTip)
			}
			exists := gitkit.BranchExists(t, weftRoot, weftBranch)
			if exists == tc.wantBranchGone {
				t.Errorf("local weft branch exists = %v; want %v", exists, !tc.wantBranchGone)
			}
			if exists {
				if got := gitkit.RevParse(t, weftRoot, "refs/heads/"+weftBranch); got != originTip {
					t.Errorf("local weft branch = %s; want origin's tip %s", got, originTip)
				}
			}
			if _, err := os.Stat(fabricengine.RecordsWorktreePath(l, slug)); !os.IsNotExist(err) {
				t.Errorf("weft worktree dir still exists at %s", fabricengine.RecordsWorktreePath(l, slug))
			}
		})
	}
}

// TestAddRollback_WarpBranchDeletedUnderEmptyPrefix pins the empty branch_prefix case:
// the rollback deletes the bare-slug warp branch Add created, together with the worktree pair.
func TestAddRollback_WarpBranchDeletedUnderEmptyPrefix(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "empty-prefix-rollback"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location

	// Inject a deterministic post-creation failure:
	// a directory at the ide launcher script path makes writeLaunchers fail after the warp worktree and its branch exist.
	// That triggers rollbackAdd.
	plantLauncherBlocker(t, l, slug)

	// Default empty branch_prefix: the warp branch is exactly the bare slug.
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err == nil {
		t.Fatalf("Add should have failed (launcher blocker)")
	}

	// The worktree pair is fully rolled back.
	if _, err := os.Stat(fabricengine.WorktreePath(l, slug)); !os.IsNotExist(err) {
		t.Errorf("warp worktree dir still exists at %s after rollback", fabricengine.WorktreePath(l, slug))
	}
	if _, err := os.Stat(fabricengine.RecordsWorktreePath(l, slug)); !os.IsNotExist(err) {
		t.Errorf("weft worktree dir still exists at %s after rollback", fabricengine.RecordsWorktreePath(l, slug))
	}

	if gitkit.BranchExists(t, l.WorktreePath(), slug) {
		t.Errorf("warp branch %q survived the rollback under an empty prefix; the rollback must delete the branch this Add created", slug)
	}
}

// TestAddRollback_FinishesRefilledWeftWorktree forces Add to fail after its weft worktree exists.
// A late writer refills that directory after the rollback's clean `git worktree remove`,
// and the rollback must still delete it.
func TestAddRollback_FinishesRefilledWeftWorktree(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "refilled-rollback"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftDir := fabricengine.RecordsWorktreePath(l, slug)

	plantLauncherBlocker(t, l, slug)

	refilled := false
	fabricengine.SetLeftoverRefillForTest(h.Topology, func(target string) {
		if target != weftDir {
			return
		}
		refilled = true
		output := filepath.Join(weftDir, "obj", "project.assets.json")
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			t.Errorf("refill %s: %v", weftDir, err)
			return
		}
		if err := os.WriteFile(output, []byte("{}\n"), 0o644); err != nil {
			t.Errorf("refill %s: %v", weftDir, err)
		}
	})

	if _, err := h.Topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err == nil {
		t.Fatalf("Add should have failed (launcher blocker)")
	}

	if !refilled {
		t.Fatalf("no late writer refilled %s; want the refill to run after the rollback's clean git exit", weftDir)
	}
	if _, err := os.Lstat(weftDir); !os.IsNotExist(err) {
		t.Errorf("weft worktree dir %s still on disk after the rollback (stat error = %v)", weftDir, err)
	}
}

// TestAddRollback_AdoptedWarpBranchLocalCopyDeleted forces Add to fail after it adopted a warp branch:
// a live pair's branch taken from origin.
// It asserts the rollback deletes the local copy and leaves origin's branch untouched.
func TestAddRollback_AdoptedWarpBranchLocalCopyDeleted(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "adopted-warp-rollback"
	h := removedPair(t, slug)
	l := h.Location
	// A weft branch on origin that moved past its archive tag makes the pair live, so Add adopts both branches from origin.
	pushCommitToOrigin(t, h.RecordsBare, fabricengine.RecordsBranchName(slug))
	originTip := gitkit.RevParse(t, h.CodeBare, slug)

	plantLauncherBlocker(t, l, slug)

	if _, err := h.Topology.Add(l, slug, fabricengine.AddOptions{}); err == nil {
		t.Fatalf("Add should have failed (launcher blocker)")
	}

	if gitkit.BranchExists(t, l.WorktreePath(), slug) {
		t.Errorf("local warp branch %q adopted from origin survived the rollback", slug)
	}
	if got := gitkit.RevParse(t, h.CodeBare, slug); got != originTip {
		t.Errorf("origin warp branch = %s; want unchanged %s", got, originTip)
	}
}

// TestAdd_WiresJunctionsEagerly proves a successful Add leaves the new worktree's warp junctions wired immediately.
// Add wires them in step 10b, after writeLaunchers and before the warp push.
// The pair is usable at once: no dormant state and no separate `lym init` step.
// Asserts both the repo-wide default junctions (_lym and _extra) resolve to their paired weft
// directories.
//
//testtiming:keep a successful Add leaving the new worktree's junctions wired at once, with _lym and _extra resolving to their weft directories; coverage of its blocks by other tests does not show an assertion of this
func TestAdd_WiresJunctionsEagerly(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "eager-wire-add"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	// hubforge.NewHub seeds the repo-wide config with fabricengine.ConfigTemplate()'s own
	// default pathspec; override it to "_extra" so Add's RepoWiredNames-driven wiring below
	// wires the junction name this test asserts against.
	seedRepoWideExtraFabricConfig(t, l.HubPath)

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	for _, tc := range []struct {
		name   string
		link   string
		target string
	}{
		{"_lym", fabricengine.CodeLymLink(l, slug), fabricengine.WeftLymDirFor(l, slug)},
		{"_extra", filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel, "_extra"), filepath.Join(fabricengine.RecordsWorktreePath(l, slug), l.AnchorRel, "_extra")},
	} {
		isLink, err := fslink.IsLink(tc.link)
		if err != nil || !isLink {
			t.Fatalf("%s: junction at %s is not a link immediately after Add: isLink=%v err=%v", tc.name, tc.link, isLink, err)
		}
		resolved, err := fslink.PointsTo(tc.link)
		if err != nil {
			t.Fatalf("%s: PointsTo(%s): %v", tc.name, tc.link, err)
		}
		wantResolved, err := filepath.EvalSymlinks(tc.target)
		if err != nil {
			t.Fatalf("%s: EvalSymlinks(%s): %v", tc.name, tc.target, err)
		}
		if resolved != wantResolved {
			t.Errorf("%s: junction resolves to %s; want %s", tc.name, resolved, wantResolved)
		}
	}
}

// TestAddRollback_UnwiresJunctionsOnPostWiringFailure checks two things.
// rollbackAdd removes the warp junctions Add wired in step 10b when a later step fails,
// and it keeps an adopted pre-existing weft branch.
// It reuses the adopt fixture and fails step 10c by refusing records commits.
// The failure must land after wiring: earlier, the junctions would never be wired and the test would prove nothing.
func TestAddRollback_UnwiresJunctionsOnPostWiringFailure(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "adopt-rollback-unwire"
	// hubforge.NewHub's CloneAndWire already produces the real-hub shape, so no part of the fixture is hand-assembled.
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftBranch := fabricengine.RecordsBranchName(slug)

	// Pre-create the weft branch with a unique commit that predates the Add.
	seedDir := filepath.Join(t.TempDir(), "seed")
	gitkit.MustRun(t, mustRecordsRepoRoot(t, l), "git", "worktree", "add", "-b", weftBranch, seedDir, fabricengine.RecordsBranchName("main"))
	preciousSHA := gitkit.CommitFile(t, seedDir, "precious.txt", "pre-existing weft work\n", "precious pre-existing weft work")
	gitkit.MustRun(t, mustRecordsRepoRoot(t, l), "git", "worktree", "remove", seedDir)

	// Refuse every records commit so step 10c's origin-record commit fails AFTER step 10b has
	// already wired the junctions — the mid-add failure this test covers.
	refuseRecordsCommits(t, l)

	const branchPrefix = "task/"
	warpBranch := branchPrefix + slug
	topology := fabricengine.NewTopology(fabricengine.Config{BranchPrefix: branchPrefix})
	_, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true})
	if err == nil {
		t.Fatalf("Add should have failed (records commit refused)")
	}
	if !strings.Contains(err.Error(), "commit parent branch record") {
		t.Fatalf("Add error = %v; want the step-10c commit failure, so rollback runs after wiring", err)
	}

	// rollbackAdd removed both warp junctions it wired.
	for _, tc := range []struct {
		name string
		link string
	}{
		{"_lym", fabricengine.CodeLymLink(l, slug)},
		{"_extra", filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel, "_extra")},
	} {
		if _, err := os.Lstat(tc.link); !os.IsNotExist(err) {
			t.Errorf("%s: junction link %s still present after rollback", tc.name, tc.link)
		}
	}

	// The adopted branch survives the rollback.
	if !gitkit.BranchExists(t, mustRecordsRepoRoot(t, l), weftBranch) {
		t.Fatalf("adopted weft branch %q was deleted by Add's rollback; want it preserved", weftBranch)
	}
	branchSHA := gitkit.RevParse(t, mustRecordsRepoRoot(t, l), "refs/heads/"+weftBranch)
	if branchSHA != preciousSHA {
		t.Errorf("adopted weft branch %q = %s; want the pre-existing commit %s", weftBranch, branchSHA, preciousSHA)
	}

	// Everything Add itself created is rolled back: no weft worktree dir, no
	// warp worktree dir, no warp branch.
	if _, err := os.Stat(fabricengine.RecordsWorktreePath(l, slug)); !os.IsNotExist(err) {
		t.Errorf("weft worktree dir still exists at %s", fabricengine.RecordsWorktreePath(l, slug))
	}
	if _, err := os.Stat(fabricengine.WorktreePath(l, slug)); !os.IsNotExist(err) {
		t.Errorf("warp worktree dir still exists at %s", fabricengine.WorktreePath(l, slug))
	}
	if gitkit.BranchExists(t, l.WorktreePath(), warpBranch) {
		t.Errorf("warp branch %q still exists", warpBranch)
	}
}

// TestAdd_GitFailureCarriesGitsOwnReason pins that a git failure inside Add reaches the operator
// with git's explanation attached, not a bare exit code and not a claim about cwd.
//
// Every RunGit failure in Add names its operation and carries git's output.
// A message with only an exit code, or one that blames the cwd, hides the cause.
func TestAdd_GitFailureCarriesGitsOwnReason(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location

	// Point origin at a path that does not exist, so the warp origin probe fails for a reason
	// only git can state while every fabric-side precondition still passes.
	gitkit.MustRun(t, l.WorktreePath(), "git", "remote", "set-url", "origin",
		filepath.Join(t.TempDir(), "no-such-remote.git"))

	topology := fabricengine.NewTopology(fabricengine.Config{})
	_, err := topology.Add(l, "probe-fail", fabricengine.AddOptions{})
	if err == nil {
		t.Fatal("Add() error = nil; want a probe failure against a nonexistent remote")
	}
	if !strings.Contains(err.Error(), "look up branch") {
		t.Errorf("Add() error = %q; want the warp origin probe failure", err.Error())
	}
	if strings.Contains(err.Error(), "cwd is not a valid git worktree") {
		t.Errorf("Add() error = %q; want the real cause, not a claim about cwd", err.Error())
	}
	if !strings.Contains(err.Error(), "does not appear to be a git repository") &&
		!strings.Contains(err.Error(), "fatal:") {
		t.Errorf("Add() error = %q; want git's own explanation included, not just an exit code", err.Error())
	}
}
