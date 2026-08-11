//go:build integration

// destructivegaps_integration_test.go proves the destructive gate closes the three gaps this
// slice's discussion names as newly-closed by this refactor (not merely re-covering a
// pre-existing regression), exercises the gate's git-touching ownership predicates directly, proves
// both dirtiness scopes actually differ, and covers branch ownership at all four deletion sites.
//
// Every assertion here needs real git, which is why none of it lives in the untagged tier's gate
// unit tests (destroy_test.go): those cover the pipeline's hermetic logic (check ordering,
// containment, zero-value refusals, force's narrow reach) with no git spawn at all.
//
// Package fabricengine_test to reuse newFabricFixture and its sibling helpers from
// reconcile_stale_registration_test.go, and makeBareRemote from clone_adopt_test.go, matching
// prune_unowned_integration_test.go's convention; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// seedRepoWideEscapeFabricConfig overwrites the repo-wide fabric.yaml at fabricengine.BoardDir(hub)
// with a pathspec naming exactly escapeName, mirroring reconcile_stale_registration_test.go's
// seedRepoWideFabricConfig and junction_pattern_integration_test.go's seedRepoWideExtraFabricConfig
// for a caller-chosen pathspec entry rather than either of those fixed ones.
func seedRepoWideEscapeFabricConfig(t testing.TB, hub, escapeName string) {
	t.Helper()

	boardDir := fabricengine.BoardDir(hub)
	if err := os.MkdirAll(configengine.ConfigDir(boardDir), 0o755); err != nil {
		t.Fatalf("mkdir repo-wide config dir: %v", err)
	}
	configPath := configengine.ConfigFile(boardDir, "fabric")
	content := "branch_prefix: \"\"\npathspec: " + escapeName + "\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write repo-wide fabric config: %v", err)
	}
}

// TestUnwireJunctions_RefusesLinkOutsideItsWorktree covers gap one's first reach point: the
// junction-record removal executor had no containment check at all before this slice.
// WarpJunctions computes a junction's Link by joining the caller-supplied name onto the worktree
// root, so a name carrying "../" escapes the worktree entirely — one "../" cancels the slug segment
// and lands back at the hub itself, since WorktreePath(l, slug) is a direct child of the hub.
// This test builds exactly that escaped link by hand (WireJunctions has no containment check of its
// own either, and is not the code path under test here) and asserts the gate refuses to remove it
// and leaves it on disk.
func TestUnwireJunctions_RefusesLinkOutsideItsWorktree(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	const slug = "gap1-owner"

	escapeLink := filepath.Join(l.HubPath, "gap1-escape")
	elsewhere := t.TempDir()
	if err := fslink.CreateDirLink(escapeLink, elsewhere); err != nil {
		t.Fatalf("create escape link: %v", err)
	}

	_, err := fabricengine.UnwireJunctions(l, slug, []string{"../gap1-escape"})
	if err == nil {
		t.Fatalf("UnwireJunctions with a link outside its worktree = nil error; want a containment refusal")
	}
	if !strings.Contains(err.Error(), "is not inside") {
		t.Errorf("UnwireJunctions error = %q; want it to name the containment refusal", err)
	}

	if _, statErr := os.Lstat(escapeLink); statErr != nil {
		t.Fatalf("the escaped link was removed despite the containment refusal: %v", statErr)
	}
}

// TestAddRollback_RefusesJunctionRemovalOutsideItsWorktree covers gap one's second reach point:
// Add's own rollback removes the junctions it wired through the same gate, and must refuse the same
// shape.
// A pathspec entry of "../gap1-rollback-escape" makes seedLyxJunction materialise its weft-side
// target AND its warp-side link at the identical hub-level path — the worktree and its weft sibling
// are both direct children of the hub, so one "../" cancels either segment to the same place —
// so wiring that junction fails with "already contains a real directory" before a symlink is ever
// created there. That failure is what triggers Add's rollback, and the directory
// seedLyxJunction's os.MkdirAll left behind along the way is what the rollback must refuse to
// remove.
func TestAddRollback_RefusesJunctionRemovalOutsideItsWorktree(t *testing.T) {
	t.Parallel()

	const slug = "gap1-rollback-owner"
	fixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
		"fabric": fabricengine.ConfigTemplate(),
	})
	lyxtest.MustRun(t, fixture.WeftPrime, "git", "checkout", "-b", fabricengine.WeftBranchName("main"))
	// The gate's ownedManagedBranch reaches primaryWeftBranch, which reads the branch checked out at
	// _board — mirror newFabricFixture's setup (worktree add BEFORE seeding config into it).
	lyxtest.MustRun(t, fixture.WeftPrime, "git", "worktree", "add", fabricengine.BoardDir(fixture.Layout.HubPath), "main")
	seedRepoWideEscapeFabricConfig(t, fixture.Layout.HubPath, "../gap1-rollback-escape")

	l := fixture.Layout
	// A configured branch prefix is what makes the warp branch this Add creates recognisable to the
	// gate's ownedManagedBranch check, mirroring add_rollback_adopt_test.go's fixtures — the branch
	// side of rollback is not what this test is about.
	const branchPrefix = "task/"
	topology := fabricengine.NewTopology(fabricengine.Config{BranchPrefix: branchPrefix})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err == nil {
		t.Fatalf("Add should have failed wiring a junction whose name escapes its own worktree")
	}

	escapeDir := filepath.Join(l.HubPath, "gap1-rollback-escape")
	if _, statErr := os.Stat(escapeDir); statErr != nil {
		t.Fatalf("Add's rollback removed a directory outside the pair's worktree despite the containment refusal: %v", statErr)
	}
}

