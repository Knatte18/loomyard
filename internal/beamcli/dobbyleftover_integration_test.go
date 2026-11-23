// dobbyleftover_integration_test.go pins tearDownLeftovers over a hubforge hub.
// It covers the teardown of a landed pair and of a cancelled pair that kept nothing.

package beamcli

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// readHubWorkspace regenerates the hub workspace file and returns its content.
func readHubWorkspace(t *testing.T, h *hubforge.Hub) string {
	t.Helper()
	path, err := fabricengine.RegenerateHubWorkspace(fabricengine.NewMutations(h.Path), h.Location)
	if err != nil {
		t.Fatalf("RegenerateHubWorkspace() error = %v; want nil", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the hub workspace file: %v", err)
	}
	return string(content)
}

func TestTearDownLeftovers_RemovesALandedAndACancelledPairAndTheirWorkspaceFolders(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const landed, cancelled = "landed-pair", "cancelled-pair"
	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, landed)
	hubforge.AddPair(t, h, cancelled)
	prime := h.Location

	cfg, err := boardengine.LoadConfig(configengine.Dirs{Board: h.BoardDir()}, "board")
	if err != nil {
		t.Fatalf("load board config: %v", err)
	}
	cfg.Path = h.BoardDir()
	board := boardengine.New(cfg)
	if _, err := board.UpsertTask(map[string]any{"slug": landed, "title": landed, "kind": "task", "labels": []string{"bug"}}); err != nil {
		t.Fatalf("seed the board entry of %s: %v", landed, err)
	}
	claim := "running · Run-Shed"
	if err := board.SetStatus(landed, &claim); err != nil {
		t.Fatalf("claim %s on the board: %v", landed, err)
	}

	if err := shedrun.WriteSeed(prime, cancelled, shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo}); err != nil {
		t.Fatalf("write the seed of %s: %v", cancelled, err)
	}
	record := shedengine.Status{
		CurrentProducer: "Run-Shed",
		State:           shedengine.StateCancelled,
		History:         []shedengine.HistoryEntry{},
		Cancel: &shedengine.CancelIntent{
			StartedAt: "2026-11-15T09:00:00Z",
			Machine:   "this-machine",
			Remove:    true,
			Finish:    &shedengine.CancelFinish{EndedAt: "2026-11-15T09:30:00Z", Remove: true},
		},
	}
	if err := os.MkdirAll(shedrun.RunDir(prime, cancelled), 0o755); err != nil {
		t.Fatalf("create the run directory of %s: %v", cancelled, err)
	}
	if err := state.WriteJSON(shedrun.StatusFile(prime, cancelled), shedrun.StatusLock(prime, cancelled), record); err != nil {
		t.Fatalf("write the record of %s: %v", cancelled, err)
	}
	rels := []string{shedrun.SeedRel(prime, cancelled), shedrun.StatusRel(prime, cancelled)}
	if _, committed, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), prime, rels, "test: record the cancelled run", fabricengine.EnvSyncOptions()); err != nil || !committed {
		t.Fatalf("commit the record of %s: committed=%v err=%v", cancelled, committed, err)
	}
	workspace := readHubWorkspace(t, h)
	for _, slug := range []string{landed, cancelled} {
		if !strings.Contains(workspace, slug) {
			t.Fatalf("hub workspace before the teardown = %s; want it to list %s", workspace, slug)
		}
	}

	got := <-tearDownLeftovers(prime, []dobbyengine.LeftoverPair{
		{Slug: landed, Kind: dobbyengine.LeftoverLanded},
		{Slug: cancelled, Kind: dobbyengine.LeftoverCancelled},
	}, fabricengine.ReconcileOptions{})

	if want := map[string]error{landed: nil, cancelled: nil}; !reflect.DeepEqual(got, want) {
		t.Errorf("tearDownLeftovers() = %v; want %v", got, want)
	}
	for _, slug := range []string{landed, cancelled} {
		for _, dir := range []string{h.PairCodeWorktree(slug), h.PairRecordsSibling(slug)} {
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Errorf("stat %s = %v; want the pair of %s removed", dir, err, slug)
			}
		}
	}
	if _, err := os.Stat(shedrun.RunDir(prime, cancelled)); !os.IsNotExist(err) {
		t.Errorf("stat the run directory of %s = %v; want the batten record removed with its pair", cancelled, err)
	}
	task, found, err := board.GetTask(landed)
	if err != nil || !found || task.Status == nil || *task.Status != boardengine.StatusDone {
		t.Errorf("board entry of %s = %+v, found %v, err %v; want its status done", landed, task, found, err)
	}
	workspace = readHubWorkspace(t, h)
	for _, slug := range []string{landed, cancelled} {
		if strings.Contains(workspace, slug) {
			t.Errorf("hub workspace %s after the teardown = %s; want it to list no %s", fabricengine.HubWorkspacePath(prime), workspace, slug)
		}
	}
}
