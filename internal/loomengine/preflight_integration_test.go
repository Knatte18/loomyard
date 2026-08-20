//go:build integration

// preflight_integration_test.go drives Preflight/checkResolved end-to-end
// against real git fixtures — a paired warp+fabric worktree with a wired _lyx
// junction and a seeded status.json — covering every pass/fail scenario
// across all four preconditions. It is integration-tagged because it spawns
// git via hubforge fixtures (Test Tier Purity Invariant).
//
// It is a package loomengine_test file, not an in-package test, because
// internal/loomengine sits inside internal/fabriccli's dependency set: an
// in-package test importing internal/hubforge (which imports fabriccli)
// would close a compile cycle. loomengine/export_test.go re-exports the
// unexported checkResolved seam this file drives directly.

package loomengine_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// setupPreflightFixture builds a fully-configured real hub with fabric and junction setup,
// returning the hub and the slug for WireJunctions.
func setupPreflightFixture(t *testing.T) (*hubforge.Hub, string) {
	t.Helper()

	h := hubforge.NewHub(t, ".")
	slug := filepath.Base(h.Location.WorktreePath())

	// fabricengine.ConfigTemplate() is fabric's own plain registered config: fabriccli.CloneAndWire
	// already reconciled default config for every registered module when NewHub built h, so seeding
	// it again here would be a no-op duplicate (outcome 1 of the SeedConfig triage).
	//
	// The repo-wide fabric.yaml, by contrast, is a genuine override — it names pathspec: _extra
	// rather than the template's own default — so it is retargeted onto hubforge.SeedFabricConfig,
	// not dropped.
	hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: _extra\n")

	if err := fabricengine.WireJunctions(h.Location, slug, []string{"_lyx", lyxdirs.DotLyxDirName, "_extra"}); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	// LoomStatusLock lives under the warp worktree's own .lyx tree, wired above
	// as a real junction (dotlyx-junction-wiring-and-unwire) -- WireJunctions'
	// own seedGitExclude call already keeps the .lyx junction entry itself out
	// of `git status`, so no test-local exclude is needed here.

	seedValidStatus(t, h.Location)

	// The seeded status.json (and its .lock sidecar) materialize through the
	// _lyx junction into the fabric worktree's own git repo, where they start
	// out untracked. Commit them so a freshly-built fixture is genuinely
	// clean on both sides — required now that Clean checks the fabric worktree
	// too, not just the warp.
	gitkit.MustRun(t, h.PrimeWeft(), "git", "add", "-A")
	gitkit.MustRun(t, h.PrimeWeft(), "git", "commit", "-m", "seed status")

	return h, slug
}

// seedValidStatus writes a fresh, coherent status.json seed onto shedengine.Status's shape,
// current_producer at "Preflight", with loom's own handoff fields carried in Product.
func seedValidStatus(t *testing.T, l *lyxcwd.Location) {
	t.Helper()

	writeSeed(t, l, shedengine.Status{
		CurrentProducer: "Preflight",
		State:           shedengine.StateRunning,
	}, loomengine.Status{
		Slug:   "loom-preflight-fixture",
		Parent: "main",
	})
}

// writeSeed marshals product into shed.Product and writes the composed shedengine.Status to disk,
// mirroring Preflight's own MkdirAll fix in preflight.go for LoomStatusLock's parent -- .lyx is a
// sibling tree WireJunctions never creates, and state.WriteJSON only MkdirAlls status.json's own
// parent, never the lock's.
func writeSeed(t *testing.T, l *lyxcwd.Location, shed shedengine.Status, product loomengine.Status) {
	t.Helper()

	raw, err := json.Marshal(product)
	if err != nil {
		t.Fatalf("marshal product: %v", err)
	}
	shed.Product = raw

	if err := os.MkdirAll(filepath.Dir(loomengine.LoomStatusLock(l)), 0o755); err != nil {
		t.Fatalf("mkdir status lock parent: %v", err)
	}
	if err := state.WriteJSON(loomengine.LoomStatusFile(l), loomengine.LoomStatusLock(l), shed); err != nil {
		t.Fatalf("seed status.json: %v", err)
	}
}

// commitFabricStatus commits the current state of status.json in the fabric worktree,
// isolating test scenarios from CheckWorktreeClean failures.
func commitFabricStatus(t *testing.T, h *hubforge.Hub) {
	t.Helper()

	gitkit.MustRun(t, h.PrimeWeft(), "git", "add", "-A")
	gitkit.MustRun(t, h.PrimeWeft(), "git", "commit", "-m", "update status")
}

