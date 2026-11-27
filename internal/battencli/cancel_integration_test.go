// cancel_integration_test.go drives "lym batten cancel" through clihelp.ExecuteIn over hubforge hubs.
// It covers a blocked run with no pair, a kept pair whose reconcile only judges, and a removed pair.
// It also covers the re-run that finishes an unfinished removal, and the removal of a beam.

package battencli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/runfsm"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/permkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// seedCancelRun writes and commits a batten seed and a status for slug on h's prime.
// The status is at producer in rowState, and slug's board status is set to claim.
func seedCancelRun(t *testing.T, h *hubforge.Hub, slug, producer string, rowState shedengine.State, claim string) {
	t.Helper()
	seedBoardTask(t, h, slug, "tapestry")
	board, err := boardengine.OpenHub(h.Path)
	if err != nil {
		t.Fatalf("open the hub board: %v", err)
	}
	if err := board.SetStatus(slug, &claim); err != nil {
		t.Fatalf("claim %q on the board: %v", claim, err)
	}
	seedRunRecords(t, h, slug, producer, rowState)
}

// seedRunRecords writes and commits a batten seed and a status at producer in rowState for slug on h's prime.
func seedRunRecords(t *testing.T, h *hubforge.Hub, slug, producer string, rowState shedengine.State) {
	t.Helper()
	if err := shedrun.WriteSeed(h.Location, slug, shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo}); err != nil {
		t.Fatalf("write the seed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(shedrun.StatusLock(h.Location, slug)), 0o755); err != nil {
		t.Fatalf("mkdir the status lock dir: %v", err)
	}
	status := shedengine.Status{CurrentProducer: producer, State: rowState, History: []shedengine.HistoryEntry{}}
	if err := state.WriteJSON(shedrun.StatusFile(h.Location, slug), shedrun.StatusLock(h.Location, slug), status); err != nil {
		t.Fatalf("write the status: %v", err)
	}
	rels := []string{shedrun.SeedRel(h.Location, slug), shedrun.StatusRel(h.Location, slug)}
	if _, committed, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), h.Location, rels, "test: seed the run of "+slug, fabricengine.EnvSyncOptions()); err != nil || !committed {
		t.Fatalf("commit the run records: committed=%v err=%v", committed, err)
	}
}

// runCancel runs "lym batten cancel" with args from cwd and returns its output.
func runCancel(t *testing.T, cwd string, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	clihelp.ExecuteIn(Command(), cwd, &out, append([]string{"cancel"}, args...))
	return out.String()
}

// boardStatusOf reads the status of slug's board entry; found is false when the entry is absent.
func boardStatusOf(t *testing.T, h *hubforge.Hub, slug string) (status *string, found bool) {
	t.Helper()
	board, err := boardengine.OpenHub(h.Path)
	if err != nil {
		t.Fatalf("open the hub board: %v", err)
	}
	task, found, err := board.GetTask(slug)
	if err != nil {
		t.Fatalf("read the board entry of %q: %v", slug, err)
	}
	return task.Status, found
}

// committedRecord reads slug's batten record at the prime's records HEAD; found is false when the HEAD holds none.
func committedRecord(t *testing.T, h *hubforge.Hub, slug string) (record shedengine.Status, found bool) {
	t.Helper()
	data, found, err := fabricengine.CommittedAnchoredFile(h.Location, shedrun.StatusRel(h.Location, slug))
	if err != nil {
		t.Fatalf("read the committed record of %q: %v", slug, err)
	}
	if !found {
		return shedengine.Status{}, false
	}
	record, err = shedengine.ParseStatusTolerant(data)
	if err != nil {
		t.Fatalf("parse the committed record of %q: %v", slug, err)
	}
	return record, true
}

// pushPrimeRecords pushes the prime's records side to origin, which the detached push of a test binary never does.
func pushPrimeRecords(t *testing.T, h *hubforge.Hub) {
	t.Helper()
	mustRunGit(t, h.PrimeRecords(), "push", "origin", "HEAD")
}

