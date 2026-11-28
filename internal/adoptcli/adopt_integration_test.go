// adopt_integration_test.go drives the foreground adopt over two hubforge clones of one remote:
// a task's run moves from the first clone's machine to the second clone, which runs the adopt check green.

package adoptcli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/adoptcli"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/runfsm"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	firstMachine = "otherbox"
	taskSlug     = "alpha"
)

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

	// The adopt moves only a run whose record at origin is not done, on the prime's records branch and on the pair's own.
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
	if _, err := fabricengine.PushPairAnchored(first.Location, fabricengine.EnvSyncOptions(), fabricengine.PairPush{LockWait: fabricengine.StatusPushLockWait}); err != nil {
		t.Fatalf("push the run record: %v", err)
	}
	_, pairStatusRel, err := shedrun.TreeRunFileRels(taskSlug)
	if err != nil {
		t.Fatalf("TreeRunFileRels(%q): %v", taskSlug, err)
	}
	pairRecords := first.PairRecordsSibling(taskSlug)
	gitkit.CommitFile(t, pairRecords, pairStatusRel, string(status), "test: the pair's own run record")
	gitkit.MustRun(t, pairRecords, "git", "push", "origin", gitkit.CurrentBranch(t, pairRecords))

	second := hubforge.CloneSecond(t, first)
	seedAdoptGate(t, second)
	firstState := runStateOf(t, first)

	machine, err := machinename.Name()
	if err != nil {
		t.Fatalf("machine name: %v", err)
	}
	// The task is moved with root, which the first machine owns, so the second clone adopts root.
	got, err := adoptcli.AdoptAs(second.Location, machinename.Name, []string{boardengine.RootSubject}, false)
	if err != nil {
		t.Fatalf("the second clone's adopt: %v", err)
	}

	moved := slugOutcomeOf(t, got, taskSlug)
	if !moved.PairAdded || moved.Run != "taken over" || moved.OwnerRecorded || moved.Error != "" || !got.PrimeRecorded || got.PrimeCheck != "green" {
		t.Errorf("second clone's outcome = %+v, prime recorded = %v, prime check = %q; want the pair added, the run taken over, no owner of the task's own and a green check of root", moved, got.PrimeRecorded, got.PrimeCheck)
	}
	if got.Hold != nil {
		t.Errorf("second clone's outcome holds %+v; want the hold cleared by the green check", got.Hold)
	}
	owners, err := boardengine.ReadOwners(second.BoardDir())
	if err != nil || owners.Prime != machine || owners.Entries[taskSlug].Owner != "" || len(owners.Holds) != 0 {
		t.Errorf("second clone's owners = %+v, %v; want the prime owned by %s, no owner on %s and no hold left", owners, err, machine, taskSlug)
	}
	if _, err := os.Stat(second.PairCodeWorktree(taskSlug)); err != nil {
		t.Errorf("the second clone has no pair worktree for %s: %v", taskSlug, err)
	}
	gitkit.MustRun(t, second.PrimeRecords(), "git", "fetch", "origin")
	entry, found, err := fabricengine.ReadUpstreamTakeover(second.Location, second.PrimeRecords(), taskSlug)
	if err != nil || !found || entry.Machine != machine || entry.Subject != boardengine.RootSubject {
		t.Errorf("origin's takeover entry of %s = %+v, found %v, %v; want one naming %s and root", taskSlug, entry, found, err, machine)
	}
	if secondState := runStateOf(t, second); secondState != firstState {
		t.Errorf("the run reads %v on the second clone, want the %v it read on the first", secondState, firstState)
	}
}

// seedAdoptGate commits a gate config to h's Bolt whose adopt key runs one tier that holds no test.
// The check then goes green.
func seedAdoptGate(t *testing.T, h *hubforge.Hub) {
	t.Helper()
	tier := map[string]any{"command": "true", "all_targets": ".", "timeout_min": 1, "counter": "go-json"}
	gate, err := yaml.Marshal(map[string]any{
		"slots":  1,
		"scoper": "none",
		"tiers":  map[string]any{"tier0": tier},
		"gates": map[string]any{
			"finalize": map[string]any{"tiers": []int{0}, "scope": "all"},
			"adopt":    map[string]any{"tiers": []int{0}, "scope": "all"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	gitkit.CommitFile(t, h.BoardDir(), configengine.ConfigFileRel("gate"), string(gate), "fixture: a gate whose adopt tier holds no test")
}

// runStateOf returns the state of the task's run as the hub's records and machine-local facts read.
func runStateOf(t *testing.T, h *hubforge.Hub) runfsm.State {
	t.Helper()
	records, _, err := pairteardown.ReadRun(h.Location, taskSlug)
	if err != nil {
		t.Fatalf("read the run of %s: %v", taskSlug, err)
	}
	return runfsm.Derive(records).State
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
		clihelp.ExecuteIn(adoptcli.Command(), cwd, &out, []string{"ghost"})
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
		exit := clihelp.ExecuteIn(adoptcli.Command(), tt.cwd, &out, []string{"--here"})
		got := envelope.RequireOK(t, out.String())
		if exit != 0 || got.Raw["outcome"] != "owned_here" || got.Raw["subject"] != tt.wantSubject {
			t.Errorf("%s: lym adopt --here = exit %d, envelope %+v; want exit 0, outcome owned_here and subject %q", tt.name, exit, got, tt.wantSubject)
		}
		if owners, err := boardengine.ReadOwners(h.BoardDir()); err != nil || len(owners.Holds) != 0 {
			t.Errorf("%s: the hub's holds = %+v, %v; want none written", tt.name, owners.Holds, err)
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

	here, err := machinename.ReadIdentity()
	if err != nil {
		t.Fatalf("read this machine's identity: %v", err)
	}
	hold := boardengine.Hold{Kind: boardengine.HoldKindAdopt, Machine: here.Name, At: time.Now(), Subjects: []boardengine.HoldSubject{{Subject: boardengine.RootSubject}}}
	if _, err := boardengine.WriteHold(h.BoardDir(), hold); err != nil {
		t.Fatalf("plant this machine's hold over root: %v", err)
	}

	var out bytes.Buffer
	exit := clihelp.ExecuteIn(adoptcli.Command(), h.PairCodeWorktree("b"), &out, []string{"check", "root"})

	got := envelope.RequireOK(t, out.String())
	if exit != 0 || got.Raw["already_running"] != true {
		t.Errorf("lym adopt check root from a beam = exit %d, envelope %+v; want exit 0 and already_running true", exit, got)
	}
	if _, err := os.Stat(filepath.Join(scratch, "check.json")); !os.IsNotExist(err) {
		t.Errorf("Stat(check.json) error = %v; want no record written", err)
	}
}
