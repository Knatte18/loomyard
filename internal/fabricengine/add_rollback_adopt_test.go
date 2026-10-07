//go:build integration

// add_rollback_adopt_test.go proves Add's rollback never deletes a
// pre-existing (adopted) weft branch: when Add merely adopted an existing
// <slug>-weft branch and then failed at a later step, the rollback must tear
// down only the worktree Add created — the branch, and any unpushed history
// it carries, survives. A live review round reproduced the pre-fix behavior
// (branch and its unique commit destroyed after a warp-push failure), so this
// test injects a deterministic post-adopt failure (a portal blocker file, the
// same injection TestAddRollback_DifferentialEquivalence uses) instead of a
// network failure.
//
// Package fabricengine_test to reuse the external-test-package fixture idiom
// of lifecycle_differential_test.go; shares the single TestMain in
// testmain_test.go.

package fabricengine_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// mustWeftRepoRoot resolves fabricengine.WeftRepoRoot(l), failing the test on
// any error — the shared test-package helper for the many call sites across
// this package's tests that need the weft prime worktree path, now that it is
// a fabricengine-owned resolution rather than a Layout method.
func mustWeftRepoRoot(t *testing.T, l *lyxcwd.Location) string {
	t.Helper()

	root, err := fabricengine.WeftRepoRoot(l)
	if err != nil {
		t.Fatalf("WeftRepoRoot: %v", err)
	}
	return root
}

// TestAddRollback_AdoptedWeftBranchSurvives pre-creates a weft branch carrying a unique commit,
// forces Add to fail after adopting it, and asserts the rollback removed the weft worktree but left
// the branch — still pointing at the unique commit — untouched, alongside the usual zero warp-side
// residue.
func TestAddRollback_AdoptedWeftBranchSurvives(t *testing.T) {
	t.Parallel()

	const slug = "adopt-rollback-keep"
	// hubforge.NewHub's CloneAndWire already produces exactly the real-hub shape the old fixture
	// used to hand-assemble here: the weft primary checked out on the suffixed sibling of the warp's
	// branch, a real _board worktree the gate's ownedManagedBranch/primaryWeftBranch read succeeds
	// against, and the repo-wide fabric.yaml Add's eager RepoWiredNames load needs — so none of that
	// scaffolding is seeded by hand any more.
	h := hubforge.NewHub(t, ".")
	l := h.Location
	weftBranch := fabricengine.WeftBranchName(slug)

	// Pre-create the weft branch with a unique commit that predates the Add —
	// the history the rollback must not destroy. The seeding worktree is
	// removed again so the branch is free for Add to adopt.
	seedDir := filepath.Join(t.TempDir(), "seed")
	gitkit.MustRun(t, mustWeftRepoRoot(t, l), "git", "worktree", "add", "-b", weftBranch, seedDir, fabricengine.WeftBranchName("main"))
	preciousSHA := gitkit.CommitFile(t, seedDir, "precious.txt", "pre-existing weft work\n", "precious pre-existing weft work")
	gitkit.MustRun(t, mustWeftRepoRoot(t, l), "git", "worktree", "remove", seedDir)

	// Inject a deterministic failure AFTER the adopt: a blocker file at the
	// portal location makes step 9 (createPortal) fail, triggering rollback.
	portalLink := filepath.Join(fabricengine.PortalsDir(l), slug)
	if err := os.MkdirAll(filepath.Dir(portalLink), 0o755); err != nil {
		t.Fatalf("mkdir portal parent: %v", err)
	}
	if err := os.WriteFile(portalLink, []byte("blocker"), 0o644); err != nil {
		t.Fatalf("create blocker: %v", err)
	}

	const branchPrefix = "task/"
	warpBranch := branchPrefix + slug
	topology := fabricengine.NewTopology(fabricengine.Config{BranchPrefix: branchPrefix})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err == nil {
		t.Fatalf("Add should have failed (portal blocker)")
	}

	// The adopted branch survives the rollback, still at its unique commit.
	if !gitkit.BranchExists(t, mustWeftRepoRoot(t, l), weftBranch) {
		t.Fatalf("adopted weft branch %q was deleted by Add's rollback; want it preserved", weftBranch)
	}
	branchSHA := gitkit.RevParse(t, mustWeftRepoRoot(t, l), "refs/heads/"+weftBranch)
	if branchSHA != preciousSHA {
		t.Errorf("adopted weft branch %q = %s; want the pre-existing commit %s", weftBranch, branchSHA, preciousSHA)
	}

	// Everything Add itself created is rolled back: no weft worktree dir, no
	// warp worktree dir, no warp branch.
	if _, err := os.Stat(fabricengine.WeftWorktreePath(l, slug)); !os.IsNotExist(err) {
		t.Errorf("weft worktree dir still exists at %s", fabricengine.WeftWorktreePath(l, slug))
	}
	if _, err := os.Stat(fabricengine.WorktreePath(l, slug)); !os.IsNotExist(err) {
		t.Errorf("warp worktree dir still exists at %s", fabricengine.WorktreePath(l, slug))
	}
	if gitkit.BranchExists(t, l.WorktreePath(), warpBranch) {
		t.Errorf("warp branch %q still exists", warpBranch)
	}
}