// originRecord reads slug's batten record from the prime's records branch on origin; found is false when it holds none.
func originRecord(t *testing.T, h *hubforge.Hub, slug string) (record shedengine.Status, found bool) {
	t.Helper()
	branch := strings.TrimSpace(mustRunGit(t, h.PrimeRecords(), "rev-parse", "--abbrev-ref", "HEAD"))
	rel := filepath.ToSlash(filepath.Join(h.Location.AnchorRel, shedrun.StatusRel(h.Location, slug)))
	data, err := gitexec.Run([]string{"show", branch + ":" + rel}, h.RecordsBare)
	if err != nil {
		return shedengine.Status{}, false
	}
	record, err = shedengine.ParseStatusTolerant([]byte(data))
	if err != nil {
		t.Fatalf("parse the record of %q on origin: %v", slug, err)
	}
	return record, true
}

// overviewListsSlug reports whether the prime's dobby, which the prime's window draws, lists a run for slug.
func overviewListsSlug(t *testing.T, h *hubforge.Hub, slug string) bool {
	t.Helper()
	reader, err := NewDobbyViewReader(h.Location, h.Location.WorktreeName)
	if err != nil {
		t.Fatalf("place the prime's dobby: %v", err)
	}
	view, err := reader.Read(time.Now())
	if err != nil {
		t.Fatalf("read the prime's dobby: %v", err)
	}
	return slices.ContainsFunc(view.Runs, func(run dobbyengine.ViewRun) bool { return run.Slug == slug })
}

// TestCancelIntegration_BlockedRunWithNoPair cancels a run blocked at Worktree-Create with no pair.
// The run has a claim and a tackler marker naming a guid that no strand holds.
// The cancel keeps the record with its finish, commits it, stops the tackler, and clears the claim.
// A second plain cancel is refused, and a cancel with --remove deletes the record and finds nothing left after it.
// A cancel that stops after its intent was committed leaves a record that origin's copy derives as cancelling.
func TestCancelIntegration_BlockedRunWithNoPair(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "cancel-blocked"
	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	seedCancelRun(t, h, slug, battenrecipe.NameWorktreeCreate, shedengine.StateBlocked, "running · Worktree-Create")
	const guid = "guid-no-strand-holds"
	if err := sheddriver.WriteTacklerMarker(shedrun.TacklerMarker(h.Location, slug), sheddriver.TacklerMarker{Attempt: 1, StrandGUID: guid}); err != nil {
		t.Fatalf("write the tackler marker: %v", err)
	}
	if !overviewListsSlug(t, h, slug) {
		t.Fatal("the overview draws no line for the blocked run before the cancel; the check after it would prove nothing")
	}

	got := envelope.RequireOK(t, runCancel(t, h.PrimeWorktree(), slug))

	if got.Raw["records_removed"] != false || got.Raw["record_kept"] != true || got.Raw["claim_cleared"] != true || got.Raw["tackler_stopped"] != guid || got.Raw["end_only"] != false || got.Raw["pair_removed"] != false {
		t.Errorf("envelope = %v; want the record kept, the claim cleared, the tackler stopped and nothing end-only", got.Raw)
	}
	if got.Raw["state"] != "blocked" || got.Raw["row"] != battenrecipe.NameWorktreeCreate {
		t.Errorf("envelope state, row = %v, %v; want blocked at Worktree-Create", got.Raw["state"], got.Raw["row"])
	}
	record, found := committedRecord(t, h, slug)
	if !found || record.State != shedengine.StateCancelled || record.Cancel == nil || record.Cancel.Finish == nil || record.Cancel.Finish.Remove {
		t.Errorf("committed record = %+v, found %v; want state cancelled with a finish that kept the pair", record, found)
	}
	if status, found := boardStatusOf(t, h, slug); !found || status == nil || *status != boardengine.StatusDraft {
		t.Errorf("board entry status = %v, found %v; want the entry kept as a draft", status, found)
	}
	if reading, err := readCancelMarker(h.Location, slug); err != nil || reading.Present {
		t.Errorf("readCancelMarker() = %+v, %v; want no cancel under way after a finished cancel", reading, err)
	}

	envelope.RequireErr(t, runCancel(t, h.PrimeWorktree(), slug), "is already cancelled with its pair kept")

	got = envelope.RequireOK(t, runCancel(t, h.PrimeWorktree(), slug, "--remove"))

	if got.Raw["records_removed"] != true || got.Raw["record_kept"] != false {
		t.Errorf("envelope = %v; want the record removed and none kept", got.Raw)
	}
	if pathExists(shedrun.RunDir(h.Location, slug)) || pathExists(shedrun.TacklerMarker(h.Location, slug)) {
		t.Error("the run directory or the tackler marker survived the cancel with --remove")
	}
	if _, found := committedRecord(t, h, slug); found {
		t.Error("the committed record survived the cancel with --remove; want it gone from the records HEAD")
	}
	if overviewListsSlug(t, h, slug) {
		t.Error("the overview still draws a line for the removed run")
	}

	envelope.RequireErr(t, runCancel(t, h.PrimeWorktree(), slug), "nothing of "+slug+" is left to cancel")

	// A blocked run whose directory is committed and then deleted on disk has no pair or claim.
	// Its only outstanding step is the deletion of its records.
	const deleted = "cancel-deleted-on-disk"
	seedCancelRun(t, h, deleted, battenrecipe.NameWorktreeCreate, shedengine.StateBlocked, "running · Worktree-Create")
	board, err := boardengine.OpenHub(h.Path)
	if err != nil {
		t.Fatalf("open the hub board: %v", err)
	}
	if err := board.SetStatus(deleted, nil); err != nil {
		t.Fatalf("clear the claim of %q: %v", deleted, err)
	}
	if err := os.RemoveAll(shedrun.RunDir(h.Location, deleted)); err != nil {
		t.Fatalf("delete the run directory of %q: %v", deleted, err)
	}

	got = envelope.RequireOK(t, runCancel(t, h.PrimeWorktree(), deleted))

	if got.Raw["record_kept"] != false || got.Raw["pair_removed"] != false {
		t.Errorf("envelope = %v; want no record to keep and no pair", got.Raw)
	}
	if _, found := committedRecord(t, h, deleted); found {
		t.Error("the committed record of the run deleted on disk survived; want its deletion committed")
	}

	// A cancel that stops after its intent was committed leaves a cancelling run, which origin's copy of the record shows.
	const stopped = "cancel-stopped"
	seedCancelRun(t, h, stopped, battenrecipe.NameWorktreeCreate, shedengine.StateBlocked, "running · Worktree-Create")
	deps, err := hubCancelDeps(h.Location, stopped)
	if err != nil {
		t.Fatalf("hubCancelDeps(%s): %v", stopped, err)
	}
	deps.EndForCancel = func(context.Context, pairteardown.CancelRequest) (pairteardown.CancelResult, error) {
		return pairteardown.CancelResult{}, errors.New("the cancel stopped after its intent was committed")
	}
	if _, err := cancelRun(context.Background(), stopped, false, deps); err == nil {
		t.Fatal("cancelRun() error = nil; want the stopped cancel's error")
	}
	pushPrimeRecords(t, h)
	origin, found := originRecord(t, h, stopped)
	if !found {
		t.Fatalf("origin's records branch holds no record of %q; want the committed intent", stopped)
	}
	if derived := runfsm.Derive(runfsm.Records{Batten: origin, BattenFound: true}).State; derived != runfsm.Cancelling {
		t.Errorf("state derived from origin's record = %q; want %q", derived, runfsm.Cancelling)
	}
}

