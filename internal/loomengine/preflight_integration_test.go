//go:build integration

// preflight_integration_test.go drives Preflight/checkResolved end-to-end
// against real git fixtures — a paired host+weft worktree with a wired _lyx
// junction and a seeded status.json — covering every pass/fail scenario
// across all four preconditions. It is integration-tagged because it spawns
// git via lyxtest fixtures (Test Tier Purity Invariant).

package loomengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/state"
)

// setupPreflightFixture builds a CopyPaired fixture, moves the weft primary
// onto fabric's suffixed branch naming (CopyPaired's raw fixture leaves both
// sides on "main", the warp-era equality convention; fabric's PairInSync
// requires the weft branch to be WeftBranchName(hostBranch) — see
// newFabricFixture's identical fixup in fabricengine's own regression tests),
// wires the host-weft _lyx junction (CopyPaired does not wire it — see
// WireJunctions' host-pristine invariant), and seeds a fresh, coherent
// status.json through the wired junction. Returns the fixture and the slug
// WireJunctions was keyed on, since several scenarios (junction removal)
// need the slug to rebuild the host link path.
func setupPreflightFixture(t *testing.T) (lyxtest.PairedFixture, string) {
	t.Helper()

	f := lyxtest.CopyPaired(t)
	slug := filepath.Base(f.Layout.WorktreeRoot)

	lyxtest.MustRun(t, f.WeftPrime, "git", "checkout", "-b", fabricengine.WeftBranchName("main"))

	if err := fabricengine.WireJunctions(f.Layout, slug); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	seedValidStatus(t, f.Layout)

	return f, slug
}

// seedValidStatus writes a fresh, coherent status.json seed at
// l.LoomStatusFile() via state.WriteJSON — the seed shape status-schema.md's
// "The seed / handover" section documents: only the handoff fields
// populated, every fresh-start field at its zero/null value.
func seedValidStatus(t *testing.T, l *hubgeometry.Layout) {
	t.Helper()

	s := Status{
		Slug:      "loom-preflight-fixture",
		Parent:    "main",
		Phase:     "preflight",
		Stage:     "produce",
		Narration: "now: awaiting preflight / last: — / wait: —",
	}
	if err := state.WriteJSON(l.LoomStatusFile(), l.LoomStatusLock(), s); err != nil {
		t.Fatalf("seed status.json: %v", err)
	}
}

// restoreCwd saves the process cwd and restores it via t.Cleanup. It exists
// for the two scenarios (not-a-git-repo, subdirectory invocation) that must
// exercise the public Preflight() — which resolves the process cwd via
// hubgeometry.Getwd() — rather than checkResolved's injected-Layout form.
// Because os.Chdir is process-global, callers of restoreCwd must never run
// under t.Parallel().
//
// Callers MUST invoke restoreCwd AFTER creating any t.TempDir()/fixture the
// test will os.Chdir into, never before: t.Cleanup runs LIFO, and on Windows
// a directory cannot be removed while it is the process's current working
// directory. Registering restoreCwd's chdir-back last is what makes it run
// before that TempDir's own removal, so cleanup lands back outside the
// directory before Go tries to delete it.
func restoreCwd(t *testing.T) {
	t.Helper()

	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Fatalf("restore cwd to %s: %v", orig, err)
		}
	})
}

