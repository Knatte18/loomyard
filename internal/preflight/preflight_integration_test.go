//go:build integration

// preflight_integration_test.go drives Check/CheckResolved/Wired/HubPresent end-to-end against real
// git fixtures — a paired warp+fabric worktree with a wired _lyx junction — covering every
// pass/fail scenario across the tier-1/tier-2 preconditions this package validates, plus the
// predicate split. It is integration-tagged because it spawns git via hubforge fixtures (Test Tier
// Purity Invariant).
//
// It is a package preflight_test file, not an in-package test, because internal/hubforge imports
// internal/fabriccli, and internal/preflight sits inside that dependency set: an in-package fixture
// test importing internal/hubforge would close a compile cycle. Check, CheckResolved, Wired and
// HubPresent are all already exported, so no export_test.go shim is needed.

package preflight_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/preflight"
)

// setupFixture builds a fully-configured real hub with fabric and junction setup, returning the hub
// and the slug for WireJunctions.
func setupFixture(t *testing.T) (*hubforge.Hub, string) {
	t.Helper()

	h := hubforge.NewHub(t, ".")
	slug := filepath.Base(h.Location.WorktreePath())

	hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: _extra\n")

	if err := fabricengine.WireJunctions(h.Location, slug, []string{"_lyx", lyxdirs.DotLyxDirName, "_extra"}); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	// The wired junctions materialize into the paired-sibling worktree's own git
	// repo, where they start out untracked. Commit them so a freshly-built
	// fixture is genuinely clean on both sides, since CheckResolved's
	// worktree-clean check covers the paired sibling too.
	// The commit is --allow-empty because after the clone-commit change the weft prime already
	// arrives clean, .lyx is excluded through the weft repo's .git/info/exclude, and the _extra
	// junction target materializes as an empty directory git does not track -- so this pair becomes
	// a no-op that must be allowed to succeed rather than deleted, because deleting it would silently
	// drop the guarantee if a future fixture change reintroduces untracked weft content.
	gitkit.MustRun(t, h.PrimeWeft(), "git", "add", "-A")
	gitkit.MustRun(t, h.PrimeWeft(), "git", "commit", "--allow-empty", "-m", "seed junctions")

	return h, slug
}

// assertCheckSet asserts that got's Failures carry exactly the given CheckID set
// (order-independent). An empty want asserts Report.OK.
func assertCheckSet(t *testing.T, got preflight.Report, want ...preflight.CheckID) {
	t.Helper()

	if len(want) == 0 {
		if !got.OK || len(got.Failures) != 0 {
			t.Errorf("Report = %+v; want OK with no failures", got)
		}
		return
	}

	if got.OK {
		t.Errorf("Report.OK = true; want failures %v", want)
	}

	wantSet := make(map[preflight.CheckID]bool, len(want))
	for _, c := range want {
		wantSet[c] = true
	}
	gotSet := make(map[preflight.CheckID]bool, len(got.Failures))
	for _, f := range got.Failures {
		gotSet[f.Check] = true
	}

	for c := range wantSet {
		if !gotSet[c] {
			t.Errorf("Report.Failures = %+v; missing expected CheckID %q", got.Failures, c)
		}
	}
	for c := range gotSet {
		if !wantSet[c] {
			t.Errorf("Report.Failures = %+v; unexpected CheckID %q", got.Failures, c)
		}
	}
}

// TestCheckResolved_HealthyPair is the anchor case: a fully healthy paired warp+fabric worktree
// reports OK.
func TestCheckResolved_HealthyPair(t *testing.T) {
	t.Parallel()

	h, _ := setupFixture(t)

	report, err := preflight.CheckResolved(h.Location)
	if err != nil {
		t.Fatalf("CheckResolved: %v", err)
	}
	assertCheckSet(t, report)
}

// TestCheck_NotAGitRepo asserts that Check() invoked outside any git repository reports a single
// geometry failure with no error — the report-not-error contract's most easily-regressed row.
func TestCheck_NotAGitRepo(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	report, loc, err := preflight.Check(dir)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if loc != nil {
		t.Errorf("Check() *lyxcwd.Location = %+v; want nil", loc)
	}
	assertCheckSet(t, report, preflight.CheckGeometry)
}