// TestCancelIntegration_KeptPairJudgeOnly cancels a paused run without --remove.
// The run's pair has a records branch on origin that is ahead of the local one.
// The pair is kept, the reconcile the cancel ran moved no ref, and the claim is cleared.
// The record is kept with its finish, committed, and the push of the prime's records puts it on origin.
// The way forward names the cancel with --remove.
func TestCancelIntegration_KeptPairJudgeOnly(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	slug := hubforge.TemplatePairSlug
	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{slug}})
	seedCancelRun(t, h, slug, battenrecipe.NameRunShed, shedengine.StatePaused, "running · Run-Shed")

	sibling := h.PairRecordsSibling(slug)
	branch := strings.TrimSpace(mustRunGit(t, sibling, "rev-parse", "--abbrev-ref", "HEAD"))
	if err := os.WriteFile(filepath.Join(sibling, "ahead.txt"), []byte("ahead\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRunGit(t, sibling, "add", "ahead.txt")
	mustRunGit(t, sibling, "commit", "-m", "test: advance the records branch")
	mustRunGit(t, sibling, "push", "origin", "HEAD:refs/heads/"+branch)
	mustRunGit(t, sibling, "reset", "--hard", "HEAD~1")
	oldTip := strings.TrimSpace(mustRunGit(t, sibling, "rev-parse", "HEAD"))

	got := envelope.RequireOK(t, runCancel(t, h.PrimeWorktree(), slug))

	if got.Raw["pair_removed"] != false || got.Raw["claim_cleared"] != true || got.Raw["record_kept"] != true || got.Raw["records_removed"] != false {
		t.Errorf("envelope = %v; want the pair kept, the claim cleared and the record kept", got.Raw)
	}
	if want := `run "lym batten cancel ` + slug + ` --remove" before rerunning it`; got.Raw["way_forward"] != want {
		t.Errorf("way_forward = %v; want %q", got.Raw["way_forward"], want)
	}
	if tip := strings.TrimSpace(mustRunGit(t, sibling, "rev-parse", "HEAD")); tip != oldTip {
		t.Errorf("the local records branch moved from %s to %s; want a judge-only reconcile to move no ref", oldTip, tip)
	}
	if !pathExists(h.PairCodeWorktree(slug)) {
		t.Error("the task worktree is gone after a cancel without --remove")
	}
	if status, _ := boardStatusOf(t, h, slug); status == nil || *status != boardengine.StatusDraft {
		t.Errorf("board entry status = %v; want draft", status)
	}
	record, found := committedRecord(t, h, slug)
	if !found || record.State != shedengine.StateCancelled || record.Cancel == nil || record.Cancel.Finish == nil || record.Cancel.Finish.Remove {
		t.Errorf("committed record = %+v, found %v; want state cancelled with a finish that kept the pair", record, found)
	}

	pushPrimeRecords(t, h)
	origin, found := originRecord(t, h, slug)
	if !found || origin.State != shedengine.StateCancelled || origin.Cancel == nil || origin.Cancel.Finish == nil {
		t.Errorf("record on origin's records branch = %+v, found %v; want state cancelled with its finish", origin, found)
	}
}

// TestCancelIntegration_RemoveTearsDownPairAndBranch cancels a paused run with --remove.
// The run's pair holds an unpushed commit and an uncommitted file, and its slug holds a live pause entry.
// The pair is removed with its uncommitted file snapshotted.
// The task branch is archived and deleted locally and on origin.
// The pause entry is lifted, the claim is cleared and the record is deleted with the pair.
func TestCancelIntegration_RemoveTearsDownPairAndBranch(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	slug := hubforge.TemplatePairSlug
	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{slug}})
	seedCancelRun(t, h, slug, battenrecipe.NameRunShed, shedengine.StatePaused, "running · Run-Shed")

	worktree := h.PairCodeWorktree(slug)
	tip := gitkit.CommitFile(t, worktree, "unlanded.txt", "unlanded\n", "test: unlanded task work")
	if err := os.WriteFile(filepath.Join(worktree, "uncommitted.txt"), []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pauseGeometry, err := hubgeom.PauseGeometry(h.Location)
	if err != nil {
		t.Fatalf("resolve the pause geometry: %v", err)
	}
	now := time.Now()
	if _, err := pauserecord.Update(pauseGeometry.RecordPath, now, func(record *pauserecord.Record) error {
		record.Live = append(record.Live, pauserecord.Entry{Scope: slug, Caller: "orch", PausedAt: now, GraceEnd: now.Add(time.Hour)})
		return nil
	}); err != nil {
		t.Fatalf("write the slug's pause entry: %v", err)
	}

	got := envelope.RequireOK(t, runCancel(t, h.PrimeWorktree(), slug, "--remove"))

	if got.Raw["pair_removed"] != true || got.Raw["claim_cleared"] != true || got.Raw["records_removed"] != true || got.Raw["record_kept"] != false {
		t.Errorf("envelope = %v; want the pair removed, the claim cleared and the records removed with none kept", got.Raw)
	}
	if got.Raw["snapshot_tag"] == "" {
		t.Errorf("envelope = %v; want the snapshot tag of the uncommitted file", got.Raw)
	}
	if lifted, _ := got.Raw["lifted"].([]any); len(lifted) != 1 || lifted[0] != slug {
		t.Errorf("lifted = %v; want [%s]", got.Raw["lifted"], slug)
	}
	record, err := pauserecord.Read(pauseGeometry.RecordPath)
	if err != nil {
		t.Fatalf("read the pause record: %v", err)
	}
	if len(record.Live) != 0 {
		t.Errorf("live pause entries = %v; want the slug's entry lifted", record.Live)
	}
	if _, found := committedRecord(t, h, slug); found {
		t.Error("the committed record survived the cancel with --remove; want it deleted with the pair")
	}
	if pathExists(worktree) {
		t.Error("the task worktree survived the cancel with --remove")
	}
	if gitkit.BranchExists(t, h.PrimeWorktree(), slug) || gitkit.BranchExists(t, h.CodeBare, slug) {
		t.Errorf("task branch %q survived locally or on origin", slug)
	}
	wantTag := fabriclayout.ArchiveTagPrefix + slug + "/" + tip[:12]
	if tags := strings.Fields(gitkit.Git(t, h.CodeBare, "tag", "--list", fabriclayout.ArchiveTagPrefix+slug+"/*")); !slices.Contains(tags, wantTag) {
		t.Errorf("archive tags on origin = %v; want %s", tags, wantTag)
	}
	if status, _ := boardStatusOf(t, h, slug); status == nil || *status != boardengine.StatusDraft {
		t.Errorf("board entry status = %v; want draft", status)
	}
}

