// reconcile_stale_registration_test.go covers Reconcile repairing a weft worktree deleted by hand.
// The deletion is a plain rm rather than `git worktree remove`.
// That drift leaves a stale git worktree registration that still claims the weft branch,
// and `git worktree add` refuses with "missing but already registered worktree" while it stands.
// Reconcile prunes stale registrations before it adopts a worktree, so the drift is repaired.
//
// The file also holds standalone regression guards for Prune, Cleanup and Healthy.
// Prune reports each stale registration once.
// Cleanup protects the primary's weft branch while the primary is parked off that branch.
// It also treats a pair whose warp is on a detached HEAD as checked out.
// Healthy reports a real directory in the junction's place with its own wording.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/google/go-cmp/cmp"
)

// TestReconcile_RecreatesHandDeletedWeftWorktree deletes a pair's weft worktree directory with plain os.RemoveAll.
// The stale registration stays behind.
// It asserts one Reconcile run recreates the worktree from the surviving branch with no per-pair error.
func TestReconcile_RecreatesHandDeletedWeftWorktree(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "stale-reg-recreate"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	l := h.Location
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	// Delete the weft worktree directory out from under git, as a stray rm would;
	// the registration and branch survive.
	weftPath := fabricengine.RecordsWorktreePath(l, slug)
	if err := os.RemoveAll(weftPath); err != nil {
		t.Fatalf("hand-delete weft worktree: %v", err)
	}

	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	// Result paths are emitted forward-slashed, so normalize the expectation the same way.
	var found bool
	for _, pair := range result.Pairs {
		if pair.RecordsWorktree != filepath.ToSlash(weftPath) {
			continue
		}
		found = true
		if pair.Action != fabricengine.ReconcileActionWeftRecreated {
			t.Errorf("Action = %q; want %q", pair.Action, fabricengine.ReconcileActionWeftRecreated)
		}
		if pair.Error != "" {
			t.Errorf("Error = %q; want empty (stale registration must be pruned before adopt)", pair.Error)
		}
	}
	if !found {
		t.Fatalf("Reconcile result has no pair for slug %q: %+v", slug, result.Pairs)
	}

	if info, err := os.Stat(weftPath); err != nil || !info.IsDir() {
		t.Fatalf("weft worktree not recreated at %s: %v", weftPath, err)
	}
	if got, want := gitkit.CurrentBranch(t, weftPath), fabricengine.RecordsBranchName(slug); got != want {
		t.Errorf("recreated weft worktree branch = %q; want %q", got, want)
	}
}

// TestReconcile_MissingWeftRepoIsDiagnosedByName destroys the weft prime:
// the checkout that holds the weft repo's gitdir.
// It asserts reconcile reports the missing weft repo by name with a remedy.
// Raw chdir errors from each corrective branch are not reported instead.
func TestReconcile_MissingWeftRepoIsDiagnosedByName(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location

	root, target := scratchPaths(l, l.WorktreeName)
	removeRootLink(t, root)
	if err := os.RemoveAll(h.PrimeRecords()); err != nil {
		t.Fatalf("remove weft prime: %v", err)
	}
	removeTarget(t, target)

	topology := h.Topology
	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if len(result.Pairs) == 0 {
		t.Fatal("Reconcile() returned no pairs; want the prime pair reported")
	}
	pr := result.Pairs[0]
	if !strings.Contains(pr.Error, "weft repo missing at") {
		t.Errorf("Reconcile() pair error = %q; want it to diagnose the missing weft repo by name", pr.Error)
	}
	if !strings.Contains(pr.Error, "re-clone") {
		t.Errorf("Reconcile() pair error = %q; want it to name a remedy", pr.Error)
	}
	if pr.Action != fabricengine.ReconcileActionUnmanagedReported {
		t.Errorf("Reconcile() pair action = %q; want %q (report, never a corrective attempt against a missing repo)", pr.Action, fabricengine.ReconcileActionUnmanagedReported)
	}
	if got := describeEntry(t, root); got != "absent" {
		t.Errorf("root .scratch = %q; want no link for a pair whose weft is missing", got)
	}
	if got := describeEntry(t, target); got != "absent" {
		t.Errorf("scratch target = %q; want none created for a pair whose weft is missing", got)
	}
}