// assertCheckSet asserts that got's Failures carry exactly the given CheckID set
// (order-independent). An empty want asserts Report.OK.
func assertCheckSet(t *testing.T, got loomengine.Report, want ...loomengine.CheckID) {
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

	wantSet := make(map[loomengine.CheckID]bool, len(want))
	for _, c := range want {
		wantSet[c] = true
	}
	gotSet := make(map[loomengine.CheckID]bool, len(got.Failures))
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

// TestPreflight_HealthyPairAndSeed is the anchor case: a fully healthy paired warp+fabric worktree
// with a valid fresh seed reports OK.
// Since NewHub's warp hub is a single-worktree repo, its Location.WorktreePath already equals
// the Prime worktree — this test doubles as the "Prime worktree with a healthy pair+seed" scenario
// (run-in-existing-or-prime-worktree).
func TestPreflight_HealthyPairAndSeed(t *testing.T) {
	t.Parallel()

	h, _ := setupPreflightFixture(t)

	report, err := loomengine.CheckResolvedForTest(h.Location)
	if err != nil {
		t.Fatalf("checkResolved: %v", err)
	}
	assertCheckSet(t, report)
}

// TestPreflight_NotAGitRepo asserts that Preflight() invoked outside any git repository reports a
// single geometry failure with no error.
// This exercises the public Preflight() (not checkResolved) because it needs lyxcwd.Getwd() to
// observe a non-repo cwd.
func TestPreflight_NotAGitRepo(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	report, err := loomengine.Preflight(dir)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	assertCheckSet(t, report, loomengine.CheckGeometry)
}

// TestPreflight_SubpathAnchoredHubIsNotRejectedForItsAnchor asserts that a legitimately
// subpath-anchored repo is validated on its merits rather than short-circuited.
// Preflight used to reject any AnchorRel != "." as "invoked from a subdirectory", which the strict
// cwd gate had already made impossible: a successful Resolve proves cwd equals AnchorPath(), so a
// non-"." anchor describes the repo's geometry, not where the caller stood.
// Exercises the public Preflight() for the same reason as TestPreflight_NotAGitRepo.
func TestPreflight_SubpathAnchoredHubIsNotRejectedForItsAnchor(t *testing.T) {
	t.Parallel()

	h, _ := setupPreflightFixture(t)

	sub := filepath.Join(h.PrimeWorktree(), "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", sub, err)
	}

	// Record "sub" as the recognized anchor so Resolve(sub) succeeds under the strict cwd gate
	// with AnchorRel == "sub" -- exactly the shape `lyx fabric clone --subpath sub` produces.
	anchorPath := filepath.Join(fabricengine.BoardDir(h.Location.HubPath), lyxcwd.AnchorFileName)
	if err := os.WriteFile(anchorPath, []byte("sub"), 0o644); err != nil {
		t.Fatalf("write %s: %v", anchorPath, err)
	}

	report, err := loomengine.Preflight(sub)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	for _, failure := range report.Failures {
		if failure.Check == loomengine.CheckGeometry {
			t.Errorf("Preflight() on a subpath-anchored hub reported %q: %s; want the anchor treated as legal geometry",
				failure.Check, failure.Reason)
		}
	}
}