// TestCheckResolved_PrimeNameFailure asserts that a fabricengine.PrimeName failure short-circuits
// with only a geometry failure and no other check recorded.
func TestCheckResolved_PrimeNameFailure(t *testing.T) {
	t.Parallel()

	h, _ := setupFixture(t)

	// Break `git worktree list --porcelain` at the anchor path without breaking
	// `git rev-parse --show-toplevel`, so this exercises PrimeName's own failure
	// path rather than lyxcwd.Resolve's -- CheckResolved(l) starts directly from
	// an already-resolved Location and never re-resolves.
	dotGit := filepath.Join(h.Location.WorktreePath(), ".git")
	if err := os.RemoveAll(dotGit); err != nil {
		t.Fatalf("remove %s: %v", dotGit, err)
	}

	report, err := preflight.CheckResolved(h.Location)
	if err != nil {
		t.Fatalf("CheckResolved: %v", err)
	}
	assertCheckSet(t, report, preflight.CheckGeometry)
}

// TestCheckResolved_Dirty covers all three ways cleanliness can observe a dirty repo (a
// tracked-and-modified file, a staged file, and an untracked-only file) across both sides of the
// pair: the warp side and the paired side.
func TestCheckResolved_Dirty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		dirty func(t *testing.T, h *hubforge.Hub)
	}{
		{
			name: "WarpSide",
			dirty: func(t *testing.T, h *hubforge.Hub) {
				untracked := filepath.Join(h.PrimeWorktree(), "untracked.txt")
				if err := os.WriteFile(untracked, []byte("new"), 0o644); err != nil {
					t.Fatalf("write untracked warp file: %v", err)
				}
			},
		},
		{
			name: "PairedSide",
			dirty: func(t *testing.T, h *hubforge.Hub) {
				untracked := filepath.Join(h.PrimeWeft(), "untracked.txt")
				if err := os.WriteFile(untracked, []byte("new"), 0o644); err != nil {
					t.Fatalf("write untracked paired-side file: %v", err)
				}
			},
		},
		{
			name: "WarpSideTrackedModified",
			dirty: func(t *testing.T, h *hubforge.Hub) {
				readme := filepath.Join(h.PrimeWorktree(), "README")
				if err := os.WriteFile(readme, []byte("modified"), 0o644); err != nil {
					t.Fatalf("modify README: %v", err)
				}
			},
		},
		{
			name: "WarpSideStaged",
			dirty: func(t *testing.T, h *hubforge.Hub) {
				readme := filepath.Join(h.PrimeWorktree(), "README")
				if err := os.WriteFile(readme, []byte("staged"), 0o644); err != nil {
					t.Fatalf("modify README: %v", err)
				}
				gitkit.MustRun(t, h.PrimeWorktree(), "git", "add", "README")
			},
		},
		{
			name: "BothSides",
			dirty: func(t *testing.T, h *hubforge.Hub) {
				warpUntracked := filepath.Join(h.PrimeWorktree(), "untracked.txt")
				if err := os.WriteFile(warpUntracked, []byte("new"), 0o644); err != nil {
					t.Fatalf("write untracked warp file: %v", err)
				}
				pairedUntracked := filepath.Join(h.PrimeWeft(), "untracked.txt")
				if err := os.WriteFile(pairedUntracked, []byte("new"), 0o644); err != nil {
					t.Fatalf("write untracked paired-side file: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := setupFixture(t)
			tt.dirty(t, h)

			report, err := preflight.CheckResolved(h.Location)
			if err != nil {
				t.Fatalf("CheckResolved: %v", err)
			}
			assertCheckSet(t, report, preflight.CheckWorktreeClean)
		})
	}
}

// TestCheckResolved_FabricNotReady asserts that a removed paired-sibling worktree reports
// fabric-ready.
func TestCheckResolved_FabricNotReady(t *testing.T) {
	t.Parallel()

	h, _ := setupFixture(t)

	if err := os.RemoveAll(h.PrimeWeft()); err != nil {
		t.Fatalf("remove paired-sibling worktree: %v", err)
	}

	report, err := preflight.CheckResolved(h.Location)
	if err != nil {
		t.Fatalf("CheckResolved: %v", err)
	}
	assertCheckSet(t, report, preflight.CheckFabricReady)
}

// TestCheckResolved_BranchMismatch asserts that a branch mismatch classifies as CheckFabricSync,
// not CheckJunction.
func TestCheckResolved_BranchMismatch(t *testing.T) {
	t.Parallel()

	h, _ := setupFixture(t)

	gitkit.MustRun(t, h.PrimeWorktree(), "git", "checkout", "-b", "warp-only")

	report, err := preflight.CheckResolved(h.Location)
	if err != nil {
		t.Fatalf("CheckResolved: %v", err)
	}
	assertCheckSet(t, report, preflight.CheckFabricSync)
}

// TestCheckResolved_BrokenJunction asserts that all three of Healthy's junction-drift shapes —
// missing, not-a-link, and points-elsewhere — classify as junction, via Healthy's typed Cause
// rather than a substring match.
// Each drift shape is exercised against BOTH junctions (_lyx and a second, non-_lyx junction) so the
// classification is proven to hold for the second, non-_lyx junction too — not just the one Healthy's
// underlying loop was originally written and tested against.
func TestCheckResolved_BrokenJunction(t *testing.T) {
	t.Parallel()

	shapes := []struct {
		name    string
		corrupt func(t *testing.T, warpLink string)
	}{
		{
			name: "Missing",
			corrupt: func(t *testing.T, warpLink string) {
				if err := fslink.Remove(warpLink); err != nil {
					t.Fatalf("remove junction %s: %v", warpLink, err)
				}
			},
		},
		{
			name: "NotALink",
			corrupt: func(t *testing.T, warpLink string) {
				if err := fslink.Remove(warpLink); err != nil {
					t.Fatalf("remove junction %s: %v", warpLink, err)
				}
				if err := os.Mkdir(warpLink, 0o755); err != nil {
					t.Fatalf("mkdir real dir in junction's place %s: %v", warpLink, err)
				}
			},
		},
		{
			name: "PointsElsewhere",
			corrupt: func(t *testing.T, warpLink string) {
				if err := fslink.Remove(warpLink); err != nil {
					t.Fatalf("remove junction %s: %v", warpLink, err)
				}
				wrongTarget := filepath.Join(filepath.Dir(warpLink), "not-the-fabric-junction-dir")
				if err := os.MkdirAll(wrongTarget, 0o755); err != nil {
					t.Fatalf("mkdir wrong target %s: %v", wrongTarget, err)
				}
				if err := fslink.CreateDirLink(warpLink, wrongTarget); err != nil {
					t.Fatalf("CreateDirLink(%s, %s): %v", warpLink, wrongTarget, err)
				}
			},
		},
	}

	junctions := []struct {
		name    string
		linkFor func(h *hubforge.Hub, slug string) string
	}{
		{
			name:    "Lyx",
			linkFor: func(h *hubforge.Hub, slug string) string { return fabricengine.WarpLyxLink(h.Location, slug) },
		},
		{
			name: "Extra",
			linkFor: func(h *hubforge.Hub, slug string) string {
				return filepath.Join(fabricengine.WorktreePath(h.Location, slug), h.Location.AnchorRel, "_extra")
			},
		},
	}

	for _, j := range junctions {
		for _, tt := range shapes {
			t.Run(j.name+"_"+tt.name, func(t *testing.T) {
				t.Parallel()

				h, slug := setupFixture(t)
				warpLink := j.linkFor(h, slug)
				tt.corrupt(t, warpLink)

				report, err := preflight.CheckResolved(h.Location)
				if err != nil {
					t.Fatalf("CheckResolved: %v", err)
				}
				assertCheckSet(t, report, preflight.CheckJunction)
			})
		}
	}
}

// TestCheckResolved_ConfigLoadFailed asserts the CauseConfigLoadFailed/CheckJunction equivalence
// pinned by healthy-typed-reason: a repo-wide fabric.yaml that fails to load classifies as
// CheckJunction (not a distinct CheckID of its own), same as the three junction-drift shapes
// TestCheckResolved_BrokenJunction covers.
func TestCheckResolved_ConfigLoadFailed(t *testing.T) {
	t.Parallel()

	h, _ := setupFixture(t)

	configPath := configengine.ConfigFile(fabricengine.BoardDir(h.Location.HubPath), "fabric")
	if err := os.WriteFile(configPath, []byte("not: [valid: yaml"), 0o644); err != nil {
		t.Fatalf("corrupt repo-wide fabric config: %v", err)
	}

	report, err := preflight.CheckResolved(h.Location)
	if err != nil {
		t.Fatalf("CheckResolved: %v", err)
	}
	assertCheckSet(t, report, preflight.CheckJunction)
}

// TestCheckResolved_MissingOptionalJunctionIsAJunctionFault covers a worktree whose optional
// junction was never wired at all: _lyx is fully healthy, but the second, non-_lyx junction is
// entirely absent (simulated here by removing it from an otherwise-healthy fixture, rather than
// corrupting it — the fixture never had it, full stop).
// CheckResolved must classify this as CheckJunction, never CheckFabricSync, and blocks the run
// (report.OK == false). A single Reconcile repairs it (adds the missing junction and materialises
// its fabric-side target) rather than reporting already-healthy; and a fresh CheckResolved afterward
// reports OK.
func TestCheckResolved_MissingOptionalJunctionIsAJunctionFault(t *testing.T) {
	t.Parallel()

	h, slug := setupFixture(t)

	// Simulate the missing-optional-junction state: this worktree's second,
	// non-_lyx junction was never wired, even though _lyx is fully healthy.
	extraLink := filepath.Join(fabricengine.WorktreePath(h.Location, slug), h.Location.AnchorRel, "_extra")
	if err := fslink.Remove(extraLink); err != nil {
		t.Fatalf("remove the optional junction to simulate a worktree missing it: %v", err)
	}

	report, err := preflight.CheckResolved(h.Location)
	if err != nil {
		t.Fatalf("CheckResolved: %v", err)
	}
	assertCheckSet(t, report, preflight.CheckJunction)

	// One Reconcile call repairs the missing junction: it must report
	// JunctionRepointed (the repair happened), never AlreadyHealthy.
	topology := fabricengine.NewTopology(fabricengine.Config{})
	result, err := topology.Reconcile(h.Location)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	var found bool
	for _, pair := range result.Pairs {
		if pair.WarpWorktree != filepath.ToSlash(h.Location.WorktreePath()) {
			continue
		}
		found = true
		if pair.Action != fabricengine.ReconcileActionJunctionRepointed {
			t.Errorf("Reconcile Action = %q; want %q", pair.Action, fabricengine.ReconcileActionJunctionRepointed)
		}
		if pair.Error != "" {
			t.Errorf("Reconcile Error = %q; want empty", pair.Error)
		}
	}
	if !found {
		t.Fatalf("Reconcile result has no pair for the worktree %s: %+v", h.Location.WorktreePath(), result.Pairs)
	}

	// The junction now resolves.
	if isLink, err := fslink.IsLink(extraLink); err != nil || !isLink {
		t.Fatalf("optional junction %s not restored by Reconcile: isLink=%v err=%v", extraLink, isLink, err)
	}

	// A fresh CheckResolved now reports OK: the remedy this batch documents.
	report, err = preflight.CheckResolved(h.Location)
	if err != nil {
		t.Fatalf("CheckResolved after Reconcile: %v", err)
	}
	assertCheckSet(t, report)
}

// TestCheck_SubpathAnchoredHubIsNotRejected asserts that a legitimately subpath-anchored repo is
// validated on its merits rather than short-circuited.
func TestCheck_SubpathAnchoredHubIsNotRejected(t *testing.T) {
	t.Parallel()

	h, _ := setupFixture(t)

	sub := filepath.Join(h.PrimeWorktree(), "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", sub, err)
	}

	anchorPath := filepath.Join(fabricengine.BoardDir(h.Location.HubPath), lyxcwd.AnchorFileName)
	if err := os.WriteFile(anchorPath, []byte("sub"), 0o644); err != nil {
		t.Fatalf("write %s: %v", anchorPath, err)
	}

	report, loc, err := preflight.Check(sub)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if loc == nil {
		t.Fatalf("Check() *lyxcwd.Location = nil; want non-nil")
	}
	for _, failure := range report.Failures {
		if failure.Check == preflight.CheckGeometry {
			t.Errorf("Check() on a subpath-anchored hub reported %q: %s; want the anchor treated as legal geometry",
				failure.Check, failure.Reason)
		}
	}
}