// seedRepoWideFabricConfig materializes the repo-wide fabric.yaml at fabricengine.BoltDir(hub).
// That is the Bolt checkout's _lym/config/fabric.yaml.
// A real hub built by hubforge.NewHub materializes the Bolt checkout via CloneAndWire but not this file,
// so this creates the file and its _lym/config/ first.
// The Bolt checkout is not a git repository, so the file is written directly, with no git add or commit.
// Shared by every fabricengine_test fixture that exercises the repo-wide config reads.
func seedRepoWideFabricConfig(t testing.TB, hub string) {
	t.Helper()

	boardDir := fabricengine.BoardDir(hub)
	if err := os.MkdirAll(configengine.ConfigDir(boardDir), 0o755); err != nil {
		t.Fatalf("mkdir repo-wide config dir: %v", err)
	}
	configPath := configengine.ConfigFile(boardDir, "fabric")
	if err := os.WriteFile(configPath, []byte(fabricengine.ConfigTemplate()), 0o644); err != nil {
		t.Fatalf("write repo-wide fabric config: %v", err)
	}
}

// findPruneEntryByWeftPath returns the fabricengine.PruneEntry matching
// weftPath, failing the test if none is found.
func findPruneEntryByWeftPath(t *testing.T, entries []fabricengine.PruneEntry, weftPath string) *fabricengine.PruneEntry {
	t.Helper()
	for i := range entries {
		if filepath.Clean(entries[i].RecordsWorktree) == filepath.Clean(weftPath) {
			return &entries[i]
		}
	}
	t.Fatalf("Prune: no entry for weft path %s", weftPath)
	return nil
}

// countPruneEntriesForWeft counts how many prune entries reference weftPath.
func countPruneEntriesForWeft(entries []fabricengine.PruneEntry, weftPath string) int {
	n := 0
	for i := range entries {
		if filepath.Clean(entries[i].RecordsWorktree) == filepath.Clean(weftPath) {
			n++
		}
	}
	return n
}

// findCleanupEntry returns the fabricengine.CleanupBranchEntry for branch,
// failing the test if none is found.
func findCleanupEntry(t *testing.T, entries []fabricengine.CleanupBranchEntry, branch string) *fabricengine.CleanupBranchEntry {
	t.Helper()
	for i := range entries {
		if entries[i].Branch == branch {
			return &entries[i]
		}
	}
	t.Fatalf("Cleanup: no entry for branch %q; entries=%+v", branch, entries)
	return nil
}