// TestAddRollback_LiveWeftFromOrigin forces Add to fail after it took a live pair's weft branch from origin, and asserts the rollback leaves origin untouched:
// a local branch Add created from origin is deleted,
// and a pre-existing local branch Add fast-forwarded stays at origin's tip, not rewound.
func TestAddRollback_LiveWeftFromOrigin(t *testing.T) {
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
			h := hubforge.NewHub(t, ".")
			l := h.Location
			weftBranch := fabricengine.WeftBranchName(slug)
			weftRoot := mustWeftRepoRoot(t, l)

			clone := t.TempDir()
			gitkit.MustRun(t, clone, "git", "clone", "--quiet", h.WeftBare, ".")
			gitkit.MustRun(t, clone, "git", "checkout", "--quiet", "-b", weftBranch)
			gitkit.CommitFile(t, clone, "first.txt", "first\n", "first origin work")
			gitkit.MustRun(t, clone, "git", "push", "--quiet", "origin", weftBranch)
			if tc.localBehind {
				gitkit.MustRun(t, weftRoot, "git", "fetch", "--quiet", "origin", weftBranch)
				gitkit.MustRun(t, weftRoot, "git", "branch", weftBranch, "FETCH_HEAD")
				gitkit.CommitFile(t, clone, "second.txt", "second\n", "second origin work")
				gitkit.MustRun(t, clone, "git", "push", "--quiet", "origin", weftBranch)
			}
			originTip := gitkit.RevParse(t, h.WeftBare, weftBranch)

			// A blocker at the portal location fails Add at createPortal, after the weft branch is taken from origin.
			portalLink := filepath.Join(fabricengine.PortalsDir(l), slug)
			if err := os.MkdirAll(filepath.Dir(portalLink), 0o755); err != nil {
				t.Fatalf("mkdir portal parent: %v", err)
			}
			if err := os.WriteFile(portalLink, []byte("blocker"), 0o644); err != nil {
				t.Fatalf("create blocker: %v", err)
			}

			if _, err := h.Topology.Add(l, slug, fabricengine.AddOptions{}); err == nil {
				t.Fatalf("Add should have failed (portal blocker)")
			}

			if got := gitkit.RevParse(t, h.WeftBare, weftBranch); got != originTip {
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
			if _, err := os.Stat(fabricengine.WeftWorktreePath(l, slug)); !os.IsNotExist(err) {
				t.Errorf("weft worktree dir still exists at %s", fabricengine.WeftWorktreePath(l, slug))
			}
		})
	}
}