// TestCheckResolved_MultipleSimultaneousFailures asserts that independently tripped checks (a dirty
// warp and a branch-diverged pair) are both collected into one Report rather than the first
// short-circuiting the rest.
func TestCheckResolved_MultipleSimultaneousFailures(t *testing.T) {
	t.Parallel()

	h, _ := setupFixture(t)

	untracked := filepath.Join(h.PrimeWorktree(), "untracked.txt")
	if err := os.WriteFile(untracked, []byte("new"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "checkout", "-b", "warp-only")

	report, err := preflight.CheckResolved(h.Location)
	if err != nil {
		t.Fatalf("CheckResolved: %v", err)
	}
	assertCheckSet(t, report, preflight.CheckWorktreeClean, preflight.CheckFabricSync)
}

// TestPredicates_HealthyPair asserts both predicates' positive path against the ordinary healthy
// pair the fixture builds.
// Wired is a newly exported predicate with no consumer in this task -- T7 and T8 are its first
// callers -- so without this row its true branch ships exercised only indirectly, through the
// fabricengine.Ready call inside CheckResolved.
func TestPredicates_HealthyPair(t *testing.T) {
	t.Parallel()

	h, _ := setupFixture(t)
	cwd := h.PrimeWorktree()

	loc, ok := preflight.Wired(cwd)
	if !ok || loc == nil {
		t.Errorf("Wired(%s) = (%v, %v); want (non-nil, true)", cwd, loc, ok)
	}

	loc, ok = preflight.HubPresent(cwd)
	if !ok || loc == nil {
		t.Errorf("HubPresent(%s) = (%v, %v); want (non-nil, true)", cwd, loc, ok)
	}
}

// TestPredicates_AtBoard pins why both predicates ship: with cwd at <hub>/_board, HubPresent returns
// true (the hub-level lyx directory exists there) but Wired returns false (fabricengine.Ready
// probes the paired sibling of the current worktree, not the hub, and _board has none).
func TestPredicates_AtBoard(t *testing.T) {
	t.Parallel()

	h, _ := setupFixture(t)
	board := h.BoardDir()

	if _, ok := preflight.Wired(board); ok {
		t.Errorf("Wired(%s) = true; want false", board)
	}

	loc, ok := preflight.HubPresent(board)
	if !ok || loc == nil {
		t.Errorf("HubPresent(%s) = (%v, %v); want (non-nil, true)", board, loc, ok)
	}
}

// TestResolveMode pins ResolveMode's full seven-row hub/standalone/refuse table. Rows three and
// five are the pair the design's r4 review exposed: both arrive as lyxcwd.ErrCwdOutsideAnchor from
// lyxcwd.Resolve and must diverge -- see each row's own comment below.
func TestResolveMode(t *testing.T) {
	t.Parallel()

	t.Run("NotAGitRepoAtAll", func(t *testing.T) {
		t.Parallel()

		cwd := t.TempDir()

		loc, mode, err := preflight.ResolveMode(cwd)
		if err != nil {
			t.Fatalf("ResolveMode(%s) error = %v; want nil", cwd, err)
		}
		if mode != preflight.ModeStandalone {
			t.Errorf("ResolveMode(%s) mode = %v; want ModeStandalone", cwd, mode)
		}
		if loc != nil {
			t.Errorf("ResolveMode(%s) Location = %+v; want nil", cwd, loc)
		}
	})

	t.Run("PlainRepoAtRoot", func(t *testing.T) {
		t.Parallel()

		cwd := t.TempDir()
		gitkit.MustRun(t, cwd, "git", "init", "-b", "main")

		loc, mode, err := preflight.ResolveMode(cwd)
		if err != nil {
			t.Fatalf("ResolveMode(%s) error = %v; want nil", cwd, err)
		}
		if mode != preflight.ModeStandalone {
			t.Errorf("ResolveMode(%s) mode = %v; want ModeStandalone", cwd, mode)
		}
		if loc != nil {
			t.Errorf("ResolveMode(%s) Location = %+v; want nil", cwd, loc)
		}
	})

	// A plain repo's subdirectory arrives as lyxcwd.ErrCwdOutsideAnchor from lyxcwd.Resolve,
	// exactly like a wired hub worktree's subdirectory (see the RefuseWiredWorktreeSubdirectory
	// row below) -- ResolveMode must tell the two apart by re-probing for hub geometry, not by
	// inspecting the error class, and this row pins the standalone side of that divergence.
	t.Run("PlainRepoSubdirectory", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		gitkit.MustRun(t, root, "git", "init", "-b", "main")
		sub := filepath.Join(root, "sub")
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}

		loc, mode, err := preflight.ResolveMode(sub)
		if err != nil {
			t.Fatalf("ResolveMode(%s) error = %v; want nil", sub, err)
		}
		if mode != preflight.ModeStandalone {
			t.Errorf("ResolveMode(%s) mode = %v; want ModeStandalone", sub, mode)
		}
		if loc != nil {
			t.Errorf("ResolveMode(%s) Location = %+v; want nil", sub, loc)
		}
	})

	t.Run("WiredHubWorktreeAtAnchor", func(t *testing.T) {
		t.Parallel()

		h, _ := setupFixture(t)
		cwd := h.PrimeWorktree()

		loc, mode, err := preflight.ResolveMode(cwd)
		if err != nil {
			t.Fatalf("ResolveMode(%s) error = %v; want nil", cwd, err)
		}
		if mode != preflight.ModeHub {
			t.Errorf("ResolveMode(%s) mode = %v; want ModeHub", cwd, mode)
		}
		if loc == nil {
			t.Errorf("ResolveMode(%s) Location = nil; want non-nil", cwd)
		}
	})

	// A wired hub worktree's subdirectory arrives as the exact same lyxcwd.ErrCwdOutsideAnchor
	// sentinel as a plain repo's subdirectory (see the PlainRepoSubdirectory row above) -- this is
	// the pair the design's r4 review exposed, and the whole reason ResolveMode re-probes for hub
	// geometry via lyxcwd.ResolveWorktree instead of trusting the error class alone.
	t.Run("RefuseWiredWorktreeSubdirectory", func(t *testing.T) {
		t.Parallel()

		h, _ := setupFixture(t)
		sub := filepath.Join(h.PrimeWorktree(), "sub")
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}

		loc, mode, err := preflight.ResolveMode(sub)
		if err == nil {
			t.Fatalf("ResolveMode(%s) error = nil; want non-nil", sub)
		}
		if !errors.Is(err, lyxcwd.ErrCwdOutsideAnchor) {
			t.Errorf("ResolveMode(%s) error = %v; want wrapped ErrCwdOutsideAnchor", sub, err)
		}
		// The gated error, not the second probe's, is what must reach the operator: assert its
		// message names the anchor marker file so a substitution of the second probe's error
		// would be caught here.
		if !strings.Contains(err.Error(), lyxcwd.AnchorFileName) {
			t.Errorf("ResolveMode(%s) error = %v; want message naming %s", sub, err, lyxcwd.AnchorFileName)
		}
		if mode != preflight.Mode(0) {
			t.Errorf("ResolveMode(%s) mode = %v; want the zero Mode value", sub, mode)
		}
		if loc != nil {
			t.Errorf("ResolveMode(%s) Location = %+v; want nil", sub, loc)
		}
	})

	t.Run("StaleAnchorMarker", func(t *testing.T) {
		t.Parallel()

		h, _ := setupFixture(t)
		cwd := h.PrimeWorktree()

		anchorPath := filepath.Join(h.BoardDir(), lyxcwd.AnchorFileName)
		if err := os.Remove(anchorPath); err != nil {
			t.Fatalf("remove %s: %v", anchorPath, err)
		}
		stalePath := filepath.Join(h.BoardDir(), lyxcwd.StaleAnchorFileName)
		if err := os.WriteFile(stalePath, []byte("."), 0o644); err != nil {
			t.Fatalf("write %s: %v", stalePath, err)
		}

		loc, mode, err := preflight.ResolveMode(cwd)
		if err == nil {
			t.Fatalf("ResolveMode(%s) error = nil; want non-nil", cwd)
		}
		if !errors.Is(err, lyxcwd.ErrStaleAnchorMarker) {
			t.Errorf("ResolveMode(%s) error = %v; want wrapped ErrStaleAnchorMarker", cwd, err)
		}
		if mode != preflight.Mode(0) {
			t.Errorf("ResolveMode(%s) mode = %v; want the zero Mode value", cwd, mode)
		}
		if loc != nil {
			t.Errorf("ResolveMode(%s) Location = %+v; want nil", cwd, loc)
		}
	})

	// A hub whose <hub>/_board/_lyx is missing is the design's recorded residual, not a bug: at
	// that point nothing on disk distinguishes it from a plain git repo, so it degrades to
	// standalone rather than refusing.
	t.Run("HubMissingBoardLyx", func(t *testing.T) {
		t.Parallel()

		h, _ := setupFixture(t)
		cwd := h.PrimeWorktree()

		boardLyx := filepath.Join(h.BoardDir(), lyxdirs.LyxDirName)
		if err := os.RemoveAll(boardLyx); err != nil {
			t.Fatalf("remove %s: %v", boardLyx, err)
		}

		loc, mode, err := preflight.ResolveMode(cwd)
		if err != nil {
			t.Fatalf("ResolveMode(%s) error = %v; want nil", cwd, err)
		}
		if mode != preflight.ModeStandalone {
			t.Errorf("ResolveMode(%s) mode = %v; want ModeStandalone", cwd, mode)
		}
		if loc != nil {
			t.Errorf("ResolveMode(%s) Location = %+v; want nil", cwd, loc)
		}
	})
}