// assertCheckSet asserts that got's Failures carry exactly the given set of
// CheckIDs (order-independent, no duplicates expected) — per the batch's
// "assert only on the CheckID set, not exact Reason strings" instruction. An
// empty want asserts Report.OK instead.
func assertCheckSet(t *testing.T, got Report, want ...CheckID) {
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

	wantSet := make(map[CheckID]bool, len(want))
	for _, c := range want {
		wantSet[c] = true
	}
	gotSet := make(map[CheckID]bool, len(got.Failures))
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

// TestPreflight_HealthyPairAndSeed is the anchor case: a fully healthy paired
// host+weft worktree with a valid fresh seed reports OK. Since CopyPaired's
// host hub is a single-worktree repo, its Layout.Prime already equals
// Layout.WorktreeRoot — this test doubles as the "Prime worktree with a
// healthy pair+seed" scenario (run-in-existing-or-prime-worktree).
func TestPreflight_HealthyPairAndSeed(t *testing.T) {
	t.Parallel()

	f, _ := setupPreflightFixture(t)

	report, err := checkResolved(f.Layout)
	if err != nil {
		t.Fatalf("checkResolved: %v", err)
	}
	assertCheckSet(t, report)
}

// TestPreflight_NotAGitRepo asserts that Preflight() invoked outside any git
// repository reports a single geometry failure with no error. This exercises
// the public Preflight() (not checkResolved) because it needs
// hubgeometry.Getwd() to observe a non-repo cwd.
func TestPreflight_NotAGitRepo(t *testing.T) {
	// t.TempDir() must be created before restoreCwd registers its cleanup —
	// see restoreCwd's doc comment: on Windows, cleanup must chdir back out of
	// dir before Go tries to remove it, and t.Cleanup runs LIFO.
	dir := t.TempDir()
	restoreCwd(t)

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%s): %v", dir, err)
	}

	report, err := Preflight()
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	assertCheckSet(t, report, CheckGeometry)
}

// TestPreflight_SubdirectoryInvocation asserts that Preflight() invoked from
// a subdirectory of the worktree (RelPath != ".") short-circuits with a
// single worktree-root failure. Exercises the public Preflight() for the
// same reason as TestPreflight_NotAGitRepo.
func TestPreflight_SubdirectoryInvocation(t *testing.T) {
	// setupPreflightFixture's t.TempDir()-backed fixture must be created before
	// restoreCwd registers its cleanup — see restoreCwd's doc comment: on
	// Windows, cleanup must chdir back out of the fixture before Go tries to
	// remove it, and t.Cleanup runs LIFO.
	f, _ := setupPreflightFixture(t)
	restoreCwd(t)

	sub := filepath.Join(f.Hub, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", sub, err)
	}
	if err := os.Chdir(sub); err != nil {
		t.Fatalf("Chdir(%s): %v", sub, err)
	}

	report, err := Preflight()
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	assertCheckSet(t, report, CheckWorktreeRoot)
}

// TestPreflight_EmptyPrime asserts that an injected Layout with no resolved
// Prime (main worktree) reports a geometry failure, short-circuiting before
// any of checks 2-4 run.
func TestPreflight_EmptyPrime(t *testing.T) {
	t.Parallel()

	f, _ := setupPreflightFixture(t)

	l := *f.Layout
	l.Prime = ""

	report, err := checkResolved(&l)
	if err != nil {
		t.Fatalf("checkResolved: %v", err)
	}
	assertCheckSet(t, report, CheckGeometry)
}

