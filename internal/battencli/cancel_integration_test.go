//go:build integration

// cancel_integration_test.go drives "lyx batten cancel" through RunCLIIn over hubforge hubs: a blocked run with no pair, a kept pair whose reconcile only judges, and a pair removed with its task branch.
// It stays a white-box "package battencli" test, like the other integration files here.

package battencli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// seedCancelRun writes and commits a batten seed and a status at producer in rowState for slug on h's prime, and claims slug's board entry with claim.
func seedCancelRun(t *testing.T, h *hubforge.Hub, slug, producer string, rowState shedengine.State, claim string) {
	t.Helper()
	seedBoardTask(t, h, slug, "loom")
	board, err := boardengine.OpenHub(h.Path)
	if err != nil {
		t.Fatalf("open the hub board: %v", err)
	}
	if err := board.SetStatus(slug, &claim); err != nil {
		t.Fatalf("claim %q on the board: %v", slug, err)
	}

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

// runCancel runs "lyx batten cancel" with args from cwd and returns its output.
func runCancel(t *testing.T, cwd string, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	RunCLIIn(cwd, &out, append([]string{"cancel"}, args...))
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

// overviewListsSlug reports whether the prime's overview pane draws a line for slug.
func overviewListsSlug(h *hubforge.Hub, slug string) bool {
	source := newOverviewSource(h.Location, filepath.Join(h.Path, "no-pause.json"), overviewThresholds{}, time.Now)
	for _, line := range source.frame().Lines {
		if strings.Contains(line, slug) {
			return true
		}
	}
	return false
}

// TestCancelIntegration_BlockedRunWithNoPair cancels a run blocked at Worktree-Create with no pair, a claim and a tackler marker naming a guid no strand holds:
// the records are removed and committed, the tackler is stopped, the claim is cleared, the run leaves the overview, and a second cancel finds nothing left.
func TestCancelIntegration_BlockedRunWithNoPair(t *testing.T) {
	t.Parallel()

	const slug = "cancel-blocked"
	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	seedCancelRun(t, h, slug, battenrecipe.NameWorktreeCreate, shedengine.StateBlocked, "running · Worktree-Create")
	const guid = "guid-no-strand-holds"
	if err := sheddriver.WriteTacklerMarker(shedrun.TacklerMarker(h.Location, slug), sheddriver.TacklerMarker{Attempt: 1, StrandGUID: guid}); err != nil {
		t.Fatalf("write the tackler marker: %v", err)
	}
	if !overviewListsSlug(h, slug) {
		t.Fatal("the overview draws no line for the blocked run before the cancel; the check after it would prove nothing")
	}

	got := envelope.RequireOK(t, runCancel(t, h.PrimeWorktree(), slug))

	if got.Raw["records_removed"] != true || got.Raw["claim_cleared"] != true || got.Raw["tackler_stopped"] != guid || got.Raw["end_only"] != false || got.Raw["pair_removed"] != false {
		t.Errorf("envelope = %v; want the records removed, the claim cleared, the tackler stopped and nothing end-only", got.Raw)
	}
	if got.Raw["state"] != "blocked" || got.Raw["row"] != battenrecipe.NameWorktreeCreate {
		t.Errorf("envelope state, row = %v, %v; want blocked at Worktree-Create", got.Raw["state"], got.Raw["row"])
	}
	if pathExists(shedrun.RunDir(h.Location, slug)) || pathExists(shedrun.TacklerMarker(h.Location, slug)) {
		t.Error("the run directory or the tackler marker survived the cancel")
	}
	if _, found, err := fabricengine.CommittedAnchoredFile(h.Location, shedrun.StatusRel(h.Location, slug)); err != nil || found {
		t.Errorf("committed status found=%v err=%v; want it gone from the records HEAD", found, err)
	}
	if status, found := boardStatusOf(t, h, slug); !found || status != nil {
		t.Errorf("board entry status = %v, found %v; want the entry kept with no status", status, found)
	}
	if overviewListsSlug(h, slug) {
		t.Error("the overview still draws a line for the cancelled run")
	}
	if pathExists(shedrun.CancelMarker(h.Location, slug)) {
		t.Error("the cancel marker survived a finished cancel")
	}

	envelope.RequireErr(t, runCancel(t, h.PrimeWorktree(), slug), "nothing of "+slug+" is left to cancel")
}

// TestCancelIntegration_KeptPairJudgeOnly cancels, without --remove, a paused run whose pair's records branch on origin is ahead of the local one:
// the pair is kept, the reconcile the cancel ran moved no ref, the claim is cleared, and the way forward names the cancel with --remove.
func TestCancelIntegration_KeptPairJudgeOnly(t *testing.T) {
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

	if got.Raw["pair_removed"] != false || got.Raw["claim_cleared"] != true || got.Raw["records_removed"] != true {
		t.Errorf("envelope = %v; want the pair kept, the claim cleared and the records removed", got.Raw)
	}
	if want := `run "lyx batten cancel ` + slug + ` --remove" before rerunning it`; got.Raw["way_forward"] != want {
		t.Errorf("way_forward = %v; want %q", got.Raw["way_forward"], want)
	}
	if tip := strings.TrimSpace(mustRunGit(t, sibling, "rev-parse", "HEAD")); tip != oldTip {
		t.Errorf("the local records branch moved from %s to %s; want a judge-only reconcile to move no ref", oldTip, tip)
	}
	if !pathExists(h.PairCodeWorktree(slug)) {
		t.Error("the task worktree is gone after a cancel without --remove")
	}
	if status, _ := boardStatusOf(t, h, slug); status != nil {
		t.Errorf("board entry status = %q; want it cleared", *status)
	}
}

// TestCancelIntegration_RemoveTearsDownPairAndBranch cancels with --remove a paused run whose pair holds an unpushed commit and an uncommitted file, and whose slug holds a live pause entry:
// the pair is removed with its uncommitted file snapshotted, the task branch is archived and deleted locally and on origin, the pause entry is lifted and the claim is cleared.
func TestCancelIntegration_RemoveTearsDownPairAndBranch(t *testing.T) {
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

	if got.Raw["pair_removed"] != true || got.Raw["claim_cleared"] != true || got.Raw["records_removed"] != true {
		t.Errorf("envelope = %v; want the pair removed, the claim cleared and the records removed", got.Raw)
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
	if pathExists(worktree) {
		t.Error("the task worktree survived the cancel with --remove")
	}
	if gitkit.BranchExists(t, h.PrimeWorktree(), slug) || gitkit.BranchExists(t, h.CodeBare, slug) {
		t.Errorf("task branch %q survived locally or on origin", slug)
	}
	wantTag := fabricengine.ArchiveTagPrefix + slug + "/" + tip[:12]
	if tags := strings.Fields(gitkit.Git(t, h.CodeBare, "tag", "--list", fabricengine.ArchiveTagPrefix+slug+"/*")); !slices.Contains(tags, wantTag) {
		t.Errorf("archive tags on origin = %v; want %s", tags, wantTag)
	}
	if status, _ := boardStatusOf(t, h, slug); status != nil {
		t.Errorf("board entry status = %q; want it cleared", *status)
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