// TestPrune_StaleRegistrationReportedOnce covers a warp worktree deleted by a bare removal.
// That leaves its git worktree registration stale, unlike `git worktree remove`.
// Pass 1 (missing registered warp) and Pass 2 (weft with no warp sibling) both see the same orphaned weft.
// Prune must report it exactly once so dry-run and --apply agree;
// it must not claim Removed=true when there is no weft directory to delete.
func TestPrune_StaleRegistrationReportedOnce(t *testing.T) {
	tiergate.Require(t, 1)
	t.Run("DoubleCountAvoided", func(t *testing.T) {
		t.Parallel()

		const slug = "prune-stale-reg-f2"
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := h.Location
		topology := h.Topology
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
		warpPath := fabricengine.WorktreePath(l, slug)
		weftPath := fabricengine.RecordsWorktreePath(l, slug)

		// Bare removal of the warp directory leaves the git worktree
		// registration stale (unlike `git worktree remove`), so both prune
		// passes see the orphan.
		if err := os.RemoveAll(warpPath); err != nil {
			t.Fatalf("bare-remove warp worktree: %v", err)
		}

		dry, err := topology.Prune(l, false, false)
		if err != nil {
			t.Fatalf("Prune(dry-run): %v", err)
		}
		if got := countPruneEntriesForWeft(dry.Entries, weftPath); got != 1 {
			t.Errorf("dry-run reported weft %s %d times; want exactly 1", weftPath, got)
		}

		apply, err := topology.Prune(l, true, false)
		if err != nil {
			t.Fatalf("Prune(apply): %v", err)
		}
		if got := countPruneEntriesForWeft(apply.Entries, weftPath); got != 1 {
			t.Errorf("apply reported weft %s %d times; want exactly 1", weftPath, got)
		}
		if len(dry.Entries) != len(apply.Entries) {
			t.Errorf("dry-run entry count %d != apply entry count %d; want agreement", len(dry.Entries), len(apply.Entries))
		}
		if _, err := os.Stat(weftPath); !os.IsNotExist(err) {
			t.Errorf("weft dir %s still exists after apply Prune", weftPath)
		}
	})

	t.Run("RemovedFalseWhenNoWeftDir", func(t *testing.T) {
		t.Parallel()

		const slug = "prune-stale-reg-f3"
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := h.Location
		topology := h.Topology
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
		warpPath := fabricengine.WorktreePath(l, slug)
		weftPath := fabricengine.RecordsWorktreePath(l, slug)

		// Bare-remove BOTH sides, leaving both registrations stale.
		// With the weft directory gone, Pass 1's removeStalePair has nothing to delete.
		if err := os.RemoveAll(warpPath); err != nil {
			t.Fatalf("bare-remove warp worktree: %v", err)
		}
		if err := os.RemoveAll(weftPath); err != nil {
			t.Fatalf("bare-remove weft worktree: %v", err)
		}

		apply, err := topology.Prune(l, true, false)
		if err != nil {
			t.Fatalf("Prune(apply): %v", err)
		}
		entry := findPruneEntryByWeftPath(t, apply.Entries, weftPath)
		if entry.Removed {
			t.Errorf("Removed = true for a weft worktree that no longer existed; want false")
		}
		if entry.Error != "" {
			t.Errorf("unexpected error for missing weft dir: %q", entry.Error)
		}
	})
}

// TestCleanup_PrimaryBranchSurvivesForceWhenNotCheckedOut moves the weft primary off main-weft.
// It then runs apply+force Cleanup, and main-weft must still survive.
// It is protected by fabric's live-warp-branch logic;
// git's refusal to delete a checked-out branch alone does not protect it.
//
//testtiming:keep the weft primary's branch surviving apply+force Cleanup while no worktree has it checked out; coverage of its blocks by other tests does not show an assertion of this
func TestCleanup_PrimaryBranchSurvivesForceWhenNotCheckedOut(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology

	mainWeft := fabricengine.RecordsBranchName("main")
	weftPrime := fabricengine.RecordsWorktree(l)

	// Move the weft primary off main-weft so main-weft is not the
	// checked-out branch.
	gitkit.MustRun(t, weftPrime, "git", "checkout", "-b", "primary-parked")

	res, err := topology.Cleanup(l, true, true, false)
	if err != nil {
		t.Fatalf("Cleanup(apply, force): %v", err)
	}

	for _, entry := range res.Entries {
		if entry.Branch == mainWeft {
			t.Errorf("Cleanup reported/handled primary weft branch %q; want not reported (live pair)", mainWeft)
		}
	}
	if !gitkit.BranchExists(t, mustRecordsRepoRoot(t, l), mainWeft) {
		t.Errorf("main-weft branch deleted after force Cleanup with primary parked elsewhere; want intact (F1 regression)")
	}
}