// TestCancelIntegration_FinishesAnUnfinishedRemoval re-runs the cancel that a pending removal record names.
// The slug has no run record, claim or local pair part, and only its records branch is left on origin.
// The first re-run deletes that branch.
// The record then still stands, as one of a pair gone everywhere, and the second re-run drops it.
// A third finds nothing left to cancel.
func TestCancelIntegration_FinishesAnUnfinishedRemoval(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	slug := hubforge.TemplatePairSlug
	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{slug}})
	recordsBranch := fabriclayout.RecordsBranchName(slug)
	if _, err := h.Topology.Remove(h.Location, slug, true, false, ""); err != nil {
		t.Fatalf("Remove(%s): %v", slug, err)
	}
	if gitkit.BranchExists(t, h.CodeBare, slug) {
		gitkit.MustRun(t, h.CodeBare, "git", "update-ref", "-d", "refs/heads/"+slug)
	}
	if !gitkit.BranchExists(t, h.RecordsBare, recordsBranch) {
		t.Fatalf("origin holds no %q after the local removal; the fixture needs it", recordsBranch)
	}
	// The record a removal's teardown child leaves behind when it ends before its job does.
	recordPath := filepath.Join(filepath.Dir(fabriclayout.TeardownLockPath(h.Path, slug)), slug+".pending.json")
	record := fabriclayout.PendingTeardown{Slug: slug, Kind: "remove", HandedAt: time.Now(), Worktree: h.PrimeWorktree(), Rerun: "lym batten cancel " + slug + " --remove"}
	if err := state.WriteJSON(recordPath, recordPath+".lock", record); err != nil {
		t.Fatalf("write the pending teardown record: %v", err)
	}

	envelope.RequireOK(t, runCancel(t, h.PrimeWorktree(), slug, "--remove"))

	if gitkit.BranchExists(t, h.RecordsBare, recordsBranch) {
		t.Errorf("origin still holds %q after the cancel; want it deleted", recordsBranch)
	}

	envelope.RequireOK(t, runCancel(t, h.PrimeWorktree(), slug, "--remove"))

	if pending, err := fabricengine.PendingTeardowns(h.Path); err != nil || len(pending) != 0 {
		t.Errorf("PendingTeardowns() after the cancel of a gone pair = (%+v, %v); want none", pending, err)
	}
	envelope.RequireErr(t, runCancel(t, h.PrimeWorktree(), slug, "--remove"), "nothing of "+slug+" is left to cancel")
}

