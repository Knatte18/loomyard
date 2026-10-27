// seedwiki_helper_test.go holds seedWiki and benchSizes, the board-seeding helper and the board sizes the benchmarks and concurrency tests of every tier share.
// It carries no tier tag so each tier's test binary compiles it.

package boardtest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// benchSizes is the set of board sizes (number of tasks already in board.json) each benchmark is run against.
// Every write re-renders the whole board, so cost is expected to grow with size — these sizes make that scaling visible.
var benchSizes = []int{10, 100, 1000}

// seedWiki writes a board.json with n independent (dependency-free) tasks into a fresh temp dir, seeded with _lym/config/board.yaml for config, and returns the cwd path.
// Callers must set BOARD_SKIP_GIT=1 so the benchmark measures board logic + file I/O, not git push latency.
// It takes a testing.TB so both benchmarks and concurrency tests can use it.
// For direct facade tests (not CLI), construct Board from the config's Path which is resolved from board.yaml.
func seedWiki(tb testing.TB, n int) string {
	tb.Helper()

	dir := tb.TempDir()

	// Create _lym and _lym/config directories with board.yaml config
	lymDir := filepath.Join(dir, lymdirs.LymDirName)
	if err := os.MkdirAll(lymDir, 0o755); err != nil {
		tb.Fatalf("mkdir _lym: %v", err)
	}
	configDir := configengine.ConfigDir(dir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		tb.Fatalf("mkdir _lym/config: %v", err)
	}
	configPath := configengine.ConfigFile(dir, "board")
	if err := os.WriteFile(configPath, []byte("path: board\nreadme: Home.md\ndesign_prefix: proposal-\ntypes: [bug, enhancement]\nlabels: [undecided]\n"), 0o644); err != nil {
		tb.Fatalf("write board.yaml: %v", err)
	}

	// Create board directory and board.json
	boardDir := filepath.Join(dir, "board")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		tb.Fatalf("mkdir board: %v", err)
	}

	tasks := make([]boardengine.Task, n)
	for i := range tasks {
		tasks[i] = boardengine.Task{
			ID:        i,
			Slug:      "task-" + strconv.Itoa(i),
			Title:     "Task " + strconv.Itoa(i),
			Kind:      boardengine.KindTask,
			Labels:    []string{"enhancement"},
			DependsOn: []string{},
			Brief:     "brief for task " + strconv.Itoa(i),
		}
	}

	data, err := json.MarshalIndent(map[string]any{"version": 1, "entries": tasks}, "", "  ")
	if err != nil {
		tb.Fatalf("marshal seed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(boardDir, "board.json"), data, 0o644); err != nil {
		tb.Fatalf("write seed: %v", err)
	}
	// Return the cwd (parent) for CLI benchmarks, which will call b.Chdir
	return dir
}