// TestPreflight_HostDirty covers all three ways HostClean can observe a dirty
// host worktree: a tracked-and-modified file, a staged file, and an
// untracked-only file.
func TestPreflight_HostDirty(t *testing.T) {
	tests := []struct {
		name  string
		dirty func(t *testing.T, f lyxtest.PairedFixture)
	}{
		{
			name: "TrackedModified",
			dirty: func(t *testing.T, f lyxtest.PairedFixture) {
				readme := filepath.Join(f.Hub, "README")
				if err := os.WriteFile(readme, []byte("modified"), 0o644); err != nil {
					t.Fatalf("modify README: %v", err)
				}
			},
		},
		{
			name: "Staged",
			dirty: func(t *testing.T, f lyxtest.PairedFixture) {
				readme := filepath.Join(f.Hub, "README")
				if err := os.WriteFile(readme, []byte("staged"), 0o644); err != nil {
					t.Fatalf("modify README: %v", err)
				}
				lyxtest.MustRun(t, f.Hub, "git", "add", "README")
			},
		},
		{
			name: "UntrackedOnly",
			dirty: func(t *testing.T, f lyxtest.PairedFixture) {
				untracked := filepath.Join(f.Hub, "untracked.txt")
				if err := os.WriteFile(untracked, []byte("new"), 0o644); err != nil {
					t.Fatalf("write untracked file: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, _ := setupPreflightFixture(t)
			tt.dirty(t, f)

			report, err := checkResolved(f.Layout)
			if err != nil {
				t.Fatalf("checkResolved: %v", err)
			}
			assertCheckSet(t, report, CheckWorktreeClean)
		})
	}
}

// TestPreflight_WeftWorktreeRemoved asserts that a removed weft worktree
// reports weft-pairing, and that the now-dangling host junction makes the
// seed stat fail too — classified seed-unreadable (never seed-missing)
// because check 3 already failed.
func TestPreflight_WeftWorktreeRemoved(t *testing.T) {
	t.Parallel()

	f, _ := setupPreflightFixture(t)

	if err := os.RemoveAll(f.Layout.WeftWorktree()); err != nil {
		t.Fatalf("remove weft worktree: %v", err)
	}

	report, err := checkResolved(f.Layout)
	if err != nil {
		t.Fatalf("checkResolved: %v", err)
	}
	assertCheckSet(t, report, CheckWeftPairing, CheckSeedUnreadable)
}

// TestPreflight_HostWeftDifferentBranches asserts that host and weft
// worktrees on different branches report weft-sync, and that weft-sync alone
// does NOT block the seed check (the junction and weft directory are both
// still healthy).
func TestPreflight_HostWeftDifferentBranches(t *testing.T) {
	t.Parallel()

	f, _ := setupPreflightFixture(t)

	lyxtest.MustRun(t, f.Hub, "git", "checkout", "-b", "host-only")

	report, err := checkResolved(f.Layout)
	if err != nil {
		t.Fatalf("checkResolved: %v", err)
	}
	assertCheckSet(t, report, CheckWeftSync)
}

// TestPreflight_JunctionBroken asserts that all three of PairInSync's
// junction-drift shapes — missing, not-a-link, and points-elsewhere —
// classify as junction (card 12's substring-match fix: a prefix match only
// ever caught the missing shape), and that the seed stat, which now resolves
// through a broken _lyx one way or another, is classified seed-unreadable
// (never seed-missing) because check 3 already failed in every shape.
func TestPreflight_JunctionBroken(t *testing.T) {
	tests := []struct {
		name    string
		corrupt func(t *testing.T, hostLink string)
	}{
		{
			name: "Missing",
			corrupt: func(t *testing.T, hostLink string) {
				if err := fslink.Remove(hostLink); err != nil {
					t.Fatalf("remove host junction %s: %v", hostLink, err)
				}
			},
		},
		{
			name: "NotALink",
			corrupt: func(t *testing.T, hostLink string) {
				if err := fslink.Remove(hostLink); err != nil {
					t.Fatalf("remove host junction %s: %v", hostLink, err)
				}
				if err := os.Mkdir(hostLink, 0o755); err != nil {
					t.Fatalf("mkdir real dir in junction's place %s: %v", hostLink, err)
				}
			},
		},
		{
			name: "PointsElsewhere",
			corrupt: func(t *testing.T, hostLink string) {
				if err := fslink.Remove(hostLink); err != nil {
					t.Fatalf("remove host junction %s: %v", hostLink, err)
				}
				wrongTarget := filepath.Join(filepath.Dir(hostLink), "not-the-weft-lyx-dir")
				if err := os.MkdirAll(wrongTarget, 0o755); err != nil {
					t.Fatalf("mkdir wrong target %s: %v", wrongTarget, err)
				}
				if err := fslink.CreateDirLink(hostLink, wrongTarget); err != nil {
					t.Fatalf("CreateDirLink(%s, %s): %v", hostLink, wrongTarget, err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, slug := setupPreflightFixture(t)
			hostLink := f.Layout.HostLyxLink(slug)
			tt.corrupt(t, hostLink)

			report, err := checkResolved(f.Layout)
			if err != nil {
				t.Fatalf("checkResolved: %v", err)
			}
			assertCheckSet(t, report, CheckJunction, CheckSeedUnreadable)
		})
	}
}

// TestPreflight_SeedMissing asserts that a genuinely absent seed — junction
// and weft pairing both healthy — reports seed-missing, not seed-unreadable.
func TestPreflight_SeedMissing(t *testing.T) {
	t.Parallel()

	f, _ := setupPreflightFixture(t)

	if err := os.Remove(f.Layout.LoomStatusFile()); err != nil {
		t.Fatalf("remove seed: %v", err)
	}

	report, err := checkResolved(f.Layout)
	if err != nil {
		t.Fatalf("checkResolved: %v", err)
	}
	assertCheckSet(t, report, CheckSeedMissing)
}

// TestPreflight_SeedUnknownField asserts that a seed containing an unknown
// field fails strict decode and reports seed-incoherent.
func TestPreflight_SeedUnknownField(t *testing.T) {
	t.Parallel()

	f, _ := setupPreflightFixture(t)

	const raw = `{
  "slug": "loom-preflight-fixture",
  "parent": "main",
  "phase": "preflight",
  "stage": "produce",
  "narration": "now: awaiting preflight",
  "history": [],
  "start_sha": null,
  "pause_requested": false,
  "next_action": null,
  "unknown_field": true
}`
	if err := os.WriteFile(f.Layout.LoomStatusFile(), []byte(raw), 0o644); err != nil {
		t.Fatalf("write malformed seed: %v", err)
	}

	report, err := checkResolved(f.Layout)
	if err != nil {
		t.Fatalf("checkResolved: %v", err)
	}
	assertCheckSet(t, report, CheckSeedIncoherent)
}

// TestPreflight_SeedHalfFinished asserts that a coherent-but-advanced seed
// (non-empty history, or a stamped start_sha) reports half-finished — the
// task has already run past the point Preflight is meant to gate.
func TestPreflight_SeedHalfFinished(t *testing.T) {
	tests := []struct {
		name string
		seed func() Status
	}{
		{
			name: "NonEmptyHistory",
			seed: func() Status {
				return Status{
					Slug: "loom-preflight-fixture", Parent: "main", Phase: "builder", Stage: "gate",
					Narration: "now: mid-run",
					History: []HistoryEntry{
						{Phase: "discussion", Outcome: "approved", Ts: "2026-07-17T10:01:30Z"},
					},
				}
			},
		},
		{
			name: "SetStartSha",
			seed: func() Status {
				sha := "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4"
				return Status{
					Slug: "loom-preflight-fixture", Parent: "main", Phase: "builder", Stage: "produce",
					Narration: "now: mid-run", StartSha: &sha,
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, _ := setupPreflightFixture(t)
			if err := state.WriteJSON(f.Layout.LoomStatusFile(), f.Layout.LoomStatusLock(), tt.seed()); err != nil {
				t.Fatalf("overwrite seed: %v", err)
			}

			report, err := checkResolved(f.Layout)
			if err != nil {
				t.Fatalf("checkResolved: %v", err)
			}
			assertCheckSet(t, report, CheckHalfFinished)
		})
	}
}

// TestPreflight_MultipleSimultaneousFailures asserts that independently
// tripped checks (a dirty host and a branch-diverged weft) are both
// collected into one Report rather than the first short-circuiting the rest.
func TestPreflight_MultipleSimultaneousFailures(t *testing.T) {
	t.Parallel()

	f, _ := setupPreflightFixture(t)

	untracked := filepath.Join(f.Hub, "untracked.txt")
	if err := os.WriteFile(untracked, []byte("new"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}
	lyxtest.MustRun(t, f.Hub, "git", "checkout", "-b", "host-only")

	report, err := checkResolved(f.Layout)
	if err != nil {
		t.Fatalf("checkResolved: %v", err)
	}
	assertCheckSet(t, report, CheckWorktreeClean, CheckWeftSync)
}
