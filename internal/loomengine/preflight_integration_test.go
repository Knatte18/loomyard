//go:build integration

// preflight_integration_test.go drives Preflight/checkResolved end-to-end
// against real git fixtures — a paired host+fabric worktree with a wired _lyx
// junction and a seeded status.json — covering every pass/fail scenario
// across all four preconditions. It is integration-tagged because it spawns
// git via lyxtest fixtures (Test Tier Purity Invariant).

package loomengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/state"
)

// setupPreflightFixture builds a fully-configured CopyPaired fixture with fabric
// and junction setup, returning the fixture and the slug for WireJunctions.
func setupPreflightFixture(t *testing.T) (lyxtest.PairedFixture, string) {
	t.Helper()

	f := lyxtest.CopyPaired(t)
	slug := filepath.Base(f.Layout.WorktreePath())

	lyxtest.SeedConfig(t, f.WeftPrime, map[string]string{
		"fabric": fabricengine.ConfigTemplate(),
	})
	seedRepoWideFabricConfig(t, f.Layout.HubPath)
	lyxtest.MustRun(t, f.WeftPrime, "git", "checkout", "-b", fabricengine.WeftBranchName("main"))

	if err := fabricengine.WireJunctions(f.Layout, slug, []string{"_lyx", lyxdirs.DotLyxDirName, "_extra"}); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	// LoomStatusLock lives under the host worktree's own .lyx tree, wired above
	// as a real junction (dotlyx-junction-wiring-and-unwire) -- WireJunctions'
	// own seedGitExclude call already keeps the .lyx junction entry itself out
	// of `git status`, so no test-local exclude is needed here.

	seedValidStatus(t, f.Layout)

	// The seeded status.json (and its .lock sidecar) materialize through the
	// _lyx junction into the fabric worktree's own git repo, where they start
	// out untracked. Commit them so a freshly-built fixture is genuinely
	// clean on both sides — required now that Clean checks the fabric worktree
	// too, not just the host.
	lyxtest.MustRun(t, f.WeftPrime, "git", "add", "-A")
	lyxtest.MustRun(t, f.WeftPrime, "git", "commit", "-m", "seed status")

	return f, slug
}

// seedRepoWideFabricConfig materializes the repo-wide fabric.yaml at
// <hub>/_board/_lyx/config/fabric.yaml (directly written, not committed).
// The pathspec names "_extra" rather than reading fabricengine.ConfigTemplate()'s own default,
// because setupPreflightFixture's explicit WireJunctions call wires "_extra" as this fixture's
// second, non-_lyx junction (card 3's retarget) — RepoWiredNames must agree with what is actually
// wired on disk for checkJunctionHealth/Healthy to classify each fixture as healthy where expected.
func seedRepoWideFabricConfig(t testing.TB, hub string) {
	t.Helper()

	boardDir := fabricengine.BoardDir(hub)
	if err := os.MkdirAll(configengine.ConfigDir(boardDir), 0o755); err != nil {
		t.Fatalf("mkdir repo-wide config dir: %v", err)
	}
	configPath := configengine.ConfigFile(boardDir, "fabric")
	if err := os.WriteFile(configPath, []byte("branch_prefix: \"\"\npathspec: _extra\n"), 0o644); err != nil {
		t.Fatalf("write repo-wide fabric config: %v", err)
	}
}

// seedValidStatus writes a fresh, coherent status.json seed with handoff fields only.
func seedValidStatus(t *testing.T, l *lyxcwd.Location) {
	t.Helper()

	s := Status{
		Slug:      "loom-preflight-fixture",
		Parent:    "main",
		Phase:     "preflight",
		Stage:     "produce",
		Narration: "now: awaiting preflight / last: — / wait: —",
	}
	// LoomStatusLock now lives under .lyx, a sibling tree WireJunctions never
	// creates (it only wires _lyx and the caller's configured optional
	// junctions) -- state.WriteJSON MkdirAlls the
	// status.json's own parent but not the lock's, so this fixture must
	// create the lock's parent itself, mirroring Preflight's own MkdirAll
	// fix in preflight.go.
	if err := os.MkdirAll(filepath.Dir(LoomStatusLock(l)), 0o755); err != nil {
		t.Fatalf("mkdir status lock parent: %v", err)
	}
	if err := state.WriteJSON(LoomStatusFile(l), LoomStatusLock(l), s); err != nil {
		t.Fatalf("seed status.json: %v", err)
	}
}