// TestPreflight_WarpDirty covers all three ways Clean can observe a dirty repo (a
// tracked-and-modified file, a staged file, and an untracked-only file), plus the genuinely-new
// fabric-dirty-only and both-dirty shapes now that Clean also checks the fabric side.
func TestPreflight_WarpDirty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		dirty func(t *testing.T, h *hubforge.Hub)
	}{
		{
			name: "TrackedModified",
			dirty: func(t *testing.T, h *hubforge.Hub) {
				readme := filepath.Join(h.PrimeWorktree(), "README")
				if err := os.WriteFile(readme, []byte("modified"), 0o644); err != nil {
					t.Fatalf("modify README: %v", err)
				}
			},
		},
		{
			name: "Staged",
			dirty: func(t *testing.T, h *hubforge.Hub) {
				readme := filepath.Join(h.PrimeWorktree(), "README")
				if err := os.WriteFile(readme, []byte("staged"), 0o644); err != nil {
					t.Fatalf("modify README: %v", err)
				}
				gitkit.MustRun(t, h.PrimeWorktree(), "git", "add", "README")
			},
		},
		{
			name: "UntrackedOnly",
			dirty: func(t *testing.T, h *hubforge.Hub) {
				untracked := filepath.Join(h.PrimeWorktree(), "untracked.txt")
				if err := os.WriteFile(untracked, []byte("new"), 0o644); err != nil {
					t.Fatalf("write untracked file: %v", err)
				}
			},
		},
		{
			name: "DirtyFabricOnly",
			dirty: func(t *testing.T, h *hubforge.Hub) {
				untracked := filepath.Join(h.PrimeWeft(), "untracked.txt")
				if err := os.WriteFile(untracked, []byte("new"), 0o644); err != nil {
					t.Fatalf("write untracked fabric file: %v", err)
				}
			},
		},
		{
			name: "BothDirty",
			dirty: func(t *testing.T, h *hubforge.Hub) {
				warpUntracked := filepath.Join(h.PrimeWorktree(), "untracked.txt")
				if err := os.WriteFile(warpUntracked, []byte("new"), 0o644); err != nil {
					t.Fatalf("write untracked warp file: %v", err)
				}
				fabricUntracked := filepath.Join(h.PrimeWeft(), "untracked.txt")
				if err := os.WriteFile(fabricUntracked, []byte("new"), 0o644); err != nil {
					t.Fatalf("write untracked fabric file: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := setupPreflightFixture(t)
			tt.dirty(t, h)

			report, err := loomengine.CheckResolvedForTest(h.Location)
			if err != nil {
				t.Fatalf("checkResolved: %v", err)
			}
			assertCheckSet(t, report, loomengine.CheckWorktreeClean)
		})
	}
}

// TestPreflight_FabricNotReady asserts that a removed fabric worktree reports fabric-ready,
// and that the now-dangling junction makes the seed stat fail too — classified seed-unreadable
// (never seed-missing) because check 3 already failed.
func TestPreflight_FabricNotReady(t *testing.T) {
	t.Parallel()

	h, _ := setupPreflightFixture(t)

	// Drive the not-present branch via the hub's own PrimeWeft() rather than
	// fabricengine.WeftWorktree(h.Location): check 3 now goes through
	// fabricengine.Ready(l), and PrimeWeft() is the independent source of the
	// same path.
	if err := os.RemoveAll(h.PrimeWeft()); err != nil {
		t.Fatalf("remove fabric worktree: %v", err)
	}

	report, err := loomengine.CheckResolvedForTest(h.Location)
	if err != nil {
		t.Fatalf("checkResolved: %v", err)
	}
	assertCheckSet(t, report, loomengine.CheckFabricReady, loomengine.CheckSeedUnreadable)
}

// TestPreflight_WarpFabricDifferentBranches asserts that warp and fabric worktrees on different
// branches report fabric-sync — the CauseBranchMismatch/CheckFabricSync equivalence pinned by
// healthy-typed-reason,
// and that fabric-sync alone does NOT block the seed check (the junction and fabric directory are both
// still healthy).
func TestPreflight_WarpFabricDifferentBranches(t *testing.T) {
	t.Parallel()

	h, _ := setupPreflightFixture(t)

	gitkit.MustRun(t, h.PrimeWorktree(), "git", "checkout", "-b", "warp-only")

	report, err := loomengine.CheckResolvedForTest(h.Location)
	if err != nil {
		t.Fatalf("checkResolved: %v", err)
	}
	assertCheckSet(t, report, loomengine.CheckFabricSync)
}

// TestPreflight_ConfigLoadFailed asserts the CauseConfigLoadFailed/CheckJunction equivalence pinned
// by healthy-typed-reason: a repo-wide fabric.yaml that fails to load classifies as CheckJunction
// (not a distinct CheckID of its own), same as the three junction-drift shapes TestPreflight_JunctionBroken
// covers.
// Unlike those shapes, the _lyx junction itself is still physically intact here — only the config
// read fails — so the seed check is unaffected: no CheckSeedUnreadable/CheckSeedMissing is added.
func TestPreflight_ConfigLoadFailed(t *testing.T) {
	t.Parallel()

	h, _ := setupPreflightFixture(t)

	configPath := configengine.ConfigFile(fabricengine.BoardDir(h.Location.HubPath), "fabric")
	if err := os.WriteFile(configPath, []byte("not: [valid: yaml"), 0o644); err != nil {
		t.Fatalf("corrupt repo-wide fabric config: %v", err)
	}

	report, err := loomengine.CheckResolvedForTest(h.Location)
	if err != nil {
		t.Fatalf("checkResolved: %v", err)
	}
	assertCheckSet(t, report, loomengine.CheckJunction)
}

// TestPreflight_JunctionBroken asserts that all three of Healthy's junction-drift shapes — missing,
// not-a-link, and points-elsewhere — classify as junction, via Healthy's typed Cause rather than a
// substring match.
// Each drift shape is exercised against BOTH junctions (_lyx and a second, non-_lyx junction, from
// card 15 onward) so the classification is proven to hold for the second, non-_lyx junction too —
// not just the one Healthy's underlying loop was originally written and tested against.
//
// The seed-check expectation differs by junction,
// and deliberately so: status.json lives under _lyx (LoomStatusFile(l) is _lyx-anchored), so a
// broken _lyx junction also makes the seed stat fail — classified seed-unreadable (never
// seed-missing) because check 3 already failed.
// A broken second junction, by contrast, leaves the seed fully readable through the still-healthy
// _lyx junction: check 3 still fails and still classifies as CheckJunction (never CheckFabricSync),
// but no seed failure is added at all, since check 4's stat of LoomStatusFile(l) succeeds either
// way.
// This asymmetry is exactly what "check3BlocksSeed" is named for: it only changes check 4's
// classification of a stat failure that already happened, it does not itself cause one.
func TestPreflight_JunctionBroken(t *testing.T) {
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
		name       string
		linkFor    func(h *hubforge.Hub, slug string) string
		wantChecks []loomengine.CheckID // in addition to CheckJunction, which every case wants
	}{
		{
			name:       "Lyx",
			linkFor:    func(h *hubforge.Hub, slug string) string { return fabricengine.WarpLyxLink(h.Location, slug) },
			wantChecks: []loomengine.CheckID{loomengine.CheckSeedUnreadable},
		},
		{
			name: "Extra",
			linkFor: func(h *hubforge.Hub, slug string) string {
				return filepath.Join(fabricengine.WorktreePath(h.Location, slug), h.Location.AnchorRel, "_extra")
			},
			wantChecks: nil,
		},
	}

	for _, j := range junctions {
		for _, tt := range shapes {
			t.Run(j.name+"_"+tt.name, func(t *testing.T) {
				t.Parallel()

				h, slug := setupPreflightFixture(t)
				warpLink := j.linkFor(h, slug)
				tt.corrupt(t, warpLink)

				report, err := loomengine.CheckResolvedForTest(h.Location)
				if err != nil {
					t.Fatalf("checkResolved: %v", err)
				}
				want := append([]loomengine.CheckID{loomengine.CheckJunction}, j.wantChecks...)
				assertCheckSet(t, report, want...)
			})
		}
	}
}