// TestAddRollback_WarpBranchDeletedUnderEmptyPrefix pins that under the DEFAULT empty branch_prefix the rollback deletes the bare-slug warp branch this Add created, together with the worktree pair.
func TestAddRollback_WarpBranchDeletedUnderEmptyPrefix(t *testing.T) {
	t.Parallel()

	const slug = "empty-prefix-rollback"
	h := hubforge.NewHub(t, ".")
	l := h.Location

	// Inject a deterministic post-creation failure: a blocker file at the portal location makes
	// createPortal (step 9) fail after the warp worktree and its branch already exist, triggering
	// rollbackAdd.
	portalLink := filepath.Join(fabricengine.PortalsDir(l), slug)
	if err := os.MkdirAll(filepath.Dir(portalLink), 0o755); err != nil {
		t.Fatalf("mkdir portal parent: %v", err)
	}
	if err := os.WriteFile(portalLink, []byte("blocker"), 0o644); err != nil {
		t.Fatalf("create blocker: %v", err)
	}

	// Default empty branch_prefix: the warp branch is exactly the bare slug.
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err == nil {
		t.Fatalf("Add should have failed (portal blocker)")
	}

	// The worktree pair is fully rolled back.
	if _, err := os.Stat(fabricengine.WorktreePath(l, slug)); !os.IsNotExist(err) {
		t.Errorf("warp worktree dir still exists at %s after rollback", fabricengine.WorktreePath(l, slug))
	}
	if _, err := os.Stat(fabricengine.WeftWorktreePath(l, slug)); !os.IsNotExist(err) {
		t.Errorf("weft worktree dir still exists at %s after rollback", fabricengine.WeftWorktreePath(l, slug))
	}

	if gitkit.BranchExists(t, l.WorktreePath(), slug) {
		t.Errorf("warp branch %q survived the rollback under an empty prefix; the rollback must delete the branch this Add created", slug)
	}
}

// TestAddRollback_RefusedWarpBranchDeletionLogsWarn pins the origin deletion's lease: a warp branch that moved on origin after step 11's push is left there,
// and a WARN line names the branch and the lease the deletion was pinned to.
// A pre-receive hook on the weft bare advances the warp bare's branch and declines step 12's push, which fails Add after step 11 landed.
//
// It is deliberately NOT parallel: it rebinds the process-global logger sink via SetOutput, and Go
// pauses t.Parallel() tests until the sequential ones finish, so a non-parallel test owns the sink for
// its duration with no cross-talk from a concurrently-logging sibling.
func TestAddRollback_RefusedWarpBranchDeletionLogsWarn(t *testing.T) {
	const slug = "warn-on-refused-lease"
	h := hubforge.NewHub(t, ".")
	l := h.Location
	pushedTip := gitkit.RevParse(t, h.PrimeWorktree(), "HEAD")

	clone := t.TempDir()
	gitkit.MustRun(t, clone, "git", "clone", "--quiet", h.WarpBare, ".")
	movedTip := gitkit.CommitFile(t, clone, "moved.txt", "moved\n", "moved on origin")
	gitkit.MustRun(t, clone, "git", "push", "--quiet", "origin", "HEAD:refs/heads/elsewhere")
	installPreReceive(t, h.WeftBare, "#!/bin/sh\nenv -i PATH=\"$PATH\" git --git-dir='"+filepath.ToSlash(h.WarpBare)+"' update-ref refs/heads/"+slug+" "+movedTip+"\necho declined >&2\nexit 1\n")

	var buf bytes.Buffer
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	if _, err := h.Topology.Add(l, slug, fabricengine.AddOptions{}); err == nil {
		t.Fatalf("Add should have failed (weft push declined)")
	}

	if got := gitkit.RevParse(t, h.WarpBare, slug); got != movedTip {
		t.Errorf("origin warp branch = %s; want the moved tip %s left in place", got, movedTip)
	}
	if gitkit.BranchExists(t, l.WorktreePath(), slug) {
		t.Errorf("local warp branch %q survived the rollback", slug)
	}
	logged := buf.String()
	for _, want := range []string{"left the warp branch on origin", slug, "lease=" + pushedTip} {
		if !strings.Contains(logged, want) {
			t.Errorf("WARN output lacks %q; got:\n%s", want, logged)
		}
	}
}

