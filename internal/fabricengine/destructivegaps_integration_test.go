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
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// escapeFabricConfigYAML returns a repo-wide fabric.yaml override whose pathspec names exactly
// escapeName, for a caller-chosen pathspec entry rather than a fixed one.
func escapeFabricConfigYAML(escapeName string) string {
	return "branch_prefix: \"\"\npathspec: " + escapeName + "\n"
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
	// hubforge.NewHub's CloneAndWire already checks the weft primary out on its suffixed branch and
	// materializes a real _board worktree the gate's ownedManagedBranch/primaryWeftBranch read
	// succeeds against, so only the escape-specific pathspec override is seeded here.
	h := hubforge.NewHub(t, ".")
	hubforge.SeedFabricConfig(t, h, escapeFabricConfigYAML("../gap1-rollback-escape"))

	l := h.Location
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

	err := fabricengine.RemoveWarpWorktreeDirForTest(fabricengine.NewMutations(""), l, target, false)
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

// TestRemoveWarpWorktreeDir_FallbackHonoursForce is R2's regression for the operator's --force being
// silently dropped by the fallback request.
//
// removeWarpWorktreeDir's fallback pathRequest was built without a force field at all, so it took
// Go's zero value while the PRIMARY request one screen above declared force: force. An operator who
// passed --force against a worktree git declined for a reason other than dirtiness therefore got a
// refusal whose stated remedy was "use --force" — the one thing they had already done — and a
// half-torn-down pair.
//
// `git worktree lock` is what makes `git worktree remove --force` fail on a worktree that is still
// registered, which is exactly the state that routes into the fallback with force already set. The
// no-force half runs the identical setup and must still refuse, because propagating force must not
// weaken the guard that keeps the fallback from deleting what git declined to discard.
func TestRemoveWarpWorktreeDir_FallbackHonoursForce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		force       bool
		wantRemoved bool
	}{
		{name: "ForceReachesTheFallback", force: true, wantRemoved: true},
		{name: "NoForceStillRefuses", force: false, wantRemoved: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			slug := "force-fallback-" + strings.ToLower(tt.name)
			fixture := newFabricFixture(t)
			l := fixture.Layout
			topology := fabricengine.NewTopology(fabricengine.Config{})
			if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
				t.Fatalf("setup Add: %v", err)
			}

			target := fabricengine.WorktreePath(l, slug)
			const sentinel = "UNTRACKED-WORK-NOT-YET-COMMITTED"
			if err := os.WriteFile(filepath.Join(target, "untracked-scratch.txt"), []byte(sentinel+"\n"), 0o644); err != nil {
				t.Fatalf("write untracked file: %v", err)
			}

			// Locking is what makes git refuse even under --force, so the fallback is reached with
			// force already true rather than short-circuited by a successful git removal.
			if _, err := gitexec.Run([]string{"worktree", "lock", target}, l.WorktreePath()); err != nil {
				t.Fatalf("git worktree lock %s: %v", target, err)
			}
			t.Cleanup(func() {
				_, _ = gitexec.Run([]string{"worktree", "unlock", target}, l.WorktreePath())
			})

			err := fabricengine.RemoveWarpWorktreeDirForTest(fabricengine.NewMutations(""), l, target, tt.force)

			_, statErr := os.Stat(target)
			removed := os.IsNotExist(statErr)
			if removed != tt.wantRemoved {
				t.Fatalf("worktree removed = %v; want %v (force=%v, err=%v)", removed, tt.wantRemoved, tt.force, err)
			}

			if tt.wantRemoved {
				if err != nil {
					t.Errorf("RemoveWarpWorktreeDirForTest(force=true) = %v; want nil once the fallback honours force", err)
				}
				return
			}
			if err == nil {
				t.Errorf("RemoveWarpWorktreeDirForTest(force=false) = nil; want a refusal that leaves the untracked file alone")
			}
		})
	}
}