// TestPreflight_MissingOptionalJunctionIsAJunctionFault covers a worktree whose optional junction was
// never wired at all: _lyx is fully healthy,
// but the second, non-_lyx junction is entirely absent (simulated here by removing it from an
// otherwise-healthy fixture, rather than corrupting it — the fixture never had it, full stop).
// Preflight must classify this as CheckJunction, never CheckFabricSync, and blocks the run (report.OK
// == false) — but does NOT also fail the seed check, since status.json lives under the
// still-healthy _lyx junction (see TestPreflight_JunctionBroken's doc comment for the same
// asymmetry).
// A single Reconcile repairs it (adds the missing junction and materialises its fabric-side target)
// rather than reporting already-healthy;
// and a fresh Preflight afterward reports OK — the "one lyx init or one lyx fabric reconcile"
// remedy this batch documents.
func TestPreflight_MissingOptionalJunctionIsAJunctionFault(t *testing.T) {
	t.Parallel()

	h, slug := setupPreflightFixture(t)

	// Simulate the missing-optional-junction state: this worktree's second,
	// non-_lyx junction was never wired, even though _lyx is fully healthy.
	extraLink := filepath.Join(fabricengine.WorktreePath(h.Location, slug), h.Location.AnchorRel, "_extra")
	if err := fslink.Remove(extraLink); err != nil {
		t.Fatalf("remove the optional junction to simulate a worktree missing it: %v", err)
	}

	report, err := loomengine.CheckResolvedForTest(h.Location)
	if err != nil {
		t.Fatalf("checkResolved: %v", err)
	}
	assertCheckSet(t, report, loomengine.CheckJunction)

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

	// A fresh Preflight now reports OK: the remedy this batch documents.
	report, err = loomengine.CheckResolvedForTest(h.Location)
	if err != nil {
		t.Fatalf("checkResolved after Reconcile: %v", err)
	}
	assertCheckSet(t, report)
}