// TestAddRollback_AdoptedWarpBranchLocalCopyDeleted forces Add to fail after it adopted a live pair's warp branch from origin, and asserts the rollback deletes the local copy and leaves origin's branch untouched.
func TestAddRollback_AdoptedWarpBranchLocalCopyDeleted(t *testing.T) {
	t.Parallel()

	const slug = "adopted-warp-rollback"
	h := removedPair(t, slug)
	l := h.Location
	// A weft branch on origin that moved past its archive tag makes the pair live, so Add adopts both branches from origin.
	pushCommitToOrigin(t, h.WeftBare, fabricengine.WeftBranchName(slug))
	originTip := gitkit.RevParse(t, h.WarpBare, slug)

	portalLink := filepath.Join(fabricengine.PortalsDir(l), slug)
	if err := os.MkdirAll(filepath.Dir(portalLink), 0o755); err != nil {
		t.Fatalf("mkdir portal parent: %v", err)
	}
	if err := os.WriteFile(portalLink, []byte("blocker"), 0o644); err != nil {
		t.Fatalf("create blocker: %v", err)
	}

	if _, err := h.Topology.Add(l, slug, fabricengine.AddOptions{}); err == nil {
		t.Fatalf("Add should have failed (portal blocker)")
	}

	if gitkit.BranchExists(t, l.WorktreePath(), slug) {
		t.Errorf("local warp branch %q adopted from origin survived the rollback", slug)
	}
	if got := gitkit.RevParse(t, h.WarpBare, slug); got != originTip {
		t.Errorf("origin warp branch = %s; want unchanged %s", got, originTip)
	}
}

// TestAdd_WiresJunctionsEagerly proves a successful Add leaves the new worktree's warp junctions
// wired immediately: card 20 folds WireJunctions into Add's step 10b (after writeLaunchers, before
// the warp push), so no dormant state and no separate `lyx init` step is needed for the pair to be
// usable.
// Asserts both the repo-wide default junctions (_lyx and _extra) resolve to their paired weft
// directories.
//
//testtiming:keep a successful Add leaving the new worktree's junctions wired at once, with _lyx and _extra resolving to their weft directories; coverage of its blocks by other tests does not show an assertion of this
func TestAdd_WiresJunctionsEagerly(t *testing.T) {
	t.Parallel()

	const slug = "eager-wire-add"
	h := hubforge.NewHub(t, ".")
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
		{"_lyx", fabricengine.WarpLyxLink(l, slug), fabricengine.WeftLyxDirFor(l, slug)},
		{"_extra", filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel, "_extra"), filepath.Join(fabricengine.WeftWorktreePath(l, slug), l.AnchorRel, "_extra")},
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

