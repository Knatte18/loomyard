// adopt_integration_test.go drives the takeover over two hubforge clones of one remote:
// a task's run moves from the first clone to the second and back.

package adoptcli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/adoptcli"
	"github.com/Knatte18/loomyard/internal/adoptrecord"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	firstMachine  = "otherbox"
	secondMachine = "secondbox"
	taskSlug      = "alpha"
)

// holdOwnerPause records a live owner pause of the slug's own scope in the hub's pause record.
func holdOwnerPause(t *testing.T, location *lymcwd.Location) {
	t.Helper()
	geometry, err := hubgeom.PauseGeometry(location)
	if err != nil {
		t.Fatalf("PauseGeometry: %v", err)
	}
	now := time.Now()
	if _, err := pauserecord.Update(geometry.RecordPath, now, func(record *pauserecord.Record) error {
		record.Live = append(record.Live, pauserecord.Entry{Scope: taskSlug, PausedAt: now, GraceEnd: now, Caller: pauserecord.OwnerCaller})
		return nil
	}); err != nil {
		t.Fatalf("record the owner pause: %v", err)
	}
}

func TestAdoptIntegration_SecondCloneTakesOver(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	boardkit.InstallReplay(t)

	first := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, first, taskSlug)

	board, err := boardengine.OpenHub(first.Path)
	if err != nil {
		t.Fatalf("open the first hub's board: %v", err)
	}
	if _, err := board.UpsertTask(map[string]any{"slug": taskSlug, "title": taskSlug, "kind": "task", "labels": []string{"bug"}}); err != nil {
		t.Fatalf("seed the board entry: %v", err)
	}
	runStatus := "running · Worktree-Create"
	if err := board.SetStatus(taskSlug, &runStatus); err != nil {
		t.Fatalf("hold the board entry under a run: %v", err)
	}
	if _, err := boardengine.SetOwners(first.BoardDir(), boardengine.OwnerRequest{Machine: firstMachine, Prime: true, Takeover: true}); err != nil {
		t.Fatalf("record the first machine as the prime's owner: %v", err)
	}

	// The reconcile judges only pairs with a batten run record,
	// and an adopt moves only a run whose record is not done,
	// so the first clone records a running one before the second clone exists.
	status, err := json.Marshal(shedengine.Status{CurrentProducer: "Worktree-Create", State: shedengine.StateRunning, History: []shedengine.HistoryEntry{}})
	if err != nil {
		t.Fatalf("marshal the run status: %v", err)
	}
	for path, content := range map[string]string{shedrun.StatusFile(first.Location, taskSlug): string(status), shedrun.SeedFile(first.Location, taskSlug): "{}"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create the run directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write the run record: %v", err)
		}
	}
	rels := []string{shedrun.SeedRel(first.Location, taskSlug), shedrun.StatusRel(first.Location, taskSlug)}
	if _, committed, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), first.Location, rels, "test: run record", fabricengine.EnvSyncOptions()); err != nil || !committed {
		t.Fatalf("commit the run record: committed=%v err=%v", committed, err)
	}
	if _, err := fabricengine.PushAnchored(first.Location, fabricengine.EnvSyncOptions(), fabricengine.StatusPushLockWait); err != nil {
		t.Fatalf("push the run record: %v", err)
	}

	second := hubforge.CloneSecond(t, first)
	// The first clone pushes a commit to the prime's code branch and the task's pair branch after the second clone exists.
	// Only the second clone's reconcile can bring them.
	for _, dir := range []string{first.PrimeWorktree(), first.PairCodeWorktree(taskSlug)} {
		gitkit.CommitFile(t, dir, "pushed.txt", "pushed", "pushed work")
		gitkit.MustRun(t, dir, "git", "push", "origin", gitkit.CurrentBranch(t, dir))
	}
	holdOwnerPause(t, second.Location)

	// The task is moved with root, which the first machine owns,
	// so the second clone adopts root and records no owner of the task's own.
	got, err := adoptcli.AdoptAs(second.Location, secondMachine, []string{boardengine.RootSubject}, false, false)
	if err != nil {
		t.Fatalf("the second clone's adopt: %v", err)
	}
	adopted := slugOutcomeOf(t, got, taskSlug)
	if !adopted.PairAdded || adopted.OwnerRecorded || !adopted.AdoptPaused || adopted.Error != "" || !got.PrimeRecorded {
		t.Errorf("second clone's outcome = %+v, prime recorded = %v, want the pair added, the run paused by adopt, no owner of the task's own and the prime recorded", adopted, got.PrimeRecorded)
	}
	secondRecord, found, err := adoptrecord.Read(second.Path)
	if err != nil || !found || secondRecord.State != adoptrecord.StateDone {
		t.Fatalf("second clone's adopt record = %+v, found %v, %v; want a done record", secondRecord, found, err)
	}
	var listed []string
	for _, step := range secondRecord.Steps {
		listed = append(listed, step.Name)
	}
	if !slices.Equal(listed, adoptrecord.Steps) {
		t.Errorf("second clone's adopt record lists steps %v, want every step of %v", listed, adoptrecord.Steps)
	}
	geometry, err := hubgeom.PauseGeometry(second.Location)
	if err != nil {
		t.Fatalf("PauseGeometry: %v", err)
	}
	record, err := pauserecord.Read(geometry.RecordPath)
	if err != nil || len(record.Live) != 1 || record.Live[0].Scope != taskSlug || record.Live[0].Caller != pauserecord.AdoptCaller {
		t.Errorf("second clone's pause record = %+v, %v; want the owner entry turned into the one adopt entry", record.Live, err)
	}
	if _, err := os.Stat(filepath.Join(fabricengine.AdoptScratchDir(second.Path), boardengine.RootSubject, "holds.json")); !os.IsNotExist(err) {
		t.Errorf("Stat(holds.json) error = %v; want no holds file written", err)
	}
	// The adopt runs no fabric pull, so the reconcile alone brought both branches to the tip the first clone pushed.
	for name, dirs := range map[string][2]string{
		"prime":     {first.PrimeWorktree(), second.PrimeWorktree()},
		"task pair": {first.PairCodeWorktree(taskSlug), second.PairCodeWorktree(taskSlug)},
	} {
		if want, got := gitkit.RevParse(t, dirs[0], "HEAD"), gitkit.RevParse(t, dirs[1], "HEAD"); got != want {
			t.Errorf("second clone's %s branch is at %s, want origin's tip %s", name, got, want)
		}
	}
	if _, err := os.Stat(second.PairCodeWorktree(taskSlug)); err != nil {
		t.Errorf("the second clone has no pair worktree for %s: %v", taskSlug, err)
	}
	owners, err := boardengine.ReadOwners(second.BoardDir())
	if err != nil || owners.Prime != secondMachine || owners.Entries[taskSlug].Owner != "" {
		t.Errorf("second clone's owners = %+v, %v; want the prime owned by %s and no owner on %s", owners, err, secondMachine, taskSlug)
	}

	// The first clone holds local code commits origin lacks, and its run lock is held under a slug-scope owner pause.
	pair := first.PairCodeWorktree(taskSlug)
	if err := os.WriteFile(filepath.Join(pair, "local.txt"), []byte("local work"), 0o644); err != nil {
		t.Fatalf("write the local file: %v", err)
	}
	gitkit.MustRun(t, pair, "git", "add", "local.txt")
	gitkit.MustRun(t, pair, "git", "commit", "-m", "local work origin lacks")
	holdOwnerPause(t, first.Location)
	task, err := lymcwd.ResolveWorktree(pair)
	if err != nil {
		t.Fatalf("resolve the first clone's pair: %v", err)
	}
	runLock := shedrun.RunLock(task, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(runLock), 0o755); err != nil {
		t.Fatalf("create the run lock directory: %v", err)
	}
	held, err := lock.AcquireWriteLock(runLock)
	if err != nil {
		t.Fatalf("hold the run lock: %v", err)
	}
	t.Cleanup(func() { _ = held.Release() })

	// The second machine took root over with the task, so taking them back names root.
	got, err = adoptcli.AdoptAs(first.Location, firstMachine, []string{boardengine.RootSubject}, false, false)
	if err == nil || !strings.Contains(err.Error(), "--keep-local") || !strings.Contains(err.Error(), "--drop-local") {
		t.Fatalf("the first clone's adopt back error = %v, want the held-back stop naming --keep-local and --drop-local", err)
	}
	if stopped := slugOutcomeOf(t, got, taskSlug); !stopped.Stopped || !stopped.Live || !strings.Contains(stopped.WayForward, "`lym adopt root --keep-local`") || got.PrimeRecorded {
		t.Errorf("adopt back = %+v, prime recorded = %v, want a stop on the live pair naming both ways forward under root and the prime left", stopped, got.PrimeRecorded)
	}
	if firstRecord, found, err := adoptrecord.Read(first.Path); err != nil || !found || firstRecord.State != adoptrecord.StateStopped || firstRecord.InFlight != adoptrecord.StepDone {
		t.Errorf("first clone's adopt record = %+v, found %v, %v; want it stopped at done", firstRecord, found, err)
	}

	got, err = adoptcli.AdoptAs(first.Location, firstMachine, []string{boardengine.RootSubject}, false, true)
	if err == nil {
		t.Fatalf("the first clone's --drop-local over a live pair ended ok, want the held-back stop")
	}
	if refused := slugOutcomeOf(t, got, taskSlug); !strings.Contains(refused.Error, "--keep-local") || got.PrimeRecorded {
		t.Errorf("--drop-local over a live pair = %+v, prime recorded = %v, want a refusal naming --keep-local and the prime left", refused, got.PrimeRecorded)
	}

	got, err = adoptcli.AdoptAs(first.Location, firstMachine, []string{boardengine.RootSubject}, true, false)
	if err != nil {
		t.Fatalf("the first clone's --keep-local: %v", err)
	}
	if kept := slugOutcomeOf(t, got, taskSlug); kept.Stopped || !got.PrimeRecorded {
		t.Errorf("--keep-local = %+v, prime recorded = %v, want no stop and the prime recorded", kept, got.PrimeRecorded)
	}
	owners, err = boardengine.ReadOwners(first.BoardDir())
	if err != nil || owners.Prime != firstMachine {
		t.Errorf("first clone's owners = %+v, %v; want the prime owned by %s again", owners, err, firstMachine)
	}
	if firstRecord, found, err := adoptrecord.Read(first.Path); err != nil || !found || firstRecord.State != adoptrecord.StateDone || !firstRecord.Request.KeepLocal || firstRecord.Request.DropLocal {
		t.Errorf("first clone's adopt record = %+v, found %v, %v; want it done with --keep-local alone", firstRecord, found, err)
	}
}