// TestCleanup_DetachedWarpHeadProtectsCheckedOutWeftBranch covers a live pair with a detached warp HEAD.
// Branch-space liveness cannot see such a pair.
// The checked-out protection must report the weft branch Protected in every mode.
func TestCleanup_DetachedWarpHeadProtectsCheckedOutWeftBranch(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "cleanup-detached-r5"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	// Detach the warp worktree's HEAD so branch-space liveness cannot see the pair is live.
	// Only the checked-out protection stands between Cleanup and the pair's weft branch.
	warpPath := fabricengine.WorktreePath(l, slug)
	gitkit.MustRun(t, warpPath, "git", "checkout", "--detach")

	weftBranch := fabricengine.RecordsBranchName(slug)

	// Dry-run must already report the branch protected, so dry-run and apply
	// agree about its fate.
	dry, err := topology.Cleanup(l, false, false, false)
	if err != nil {
		t.Fatalf("Cleanup(dry-run): %v", err)
	}
	dryEntry := findCleanupEntry(t, dry.Entries, weftBranch)
	if !dryEntry.Protected {
		t.Errorf("dry-run Protected = false for checked-out weft branch %q; want true", weftBranch)
	}

	forced, err := topology.Cleanup(l, true, true, false)
	if err != nil {
		t.Fatalf("Cleanup(apply, force): %v", err)
	}
	forcedEntry := findCleanupEntry(t, forced.Entries, weftBranch)
	if !forcedEntry.Protected || forcedEntry.Deleted {
		t.Errorf("apply+force entry: Protected=%v Deleted=%v; want Protected=true Deleted=false", forcedEntry.Protected, forcedEntry.Deleted)
	}
	if forcedEntry.Error != "" {
		t.Errorf("apply+force entry Error = %q; want empty (no doomed delete attempt)", forcedEntry.Error)
	}
	if !gitkit.BranchExists(t, mustRecordsRepoRoot(t, l), weftBranch) {
		t.Errorf("checked-out weft branch %q deleted; want intact", weftBranch)
	}
}

// TestHealthy_RealDirNotAJunction checks the reason reported when a real directory sits where the _lym junction belongs.
// The reason is "warp _lym is not a junction", not "junction missing".
//
//testtiming:keep a real directory where the _lym junction belongs reporting "warp _lym is not a junction"; coverage of its blocks by other tests does not show an assertion of this
func TestHealthy_RealDirNotAJunction(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "pairinsync-realdir"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
	if err := fabricengine.WireJunctions(l, slug, []string{"_lym", "_extra"}); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	warpLayout, err := lymcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(warp): %v", err)
	}

	// Replace the junction with a real directory:
	// the drift a hand-created _lym or a migration from a repo that predates weft leaves behind.
	warpLink := fabricengine.WarpLymLinkHere(warpLayout)
	if err := os.Remove(warpLink); err != nil {
		t.Fatalf("remove warp junction: %v", err)
	}
	if err := os.Mkdir(warpLink, 0o755); err != nil {
		t.Fatalf("mkdir real dir in junction's place: %v", err)
	}

	ok, reason, err := fabricengine.Healthy(warpLayout)
	if err != nil {
		t.Fatalf("fabricengine.Healthy: %v", err)
	}
	if ok {
		t.Errorf("Healthy = true with a real _lym directory; want false")
	}
	if reason.Cause != fabricengine.CauseNotAJunction || reason.Detail != "_lym is not a junction" {
		t.Errorf("Healthy reason = %+v; want {Cause: %q, Detail: %q}", reason, fabricengine.CauseNotAJunction, "_lym is not a junction")
	}
}

