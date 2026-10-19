//go:build integration

// verbs_regenerate_integration_test.go proves the fabric verbs that create, remove or switch a warp worktree regenerate the hub workspace file,
// and that a failed regeneration never changes a verb's result.
// Shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// TestVerbsRegenerateHubWorkspace reads the hub workspace file's folder names after each verb.
// The top level is not parallel: some rows set a process-global hook or environment variable, or capture the logger,
// so they run before the parallel rows are released and no verb runs beside them.
func TestVerbsRegenerateHubWorkspace(t *testing.T) {
	t.Run("AddFailedByTheHookStillRegenerates", func(t *testing.T) {
		// Serial: the hook is process-global.
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		removeWorkspaceFile(t, h)
		fabricengine.SetAddFailAfterWarpHookForTest(t, func() error { return errors.New("injected failure after the warp worktree") })

		if _, err := h.Topology.Add(h.Location, "failed-task", fabricengine.AddOptions{}); err == nil {
			t.Fatal("Add error = nil; want the injected failure")
		}

		assertFolders(t, h, fabricengine.HubWorkspacePath(h.Location), primeBranch(t, h), "Bolt")
	})

	t.Run("RemoveFailedAfterTakingTheTaskWorktreeDropsThePair", func(t *testing.T) {
		// Serial: t.Setenv sets a process-global variable, and a failed Remove respawns the pair's push.
		t.Setenv("FABRIC_SKIP_PUSH", "1")
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPairWith(t, h, "locked-pair", fabricengine.AddOptions{SkipPush: true})
		gitkit.MustRun(t, h.PrimeRecords(), "git", "worktree", "lock", fabricengine.RecordsWorktreePath(h.Location, "locked-pair"))

		if _, err := h.Topology.Remove(h.Location, "locked-pair", true, false); err == nil {
			t.Fatal("Remove error = nil; want the failed weft teardown")
		}

		assertFolders(t, h, fabricengine.HubWorkspacePath(h.Location), primeBranch(t, h), "Bolt")
	})

	t.Run("FailedRegenerationWarnsAndLeavesTheVerbResultAlone", func(t *testing.T) {
		// Serial: it captures the logger.
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		path := fabricengine.HubWorkspacePath(h.Location)
		settings := filepath.Join(h.PrimeWorktree(), ".vscode", "settings.json")
		if err := os.MkdirAll(settings, 0o755); err != nil {
			t.Fatalf("plant a directory at settings.json: %v", err)
		}
		logs := logcapture.Capture(t)

		res := hubforge.AddPair(t, h, "a-task")

		m := res.Mutated()
		if recordedFileWrite(&m, h.Path, path) {
			t.Errorf("Add recorded a file_written for the workspace file despite the failed regeneration: %+v", m.Entries())
		}
		if got := logs.String(); !strings.Contains(got, "lyx fabric reconcile") {
			t.Errorf("log = %q; want a warning naming lyx fabric reconcile", got)
		}

		if err := os.Remove(settings); err != nil {
			t.Fatalf("remove the planted directory: %v", err)
		}
		if _, err := h.Topology.Reconcile(h.Location); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		assertFolders(t, h, path, primeBranch(t, h), "Bolt", "a-task")
	})

	t.Run("CloneHoldsTheFile", func(t *testing.T) {
		t.Parallel()
		h := hubforge.NewHub(t, ".")

		assertFolders(t, h, fabricengine.HubWorkspacePath(h.Location), primeBranch(t, h), "Bolt")
	})

	t.Run("AddAddLocalAndRemoveTrackThePairsAndRecordTheWrite", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		path := fabricengine.HubWorkspacePath(h.Location)
		prime := primeBranch(t, h)

		added := hubforge.AddPair(t, h, "a-task")
		addedRecord := added.Mutated()
		if !recordedFileWrite(&addedRecord, h.Path, path) {
			t.Errorf("Add recorded no file_written for the workspace file: %+v", addedRecord.Entries())
		}
		assertFolders(t, h, path, prime, "Bolt", "a-task")

		local, err := h.Topology.AddLocal(h.Location, "local-task")
		if err != nil {
			t.Fatalf("AddLocal: %v", err)
		}
		localRecord := local.Mutated()
		if !recordedFileWrite(&localRecord, h.Path, path) {
			t.Errorf("AddLocal recorded no file_written for the workspace file: %+v", localRecord.Entries())
		}
		assertFolders(t, h, path, prime, "Bolt", "a-task", "local-task")

		removed, err := h.Topology.Remove(h.Location, "a-task", true, false)
		if err != nil {
			t.Fatalf("Remove: %v", err)
		}
		removedRecord := removed.Mutated()
		if !recordedFileWrite(&removedRecord, h.Path, path) {
			t.Errorf("Remove recorded no file_written for the workspace file: %+v", removedRecord.Entries())
		}
		assertFolders(t, h, path, prime, "Bolt", "local-task")
	})

	t.Run("PruneRegeneratesOnlyWhenApplying", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		path := fabricengine.HubWorkspacePath(h.Location)
		hubforge.AddPair(t, h, "a-task")
		hubforge.AddPair(t, h, "gone")
		if err := os.RemoveAll(h.PairCodeWorktree("gone")); err != nil {
			t.Fatalf("delete the pair's worktree: %v", err)
		}
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read workspace file: %v", err)
		}

		if _, err := h.Topology.Prune(h.Location, false, false); err != nil {
			t.Fatalf("Prune dry run: %v", err)
		}
		if after, err := os.ReadFile(path); err != nil || string(after) != string(before) {
			t.Errorf("a dry-run Prune changed the workspace file: err %v, got %q, want %q", err, after, before)
		}

		if _, err := h.Topology.Prune(h.Location, true, false); err != nil {
			t.Fatalf("Prune apply: %v", err)
		}
		assertFolders(t, h, path, primeBranch(t, h), "Bolt", "a-task")
	})

	t.Run("CheckoutRenamesTheFirstFolderAndRegeneratesOnRollback", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		path := fabricengine.HubWorkspacePath(h.Location)
		l := h.Location
		original := primeBranch(t, h)

		// Free the pair's branches by removing its worktrees, so the prime can switch onto them.
		const target = "checkout-target"
		hubforge.AddPair(t, h, target)
		gitkit.MustRun(t, h.PrimeWorktree(), "git", "worktree", "remove", "--force", h.PairCodeWorktree(target))
		gitkit.MustRun(t, h.PrimeRecords(), "git", "worktree", "remove", "--force", h.PairRecordsSibling(target))

		if _, err := h.Topology.Checkout(l, target); err != nil {
			t.Fatalf("Checkout: %v", err)
		}
		assertFolders(t, h, path, target, "Bolt")

		// A junction-wiring failure after both sides switched rolls back, and the file still lists the original branch.
		removeWorkspaceFile(t, h)
		slug := filepath.Base(l.WorktreePath())
		if err := fabricengine.WireJunctions(l, slug, []string{"_lyx", "_extra"}); err != nil {
			t.Fatalf("setup WireJunctions: %v", err)
		}
		gitkit.MustRun(t, l.WorktreePath(), "git", "branch", original+"-rollback")
		warpLyx := fabricengine.WarpLyxLinkHere(l)
		if err := os.Remove(warpLyx); err != nil {
			t.Fatalf("remove warp junction to corrupt it: %v", err)
		}
		if err := os.MkdirAll(warpLyx, 0o755); err != nil {
			t.Fatalf("create real _lyx dir: %v", err)
		}
		if _, err := h.Topology.Checkout(l, original+"-rollback"); err == nil {
			t.Fatal("Checkout error = nil; want a junction-wiring failure")
		}
		assertFolders(t, h, path, target, "Bolt")
	})

	t.Run("CheckoutRefusedByADirtyWeftRegeneratesNothing", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		plantTrackedDirt(t, h.PrimeRecords(), "dirtyWeftTracked")
		removeWorkspaceFile(t, h)

		if _, err := h.Topology.Checkout(h.Location, "anything"); err == nil {
			t.Fatal("Checkout error = nil; want the dirty weft refusal")
		}

		assertNoWorkspaceFile(t, h)
	})

	t.Run("PreFlightRefusalsRegenerateNothing", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPair(t, h, "dirty-pair")
		plantTrackedDirt(t, h.PairCodeWorktree("dirty-pair"), "dirtyWarpTracked")
		removeWorkspaceFile(t, h)

		if _, err := h.Topology.Remove(h.Location, "dirty-pair", false, false); err == nil {
			t.Fatal("Remove error = nil; want the dirty worktree refusal")
		}
		assertNoWorkspaceFile(t, h)

		plantTrackedDirt(t, h.PrimeWorktree(), "dirtyWarpTracked")
		if _, err := h.Topology.Add(h.Location, "other-task", fabricengine.AddOptions{}); err == nil {
			t.Fatal("Add error = nil; want the dirty prime refusal")
		}
		assertNoWorkspaceFile(t, h)
	})
}