// slugOutcomeOf returns the outcome of slug in the verb's result, failing the test when the result lacks it.
func slugOutcomeOf(t *testing.T, got adoptcli.Outcome, slug string) adoptcli.SlugOutcome {
	t.Helper()
	for _, entry := range got.Slugs {
		if entry.Slug == slug {
			return entry
		}
	}
	t.Fatalf("outcome has no entry for %q: %+v", slug, got.Slugs)
	return adoptcli.SlugOutcome{}
}

func TestCommand_RunsFromABeam(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	boardkit.InstallReplay(t)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{Beam: true})
	hubforge.AddPairWith(t, h, "p", fabricengine.AddOptions{})
	primeName, err := fabricengine.PrimeName(h.Location)
	if err != nil {
		t.Fatalf("prime name: %v", err)
	}

	run := func(cwd string) string {
		var out bytes.Buffer
		adoptcli.RunCLIIn(cwd, &out, []string{"ghost"})
		return out.String()
	}

	envelope.RequireErr(t, run(h.PairCodeWorktree("b")), "is not on the board")
	envelope.RequireErr(t, run(h.PairCodeWorktree("p")), `"`+primeName+`"`)
}

func TestCommand_HereAdoptsTheSubjectOfItsWorktree(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	boardkit.InstallReplay(t)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{Beam: true})
	machine, err := machinename.Name()
	if err != nil {
		t.Fatalf("machine name: %v", err)
	}
	board, err := boardengine.OpenHub(h.Path)
	if err != nil {
		t.Fatalf("open the hub's board: %v", err)
	}
	if _, err := board.UpsertTask(map[string]any{"slug": "b", "title": "b", "kind": "project", "labels": []string{"bug"}}); err != nil {
		t.Fatalf("seed the beam project: %v", err)
	}
	if _, err := boardengine.SetOwners(h.BoardDir(), boardengine.OwnerRequest{Machine: machine, Slugs: []string{"b"}, Beam: true}); err != nil {
		t.Fatalf("record this machine as the beam's owner: %v", err)
	}

	tests := []struct {
		name        string
		cwd         string
		wantSubject string
	}{
		{"FromThePrimeTheSubjectIsRoot", h.PrimeWorktree(), boardengine.RootSubject},
		{"FromABeamTheSubjectIsTheBeam", h.PairCodeWorktree("b"), "b"},
	}
	for _, tt := range tests {
		var out bytes.Buffer
		exit := adoptcli.RunCLIIn(tt.cwd, &out, []string{"--here"})
		got := envelope.RequireOK(t, out.String())
		if exit != 0 || got.Raw["outcome"] != "owned_here" || got.Raw["subject"] != tt.wantSubject {
			t.Errorf("%s: lym adopt --here = exit %d, envelope %+v; want exit 0, outcome owned_here and subject %q", tt.name, exit, got, tt.wantSubject)
		}
		if _, found, err := adoptrecord.Read(h.Path); found || err != nil {
			t.Errorf("%s: adopt record found %v, error %v; want none written", tt.name, found, err)
		}
	}
}

func TestCheckCmd_FromABeamExitsZeroWhileOneRuns(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	boardkit.InstallReplay(t)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{Beam: true})
	scratch := filepath.Join(fabricengine.AdoptScratchDir(h.Path), "root")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	held, err := lock.AcquireWriteLock(filepath.Join(scratch, "check.lock"))
	if err != nil {
		t.Fatalf("hold the check lock: %v", err)
	}
	t.Cleanup(func() { _ = held.Release() })

	var out bytes.Buffer
	exit := adoptcli.RunCLIIn(h.PairCodeWorktree("b"), &out, []string{"check", "root"})

	got := envelope.RequireOK(t, out.String())
	if exit != 0 || got.Raw["already_running"] != true {
		t.Errorf("lym adopt check root from a beam = exit %d, envelope %+v; want exit 0 and already_running true", exit, got)
	}
	if _, err := os.Stat(filepath.Join(scratch, "check.json")); !os.IsNotExist(err) {
		t.Errorf("Stat(check.json) error = %v; want no record written", err)
	}
}
