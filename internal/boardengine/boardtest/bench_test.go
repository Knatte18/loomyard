// bench_test.go — offline (Tier 1) benchmarks for the core board commands.
//
// Benchmarks the pure Render and the Board facade (Board.UpsertTask) across board
// sizes of 10/100/1000 tasks, with git skipped (BOARD_SKIP_GIT=1) so they measure
// board logic + file I/O only and stay in the default offline loop. Also defines
// seedWiki, the task-seeding helper shared across this package. The CLI-driven
// upsert/get/list benchmarks require a real git repo (config resolution) and so
// live behind `//go:build integration` in bench_cli_test.go.

package boardtest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// benchSizes is the set of board sizes (number of tasks already in tasks.json)
// each benchmark is run against. Every write re-renders the whole board, so cost
// is expected to grow with size — these sizes make that scaling visible.
var benchSizes = []int{10, 100, 1000}

// seedWiki writes a tasks.json with n independent (dependency-free) tasks into a
// fresh temp dir, seeded with _lyx/config/board.yaml for config, and returns the cwd path.
// Callers must set BOARD_SKIP_GIT=1 so the benchmark measures board logic + file I/O,
// not git push latency. It takes a testing.TB so both benchmarks and concurrency tests
// can use it. For direct facade tests (not CLI), construct Board from the config's Path
// which is resolved from board.yaml.
func seedWiki(tb testing.TB, n int) string {
	tb.Helper()

	dir := tb.TempDir()

	// Create _lyx and _lyx/config directories with board.yaml config
	lyxDir := filepath.Join(dir, hubgeometry.LyxDirName)
	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
		tb.Fatalf("mkdir _lyx: %v", err)
	}
	configDir := hubgeometry.ConfigDir(dir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		tb.Fatalf("mkdir _lyx/config: %v", err)
	}
	configPath := hubgeometry.ConfigFile(dir, "board")
	if err := os.WriteFile(configPath, []byte("path: board\nhome: Home.md\nsidebar: _Sidebar.md\nproposal_prefix: proposal-\n"), 0o644); err != nil {
		tb.Fatalf("write board.yaml: %v", err)
	}

	// Create board directory and tasks.json
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
			DependsOn: []string{},
			Brief:     "brief for task " + strconv.Itoa(i),
		}
	}

	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		tb.Fatalf("marshal seed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(boardDir, "tasks.json"), data, 0o644); err != nil {
		tb.Fatalf("write seed: %v", err)
	}
	// Return the cwd (parent) for CLI benchmarks, which will call b.Chdir
	return dir
}

// BenchmarkRender measures the pure Render function (tasks → markdown content),
// no I/O. A quarter of the tasks have a body so proposal-*.md generation is
// exercised. Render runs once inside every write (writeOp).
func BenchmarkRender(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			tasks := make([]boardengine.Task, n)
			for i := range tasks {
				body := ""
				if i%4 == 0 {
					body = "proposal body for task " + strconv.Itoa(i)
				}
				tasks[i] = boardengine.Task{
					ID:        i,
					Slug:      "task-" + strconv.Itoa(i),
					Title:     "Task " + strconv.Itoa(i),
					DependsOn: []string{},
					Brief:     "brief for task " + strconv.Itoa(i),
					Body:      body,
				}
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := boardengine.Render(tasks, boardengine.Outputs{Home: "Home.md", Sidebar: "_Sidebar.md", ProposalPrefix: "proposal-"}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// The CLI-driven command benchmarks (Upsert/Get/List) live in the
// integration-tagged bench_cli_test.go: they drive boardcli.RunCLI, whose config
// resolution requires a real git repo, so they spawn git and belong in Tier 2.

// BenchmarkUpsertFacade measures Board.UpsertTask directly, bypassing the CLI's
// flag parsing and JSON (un)marshalling. The gap to BenchmarkUpsert is the
// per-command CLI overhead.
func BenchmarkUpsertFacade(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			dir := seedWiki(b, n)
			cfg := boardengine.Config{Path: filepath.Join(dir, "board"), Home: "Home.md", Sidebar: "_Sidebar.md", ProposalPrefix: "proposal-", SkipGit: true}
			w := boardengine.New(cfg)
			fields := map[string]any{"slug": "task-0", "title": "Updated"}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := w.UpsertTask(fields); err != nil {
					b.Fatalf("UpsertTask: %v", err)
				}
			}
		})
	}
}
