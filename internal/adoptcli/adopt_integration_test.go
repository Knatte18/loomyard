//go:build tier1

// adopt_integration_test.go drives the takeover over two hubforge clones of one remote: the second clone adopts root and so moves the run of a task only the first holds, and the first adopts it back over local commits and a live pair.

package adoptcli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/adoptcli"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

const (
	firstMachine  = "otherbox"
	secondMachine = "secondbox"
	taskSlug      = "alpha"
)

// holdOwnerPause records a live owner pause of the slug's own scope in the hub's pause record.
func holdOwnerPause(t *testing.T, location *lyxcwd.Location) {
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
	t.Parallel()
	boardkit.InstallReplay(t)

	first := hubforge.NewHub(t, ".")
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

	// The reconcile judges only pairs with a batten run record, and an adopt moves only a run whose record is not done, so the first clone records a running one before the second clone exists.
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
	holdOwnerPause(t, second.Location)

	// The task is moved with root, which the first machine owns, so the second clone adopts root and records no owner of the task's own.
	got, err := adoptcli.AdoptAs(second.Location, secondMachine, []string{boardengine.RootSubject}, false, false)
	if err != nil {
		t.Fatalf("the second clone's adopt: %v", err)
	}
	adopted := slugOutcomeOf(t, got, taskSlug)
	// A test binary spawns no first pass, so the resume is attempted and refused after the pair is added and the pause lifted.
	if !adopted.PairAdded || adopted.OwnerRecorded || !adopted.PauseLifted || !strings.HasPrefix(adopted.Error, "resume the run:") || !got.PrimeRecorded {
		t.Errorf("second clone's outcome = %+v, prime recorded = %v, want the pair added, the pause lifted, the resume attempted, no owner of the task's own and the prime recorded", adopted, got.PrimeRecorded)
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
	task, err := lyxcwd.ResolveWorktree(pair)
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
	if err != nil {
		t.Fatalf("the first clone's adopt back: %v", err)
	}
	if stopped := slugOutcomeOf(t, got, taskSlug); !stopped.Stopped || !stopped.Live || !strings.Contains(stopped.WayForward, "`lyx adopt root --keep-local`") || got.PrimeRecorded {
		t.Errorf("adopt back = %+v, prime recorded = %v, want a stop on the live pair naming both ways forward under root and the prime left", stopped, got.PrimeRecorded)
	}

	got, err = adoptcli.AdoptAs(first.Location, firstMachine, []string{boardengine.RootSubject}, false, true)
	if err != nil {
		t.Fatalf("the first clone's --drop-local: %v", err)
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
	t.Parallel()
	boardkit.InstallReplay(t)

	h := hubforge.NewHub(t, ".")
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
