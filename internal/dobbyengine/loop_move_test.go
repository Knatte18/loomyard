// loop_move_test.go pins the loop's gate on an anchor change.
// While a move record stands, the loop spawns nothing; the first tick after it is gone starts the held-back work.

package dobbyengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRun_NoPassWhileAMoveStands drives ticks with a move record present and then removed.
// While the record stands, no pass, task or tidy agent starts, though a landing is still counted.
// The next tick starts the due pass, the ready task and the tidy agent.
func TestRun_NoPassWhileAMoveStands(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	h := newLoopHarness(t, 3)
	record := filepath.Join(t.TempDir(), "anchor-move.json")
	if err := os.WriteFile(record, []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) = %v; want nil", record, err)
	}
	stencils := filepath.Join(t.TempDir(), "stencils")
	stencilkit.SeedInto(t, stencils)
	h.geom.MoveRecords = []string{record}
	h.geom.TidyDir = filepath.Join(filepath.Dir(h.geom.StatePath), TidyDirName)

	passes, fullPasses := map[string]int{}, map[string]int{}
	shuttle := &tidyShuttleFake{}
	landingReads := 0
	h.deps.Runs = func() ([]RunView, error) { return []RunView{runningRun("due")}, nil }
	h.deps.SpawnPass = func(slug string, continuePass, _ bool) (<-chan PassExit, error) {
		passes[slug]++
		if !continuePass {
			fullPasses[slug]++
		}
		exit := make(chan PassExit, 1)
		exit <- PassExit{}
		return exit, nil
	}
	h.deps.StatusExists = func(string) (bool, error) { return true, nil }
	h.deps.PauseCovers = func(string) (bool, error) { return false, nil }
	h.deps.Tasks = func() ([]TaskView, error) {
		return []TaskView{{Slug: "fresh", Kind: boardengine.KindTask, Parent: h.geom.WorktreeName}}, nil
	}
	h.deps.LoadConfig = func() (Config, error) { return Config{MaxRuns: 5, BoardTidyEvery: 1, BoardTidyTimeoutMin: 30}, nil }
	// The first read baselines the landings, so the one landing shows from the second tick on.
	h.deps.Landings = func() ([]string, error) {
		landingReads++
		if landingReads == 1 {
			return nil, nil
		}
		return []string{"landed"}, nil
	}
	h.deps.TidyShuttle = shuttle
	h.deps.TidyModel = shuttleengine.ModelChoice{Engine: "b", Model: "sonnet"}
	h.deps.StencilsDir = stencils
	h.deps.NearestOrch = func() (string, error) { return "ly:orch", nil }

	h.perTick = func(n int) error {
		switch n {
		case 1, 2:
			if len(passes) != 0 || len(shuttle.started) != 0 {
				t.Errorf("after tick %d with the record standing, passes = %v and tidy agents = %d; want none", n, passes, len(shuttle.started))
			}
			if n == 2 {
				if err := os.Remove(record); err != nil {
					t.Errorf("Remove(%s) = %v; want nil", record, err)
				}
			}
		case 3:
			if passes["due"] != 1 || passes["fresh"] != 1 || len(shuttle.started) != 1 {
				t.Errorf("after tick 3 with the record gone, passes = %v and tidy agents = %d; want one pass of the due run, one first pass of the ready task and one tidy agent", passes, len(shuttle.started))
			}
		}
		return nil
	}

	if err := h.run(t); err != nil {
		t.Fatalf("Run() = %v; want nil", err)
	}
	if len(h.ticks) != 3 {
		t.Errorf("ticks = %d; want 3", len(h.ticks))
	}
	if fullPasses["due"] != 1 {
		t.Errorf("full passes of the due run = %d; want the full pass the dobby's start owed kept through the ticks the record stood", fullPasses["due"])
	}
}