// TestOwnership_RegisteredLinkedWorktreeKind covers the git-touching predicate behind
// ownedRegisteredLinkedWorktree — isRegisteredLinkedWorktreeIn — against every shape its own doc
// comment names.
func TestOwnership_RegisteredLinkedWorktreeKind(t *testing.T) {
	t.Parallel()

	const slug = "ownership-registered"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}
	repoDir := l.WorktreePath()

	t.Run("AcceptsRegisteredLinkedWorktree", func(t *testing.T) {
		linked := fabricengine.WorktreePath(l, slug)
		if !fabricengine.IsRegisteredLinkedWorktreeInForTest(repoDir, linked) {
			t.Errorf("IsRegisteredLinkedWorktreeInForTest(%s) = false; want true for a real registered linked worktree", linked)
		}
	})

	t.Run("RefusesMainWorktree", func(t *testing.T) {
		if fabricengine.IsRegisteredLinkedWorktreeInForTest(repoDir, repoDir) {
			t.Errorf("IsRegisteredLinkedWorktreeInForTest(main) = true; want false — the main worktree is not a linked one")
		}
	})

	t.Run("RefusesUnrelatedGitCloneAtFabricShapedPath", func(t *testing.T) {
		// A real, independently-initialised clone parked at a "*-weft"-shaped path inside the hub —
		// one of the eight original defects this slice closes — not merely an empty directory.
		clone := filepath.Join(l.HubPath, "unrelated-weft")
		if err := os.MkdirAll(clone, 0o755); err != nil {
			t.Fatalf("mkdir clone: %v", err)
		}
		gitkit.MustRun(t, clone, "git", "init", "-b", "main", ".")
		if fabricengine.IsRegisteredLinkedWorktreeInForTest(repoDir, clone) {
			t.Errorf("IsRegisteredLinkedWorktreeInForTest(unrelated clone) = true; want false")
		}
	})

	t.Run("RefusesPlainDirectory", func(t *testing.T) {
		plain := filepath.Join(l.HubPath, "plain-dir")
		if err := os.MkdirAll(plain, 0o755); err != nil {
			t.Fatalf("mkdir plain: %v", err)
		}
		if fabricengine.IsRegisteredLinkedWorktreeInForTest(repoDir, plain) {
			t.Errorf("IsRegisteredLinkedWorktreeInForTest(plain dir) = true; want false")
		}
	})

	t.Run("UnenumerableRepoAnswersFalse", func(t *testing.T) {
		notARepo := t.TempDir()
		target := filepath.Join(notARepo, "child")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatalf("mkdir target: %v", err)
		}
		if fabricengine.IsRegisteredLinkedWorktreeInForTest(notARepo, target) {
			t.Errorf("IsRegisteredLinkedWorktreeInForTest(unenumerable repo) = true; want false, the conservative direction")
		}
	})
}

// TestOwnership_WarpCheckoutKind covers isWarpCheckout — deliberately the counterpart, not a
// synonym, of isRegisteredLinkedWorktreeIn: it admits the hub's prime warp worktree, which the
// registered-linked kind deliberately excludes, and that single divergence is why the two kinds
// exist as separate closed-enum members rather than one.
func TestOwnership_WarpCheckoutKind(t *testing.T) {
	t.Parallel()

	const slug = "ownership-warpcheckout"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}
	repoDir := l.WorktreePath()
	linked := fabricengine.WorktreePath(l, slug)

	t.Run("AcceptsPrimeWarpWorktree", func(t *testing.T) {
		if !fabricengine.IsWarpCheckoutForTest(repoDir, repoDir) {
			t.Errorf("IsWarpCheckoutForTest(prime) = false; want true — the case the registered-linked kind deliberately refuses")
		}
	})

	t.Run("AcceptsLinkedWorktreeOfSameRepo", func(t *testing.T) {
		if !fabricengine.IsWarpCheckoutForTest(repoDir, linked) {
			t.Errorf("IsWarpCheckoutForTest(linked) = false; want true")
		}
	})

	t.Run("RefusesPathNotAWorktreeOfThisRepoAtAll", func(t *testing.T) {
		outsider := t.TempDir()
		if fabricengine.IsWarpCheckoutForTest(repoDir, outsider) {
			t.Errorf("IsWarpCheckoutForTest(outsider) = true; want false")
		}
	})

	// The one assertion that stops a future edit collapsing the two kinds into one.
	t.Run("DisagreesWithRegisteredLinkedKindOnMainWorktree", func(t *testing.T) {
		if fabricengine.IsRegisteredLinkedWorktreeInForTest(repoDir, repoDir) {
			t.Fatalf("precondition failed: main worktree unexpectedly accepted by the registered-linked kind")
		}
		if !fabricengine.IsWarpCheckoutForTest(repoDir, repoDir) {
			t.Fatalf("precondition failed: main worktree unexpectedly refused by the warp-checkout kind")
		}
	})
}

