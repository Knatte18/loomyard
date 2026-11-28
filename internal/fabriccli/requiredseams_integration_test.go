// requiredseams_integration_test.go checks the installed seam adapters against run records the real shed module writes.
// It also checks the refusal of a read whose seam is unset.

package fabriccli_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestInstallRequiredSeams is not parallel: it replaces the process's installed seams.
func TestInstallRequiredSeams(t *testing.T) {
	tiergate.Require(t, 1)

	const slug = "seams"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, slug)
	pair, err := lymcwd.NamedWorktree(h.Path, slug)
	if err != nil {
		t.Fatalf("NamedWorktree(%q): %v", slug, err)
	}
	records := h.PairRecordsSibling(slug)

	restorePaths := fabricengine.SetRunRecordPaths(nil)
	t.Cleanup(restorePaths)
	restoreCodec := fabricengine.SetStatusCodec(nil)
	t.Cleanup(restoreCodec)
	t.Cleanup(fabricengine.SetRunLocks(nil))

	wantRefusal := func(call string, err error, seam fabriccore.RequiredSeam) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), string(seam)) {
			t.Errorf("%s error = %v; want a refusal naming %s", call, err, seam)
		}
	}
	_, _, err = fabricengine.OriginRunStatus(h.Location, slug)
	wantRefusal("OriginRunStatus() with no paths", err, fabriccore.SeamRunRecordPaths)
	_, err = fabricengine.OriginCodeTip(h.Location, slug)
	wantRefusal("OriginCodeTip() with no paths", err, fabriccore.SeamRunRecordPaths)

	gitkit.CommitFile(t, records, "unpushed.txt", "unpushed\n", "unpushed")
	onOrigin := func() string {
		return gitkit.Git(t, records, "ls-remote", "origin", "refs/heads/"+gitkit.CurrentBranch(t, records))
	}
	before := onOrigin()
	_, err = fabricengine.CoalescePushRequest(fabriclayout.PushSpawn{WeftPath: records}, fabricengine.SyncOptions{}, fabriclayout.MachineNames{})
	wantRefusal("CoalescePushRequest() with no paths", err, fabriccore.SeamRunRecordPaths)
	_, err = fabricengine.PushPairAnchored(pair, fabricengine.SyncOptions{}, fabriclayout.PairPush{LockWait: fabriclayout.LockWaitUnbounded})
	wantRefusal("PushPairAnchored() with no paths", err, fabriccore.SeamRunRecordPaths)
	if got := onOrigin(); got != before {
		t.Errorf("origin's records branch = %q; want it unchanged at %q", got, before)
	}
	restorePaths()
	_, err = fabricengine.OriginCodeTip(h.Location, slug)
	wantRefusal("OriginCodeTip() with no codec", err, fabriccore.SeamStatusCodec)

	fabriccli.InstallRequiredSeams()
	// Shed maps its reserved id to the worktree's own run directory, so no reconcile shows this part of the adapter.
	if isReserved := fabriccli.RunRecordPathsForTest().Reserved; !isReserved(shedrun.SelfRunID) || isReserved(slug) {
		t.Errorf("Reserved(%q), Reserved(%q) = %v, %v; want true, false", shedrun.SelfRunID, slug, isReserved(shedrun.SelfRunID), isReserved(slug))
	}

	// The prime's records are left one commit behind origin, and shed's prime lock is held.
	primeRecords := h.PrimeRecords()
	primeRecordsBranch := gitkit.CurrentBranch(t, primeRecords)
	gitkit.CommitFile(t, primeRecords, "ahead.txt", "ahead\n", "ahead")
	gitkit.MustRun(t, primeRecords, "git", "push", "origin", primeRecordsBranch)
	gitkit.MustRun(t, primeRecords, "git", "reset", "--keep", "HEAD~1")
	lockPath := shedrun.PrimeRunLock(h.Location)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(lockPath), err)
	}
	held, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireWriteLock(%s): %v", lockPath, err)
	}
	primeBefore := gitkit.RevParse(t, primeRecords, "HEAD")
	report, err := fabricengine.ReconcileOrigin(h.Location, fabriclayout.ReconcileScope{}, fabricengine.OriginSeams{}, fabricengine.ReconcileOptions{})
	if err != nil {
		t.Fatalf("ReconcileOrigin() error = %v; want nil", err)
	}
	if err := held.Release(); err != nil {
		t.Fatalf("release %s: %v", lockPath, err)
	}
	busy := slices.IndexFunc(report.Branches, func(b fabriclayout.BranchOutcome) bool {
		return b.Prime && b.Repo == fabriclayout.RepoRecords
	})
	if busy < 0 || report.Branches[busy].State != fabriclayout.BranchBusy || report.Branches[busy].BusyLock != "prime" {
		t.Errorf("ReconcileOrigin() branches = %+v; want the prime's records busy on the prime lock", report.Branches)
	}
	if got := gitkit.RevParse(t, primeRecords, "HEAD"); got != primeBefore {
		t.Errorf("prime records HEAD = %q; want %q, nothing moved", got, primeBefore)
	}

	statusPath := shedrun.StatusFile(pair, slug)
	_, statusRel, err := shedrun.TreeRunFileRels(slug)
	if err != nil {
		t.Fatalf("TreeRunFileRels(%q): %v", slug, err)
	}
	commitStatus := func(path, message string, status shedengine.Status) {
		t.Helper()
		err := state.UpdateJSON(path, shedrun.StatusLock(pair, slug), func(shedengine.Status, bool) (shedengine.Status, error) {
			return status, nil
		})
		if err != nil {
			t.Fatalf("UpdateJSON(%s): %v", path, err)
		}
		gitkit.MustRun(t, records, "git", "add", statusRel)
		gitkit.MustRun(t, records, "git", "commit", "-m", message)
		gitkit.MustRun(t, records, "git", "push", "origin", gitkit.CurrentBranch(t, records))
		gitkit.MustRun(t, records, "git", "fetch")
	}

	commitStatus(statusPath, "run status", shedengine.Status{State: shedengine.StatePaused})
	written, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", statusPath, err)
	}
	got, found, err := fabricengine.OriginRunStatus(h.Location, slug)
	if err != nil || !found || string(got) != string(written) {
		t.Errorf("OriginRunStatus(%q) = %q, found %v, %v; want %q, true, nil", slug, got, found, err, written)
	}

	message := "transition\n\n" + fabriclayout.TransitionTrailerKey + ": " + slug + "\n" +
		fabriclayout.WarpSHATrailerKey + ": " + gitkit.RevParse(t, h.PairCodeWorktree(slug), "HEAD")
	commitStatus(filepath.Join(records, filepath.FromSlash(statusRel)), message, shedengine.Status{
		State: shedengine.StateRunning,
		History: []shedengine.HistoryEntry{
			{Producer: "first", Outcome: shedengine.Done, Machine: "oslo"},
			{Producer: "second", Outcome: shedengine.Done, Machine: "bergen"},
		},
	})
	tip, err := fabricengine.OriginCodeTip(h.Location, slug)
	if err != nil || tip.Machine != "bergen" {
		t.Errorf("OriginCodeTip(%q) machine = %q, %v; want %q, nil", slug, tip.Machine, err, "bergen")
	}
}