// TestCleanup_VerdictIsIndependentOfDryRunAndForce proves a dry run reports what the same flags plus --apply would do,
// and that Cleanup's force parameter is reserved: it changes no verdict and answers no gate.
// A reserved flag that no test covers can quietly regrow a gate.
// `lym fabric cleanup --force` is still accepted and documented as answering no gate, so the test asserts the property.
// The fixture holds an orphan weft branch beside an unmanaged non-suffixed branch,
// so both an unprotected and a protected verdict are covered.
// --apply alone (no --force) deletes the orphan, and --apply with force deletes exactly the orphan.
func TestCleanup_VerdictIsIndependentOfDryRunAndForce(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: t.Setenv("FABRIC_SKIP_PUSH") sets a process-global variable.
	t.Setenv("FABRIC_SKIP_PUSH", "1")

	const slug = "cleanup-verdict-parity"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
	if _, err := topology.Remove(l, slug, true, false); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Remove deletes the pair's weft branch, so re-create an orphaned one by hand: a weft branch
	// whose paired warp branch no warp worktree is on.
	weftRepoRoot, err := fabricengine.RecordsRepoRoot(l)
	if err != nil {
		t.Fatalf("RecordsRepoRoot: %v", err)
	}
	orphan := fabricengine.RecordsBranchName(slug)
	gitkit.MustRun(t, weftRepoRoot, "git", "branch", orphan)
	const unmanaged = "legacy-notes"
	gitkit.MustRun(t, weftRepoRoot, "git", "branch", unmanaged)

	findEntry := func(t *testing.T, entries []fabricengine.CleanupBranchEntry) fabricengine.CleanupBranchEntry {
		t.Helper()
		for _, e := range entries {
			if e.Branch == orphan {
				return e
			}
		}
		t.Fatalf("cleanup result has no entry for %q: %+v", orphan, entries)
		return fabricengine.CleanupBranchEntry{}
	}

	// The two dry runs change nothing, so they come first, over the untouched fixture.
	dryWithoutForce, err := topology.Cleanup(l, false, false, false)
	if err != nil {
		t.Fatalf("Cleanup(dry, force=false): %v", err)
	}
	dryWithForce, err := topology.Cleanup(l, false, true, false)
	if err != nil {
		t.Fatalf("Cleanup(dry, force=true): %v", err)
	}
	if diff := cmp.Diff(dryWithoutForce.Entries, dryWithForce.Entries); diff != "" {
		t.Errorf("Cleanup(dry) entries differ between force=false and force=true (-without +with):\n%s\nforce is reserved and must answer no gate in this verb", diff)
	}

	// The first apply deletes the orphan, so the force=true apply below re-creates it.
	applied, err := topology.Cleanup(l, true, false, false)
	if err != nil {
		t.Fatalf("Cleanup(apply): %v", err)
	}

	dryEntry := findEntry(t, dryWithoutForce.Entries)
	appliedEntry := findEntry(t, applied.Entries)
	if dryEntry.Protected != appliedEntry.Protected {
		t.Errorf("dry-run Protected = %v; --apply Protected = %v; want them to agree",
			dryEntry.Protected, appliedEntry.Protected)
	}
	if dryEntry.Protected {
		t.Errorf("dry-run Protected = true for orphan branch %q; want false (no fold-back gate protects it)", orphan)
	}
	if !appliedEntry.Deleted {
		t.Fatalf("--apply did not delete %q without --force; want it deleted (no fold-back gate protects an orphan)", orphan)
	}
	if gitkit.BranchExists(t, weftRepoRoot, orphan) {
		t.Errorf("orphan branch %q still exists in the weft repo after --apply; want deleted", orphan)
	}

	gitkit.MustRun(t, weftRepoRoot, "git", "branch", orphan)
	applied, err = topology.Cleanup(l, true, true, false)
	if err != nil {
		t.Fatalf("Cleanup(apply, force=true): %v", err)
	}
	for _, entry := range applied.Entries {
		switch entry.Branch {
		case orphan:
			if !entry.Deleted {
				t.Errorf("Cleanup(apply, force=true) left orphan %q undeleted; want deleted", orphan)
			}
		case unmanaged:
			if entry.Deleted {
				t.Errorf("Cleanup(apply, force=true) deleted unmanaged branch %q; want it protected — force may not answer the gate's own carve-outs", unmanaged)
			}
		}
	}
	if gitkit.BranchExists(t, weftRepoRoot, orphan) {
		t.Errorf("orphan branch %q still exists after Cleanup(apply, force=true); want deleted", orphan)
	}
	if !gitkit.BranchExists(t, weftRepoRoot, unmanaged) {
		t.Errorf("unmanaged branch %q was deleted by Cleanup(apply, force=true); want it to survive", unmanaged)
	}
}