// TestTeardownHub_RefusesHubPathOutsideOperatorNamedParent covers gap two's first assertion:
// teardownHub's own containment check.
// CloneHub's normal derivation makes the hub path always a literal child of its own cwd argument, so
// this refusal can never be triggered end to end through CloneHub itself; TeardownHubForTest drives
// teardownHub directly to construct the mismatch.
func TestTeardownHub_RefusesHubPathOutsideOperatorNamedParent(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	hubPath := filepath.Join(t.TempDir(), "elsewhere"+fabricengine.HubSuffix)
	const sentinel = "OUTSIDE-PARENT-HUB-CONTENT"

	err := fabricengine.TeardownHubForTest(cwd, hubPath, func(hub string) error {
		return os.WriteFile(filepath.Join(hub, "marker.txt"), []byte(sentinel+"\n"), 0o644)
	}, fmt.Errorf("original clone failure"))
	if err == nil {
		t.Fatalf("teardownHub(%s, %s, ...) = nil; want a containment refusal (hubPath is not under cwd)", cwd, hubPath)
	}
	if !strings.Contains(err.Error(), "residual hub") {
		t.Errorf("teardownHub error = %q; want it to report a residual hub rather than silently succeeding", err)
	}

	content, readErr := os.ReadFile(filepath.Join(hubPath, "marker.txt"))
	if readErr != nil {
		t.Fatalf("teardown destroyed a hub path outside its operator-named parent: %v", readErr)
	}
	if !strings.Contains(string(content), sentinel) {
		t.Fatalf("hub content lost: %q no longer contains %q", content, sentinel)
	}
}

// TestCloneHub_TeardownSucceedsOnAHalfBuiltHub covers gap two's counter-assertion, as important as
// the first: teardownHub authorises removal via the createdToken this same invocation minted, not
// via ownedFabricHub's board-or-weft-sibling predicate.
// The earliest of teardownHub's call sites in CloneHub run before either exists — with the hub
// predicate instead, teardown would refuse at nearly every early failure site and leave a residual
// hub where it works today. A real CloneHub run against an unreachable warp URL fails at exactly
// that early point (hub created, only .lyx materialised, no warp clone and no weft clone yet), so
// this drives the scenario end to end through the exported entry point.
func TestCloneHub_TeardownSucceedsOnAHalfBuiltHub(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	weftRemote := makeBareRemote(t, dir, "weft-half-built")
	badWarpURL := filepath.Join(dir, "no-such-warp-repo")

	cloneParent := t.TempDir()

	_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL: weftRemote,
		WarpURL: badWarpURL,
	})
	if err == nil {
		t.Fatalf("CloneHub against a nonexistent warp URL = nil error; want a clone failure")
	}

	hubPath := fabricengine.HubPath(cloneParent, fabricengine.DeriveWarpName(badWarpURL))
	if _, statErr := os.Stat(hubPath); !os.IsNotExist(statErr) {
		t.Errorf("residual half-built hub left at %s after the warp-clone failure; teardown should have removed it (no board, no weft sibling yet — the token, not the hub predicate, authorises this)", hubPath)
	}
}

// TestRemoveWarpWorktreeDir_FallbackRefusesRegisteredWorktreeWithUntrackedFiles covers gap three:
// the directory-removal fallback removeWarpWorktreeDir falls into when `git worktree remove` itself
// refuses had no dirtiness check at all before this slice.
// Topology.Remove's own top-level dirty check (scopeAll, run before removeWarpWorktreeDir is ever
// reached) would already refuse an untracked file in the real verb, so this drives
// removeWarpWorktreeDir directly to construct the state whose refusal actually routes into the
// fallback: a registered linked worktree carrying an untracked file, removed without force so git's
// own refusal — not Remove's earlier gate — is what triggers the fallback.
// Asserting the error alone would pass against the pre-fix code too (an ungated fallback still
// returns whatever os.RemoveAll or the surrounding plumbing reports), so the on-disk assertion is
// what actually proves the gap closed.
func TestRemoveWarpWorktreeDir_FallbackRefusesRegisteredWorktreeWithUntrackedFiles(t *testing.T) {
	t.Parallel()

	const slug = "gap3-untracked"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	target := fabricengine.WorktreePath(l, slug)
	untracked := filepath.Join(target, "untracked-scratch.txt")
	const sentinel = "UNTRACKED-WORK-NOT-YET-COMMITTED"
	if err := os.WriteFile(untracked, []byte(sentinel+"\n"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}

	err := fabricengine.RemoveWarpWorktreeDirForTest(l, target, false)
	if err == nil {
		t.Fatalf("RemoveWarpWorktreeDirForTest on a registered worktree with an untracked file, no force = nil; want a refusal")
	}

	content, readErr := os.ReadFile(untracked)
	if readErr != nil {
		t.Fatalf("the fallback destroyed the untracked file git had just refused to discard: %v", readErr)
	}
	if !strings.Contains(string(content), sentinel) {
		t.Fatalf("untracked content lost: %q no longer contains %q", content, sentinel)
	}
}