// TestOwnership_FabricHubKind covers looksLikeHub, the predicate behind ownedFabricHub, against
// every shape resetHub's guard names: either structural mark is enough, and an unreadable directory
// is refused rather than treated as absent.
func TestOwnership_FabricHubKind(t *testing.T) {
	t.Parallel()

	t.Run("AcceptsRealHub", func(t *testing.T) {
		t.Parallel()
		fixture := newFabricFixture(t)
		if !fabricengine.LooksLikeHubForTest(fixture.Layout.HubPath) {
			t.Errorf("LooksLikeHubForTest(real hub) = false; want true")
		}
	})

	t.Run("AcceptsBoardEntryOnly", func(t *testing.T) {
		t.Parallel()
		hub := t.TempDir()
		if err := os.MkdirAll(filepath.Join(hub, fabricengine.BoardDirName), 0o755); err != nil {
			t.Fatalf("mkdir board: %v", err)
		}
		if !fabricengine.LooksLikeHubForTest(hub) {
			t.Errorf("LooksLikeHubForTest(board-entry-only) = false; want true")
		}
	})

	t.Run("AcceptsWeftSiblingOnly", func(t *testing.T) {
		t.Parallel()
		hub := t.TempDir()
		if err := os.MkdirAll(filepath.Join(hub, "warp-weft"), 0o755); err != nil {
			t.Fatalf("mkdir weft sibling: %v", err)
		}
		if !fabricengine.LooksLikeHubForTest(hub) {
			t.Errorf("LooksLikeHubForTest(weft-sibling-only) = false; want true")
		}
	})

	t.Run("RefusesNeitherMarker", func(t *testing.T) {
		t.Parallel()
		hub := t.TempDir()
		if err := os.MkdirAll(filepath.Join(hub, "not-a-fabric-marker"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if fabricengine.LooksLikeHubForTest(hub) {
			t.Errorf("LooksLikeHubForTest(neither marker) = true; want false")
		}
	})

	t.Run("RefusesUnreadableDirectory", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" {
			t.Skip("os.Chmod does not restrict directory listing on Windows")
		}
		if os.Geteuid() == 0 {
			t.Skip("running as root; a 0o000 directory is still readable")
		}
		parent := t.TempDir()
		hub := filepath.Join(parent, "unreadable"+fabricengine.HubSuffix)
		if err := os.MkdirAll(hub, 0o755); err != nil {
			t.Fatalf("mkdir hub: %v", err)
		}
		if err := os.Chmod(hub, 0o000); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(hub, 0o755) })
		if fabricengine.LooksLikeHubForTest(hub) {
			t.Errorf("LooksLikeHubForTest(unreadable) = true; want false, the conservative direction")
		}
	})
}