// reconcilePairFor returns the result entry for slug's pair, failing the test when the result has none.
func reconcilePairFor(t *testing.T, result fabricengine.ReconcileResult, slug string) fabricengine.ReconcilePairResult {
	t.Helper()
	for _, pair := range result.Pairs {
		if filepath.Base(pair.CodeWorktree) == slug {
			return pair
		}
	}
	t.Fatalf("Reconcile() reported no pair for slug %q", slug)
	return fabricengine.ReconcilePairResult{}
}

// TestReconcile_RestoresDeletedLaunchers proves Reconcile restores a pair's missing launcher directory and reports it.
// It also proves a failed restore stays a Detail note.
// The report matters: a pair whose directory was deleted must not read already_healthy.
func TestReconcile_RestoresDeletedLaunchers(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location

	reconcile := func(t *testing.T) fabricengine.ReconcileResult {
		t.Helper()
		result, err := h.Topology.Reconcile(l)
		if err != nil {
			t.Fatalf("Reconcile() error = %v", err)
		}
		return result
	}

	t.Run("RestoresAndThenReadsHealthy", func(t *testing.T) {
		const slug = "launchers-repair"
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
		launcherDir := fabricengine.LauncherDir(l, slug)
		if err := os.RemoveAll(launcherDir); err != nil {
			t.Fatalf("delete launcher dir: %v", err)
		}

		if got := reconcilePairFor(t, reconcile(t), slug).Action; got != fabricengine.ReconcileActionLaunchersRestored {
			t.Errorf("Reconcile() action = %q; want %q", got, fabricengine.ReconcileActionLaunchersRestored)
		}
		if _, err := os.Stat(launcherDir); err != nil {
			t.Errorf("Reconcile() left the launcher dir missing at %s: %v", launcherDir, err)
		}
		if got := reconcilePairFor(t, reconcile(t), slug).Action; got != fabricengine.ReconcileActionAlreadyHealthy {
			t.Errorf("second Reconcile() action = %q; want %q", got, fabricengine.ReconcileActionAlreadyHealthy)
		}
	})

	t.Run("KeepsJunctionRepointedAndNotesTheRestore", func(t *testing.T) {
		const slug = "launchers-junction"
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
		if err := os.RemoveAll(fabricengine.LauncherDir(l, slug)); err != nil {
			t.Fatalf("delete launcher dir: %v", err)
		}
		if err := fslink.Remove(filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel, "_lym")); err != nil {
			t.Fatalf("delete warp junction: %v", err)
		}

		pair := reconcilePairFor(t, reconcile(t), slug)

		if pair.Action != fabricengine.ReconcileActionJunctionRepointed {
			t.Errorf("Reconcile() action = %q; want %q", pair.Action, fabricengine.ReconcileActionJunctionRepointed)
		}
		if !strings.Contains(pair.Detail, "launcher scripts restored") {
			t.Errorf("Reconcile() detail = %q; want it to note the launcher restore", pair.Detail)
		}
	})

	t.Run("FailedRestoreIsADetailNote", func(t *testing.T) {
		const slug = "launchers-blocked"
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
		launcherRoot := filepath.Join(l.HubPath, "_launchers", l.AnchorRel)
		if err := os.RemoveAll(launcherRoot); err != nil {
			t.Fatalf("delete launcher root: %v", err)
		}
		outside := t.TempDir()
		if err := fslink.CreateDirLink(launcherRoot, outside); err != nil {
			t.Fatalf("link the launcher root outside the hub: %v", err)
		}

		pair := reconcilePairFor(t, reconcile(t), slug)

		if pair.Action != fabricengine.ReconcileActionAlreadyHealthy || pair.Error != "" {
			t.Errorf("Reconcile() action = %q, error = %q; want %q and no error", pair.Action, pair.Error, fabricengine.ReconcileActionAlreadyHealthy)
		}
		if !strings.Contains(pair.Detail, "launcher restore failed") {
			t.Errorf("Reconcile() detail = %q; want it to note the failed launcher restore", pair.Detail)
		}
	})
}
