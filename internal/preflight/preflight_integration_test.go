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
	"os"
	"path/filepath"
	"testing"

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
	gitkit.MustRun(t, h.PrimeWeft(), "git", "add", "-A")
	gitkit.MustRun(t, h.PrimeWeft(), "git", "commit", "-m", "seed junctions")

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

// TestCheckResolved_Dirty covers both sides of worktree pair cleanliness: a dirty warp side and a
// dirty paired side, each reporting CheckWorktreeClean.
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

// TestCheckResolved_BrokenJunction asserts that a broken junction classifies as CheckJunction.
func TestCheckResolved_BrokenJunction(t *testing.T) {
	t.Parallel()

	h, _ := setupFixture(t)

	link := fabricengine.WarpLyxLinkHere(h.Location)
	if err := fslink.Remove(link); err != nil {
		t.Fatalf("remove junction %s: %v", link, err)
	}

	report, err := preflight.CheckResolved(h.Location)
	if err != nil {
		t.Fatalf("CheckResolved: %v", err)
	}
	assertCheckSet(t, report, preflight.CheckJunction)
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