// TestPreflight_SeedMissing asserts that a genuinely absent seed — junction and fabric pairing both
// healthy — reports seed-missing, not seed-unreadable.
func TestPreflight_SeedMissing(t *testing.T) {
	t.Parallel()

	h, _ := setupPreflightFixture(t)

	if err := os.Remove(loomengine.LoomStatusFile(h.Location)); err != nil {
		t.Fatalf("remove seed: %v", err)
	}
	commitFabricStatus(t, h)

	report, err := loomengine.CheckResolvedForTest(h.Location)
	if err != nil {
		t.Fatalf("checkResolved: %v", err)
	}
	assertCheckSet(t, report, loomengine.CheckSeedMissing)
}

// TestPreflight_SeedUnknownField asserts that a seed containing an unknown field fails strict
// decode and reports seed-incoherent.
func TestPreflight_SeedUnknownField(t *testing.T) {
	t.Parallel()

	h, _ := setupPreflightFixture(t)

	const raw = `{
  "current_producer": "Preflight",
  "state": "running",
  "error": "",
  "pause_requested": false,
  "activity": {"now": "", "last": "", "wait": ""},
  "history": [],
  "product": {"slug": "loom-preflight-fixture", "parent": "main", "start_sha": null},
  "unknown_field": true
}`
	if err := os.WriteFile(loomengine.LoomStatusFile(h.Location), []byte(raw), 0o644); err != nil {
		t.Fatalf("write malformed seed: %v", err)
	}
	commitFabricStatus(t, h)

	report, err := loomengine.CheckResolvedForTest(h.Location)
	if err != nil {
		t.Fatalf("checkResolved: %v", err)
	}
	assertCheckSet(t, report, loomengine.CheckSeedIncoherent)
}

// TestPreflight_SeedHalfFinished asserts that a coherent-but-advanced seed (a history entry naming
// a producer other than "Preflight", or a stamped start_sha) reports half-finished — the task has
// already run past the point Preflight is meant to gate.
func TestPreflight_SeedHalfFinished(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		shed func() shedengine.Status
		prod func() loomengine.Status
	}{
		{
			name: "HistoryNamingLaterProducer",
			shed: func() shedengine.Status {
				return shedengine.Status{
					CurrentProducer: "Preflight",
					State:           shedengine.StateRunning,
					History: []shedengine.HistoryEntry{
						{Producer: "Preflight", Outcome: shedengine.Done, At: "2026-07-17T10:01:30Z"},
						{Producer: "Discussion-Write", Outcome: shedengine.Stuck, At: "2026-07-17T10:05:00Z"},
					},
				}
			},
			prod: func() loomengine.Status {
				return loomengine.Status{Slug: "loom-preflight-fixture", Parent: "main"}
			},
		},
		{
			name: "SetStartSha",
			shed: func() shedengine.Status {
				return shedengine.Status{CurrentProducer: "Preflight", State: shedengine.StateRunning}
			},
			prod: func() loomengine.Status {
				sha := "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4"
				return loomengine.Status{Slug: "loom-preflight-fixture", Parent: "main", StartSha: &sha}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := setupPreflightFixture(t)
			writeSeed(t, h.Location, tt.shed(), tt.prod())
			commitFabricStatus(t, h)

			report, err := loomengine.CheckResolvedForTest(h.Location)
			if err != nil {
				t.Fatalf("checkResolved: %v", err)
			}
			assertCheckSet(t, report, loomengine.CheckHalfFinished)
		})
	}
}

// TestPreflight_MultipleSimultaneousFailures asserts that independently tripped checks (a dirty
// warp and a branch-diverged fabric) are both collected into one Report rather than the first
// short-circuiting the rest.
func TestPreflight_MultipleSimultaneousFailures(t *testing.T) {
	t.Parallel()

	h, _ := setupPreflightFixture(t)

	untracked := filepath.Join(h.PrimeWorktree(), "untracked.txt")
	if err := os.WriteFile(untracked, []byte("new"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "checkout", "-b", "warp-only")

	report, err := loomengine.CheckResolvedForTest(h.Location)
	if err != nil {
		t.Fatalf("checkResolved: %v", err)
	}
	assertCheckSet(t, report, loomengine.CheckWorktreeClean, loomengine.CheckFabricSync)
}
