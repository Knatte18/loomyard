// destructivegaps_integration_test.go tests the destructive gate against real git.
// It covers the gate's git-touching ownership predicates and branch ownership at every deletion site.
// It also checks that its two dirtiness scopes differ.
// The hermetic checks, which spawn no git, are in destroy_test.go.
//
// The file reuses mustRecordsRepoRoot (add_rollback_adopt_test.go) and makeBareRemote (clone_adopt_test.go).

package fabricengine_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// escapeFabricConfigYAML returns a repo-wide fabric.yaml override with an empty branch_prefix and pathspec escapeName.
func escapeFabricConfigYAML(escapeName string) string {
	return "branch_prefix: \"\"\npathspec: " + escapeName + "\n"
}

// TestUnwireJunctions_RefusesLinkOutsideItsWorktree asserts that the gate refuses a link outside its worktree,
// and that the link stays on disk.
// WarpJunctions joins the caller-supplied name onto the worktree root, so a name carrying "../" escapes the worktree.
// WorktreePath(l, slug) is a direct child of the hub, so one "../" cancels the slug segment and lands at the hub.
// The test creates that link by hand, because wiring it is not the code path under test.
//
//testtiming:keep the junction-removal executor refusing a link that escapes the worktree and leaving it on disk; coverage of its blocks by other tests does not show an assertion of this
func TestUnwireJunctions_RefusesLinkOutsideItsWorktree(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
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

// TestAddRollback_RefusesJunctionRemovalOutsideItsWorktree checks that Add's rollback refuses an escaping junction.
// The pathspec entry "../gap1-rollback-escape" makes seedLymJunction put its weft target and warp link at one path:
// the worktree and its weft sibling are both direct children of the hub, so one "../" cancels either segment.
// Wiring that junction therefore fails with "already contains a real directory" before any symlink is created.
// That failure triggers Add's rollback,
// and the rollback must refuse to remove the directory that seedLymJunction's os.MkdirAll left behind.
func TestAddRollback_RefusesJunctionRemovalOutsideItsWorktree(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "gap1-rollback-owner"
	// The copied hub's template comes from hubforge.NewHub, which checks the weft primary out on its suffixed branch.
	// It also materializes a real Bolt checkout worktree that ownedManagedBranch and primaryWeftBranch read successfully.
	// So only the escape-specific pathspec override is seeded here.
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.SeedFabricConfig(t, h, escapeFabricConfigYAML("../gap1-rollback-escape"))

	l := h.Location
	// The branch prefix is incidental: the branch side of rollback is not what this test is about.
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

// TestTeardownHub_RefusesHubPathOutsideOperatorNamedParent asserts the containment check of teardownHub.
// CloneHub always derives the hub path as a literal child of its cwd argument,
// so the refusal cannot be reached through CloneHub, and TeardownHubForTest drives teardownHub directly.
func TestTeardownHub_RefusesHubPathOutsideOperatorNamedParent(t *testing.T) {
	tiergate.Require(t, 1)
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

// TestCloneHub_TeardownSucceedsOnAHalfBuiltHub checks that teardownHub removes a half-built hub.
// It authorises that removal with the createdToken this CloneHub invocation minted.
// The earliest teardownHub call sites in CloneHub run before the board and the weft sibling exist.
// Teardown there cannot depend on either of them.
// A check that needed them would refuse at nearly every early failure site and leave a residual hub.
// The test drives a real CloneHub run against an unreachable warp URL, which fails at that early point:
// only the hub directory exists, with no warp clone, weft clone, board worktree or hub scratch tree.
// It calls the exported entry point, so the whole scenario runs end to end.
//
//testtiming:keep teardownHub removing a half-built hub on the token it minted though neither side exists yet; coverage of its blocks by other tests does not show an assertion of this
func TestCloneHub_TeardownSucceedsOnAHalfBuiltHub(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	dir := t.TempDir()
	weftRemote := makeBareRemote(t, dir, "weft-half-built")
	badWarpURL := filepath.Join(dir, "no-such-warp-repo")

	cloneParent := t.TempDir()

	_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:   weftRemote,
		WarpURL:   badWarpURL,
		Shortname: "tst",
	})
	if err == nil {
		t.Fatalf("CloneHub against a nonexistent warp URL = nil error; want a clone failure")
	}

	hubPath := fabricengine.HubPath(cloneParent, fabricengine.DeriveWarpName(badWarpURL))
	if _, statErr := os.Stat(hubPath); !os.IsNotExist(statErr) {
		t.Errorf("residual half-built hub left at %s after the warp-clone failure; teardown should have removed it (no board, no weft sibling yet — the token, not the hub predicate, authorises this)", hubPath)
	}
}

// TestRemoveWarpWorktreeDir_FallbackRefusesRegisteredWorktreeWithUntrackedFiles checks that the fallback refuses.
// removeWarpWorktreeDir runs the fallback, a directory removal, when `git worktree remove` itself refuses.
// The remove verb's top-level dirty check runs before removeWarpWorktreeDir,
// and it would already refuse an untracked file, so the test calls removeWarpWorktreeDir directly.
// The test builds a registered linked worktree with an untracked file, removed without force.
// Git's own refusal, not the earlier gate, is what triggers the fallback.
// Asserting the error alone would also pass for an ungated fallback.
// Such a fallback returns whatever os.RemoveAll or the surrounding plumbing reports.
// The test therefore asserts that the worktree stays on disk.
//
//testtiming:keep the directory-removal fallback refusing a registered worktree with untracked files and leaving it on disk; coverage of its blocks by other tests does not show an assertion of this
func TestRemoveWarpWorktreeDir_FallbackRefusesRegisteredWorktreeWithUntrackedFiles(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "gap3-untracked"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

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

// TestRemoveWarpWorktreeDir_FallbackHonoursForce checks that --force reaches the fallback removal,
// so an operator's --force is not silently dropped there.
//
// The fallback request must carry force.
// Left at Go's zero value, the fallback refuses even though the operator passed --force,
// and the pair is left half torn down.
//
// `git worktree lock` makes `git worktree remove --force` fail on a worktree that is still registered.
// That registered state is what routes into the fallback with force already set.
// The no-force half runs the identical setup and must still refuse.
// Propagating force must not weaken the guard against deleting what git declined to discard.
func TestRemoveWarpWorktreeDir_FallbackHonoursForce(t *testing.T) {
	tiergate.Require(t, 1)
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
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			l := h.Location
			hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

			target := fabricengine.WorktreePath(l, slug)
			const sentinel = "UNTRACKED-WORK-NOT-YET-COMMITTED"
			if err := os.WriteFile(filepath.Join(target, "untracked-scratch.txt"), []byte(sentinel+"\n"), 0o644); err != nil {
				t.Fatalf("write untracked file: %v", err)
			}

			// Locking makes git refuse even under --force, so the fallback is reached with force already true.
			// Without the lock, a successful git removal would skip the fallback.
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

// TestRemoveWarpWorktreeDir_LeftoverRetry drives Topology.Remove over a pair whose task worktree is left behind.
// `git worktree remove` deregisters that worktree and fails to delete it.
// The test asserts the late-writer retry of the directory it leaves.
// Off Windows, git is made to fail by a read-only subdirectory that holds a file committed on the pair's branch,
// so the pair stays clean (a directory's mode is not tracked) and passes the remove verb's dirty checks.
// On Windows, a committed file held open by its writer makes git's delete fail,
// and the first contained removal fails with a sharing violation.
func TestRemoveWarpWorktreeDir_LeftoverRetry(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const lockedDirName = "locked"
	type outcome struct {
		res         fabricengine.RemoveResult
		err         error
		target      string
		removeCalls int
		sleepCalls  int
	}
	// setup returns the seam's remove and sleep for one row, given the read-only directory's path;
	// a nil remove or sleep keeps production.
	tests := []struct {
		name  string
		slug  string
		setup func(lockedDir string, o *outcome) (remove func(container, target string) (bool, bool, error), sleep func(time.Duration))
		check func(t *testing.T, slug string, o outcome)
	}{
		{
			name: "a leftover whose first removal reports not-empty is removed on a retry",
			slug: "leftover-retried",
			setup: func(lockedDir string, o *outcome) (func(string, string) (bool, bool, error), func(time.Duration)) {
				return func(container, path string) (bool, bool, error) {
						o.removeCalls++
						if o.removeCalls == 1 {
							return false, false, &os.PathError{Op: "unlinkat", Path: path, Err: syscall.ENOTEMPTY}
						}
						if err := os.Chmod(lockedDir, 0o755); err != nil {
							return false, false, err
						}
						return true, true, os.RemoveAll(path)
					},
					func(time.Duration) { o.sleepCalls++ }
			},
			check: func(t *testing.T, slug string, o outcome) {
				if o.err != nil {
					t.Fatalf("Remove error = %v; want the leftover finished on the retry", o.err)
				}
				if _, statErr := os.Lstat(o.target); !os.IsNotExist(statErr) {
					t.Errorf("target %s still on disk after Remove (stat error = %v)", o.target, statErr)
				}
				want := fabricengine.Mutation{Kind: fabricengine.KindPathRemoved, Target: slug, Detail: "recursive"}
				if !slices.Contains(o.res.Mutations.Entries(), want) {
					t.Errorf("mutation record = %+v; want it to hold %+v", o.res.Mutations.Entries(), want)
				}
				if o.removeCalls != 2 || o.sleepCalls != 1 {
					t.Errorf("remove ran %d and sleep %d times; want 2 and 1", o.removeCalls, o.sleepCalls)
				}
			},
		},
		{
			name: "a leftover still not empty after the last attempt is returned",
			slug: "leftover-exhausted",
			setup: func(lockedDir string, o *outcome) (func(string, string) (bool, bool, error), func(time.Duration)) {
				return func(container, path string) (bool, bool, error) {
						o.removeCalls++
						return false, false, &os.PathError{Op: "unlinkat", Path: path, Err: syscall.ENOTEMPTY}
					},
					func(time.Duration) { o.sleepCalls++ }
			},
			check: func(t *testing.T, slug string, o outcome) {
				if !errors.Is(o.err, syscall.ENOTEMPTY) {
					t.Fatalf("Remove error = %v; want it to wrap ENOTEMPTY", o.err)
				}
				if !strings.Contains(o.err.Error(), o.target) || !strings.Contains(o.err.Error(), "not a linked worktree") {
					t.Errorf("Remove error = %v; want it to name %s as not a linked worktree", o.err, o.target)
				}
				if o.removeCalls != 3 || o.sleepCalls != 2 {
					t.Errorf("remove ran %d and sleep %d times; want 3 and 2", o.removeCalls, o.sleepCalls)
				}
				if _, statErr := os.Lstat(o.target); statErr != nil {
					t.Errorf("target %s is gone after a failed finish (stat error = %v); want it on disk", o.target, statErr)
				}
			},
		},
		{
			name: "another removal failure is not retried",
			slug: "leftover-other-failure",
			setup: func(lockedDir string, o *outcome) (func(string, string) (bool, bool, error), func(time.Duration)) {
				return func(container, path string) (bool, bool, error) {
						o.removeCalls++
						return false, false, &os.PathError{Op: "unlinkat", Path: path, Err: fs.ErrPermission}
					},
					func(time.Duration) { o.sleepCalls++ }
			},
			check: func(t *testing.T, slug string, o outcome) {
				if !errors.Is(o.err, fs.ErrPermission) {
					t.Fatalf("Remove error = %v; want it to wrap the permission error", o.err)
				}
				if o.removeCalls != 1 || o.sleepCalls != 0 {
					t.Errorf("remove ran %d and sleep %d times; want 1 and 0", o.removeCalls, o.sleepCalls)
				}
			},
		},
	}

	for _, tt := range tests {
		// A read-only directory stops git's delete only off Windows, so Windows generates no subtest for a row.
		if runtime.GOOS == "windows" {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if os.Geteuid() == 0 {
				t.Skip("a read-only directory stops git's delete only for a non-root user")
			}

			slug := tt.slug
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			l := h.Location
			hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
			target := fabricengine.WorktreePath(l, slug)
			gitkit.CommitFile(t, target, lockedDirName+"/held.txt", "committed\n", "add a file under the directory made read-only")
			lockedDir := filepath.Join(target, lockedDirName)
			if err := os.Chmod(lockedDir, 0o555); err != nil {
				t.Fatalf("make %s read-only: %v", lockedDir, err)
			}
			t.Cleanup(func() { _ = os.Chmod(lockedDir, 0o755) })

			o := outcome{target: target}
			remove, sleep := tt.setup(lockedDir, &o)
			fabricengine.SetLeftoverSeamForTest(h.Topology, remove, sleep)
			o.res, o.err = h.Topology.Remove(l, slug, false, false)

			tt.check(t, slug, o)
		})
	}

	if runtime.GOOS == "windows" {
		t.Run("a file held open by its writer is removed once the writer closes it", func(t *testing.T) {
			t.Parallel()

			const slug = "leftover-held-open"
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			l := h.Location
			hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
			target := fabricengine.WorktreePath(l, slug)
			gitkit.CommitFile(t, target, "held.txt", "committed\n", "add a file its writer holds open")
			held, err := os.Open(filepath.Join(target, "held.txt"))
			if err != nil {
				t.Fatalf("open held file: %v", err)
			}
			t.Cleanup(func() { _ = held.Close() })

			sleepCalls := 0
			fabricengine.SetLeftoverSeamForTest(h.Topology, nil, func(time.Duration) {
				sleepCalls++
				_ = held.Close()
			})
			res, err := h.Topology.Remove(l, slug, false, false)

			if err != nil {
				t.Fatalf("Remove error = %v; want the leftover finished once the writer closed the file", err)
			}
			want := fabricengine.Mutation{Kind: fabricengine.KindPathRemoved, Target: slug, Detail: "recursive"}
			if !slices.Contains(res.Mutations.Entries(), want) {
				t.Errorf("mutation record = %+v; want it to hold %+v", res.Mutations.Entries(), want)
			}
			if sleepCalls != 1 {
				t.Errorf("sleep ran %d times; want 1", sleepCalls)
			}
		})
	}
}

// TestOwnership_RegisteredLinkedWorktreeKind covers isRegisteredLinkedWorktreeIn over each shape of path.
// It is git-touching: it reports whether a path is a registered linked worktree of a given repository.
func TestOwnership_RegisteredLinkedWorktreeKind(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "ownership-registered"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
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
		// An independently initialised clone at a "*-weft"-shaped path inside the hub is not a linked worktree.
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

// TestOwnership_WarpCheckoutKind covers isAnyWorktreeOf, which accepts any worktree of the repo.
// That includes the hub's prime warp worktree, and a path that is not a worktree of the repo is refused.
func TestOwnership_WarpCheckoutKind(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "ownership-warpcheckout"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
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

	// Fails if isAnyWorktreeOf and isRegisteredLinkedWorktreeIn agree on the main worktree.
	t.Run("DisagreesWithRegisteredLinkedKindOnMainWorktree", func(t *testing.T) {
		if fabricengine.IsRegisteredLinkedWorktreeInForTest(repoDir, repoDir) {
			t.Fatalf("precondition failed: main worktree unexpectedly accepted by the registered-linked kind")
		}
		if !fabricengine.IsWarpCheckoutForTest(repoDir, repoDir) {
			t.Fatalf("precondition failed: main worktree unexpectedly refused by the warp-checkout kind")
		}
	})
}

// TestOwnership_FabricHubKind covers looksLikeHub: either of its two structural marks is enough to recognise a hub,
// and an unreadable directory is refused rather than treated as absent.
func TestOwnership_FabricHubKind(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("AcceptsRealHub", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		if !fabricengine.LooksLikeHubForTest(h.Location.HubPath) {
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

	// AcceptsLegacySuffixedHubDirectory pins that looksLikeHub judges structure alone.
	// A directory named loomyard-HUB that holds the board directory resolves as a hub,
	// and its name is never grounds for refusal.
	t.Run("AcceptsLegacySuffixedHubDirectory", func(t *testing.T) {
		t.Parallel()
		hub := filepath.Join(t.TempDir(), "loomyard-HUB")
		if err := os.MkdirAll(filepath.Join(hub, fabricengine.BoardDirName), 0o755); err != nil {
			t.Fatalf("mkdir board: %v", err)
		}
		if !fabricengine.LooksLikeHubForTest(hub) {
			t.Errorf("LooksLikeHubForTest(legacy-suffixed hub) = false; want true")
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

	if runtime.GOOS != "windows" {
		t.Run("RefusesUnreadableDirectory", func(t *testing.T) {
			t.Parallel()
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
}

// assertBranchGateRefusesBothForceModes asserts that DeleteBranchForTest refuses branch under both force modes.
// The branch is a weft branch in the repo at weftRoot, gated by ownedManagedBranch(l).
// Each refusal must carry wantSubstring in its error, and the branch must survive both attempts.
// The force=true attempt pins that force does not satisfy branch ownership.
func assertBranchGateRefusesBothForceModes(t *testing.T, l *lymcwd.Location, weftRoot, branch, wantSubstring string) {
	t.Helper()
	for _, force := range []bool{false, true} {
		err := fabricengine.DeleteBranchForTest(l, weftRoot, branch, force)
		if err == nil {
			t.Fatalf("DeleteBranchForTest(%q, force=%v) = nil error; want a refusal", branch, force)
		}
		if !strings.Contains(err.Error(), wantSubstring) {
			t.Errorf("DeleteBranchForTest(%q, force=%v) error = %q; want it to contain %q", branch, force, err, wantSubstring)
		}
		if !gitkit.BranchExists(t, weftRoot, branch) {
			t.Fatalf("DeleteBranchForTest(%q, force=%v) deleted a branch the gate should have refused", branch, force)
		}
	}
}

// TestBranchOwnership_ManagedBranchKind covers the refusal shapes of ownedManagedBranch and resolveManagedBranch.
// The refused shapes are the primary weft branch, an unreadable primary, and a branch checked out at some worktree.
// A name outside fabric's own scheme is refused as well.
// Each shape is tested under both force=false and force=true.
// A positive case shows the kind is not always refusing.
func TestBranchOwnership_ManagedBranchKind(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weftRoot := mustRecordsRepoRoot(t, l)
	primaryWeft := fabricengine.RecordsBranchName("main")

	t.Run("RefusesPrimaryWeftBranch", func(t *testing.T) {
		assertBranchGateRefusesBothForceModes(t, l, weftRoot, primaryWeft, "primary weft branch")
	})

	t.Run("RefusesWhenPrimaryUnreadable", func(t *testing.T) {
		// A hub whose board worktree is gone cannot answer which branch is the repo's primary weft branch.
		// The gate fails closed in that case, and that direction must never invert.
		// These deletions are irreversible.
		unreadable := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		ul := unreadable.Location
		uWeftRoot := mustRecordsRepoRoot(t, ul)
		if err := os.RemoveAll(fabricengine.BoardDir(ul.HubPath)); err != nil {
			t.Fatalf("remove board worktree: %v", err)
		}
		const orphan = "orphan-primary-unreadable-weft"
		gitkit.MustRun(t, uWeftRoot, "git", "branch", orphan)
		assertBranchGateRefusesBothForceModes(t, ul, uWeftRoot, orphan, "cannot determine the repo's primary weft branch")
	})

	t.Run("RefusesCheckedOutBranch", func(t *testing.T) {
		checkedOut := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		cl := checkedOut.Location
		cWeftRoot := mustRecordsRepoRoot(t, cl)
		topology := checkedOut.Topology
		const slug = "branch-ownership-checkedout"
		if _, err := topology.Add(cl, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
			t.Fatalf("setup Add: %v", err)
		}
		assertBranchGateRefusesBothForceModes(t, cl, cWeftRoot, fabricengine.RecordsBranchName(slug), "checked out at")
	})

	t.Run("RefusesUnmanagedName", func(t *testing.T) {
		gitkit.MustRun(t, weftRoot, "git", "branch", "plain-unmanaged-branch")
		assertBranchGateRefusesBothForceModes(t, l, weftRoot, "plain-unmanaged-branch", "not a name fabric's own scheme constructs")
	})

	t.Run("AcceptsOrphanedFabricNamedNonPrimaryNonCheckedOutBranch", func(t *testing.T) {
		const orphan = "orphan-task-weft"
		gitkit.MustRun(t, weftRoot, "git", "branch", orphan)
		err := fabricengine.DeleteBranchForTest(l, weftRoot, orphan, false)
		if err != nil {
			t.Fatalf("DeleteBranchForTest(%q) = err %v; want a clean deletion", orphan, err)
		}
		if gitkit.BranchExists(t, weftRoot, orphan) {
			t.Errorf("branch %q still exists after an accepted deletion", orphan)
		}
	})
}

// TestReconcile_ReportsAPairThatVanishedMidWalkAsSuch covers a pair whose directory is deleted mid-walk.
// Reconcile reads `git worktree list` once, before the per-pair loop, so this race cannot be avoided.
// A concurrent `lym fabric remove` or `prune` can delete a pair's directory before the loop reaches it.
//
// Such a pair must be reported as vanished.
// It must not surface as ReconcileActionUnmanagedReported, which means the pair is not fabric's to manage.
// It must not set pr.Error, since a concurrent teardown would then make every enclosing reconcile exit non-zero.
//
// Deleting the directory directly, with the registration left in place, reproduces the state without concurrency.
// That keeps the test deterministic rather than timing-dependent.
func TestReconcile_ReportsAPairThatVanishedMidWalkAsSuch(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "vanished-mid-walk"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, slug)

	warpPath := h.PairCodeWorktree(slug)
	if err := os.RemoveAll(warpPath); err != nil {
		t.Fatalf("remove warp worktree directory (leaving git's registration behind): %v", err)
	}

	result, err := h.Topology.Reconcile(h.Location)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	var found bool
	for _, pair := range result.Pairs {
		if filepath.Base(filepath.FromSlash(pair.CodeWorktree)) != slug {
			continue
		}
		found = true
		if pair.Action != fabricengine.ReconcileActionVanishedMidWalk {
			t.Errorf("vanished pair Action = %q; want %q", pair.Action, fabricengine.ReconcileActionVanishedMidWalk)
		}
		if pair.Error != "" {
			t.Errorf("vanished pair Error = %q; want empty — nothing failed to reconcile, the pair stopped existing", pair.Error)
		}
		if pair.Detail == "" {
			t.Errorf("vanished pair carries no Detail; want one naming the concurrent teardown")
		}
	}
	if !found {
		t.Fatalf("Reconcile reported no pair for the vanished slug %q; pairs: %+v", slug, result.Pairs)
	}
}