// TestAddRollback_UnwiresJunctionsOnPostWiringFailure covers card 21: rollbackAdd must remove the
// warp junctions Add's step 10b wired when a later step (the warp push) fails, while still
// preserving an adopted pre-existing weft branch exactly as
// TestAddRollback_AdoptedWeftBranchSurvives does.
// Reuses that test's adopt fixture,
// but injects the failure via a broken warp origin remote instead of a portal blocker so the
// failure lands AFTER wiring (step 10b) rather than before it (step 9) — otherwise the junctions
// would never have been wired and this test would prove nothing.
func TestAddRollback_UnwiresJunctionsOnPostWiringFailure(t *testing.T) {
	t.Parallel()

	const slug = "adopt-rollback-unwire"
	// See TestAddRollback_AdoptedWeftBranchSurvives's comment: hubforge.NewHub's CloneAndWire
	// already produces the real-hub shape this fixture used to hand-assemble.
	h := hubforge.NewHub(t, ".")
	l := h.Location
	weftBranch := fabricengine.WeftBranchName(slug)

	// Pre-create the weft branch with a unique commit that predates the Add,
	// exactly as TestAddRollback_AdoptedWeftBranchSurvives does.
	seedDir := filepath.Join(t.TempDir(), "seed")
	gitkit.MustRun(t, mustWeftRepoRoot(t, l), "git", "worktree", "add", "-b", weftBranch, seedDir, fabricengine.WeftBranchName("main"))
	preciousSHA := gitkit.CommitFile(t, seedDir, "precious.txt", "pre-existing weft work\n", "precious pre-existing weft work")
	gitkit.MustRun(t, mustWeftRepoRoot(t, l), "git", "worktree", "remove", seedDir)

	// Break the warp origin remote so step 11's push fails AFTER step 10b has
	// already wired the junctions — the mid-add failure this test covers.
	gitkit.MustRun(t, l.WorktreePath(), "git", "remote", "set-url", "origin", filepath.Join(t.TempDir(), "no-such-remote"))

	const branchPrefix = "task/"
	warpBranch := branchPrefix + slug
	topology := fabricengine.NewTopology(fabricengine.Config{BranchPrefix: branchPrefix})
	// SkipPush skips Add's pre-flight probes of origin, which would otherwise refuse on the broken URL before any mutation;
	// step 11's warp push ignores SkipPush, so it still fails after wiring.
	_, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true})
	if err == nil {
		t.Fatalf("Add should have failed (broken origin remote)")
	}
	if !strings.Contains(err.Error(), "push branch") {
		t.Fatalf("Add error = %v; want the step-11 push failure, so rollback runs after wiring", err)
	}

	// rollbackAdd removed both warp junctions it wired.
	for _, tc := range []struct {
		name string
		link string
	}{
		{"_lyx", fabricengine.WarpLyxLink(l, slug)},
		{"_extra", filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel, "_extra")},
	} {
		if _, err := os.Lstat(tc.link); !os.IsNotExist(err) {
			t.Errorf("%s: junction link %s still present after rollback", tc.name, tc.link)
		}
	}

	// The adopted branch still survives the rollback, exactly as the portal-
	// blocker variant of this scenario asserts.
	if !gitkit.BranchExists(t, mustWeftRepoRoot(t, l), weftBranch) {
		t.Fatalf("adopted weft branch %q was deleted by Add's rollback; want it preserved", weftBranch)
	}
	branchSHA := gitkit.RevParse(t, mustWeftRepoRoot(t, l), "refs/heads/"+weftBranch)
	if branchSHA != preciousSHA {
		t.Errorf("adopted weft branch %q = %s; want the pre-existing commit %s", weftBranch, branchSHA, preciousSHA)
	}

	// Everything Add itself created is rolled back: no weft worktree dir, no
	// warp worktree dir, no warp branch.
	if _, err := os.Stat(fabricengine.WeftWorktreePath(l, slug)); !os.IsNotExist(err) {
		t.Errorf("weft worktree dir still exists at %s", fabricengine.WeftWorktreePath(l, slug))
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
// Six paths in Add used to answer every RunGit failure with "cwd is not a valid git worktree" — a
// claim Add's own first status probe had already disproved — and the paths that did name the
// operation reported only "(git exit %d)". A live round watched two simultaneous `lyx fabric add`
// calls for one slug report "failed (git exit 255)" with git's actual reason discarded.
func TestAdd_GitFailureCarriesGitsOwnReason(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	l := h.Location

	// Point origin at a path that does not exist, so the push at the end of Add fails for a reason
	// only git can state while every fabric-side precondition still passes.
	gitkit.MustRun(t, l.WorktreePath(), "git", "remote", "set-url", "origin",
		filepath.Join(t.TempDir(), "no-such-remote.git"))

	topology := fabricengine.NewTopology(fabricengine.Config{})
	// SkipPush skips Add's pre-flight probes of origin, so the failure comes from step 11's warp push, which ignores SkipPush.
	_, err := topology.Add(l, "push-fail", fabricengine.AddOptions{SkipPush: true})
	if err == nil {
		t.Fatal("Add() error = nil; want a push failure against a nonexistent remote")
	}
	if !strings.Contains(err.Error(), "push branch") {
		t.Errorf("Add() error = %q; want the step-11 push failure", err.Error())
	}
	if strings.Contains(err.Error(), "cwd is not a valid git worktree") {
		t.Errorf("Add() error = %q; want the real cause, not a claim about cwd", err.Error())
	}
	if !strings.Contains(err.Error(), "does not appear to be a git repository") &&
		!strings.Contains(err.Error(), "fatal:") {
		t.Errorf("Add() error = %q; want git's own explanation included, not just an exit code", err.Error())
	}
}
