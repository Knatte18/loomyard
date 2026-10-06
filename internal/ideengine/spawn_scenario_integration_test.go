//go:build integration

// spawn_scenario_integration_test.go drives Spawn and SpawnDriven as scenarios over one hubforge hub per anchor, so a hub is built once per anchor instead of once per check.
// Serial by design: every step swaps the package-level CodeLauncher, and the driven steps swap the logger output.

package ideengine

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// spawnStepRunner returns a runner of named steps over h.
// Every step starts with the repo-wide info/exclude restored to its state in a fresh hub, because Spawn appends to it and an earlier step's lines would otherwise satisfy a later step's exclude assertions.
func spawnStepRunner(t *testing.T, h *hubforge.Hub) func(name string, step func(t *testing.T)) bool {
	t.Helper()
	excludePath := sharedExcludePath(t, fabricengine.WorktreePath(h.Location, primeOf(t, h)))
	baseline, _ := os.ReadFile(excludePath)
	return func(name string, step func(t *testing.T)) bool {
		return t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(excludePath, baseline, 0o644); err != nil {
				t.Fatalf("restore info/exclude: %v", err)
			}
			step(t)
		})
	}
}

// TestSpawnScenario drives every Spawn and SpawnDriven check that does not depend on the hub's anchor against one hub at the repo root.
// Steps run in this order: the prime-touching steps follow the task-pair ones, and the tracked-tasks.json commit on the prime is last, because it leaves the prime's tasks.json tracked.
func TestSpawnScenario(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	step := spawnStepRunner(t, h)

	if !step("task pair stays clean through exclude with an unrelated gitignore", func(t *testing.T) {
		checkTaskPairStaysClean(t, h, "clean-unrelated", "build/\n", true)
	}) {
		return
	}
	if !step("task pair stays clean when the repo already ignores .vscode", func(t *testing.T) {
		checkTaskPairStaysClean(t, h, "clean-ignored", ".vscode/\n", false)
	}) {
		return
	}
	if !step("spawn overwrites tasks and keeps settings", func(t *testing.T) {
		checkOverwritesTasksKeepsSettings(t, h, "overwrite")
	}) {
		return
	}
	if !step("spawn skips a tracked .vscode on a task pair", func(t *testing.T) {
		checkSkipsTrackedVSCode(t, h, "tracked", false)
	}) {
		return
	}
	if !step("prime name failure opens the bare folder", func(t *testing.T) {
		checkPrimeNameFailureOpensBareFolder(t, h, "prime-name-failure")
	}) {
		return
	}
	if !step("task slug opens the bare folder", func(t *testing.T) {
		checkTaskSlugOpensBareFolder(t, h, "bare-folder")
	}) {
		return
	}
	if !step("driven spawn writes attach-only and excludes", func(t *testing.T) {
		checkDrivenWritesAttachOnlyAndExcludes(t, h, "driven-writes")
	}) {
		return
	}
	if !step("driven spawn overwrites tasks and keeps settings", func(t *testing.T) {
		checkDrivenOverwritesTasksKeepsSettings(t, h, "driven-overwrite")
	}) {
		return
	}
	if !step("driven spawn skips a tracked .vscode", func(t *testing.T) {
		checkDrivenSkipsTrackedVSCode(t, h, "driven-tracked")
	}) {
		return
	}
	if !step("prime writes the hub workspace", func(t *testing.T) {
		checkPrimeWritesHubWorkspace(t, h)
	}) {
		return
	}
	if !step("prime splices settings and regenerates", func(t *testing.T) {
		checkPrimeSplicesSettingsAndRegenerates(t, h)
	}) {
		return
	}
	if !step("prime settings read failure", func(t *testing.T) {
		checkPrimeSettingsReadFailure(t, h)
	}) {
		return
	}
	if !step("prime workspace survives topology verbs", func(t *testing.T) {
		checkPrimeWorkspaceSurvivesTopologyVerbs(t, h, "topology")
	}) {
		return
	}
	step("spawn skips a tracked .vscode on the prime", func(t *testing.T) {
		checkSkipsTrackedVSCode(t, h, "", true)
	})
}

// TestSpawnAnchoredScenario drives the checks whose result depends on the anchor against one hub anchored in a subdirectory.
// The steps depend on no earlier step's state.
func TestSpawnAnchoredScenario(t *testing.T) {
	h := hubforge.NewHub(t, "wts/some-task")
	step := spawnStepRunner(t, h)

	if !step("driven spawn writes attach-only and excludes", func(t *testing.T) {
		checkDrivenWritesAttachOnlyAndExcludes(t, h, "driven-writes")
	}) {
		return
	}
	step("prime writes the hub workspace", func(t *testing.T) {
		checkPrimeWritesHubWorkspace(t, h)
	})
}
