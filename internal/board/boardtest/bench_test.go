// bench_test.go — no-git benchmarks for the core board commands.
//
// Benchmarks the pure Render plus upsert / get / list (via the CLI entrypoint)
// and the Board facade across board sizes of 10/100/1000 tasks, with git skipped
// (BOARD_SKIP_GIT=1) so they measure board logic + file I/O only. Also defines
// seedWiki, the task-seeding helper shared across this package.

package boardtest

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Knatte18/loomyard/internal/board"
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
	lyxDir := filepath.Join(dir, "_lyx")
	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
		tb.Fatalf("mkdir _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		tb.Fatalf("mkdir _lyx/config: %v", err)
	}
	configPath := filepath.Join(configDir, "board.yaml")
	if err := os.WriteFile(configPath, []byte("path: board\nhome: Home.md\nsidebar: _Sidebar.md\nproposal_prefix: proposal-\n"), 0o644); err != nil {
		tb.Fatalf("write board.yaml: %v", err)
	}

	// Create board directory and tasks.json
	boardDir := filepath.Join(dir, "board")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		tb.Fatalf("mkdir board: %v", err)
	}

	tasks := make([]board.Task, n)
	for i := range tasks {
		tasks[i] = board.Task{
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
			tasks := make([]board.Task, n)
			for i := range tasks {
				body := ""
				if i%4 == 0 {
					body = "proposal body for task " + strconv.Itoa(i)
				}
				tasks[i] = board.Task{
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
				if _, err := board.Render(tasks, board.Outputs{Home: "Home.md", Sidebar: "_Sidebar.md", ProposalPrefix: "proposal-"}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkUpsert measures a full "upsert" command through the CLI entrypoint:
// JSON parse → dispatch → lock → load → mutate → render all tasks → write files.
// It updates an existing task so the per-op work is stable across iterations.
// CLI-bench numbers now include the added os.Getwd() + LoadConfig cost from cwd-based config.
func BenchmarkUpsert(b *testing.B) {
	b.Setenv("BOARD_SKIP_GIT", "1")
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			dir := seedWiki(b, n)
			b.Chdir(dir)
			args := []string{"upsert", `{"slug":"task-0","title":"Updated"}`}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if code := board.RunCLI(io.Discard, args); code != 0 {
					b.Fatalf("RunCLI upsert exit %d", code)
				}
			}
		})
	}
}

// BenchmarkGet measures a "get" command: the read path (load tasks.json, look up
// one task by slug). No render, no write.
func BenchmarkGet(b *testing.B) {
	b.Setenv("BOARD_SKIP_GIT", "1")
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			dir := seedWiki(b, n)
			b.Chdir(dir)
			args := []string{"get", `{"id_or_slug":"task-0"}`}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if code := board.RunCLI(io.Discard, args); code != 0 {
					b.Fatalf("RunCLI get exit %d", code)
				}
			}
		})
	}
}

// BenchmarkList measures a "list" command: load all tasks, compute layers and
// has_proposal, and serialise the brief view.
func BenchmarkList(b *testing.B) {
	b.Setenv("BOARD_SKIP_GIT", "1")
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			dir := seedWiki(b, n)
			b.Chdir(dir)
			args := []string{"list"}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if code := board.RunCLI(io.Discard, args); code != 0 {
					b.Fatalf("RunCLI list exit %d", code)
				}
			}
		})
	}
}

// BenchmarkUpsertFacade measures Board.UpsertTask directly, bypassing the CLI's
// flag parsing and JSON (un)marshalling. The gap to BenchmarkUpsert is the
// per-command CLI overhead.
func BenchmarkUpsertFacade(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			dir := seedWiki(b, n)
			cfg := board.Config{Path: filepath.Join(dir, "board"), Home: "Home.md", Sidebar: "_Sidebar.md", ProposalPrefix: "proposal-", SkipGit: true}
			w := board.New(cfg)
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