// TestWorktreeDirty_BothScopesAcrossFourStates asserts scopeTracked and scopeAll separately against
// four worktree states, so the one case that justifies dirtiness scope being a declared per-request
// parameter rather than a constant — untracked-only — is proven to actually differ between them,
// not merely assumed.
func TestWorktreeDirty_BothScopesAcrossFourStates(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	dir := fixture.Layout.WorktreePath()

	assertScopes := func(t *testing.T, wantTracked, wantAll bool) {
		t.Helper()
		trackedDirty, _, err := fabricengine.WorktreeDirtyTrackedForTest(dir)
		if err != nil {
			t.Fatalf("WorktreeDirtyTrackedForTest: %v", err)
		}
		if trackedDirty != wantTracked {
			t.Errorf("scopeTracked dirty = %v; want %v", trackedDirty, wantTracked)
		}
		allDirty, _, err := fabricengine.WorktreeDirtyAllForTest(dir)
		if err != nil {
			t.Fatalf("WorktreeDirtyAllForTest: %v", err)
		}
		if allDirty != wantAll {
			t.Errorf("scopeAll dirty = %v; want %v", allDirty, wantAll)
		}
	}

	t.Run("Clean", func(t *testing.T) {
		assertScopes(t, false, false)
	})

	t.Run("UntrackedFileOnly", func(t *testing.T) {
		untracked := filepath.Join(dir, "scratch-untracked.txt")
		if err := os.WriteFile(untracked, []byte("scratch\n"), 0o644); err != nil {
			t.Fatalf("write untracked: %v", err)
		}
		t.Cleanup(func() { _ = os.Remove(untracked) })
		// The proof case: scopeTracked must not see it, scopeAll must.
		assertScopes(t, false, true)
	})

	t.Run("TrackedModification", func(t *testing.T) {
		readme := filepath.Join(dir, "README")
		original, err := os.ReadFile(readme)
		if err != nil {
			t.Fatalf("read README: %v", err)
		}
		if err := os.WriteFile(readme, append(append([]byte{}, original...), []byte("\nmodified\n")...), 0o644); err != nil {
			t.Fatalf("modify README: %v", err)
		}
		t.Cleanup(func() { _ = os.WriteFile(readme, original, 0o644) })
		assertScopes(t, true, true)
	})

	t.Run("StagedChange", func(t *testing.T) {
		staged := filepath.Join(dir, "staged-new.txt")
		if err := os.WriteFile(staged, []byte("staged\n"), 0o644); err != nil {
			t.Fatalf("write staged file: %v", err)
		}
		gitkit.MustRun(t, dir, "git", "add", "staged-new.txt")
		t.Cleanup(func() {
			gitkit.MustRun(t, dir, "git", "reset", "--", "staged-new.txt")
			_ = os.Remove(staged)
		})
		assertScopes(t, true, true)
	})
}

// assertBranchGateRefusesBothForceModes asserts DeleteBranchForTest refuses branch (a weft branch
// in the repo at weftRoot, gated via ownedManagedBranch(l, "")) under both force=false and
// force=true, with the same wantSubstring reason each time, and that branch survives both attempts.
// The force=true half is what proves the property card 34 exists to pin: force reaches only
// Cleanup's own folded-back-raddle gate, never the branch-ownership kind itself — a
// force-satisfies-ownership reading is how the slice's original defect would return behind a flag.
func assertBranchGateRefusesBothForceModes(t *testing.T, l *lyxcwd.Location, weftRoot, branch, wantSubstring string) {
	t.Helper()
	for _, force := range []bool{false, true} {
		err := fabricengine.DeleteBranchForTest(l, weftRoot, branch, "", force)
		if err == nil {
			t.Fatalf("DeleteBranchForTest(%q, force=%v) = nil error; want a refusal", branch, force)
		}
		if !strings.Contains(err.Error(), wantSubstring) {
			t.Errorf("DeleteBranchForTest(%q, force=%v) error = %q; want it to contain %q", branch, force, err, wantSubstring)
		}
		if !branchExistsAt(t, weftRoot, branch) {
			t.Fatalf("DeleteBranchForTest(%q, force=%v) deleted a branch the gate should have refused", branch, force)
		}
	}
}