// commitFabricStatus commits the current state of status.json in the fabric worktree,
// isolating test scenarios from CheckWorktreeClean failures.
func commitFabricStatus(t *testing.T, f lyxtest.PairedFixture) {
	t.Helper()

	lyxtest.MustRun(t, f.WeftPrime, "git", "add", "-A")
	lyxtest.MustRun(t, f.WeftPrime, "git", "commit", "-m", "update status")
}

// restoreCwd saves the process cwd and restores it via t.Cleanup. Call it AFTER
// creating any t.TempDir()/fixture: t.Cleanup runs LIFO, so chdir-back must run
// before TempDir removal (required on Windows).
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

// assertCheckSet asserts that got's Failures carry exactly the given CheckID set
// (order-independent). An empty want asserts Report.OK.
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

// TestPreflight_HealthyPairAndSeed is the anchor case: a fully healthy paired host+fabric worktree
// with a valid fresh seed reports OK.
// Since CopyPaired's host hub is a single-worktree repo, its Layout.Prime already equals
// Layout.WorktreeRoot — this test doubles as the "Prime worktree with a healthy pair+seed" scenario
// (run-in-existing-or-prime-worktree).
func TestPreflight_HealthyPairAndSeed(t *testing.T) {
	t.Parallel()

	f, _ := setupPreflightFixture(t)

	report, err := checkResolved(f.Layout)
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

// TestPreflight_SubdirectoryInvocation asserts that Preflight() invoked from a subdirectory of the
// worktree (RelPath != ".")
// short-circuits with a single worktree-root failure.
// Exercises the public Preflight() for the same reason as TestPreflight_NotAGitRepo.
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

	// Record "sub" as the recognized anchor so Resolve(sub) succeeds under the
	// strict cwd gate with AnchorRel == "sub" -- an unrecorded subdirectory
	// invocation is now ErrCwdOutsideAnchor, a hard error, not the soft
	// CheckWorktreeRoot report this test exists to prove.
	anchorPath := filepath.Join(fabricengine.BoardDir(f.Layout.HubPath), lyxcwd.AnchorFileName)
	if err := os.WriteFile(anchorPath, []byte("sub"), 0o644); err != nil {
		t.Fatalf("write %s: %v", anchorPath, err)
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

// TestPreflight_HostDirty covers all three ways Clean can observe a dirty repo (a
// tracked-and-modified file, a staged file, and an untracked-only file), plus the genuinely-new
// fabric-dirty-only and both-dirty shapes now that Clean also checks the fabric side.
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
		{
			name: "DirtyFabricOnly",
			dirty: func(t *testing.T, f lyxtest.PairedFixture) {
				untracked := filepath.Join(f.WeftPrime, "untracked.txt")
				if err := os.WriteFile(untracked, []byte("new"), 0o644); err != nil {
					t.Fatalf("write untracked fabric file: %v", err)
				}
			},
		},
		{
			name: "BothDirty",
			dirty: func(t *testing.T, f lyxtest.PairedFixture) {
				hostUntracked := filepath.Join(f.Hub, "untracked.txt")
				if err := os.WriteFile(hostUntracked, []byte("new"), 0o644); err != nil {
					t.Fatalf("write untracked host file: %v", err)
				}
				fabricUntracked := filepath.Join(f.WeftPrime, "untracked.txt")
				if err := os.WriteFile(fabricUntracked, []byte("new"), 0o644); err != nil {
					t.Fatalf("write untracked fabric file: %v", err)
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

// TestPreflight_FabricNotReady asserts that a removed fabric worktree reports fabric-ready,
// and that the now-dangling junction makes the seed stat fail too — classified seed-unreadable
// (never seed-missing) because check 3 already failed.
func TestPreflight_FabricNotReady(t *testing.T) {
	t.Parallel()

	f, _ := setupPreflightFixture(t)

	// Drive the not-present branch via the lyxtest fixture's own WeftPrime
	// field rather than fabricengine.WeftWorktree(f.Layout): check 3 now
	// goes through fabricengine.Ready(l), and the fixture field is the
	// independent source of the same path.
	if err := os.RemoveAll(f.WeftPrime); err != nil {
		t.Fatalf("remove fabric worktree: %v", err)
	}

	report, err := checkResolved(f.Layout)
	if err != nil {
		t.Fatalf("checkResolved: %v", err)
	}
	assertCheckSet(t, report, CheckFabricReady, CheckSeedUnreadable)
}

// TestPreflight_HostFabricDifferentBranches asserts that host and fabric worktrees on different
// branches report fabric-sync — the CauseBranchMismatch/CheckFabricSync equivalence pinned by
// healthy-typed-reason,
// and that fabric-sync alone does NOT block the seed check (the junction and fabric directory are both
// still healthy).
func TestPreflight_HostFabricDifferentBranches(t *testing.T) {
	t.Parallel()

	f, _ := setupPreflightFixture(t)

	lyxtest.MustRun(t, f.Hub, "git", "checkout", "-b", "host-only")

	report, err := checkResolved(f.Layout)
	if err != nil {
		t.Fatalf("checkResolved: %v", err)
	}
	assertCheckSet(t, report, CheckFabricSync)
}

// TestPreflight_ConfigLoadFailed asserts the CauseConfigLoadFailed/CheckJunction equivalence pinned
// by healthy-typed-reason: a repo-wide fabric.yaml that fails to load classifies as CheckJunction
// (not a distinct CheckID of its own), same as the three junction-drift shapes TestPreflight_JunctionBroken
// covers.
// Unlike those shapes, the _lyx junction itself is still physically intact here — only the config
// read fails — so the seed check is unaffected: no CheckSeedUnreadable/CheckSeedMissing is added.
func TestPreflight_ConfigLoadFailed(t *testing.T) {
	t.Parallel()

	f, _ := setupPreflightFixture(t)

	configPath := configengine.ConfigFile(fabricengine.BoardDir(f.Layout.HubPath), "fabric")
	if err := os.WriteFile(configPath, []byte("not: [valid: yaml"), 0o644); err != nil {
		t.Fatalf("corrupt repo-wide fabric config: %v", err)
	}

	report, err := checkResolved(f.Layout)
	if err != nil {
		t.Fatalf("checkResolved: %v", err)
	}
	assertCheckSet(t, report, CheckJunction)
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
	shapes := []struct {
		name    string
		corrupt func(t *testing.T, hostLink string)
	}{
		{
			name: "Missing",
			corrupt: func(t *testing.T, hostLink string) {
				if err := fslink.Remove(hostLink); err != nil {
					t.Fatalf("remove junction %s: %v", hostLink, err)
				}
			},
		},
		{
			name: "NotALink",
			corrupt: func(t *testing.T, hostLink string) {
				if err := fslink.Remove(hostLink); err != nil {
					t.Fatalf("remove junction %s: %v", hostLink, err)
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
					t.Fatalf("remove junction %s: %v", hostLink, err)
				}
				wrongTarget := filepath.Join(filepath.Dir(hostLink), "not-the-fabric-junction-dir")
				if err := os.MkdirAll(wrongTarget, 0o755); err != nil {
					t.Fatalf("mkdir wrong target %s: %v", wrongTarget, err)
				}
				if err := fslink.CreateDirLink(hostLink, wrongTarget); err != nil {
					t.Fatalf("CreateDirLink(%s, %s): %v", hostLink, wrongTarget, err)
				}
			},
		},
	}

	junctions := []struct {
		name       string
		linkFor    func(f lyxtest.PairedFixture, slug string) string
		wantChecks []CheckID // in addition to CheckJunction, which every case wants
	}{
		{
			name:       "Lyx",
			linkFor:    func(f lyxtest.PairedFixture, slug string) string { return fabricengine.HostLyxLink(f.Layout, slug) },
			wantChecks: []CheckID{CheckSeedUnreadable},
		},
		{
			name: "Extra",
			linkFor: func(f lyxtest.PairedFixture, slug string) string {
				return filepath.Join(fabricengine.WorktreePath(f.Layout, slug), f.Layout.AnchorRel, "_extra")
			},
			wantChecks: nil,
		},
	}

	for _, j := range junctions {
		for _, tt := range shapes {
			t.Run(j.name+"_"+tt.name, func(t *testing.T) {
				t.Parallel()

				f, slug := setupPreflightFixture(t)
				hostLink := j.linkFor(f, slug)
				tt.corrupt(t, hostLink)

				report, err := checkResolved(f.Layout)
				if err != nil {
					t.Fatalf("checkResolved: %v", err)
				}
				want := append([]CheckID{CheckJunction}, j.wantChecks...)
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

	f, slug := setupPreflightFixture(t)

	// Simulate the missing-optional-junction state: this worktree's second,
	// non-_lyx junction was never wired, even though _lyx is fully healthy.
	extraLink := filepath.Join(fabricengine.WorktreePath(f.Layout, slug), f.Layout.AnchorRel, "_extra")
	if err := fslink.Remove(extraLink); err != nil {
		t.Fatalf("remove the optional junction to simulate a worktree missing it: %v", err)
	}

	report, err := checkResolved(f.Layout)
	if err != nil {
		t.Fatalf("checkResolved: %v", err)
	}
	assertCheckSet(t, report, CheckJunction)

	// One Reconcile call repairs the missing junction: it must report
	// JunctionRepointed (the repair happened), never AlreadyHealthy.
	topology := fabricengine.NewTopology(fabricengine.Config{})
	result, err := topology.Reconcile(f.Layout)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	var found bool
	for _, pair := range result.Pairs {
		if pair.HostWorktree != filepath.ToSlash(f.Layout.WorktreePath()) {
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
		t.Fatalf("Reconcile result has no pair for the worktree %s: %+v", f.Layout.WorktreePath(), result.Pairs)
	}

	// The junction now resolves.
	if isLink, err := fslink.IsLink(extraLink); err != nil || !isLink {
		t.Fatalf("optional junction %s not restored by Reconcile: isLink=%v err=%v", extraLink, isLink, err)
	}

	// A fresh Preflight now reports OK: the remedy this batch documents.
	report, err = checkResolved(f.Layout)
	if err != nil {
		t.Fatalf("checkResolved after Reconcile: %v", err)
	}
	assertCheckSet(t, report)
}

// TestPreflight_SeedMissing asserts that a genuinely absent seed — junction and fabric pairing both
// healthy — reports seed-missing, not seed-unreadable.
func TestPreflight_SeedMissing(t *testing.T) {
	t.Parallel()

	f, _ := setupPreflightFixture(t)

	if err := os.Remove(LoomStatusFile(f.Layout)); err != nil {
		t.Fatalf("remove seed: %v", err)
	}
	commitFabricStatus(t, f)

	report, err := checkResolved(f.Layout)
	if err != nil {
		t.Fatalf("checkResolved: %v", err)
	}
	assertCheckSet(t, report, CheckSeedMissing)
}

// TestPreflight_SeedUnknownField asserts that a seed containing an unknown field fails strict
// decode and reports seed-incoherent.
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
	if err := os.WriteFile(LoomStatusFile(f.Layout), []byte(raw), 0o644); err != nil {
		t.Fatalf("write malformed seed: %v", err)
	}
	commitFabricStatus(t, f)

	report, err := checkResolved(f.Layout)
	if err != nil {
		t.Fatalf("checkResolved: %v", err)
	}
	assertCheckSet(t, report, CheckSeedIncoherent)
}

// TestPreflight_SeedHalfFinished asserts that a coherent-but-advanced seed (non-empty history, or a
// stamped start_sha) reports half-finished — the task has already run past the point Preflight is
// meant to gate.
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
			if err := state.WriteJSON(LoomStatusFile(f.Layout), LoomStatusLock(f.Layout), tt.seed()); err != nil {
				t.Fatalf("overwrite seed: %v", err)
			}
			commitFabricStatus(t, f)

			report, err := checkResolved(f.Layout)
			if err != nil {
				t.Fatalf("checkResolved: %v", err)
			}
			assertCheckSet(t, report, CheckHalfFinished)
		})
	}
}

// TestPreflight_MultipleSimultaneousFailures asserts that independently tripped checks (a dirty
// host and a branch-diverged fabric) are both collected into one Report rather than the first
// short-circuiting the rest.
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
	assertCheckSet(t, report, CheckWorktreeClean, CheckFabricSync)
}
