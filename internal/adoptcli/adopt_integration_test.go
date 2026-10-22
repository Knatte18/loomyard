//go:build integration

// adopt_integration_test.go drives the takeover over two hubforge clones of one remote: the second clone adopts a task only the first holds, and the first adopts it back over local commits and a live pair.

package adoptcli_test

import (
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
	if _, err := boardengine.SetOwners(first.BoardDir(), boardengine.OwnerRequest{Machine: firstMachine, Slugs: []string{taskSlug}, Prime: true, Takeover: true}); err != nil {
		t.Fatalf("record the first machine as owner: %v", err)
	}

	second := hubforge.CloneSecond(t, first)
	holdOwnerPause(t, second.Location)

	got, err := adoptcli.AdoptAs(second.Location, secondMachine, []string{taskSlug}, false, false)
	if err != nil {
		t.Fatalf("the second clone's adopt: %v", err)
	}
	adopted := got.Slugs[0]
	if !adopted.PairAdded || !adopted.OwnerRecorded || !adopted.PauseLifted || adopted.Run != "none" || adopted.Error != "" {
		t.Errorf("second clone's outcome = %+v, want the pair added, the owner recorded, the pause lifted and no run", adopted)
	}
	if _, err := os.Stat(second.PairCodeWorktree(taskSlug)); err != nil {
		t.Errorf("the second clone has no pair worktree for %s: %v", taskSlug, err)
	}
	owners, err := boardengine.ReadOwners(second.BoardDir())
	if err != nil || owners.Entries[taskSlug].Owner != secondMachine {
		t.Errorf("second clone's owners = %+v, %v; want %s owned by %s", owners, err, taskSlug, secondMachine)
	}

	// The reconcile judges only pairs with a batten run record, so the first clone records one, finished so that no run is left to resume.
	status, err := json.Marshal(shedengine.Status{CurrentProducer: "Worktree-Teardown", State: shedengine.StateDone, History: []shedengine.HistoryEntry{}})
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

	got, err = adoptcli.AdoptAs(first.Location, firstMachine, []string{taskSlug}, false, false)
	if err != nil {
		t.Fatalf("the first clone's adopt back: %v", err)
	}
	if stopped := got.Slugs[0]; !stopped.Stopped || !stopped.Live || !strings.Contains(stopped.WayForward, "--keep-local") || stopped.OwnerRecorded {
		t.Errorf("adopt back = %+v, want a stop on the live pair naming both ways forward", stopped)
	}

	got, err = adoptcli.AdoptAs(first.Location, firstMachine, []string{taskSlug}, false, true)
	if err != nil {
		t.Fatalf("the first clone's --drop-local: %v", err)
	}
	if refused := got.Slugs[0]; !strings.Contains(refused.Error, "--keep-local") || refused.OwnerRecorded {
		t.Errorf("--drop-local over a live pair = %+v, want a refusal naming --keep-local", refused)
	}

	got, err = adoptcli.AdoptAs(first.Location, firstMachine, []string{taskSlug}, true, false)
	if err != nil {
		t.Fatalf("the first clone's --keep-local: %v", err)
	}
	if kept := got.Slugs[0]; !kept.OwnerRecorded || kept.Stopped {
		t.Errorf("--keep-local = %+v, want the owner recorded", kept)
	}
	owners, err = boardengine.ReadOwners(first.BoardDir())
	if err != nil || owners.Entries[taskSlug].Owner != firstMachine {
		t.Errorf("first clone's owners = %+v, %v; want %s owned by %s again", owners, err, taskSlug, firstMachine)
	}
}