// TestCancelIntegration_RemoveOfABeamLeavesAPlainProject cancels beams with --remove over a real hub board.
// The first beam has a paused run, a draft member with no pair, and a pair whose origin record has no beam mark.
// The removal ends the beam, leaves its member, and warns of no orch left unstopped.
// The second beam has only its flag left once a removal fails to clear it.
// A board that cannot be read refuses that cancel, and the re-run clears the flag.
func TestCancelIntegration_RemoveOfABeamLeavesAPlainProject(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const lone, member = "cancel-lone-beam", "cancel-beam-member"
	slug := hubforge.TemplatePairSlug
	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{slug}})
	board, err := boardengine.OpenHub(h.Path)
	if err != nil {
		t.Fatalf("open the hub board: %v", err)
	}
	for _, entry := range []map[string]any{
		{"slug": slug, "kind": "project"},
		{"slug": lone, "kind": "project"},
		{"slug": member, "kind": "task", "project": slug},
	} {
		entry["title"] = entry["slug"]
		entry["labels"] = []string{"bug"}
		if _, err := board.UpsertTask(entry); err != nil {
			t.Fatalf("seed board entry %v: %v", entry["slug"], err)
		}
	}
	machine, err := machinename.Name()
	if err != nil {
		t.Fatalf("machine name: %v", err)
	}
	if _, err := boardengine.SetOwners(h.BoardDir(), boardengine.OwnerRequest{Machine: machine, Slugs: []string{slug, lone}, Beam: true}); err != nil {
		t.Fatalf("flag the beam projects: %v", err)
	}
	seedRunRecords(t, h, slug, battenrecipe.NameRunShed, shedengine.StatePaused)

	got := envelope.RequireOK(t, runCancel(t, h.PrimeWorktree(), slug, "--remove"))

	if got.Raw["beam_ended"] != true || got.Raw["pair_removed"] != true {
		t.Errorf("envelope = %v; want the pair removed and the beam ended", got.Raw)
	}
	if warnings, _ := got.Raw["warnings"].([]any); slices.ContainsFunc(warnings, func(warning any) bool {
		text, _ := warning.(string)
		return strings.Contains(text, "the beam's orch was not stopped")
	}) {
		t.Errorf("warnings = %v; want no orch stop attempted on an unmarked pair", warnings)
	}
	ended, found, err := board.GetTask(slug)
	if err != nil || !found {
		t.Fatalf("GetTask(%s) = found %v, %v; want the project kept", slug, found, err)
	}
	if ended.Beam || ended.Recipe != "" || ended.OwnerMachine != "" || ended.BeamColor != "" {
		t.Errorf("beam, recipe, owner, colour = %v, %q, %q, %q; want false and all empty", ended.Beam, ended.Recipe, ended.OwnerMachine, ended.BeamColor)
	}
	kept, found, err := board.GetTask(member)
	if err != nil || !found || kept.Project != slug || kept.Status == nil || *kept.Status != boardengine.StatusDraft {
		t.Errorf("member = %+v, found %v, %v; want a draft still under %s", kept, found, err, slug)
	}
	if _, err := board.UpsertTask(map[string]any{"slug": slug, "beam": false}); err != nil {
		t.Errorf("UpsertTask(%s, beam false) error = %v; want nil once the beam is ended", slug, err)
	}

	boardFile := filepath.Join(h.BoardDir(), "board.json")
	// Each subtest makes the board unreadable partway through a cancel, and its end restores it.
	t.Run("FailedFlagClearNamesTheRerunAfterThePush", func(t *testing.T) {
		seedRunRecords(t, h, lone, battenrecipe.NameRunShed, shedengine.StatePaused)
		deps, err := hubCancelDeps(h.Location, lone)
		if err != nil {
			t.Fatalf("hubCancelDeps(%s): %v", lone, err)
		}
		push, pushed := deps.PushRecords, false
		deps.PushRecords = func() (bool, error) {
			owed, err := push()
			pushed = true
			permkit.Chmod(t, boardFile, 0)
			return owed, err
		}

		_, err = cancelRun(context.Background(), lone, true, deps)

		want := "could not clear the beam flag of " + lone
		if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), `run "lym batten cancel `+lone+` --remove" again`) {
			t.Errorf("cancelRun() error = %v; want %q naming the re-run", err, want)
		}
		if !pushed {
			t.Error("the records were not pushed before the failed flag clear")
		}
	})

	t.Run("UnreadableBoardRefusesTheFlagAloneCancel", func(t *testing.T) {
		deps, err := hubCancelDeps(h.Location, lone)
		if err != nil {
			t.Fatalf("hubCancelDeps(%s): %v", lone, err)
		}
		list := deps.LiveRecords
		deps.LiveRecords = func() ([]runproc.Record, error) {
			permkit.Chmod(t, boardFile, 0)
			return list()
		}

		_, err = cancelRun(context.Background(), lone, true, deps)

		if want := "could not read the board entry of " + lone; err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("cancelRun() error = %v; want it to contain %q", err, want)
		}
	})

	got = envelope.RequireOK(t, runCancel(t, h.PrimeWorktree(), lone, "--remove"))

	if got.Raw["beam_ended"] != true {
		t.Errorf("envelope = %v; want the flag left alone cleared", got.Raw)
	}
}

// mustRunGit runs git with args in dir and returns its output, failing the test on an error.
func mustRunGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitexec.Run(args, dir)
	if err != nil {
		t.Fatalf("git %s in %s: %v", strings.Join(args, " "), dir, err)
	}
	return out
}
