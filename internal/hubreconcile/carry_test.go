// carry_test.go covers the once-per-machine carry of the board entries' beam settings into the hub's worktrees.

package hubreconcile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// carryHub lays out a board dir holding boardJSON, and a prime and one worktree called task-a, under a temp dir.
// It returns the walk geometry and the hub's code worktrees, the prime first.
func carryHub(t *testing.T, boardJSON string) (Geometry, []fabricengine.CodeWorktree) {
	t.Helper()
	hub := t.TempDir()
	geom := Geometry{BoardDir: filepath.Join(hub, "board")}
	prime := filepath.Join(hub, "prime")
	task := filepath.Join(hub, "task-a")
	for _, dir := range []string{configengine.ScratchDir(geom.BoardDir), filepath.Join(prime, "_lym"), filepath.Join(task, "_lym")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(geom.BoardDir, "board.json"), []byte(boardJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(board.json) = %v; want nil", err)
	}
	return geom, []fabricengine.CodeWorktree{{Path: prime, Anchor: prime, Main: true}, {Path: task, Anchor: task}}
}

func TestCarryBeamTaskSettings_RunsOncePerMachine(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	geom, worktrees := carryHub(t, `{"version":1,"entries":[{"id":0,"slug":"task-a","title":"A","kind":"task",`+
		`"run_settings":{"beam":{"pull_request":"merge-in"}}}]}`)
	local := configengine.LocalFile(worktrees[1].Anchor, "beam")

	carryBeamTaskSettings(geom, worktrees, "build")

	if got, err := os.ReadFile(local); err != nil || !strings.Contains(string(got), "pull_request: merge-in") {
		t.Fatalf("local beam.yaml = %q, %v after the first carry; want it to set pull_request: merge-in", got, err)
	}
	if _, err := os.Stat(geom.carriedMarker(dobbyengine.PerTaskSettingsReinterpretationID)); err != nil {
		t.Errorf("Stat(marker) = %v; want the marker written", err)
	}

	if err := os.Remove(local); err != nil {
		t.Fatalf("Remove(%q) = %v; want nil", local, err)
	}
	carryBeamTaskSettings(geom, worktrees, "build")

	if _, err := os.Stat(local); !os.IsNotExist(err) {
		t.Errorf("Stat(local beam.yaml) = %v after the second carry; want the file still absent beside the marker", err)
	}
}

func TestCarryBeamTaskSettings_UnreadableBoardLeavesNoMarker(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	geom, worktrees := carryHub(t, `{not json`)

	carryBeamTaskSettings(geom, worktrees, "build")

	if _, err := os.Stat(geom.carriedMarker(dobbyengine.PerTaskSettingsReinterpretationID)); !os.IsNotExist(err) {
		t.Errorf("Stat(marker) = %v; want no marker, so the next walk tries again", err)
	}
}
