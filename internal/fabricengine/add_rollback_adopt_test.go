// add_rollback_adopt_test.go proves that Add's rollback never deletes a pre-existing (adopted) weft branch.
// When Add adopts an existing <slug>-weft branch and then fails at a later step:
// the rollback tears down only the worktree Add created; the branch, and any unpushed history it carries, survives.
// This test injects a deterministic post-adopt failure instead of a network failure.
// The failure is a real directory where a junction goes.
//
// Package fabricengine_test, reusing the external-test-package fixture idiom of lifecycle_differential_test.go;
// it shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
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

// failAddBeforeWiring makes Add on topology fail once the pair's worktrees and weft branch exist.
// Before the junctions are wired it plants a real directory where the pair's `_lym` junction goes, which wiring refuses.
// The failure sends Add through its rollback.
func failAddBeforeWiring(tb testing.TB, topology *fabricengine.Topology, l *lymcwd.Location, slug string) {
	tb.Helper()

	fabricengine.SetBeforeWireForTest(topology, func() {
		if err := os.MkdirAll(fabricengine.CodeLymLink(l, slug), 0o755); err != nil {
			tb.Errorf("plant the real directory at the _lym junction of %s: %v", slug, err)
		}
	})
}

// TestAddRollback_AdoptedWeftBranchSurvives pre-creates a weft branch carrying a unique commit,
// forces Add to fail after adopting it, and asserts the rollback removes only the weft worktree.
// The branch stays at that unique commit, and no warp-side residue is left.
//
//testtiming:keep an adopted weft branch surviving the rollback at its pre-existing unpushed commit; coverage of its blocks by other tests does not show an assertion of this
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
	// a real directory at the _lym junction path makes junction wiring fail, triggering rollback.
	const branchPrefix = "task/"
	warpBranch := branchPrefix + slug
	topology := fabricengine.NewTopology(fabricengine.Config{BranchPrefix: branchPrefix})
	failAddBeforeWiring(t, topology, l, slug)
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err == nil {
		t.Fatalf("Add should have failed (junction blocker)")
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

			// A blocker at the _lym junction path fails Add at junction wiring, after the weft branch is taken from origin.
			failAddBeforeWiring(t, h.Topology, l, slug)

			if _, err := h.Topology.Add(l, slug, fabricengine.AddOptions{}); err == nil {
				t.Fatalf("Add should have failed (junction blocker)")
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

// TestAddRollback_AdoptedWarpBranchLocalCopyDeleted forces Add to fail after it adopted a warp branch:
// a live pair's branch taken from origin.
// It asserts the rollback deletes the local copy and leaves origin's branch untouched.
//
//testtiming:keep origin's warp branch left untouched at its tip when the rollback deletes the adopted local copy; coverage of its blocks by other tests does not show an assertion of this
func TestAddRollback_AdoptedWarpBranchLocalCopyDeleted(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "adopted-warp-rollback"
	h := removedPair(t, slug)
	l := h.Location
	// A weft branch on origin that moved past its archive tag makes the pair live, so Add adopts both branches from origin.
	pushCommitToOrigin(t, h.RecordsBare, fabricengine.RecordsBranchName(slug))
	originTip := gitkit.RevParse(t, h.CodeBare, slug)

	failAddBeforeWiring(t, h.Topology, l, slug)

	if _, err := h.Topology.Add(l, slug, fabricengine.AddOptions{}); err == nil {
		t.Fatalf("Add should have failed (junction blocker)")
	}

	if gitkit.BranchExists(t, l.WorktreePath(), slug) {
		t.Errorf("local warp branch %q adopted from origin survived the rollback", slug)
	}
	if got := gitkit.RevParse(t, h.CodeBare, slug); got != originTip {
		t.Errorf("origin warp branch = %s; want unchanged %s", got, originTip)
	}
}

// TestAdd_WiresJunctionsEagerly proves a successful Add leaves the new worktree's warp junctions wired immediately.
// Add wires them in step 10b, before the warp push.
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
