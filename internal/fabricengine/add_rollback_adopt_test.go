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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// shaOf returns the commit SHA rev names in the repo at dir, failing the test
// on any git error.
func shaOf(t *testing.T, dir, rev string) string {
	t.Helper()

	out, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", rev}, dir)
	if err != nil || exitCode != 0 {
		t.Fatalf("rev-parse %s in %s: err=%v exit=%d", rev, dir, err, exitCode)
	}
	return strings.TrimSpace(out)
}

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
	if err := os.WriteFile(filepath.Join(seedDir, "precious.txt"), []byte("pre-existing weft work\n"), 0o644); err != nil {
		t.Fatalf("write precious.txt: %v", err)
	}
	gitkit.MustRun(t, seedDir, "git", "add", "precious.txt")
	gitkit.MustRun(t, seedDir, "git", "commit", "-m", "precious pre-existing weft work")
	preciousSHA := shaOf(t, seedDir, "HEAD")
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

	// The gate's ownedManagedBranch requires either a "-weft" suffix or a configured branch prefix
	// to recognize a branch as fabric-managed; a bare slug carries neither, so rollbackAdd's warp
	// branch deletion would be refused as unmanaged with an empty prefix. A nonempty BranchPrefix
	// here makes the warp branch this Add creates recognizable to the gate, matching how a real
	// deployment configures fabric.
	const branchPrefix = "task/"
	warpBranch := branchPrefix + slug
	topology := fabricengine.NewTopology(fabricengine.Config{BranchPrefix: branchPrefix})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err == nil {
		t.Fatalf("Add should have failed (portal blocker)")
	}

	// The adopted branch survives the rollback, still at its unique commit.
	if !branchExistsAt(t, mustWeftRepoRoot(t, l), weftBranch) {
		t.Fatalf("adopted weft branch %q was deleted by Add's rollback; want it preserved", weftBranch)
	}
	branchSHA := shaOf(t, mustWeftRepoRoot(t, l), "refs/heads/"+weftBranch)
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
	if branchExistsAt(t, l.WorktreePath(), warpBranch) {
		t.Errorf("warp branch %q still exists", warpBranch)
	}
}

// TestAdd_WiresJunctionsEagerly proves a successful Add leaves the new worktree's warp junctions
// wired immediately: card 20 folds WireJunctions into Add's step 10b (after writeLaunchers, before
// the warp push), so no dormant state and no separate `lyx init` step is needed for the pair to be
// usable.
// Asserts both the repo-wide default junctions (_lyx and _extra) resolve to their paired weft
// directories.
func TestAdd_WiresJunctionsEagerly(t *testing.T) {
	t.Parallel()

	const slug = "eager-wire-add"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	// newFabricFixture seeds the repo-wide config with fabricengine.ConfigTemplate()'s own
	// default pathspec; override it to "_extra" so Add's RepoWiredNames-driven wiring below
	// wires the junction name this test asserts against.
	seedRepoWideExtraFabricConfig(t, l.HubPath)

	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

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
	if err := os.WriteFile(filepath.Join(seedDir, "precious.txt"), []byte("pre-existing weft work\n"), 0o644); err != nil {
		t.Fatalf("write precious.txt: %v", err)
	}
	gitkit.MustRun(t, seedDir, "git", "add", "precious.txt")
	gitkit.MustRun(t, seedDir, "git", "commit", "-m", "precious pre-existing weft work")
	preciousSHA := shaOf(t, seedDir, "HEAD")
	gitkit.MustRun(t, mustWeftRepoRoot(t, l), "git", "worktree", "remove", seedDir)

	// Break the warp origin remote so step 11's push fails AFTER step 10b has
	// already wired the junctions — the mid-add failure this test covers.
	gitkit.MustRun(t, l.WorktreePath(), "git", "remote", "set-url", "origin", filepath.Join(t.TempDir(), "no-such-remote"))

	// See TestAddRollback_AdoptedWeftBranchSurvives's comment: a configured branch prefix is what
	// makes the warp branch this Add creates recognizable to the gate's ownedManagedBranch check.
	const branchPrefix = "task/"
	warpBranch := branchPrefix + slug
	topology := fabricengine.NewTopology(fabricengine.Config{BranchPrefix: branchPrefix})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{}); err == nil {
		t.Fatalf("Add should have failed (broken origin remote)")
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
	if !branchExistsAt(t, mustWeftRepoRoot(t, l), weftBranch) {
		t.Fatalf("adopted weft branch %q was deleted by Add's rollback; want it preserved", weftBranch)
	}
	branchSHA := shaOf(t, mustWeftRepoRoot(t, l), "refs/heads/"+weftBranch)
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
	if branchExistsAt(t, l.WorktreePath(), warpBranch) {
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
	_, err := topology.Add(l, "push-fail", fabricengine.AddOptions{})
	if err == nil {
		t.Fatal("Add() error = nil; want a push failure against a nonexistent remote")
	}
	if strings.Contains(err.Error(), "cwd is not a valid git worktree") {
		t.Errorf("Add() error = %q; want the real cause, not a claim about cwd", err.Error())
	}
	if !strings.Contains(err.Error(), "does not appear to be a git repository") &&
		!strings.Contains(err.Error(), "fatal:") {
		t.Errorf("Add() error = %q; want git's own explanation included, not just an exit code", err.Error())
	}
}