// TestBranchOwnership_ManagedBranchKind covers ownedManagedBranch/resolveManagedBranch's four
// refusal shapes — the primary weft branch, an unreadable primary, a branch checked out at some
// worktree, and a name outside fabric's own scheme — each under both force=false and force=true,
// plus the positive case that proves the kind is not trivially always-refusing.
func TestBranchOwnership_ManagedBranchKind(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	weftRoot := mustWeftRepoRoot(t, l)
	primaryWeft := fabricengine.WeftBranchName("main")

	t.Run("RefusesPrimaryWeftBranch", func(t *testing.T) {
		assertBranchGateRefusesBothForceModes(t, l, weftRoot, primaryWeft, "primary weft branch")
	})

	t.Run("RefusesWhenPrimaryUnreadable", func(t *testing.T) {
		// A hub whose board worktree is gone cannot answer "what is the repo's primary weft
		// branch" at all — the inherited fail-closed direction that must never invert, since these
		// deletions are irreversible.
		unreadable := newFabricFixture(t)
		ul := unreadable.Layout
		uWeftRoot := mustWeftRepoRoot(t, ul)
		if err := os.RemoveAll(fabricengine.BoardDir(ul.HubPath)); err != nil {
			t.Fatalf("remove board worktree: %v", err)
		}
		const orphan = "orphan-primary-unreadable-weft"
		gitkit.MustRun(t, uWeftRoot, "git", "branch", orphan)
		assertBranchGateRefusesBothForceModes(t, ul, uWeftRoot, orphan, "cannot determine the repo's primary weft branch")
	})

	t.Run("RefusesCheckedOutBranch", func(t *testing.T) {
		checkedOut := newFabricFixture(t)
		cl := checkedOut.Layout
		cWeftRoot := mustWeftRepoRoot(t, cl)
		topology := fabricengine.NewTopology(fabricengine.Config{})
		const slug = "branch-ownership-checkedout"
		if _, err := topology.Add(cl, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
			t.Fatalf("setup Add: %v", err)
		}
		assertBranchGateRefusesBothForceModes(t, cl, cWeftRoot, fabricengine.WeftBranchName(slug), "checked out at")
	})

	t.Run("RefusesUnmanagedName", func(t *testing.T) {
		gitkit.MustRun(t, weftRoot, "git", "branch", "plain-unmanaged-branch")
		assertBranchGateRefusesBothForceModes(t, l, weftRoot, "plain-unmanaged-branch", "not a name fabric's own scheme constructs")
	})

	t.Run("AcceptsOrphanedFabricNamedNonPrimaryNonCheckedOutBranch", func(t *testing.T) {
		const orphan = "orphan-task-weft"
		gitkit.MustRun(t, weftRoot, "git", "branch", orphan)
		err := fabricengine.DeleteBranchForTest(l, weftRoot, orphan, "", false)
		if err != nil {
			t.Fatalf("DeleteBranchForTest(%q) = err %v; want a clean deletion", orphan, err)
		}
		if branchExistsAt(t, weftRoot, orphan) {
			t.Errorf("branch %q still exists after an accepted deletion", orphan)
		}
	})
}

// TestBranchOwnership_RefusalHoldsAtOtherDeletionSites drives the branch-ownership kind through a
// deletion site other than Cleanup — Add's own rollback, the cheapest of the other three since it
// already has integration cover (add_rollback_adopt_test.go) — and asserts the branch the gate
// refuses there survives the rollback. This is the entire reason the ownership logic moved into the
// shared gate rather than staying local to Cleanup: the same refusal must hold everywhere a branch
// is deleted, not just at the one site it was first noticed missing from.
func TestBranchOwnership_RefusalHoldsAtOtherDeletionSites(t *testing.T) {
	t.Parallel()

	const slug = "branch-ownership-rollback-refused"
	fixture := newFabricFixture(t)
	l := fixture.Layout

	// No BranchPrefix, and the slug itself carries no "-weft" suffix: the gate's ownedManagedBranch
	// has no scheme to recognise this warp branch by, so rollbackAdd's own attempt to delete it must
	// be refused exactly as Cleanup's would be for an equivalently-unmanaged name.
	topology := fabricengine.NewTopology(fabricengine.Config{})

	// Break the warp origin remote so Add's final push fails after the warp branch and worktree
	// already exist — the same post-creation-failure injection
	// TestAddRollback_UnwiresJunctionsOnPostWiringFailure uses — triggering rollbackAdd.
	gitkit.MustRun(t, l.WorktreePath(), "git", "remote", "set-url", "origin", filepath.Join(t.TempDir(), "no-such-remote"))

	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err == nil {
		t.Fatalf("Add should have failed (broken origin remote)")
	}

	if !branchExistsAt(t, l.WorktreePath(), slug) {
		t.Fatalf("warp branch %q — which the gate must refuse to delete, since it carries neither the -weft suffix nor a configured prefix — did not survive Add's rollback", slug)
	}
}
