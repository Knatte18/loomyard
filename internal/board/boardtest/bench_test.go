// bench_test.go — no-git benchmarks for the core wiki commands.
//
// Benchmarks the pure Render plus upsert / get / list (via the CLI entrypoint)
// and the Wiki facade across wiki sizes of 10/100/1000 tasks, with git skipped
// (WIKI_SKIP_GIT=1) so they measure wiki logic + file I/O only. Also defines
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

	"github.com/Knatte18/mhgo/internal/board"
)

// benchSizes is the set of wiki sizes (number of tasks already in tasks.json)
// each benchmark is run against. Every write re-renders the whole wiki, so cost
// is expected to grow with size — these sizes make that scaling visible.
var benchSizes = []int{10, 100, 1000}

// seedWiki writes a tasks.json with n independent (dependency-free) tasks into a
// fresh temp dir and returns its path. Callers must set WIKI_SKIP_GIT=1 so the
// benchmark measures wiki logic + file I/O, not git push latency. It takes a
// testing.TB so both benchmarks and concurrency tests can use it.
func seedWiki(tb testing.TB, n int) string {
	tb.Helper()

	dir := tb.TempDir()
	tasks := make([]wiki.Task, n)
	for i := range tasks {
		tasks[i] = wiki.Task{
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
	if err := os.WriteFile(filepath.Join(dir, "tasks.json"), data, 0o644); err != nil {
		tb.Fatalf("write seed: %v", err)
	}
	return dir
}

// BenchmarkRender measures the pure Render function (tasks → markdown content),
// no I/O. A quarter of the tasks have a body so proposal-*.md generation is
// exercised. Render runs once inside every write (writeOp).
func BenchmarkRender(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			tasks := make([]wiki.Task, n)
			for i := range tasks {
				body := ""
				if i%4 == 0 {
					body = "proposal body for task " + strconv.Itoa(i)
				}
				tasks[i] = wiki.Task{
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
				if _, err := wiki.Render(tasks); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkUpsert measures a full "upsert" command through the CLI entrypoint:
// JSON parse → dispatch → lock → load → mutate → render all tasks → write files.
// It updates an existing task so the per-op work is stable across iterations.
func BenchmarkUpsert(b *testing.B) {
	b.Setenv("WIKI_SKIP_GIT", "1")
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			dir := seedWiki(b, n)
			args := []string{"--wiki-path", dir, "upsert", `{"slug":"task-0","title":"Updated"}`}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if code := wiki.RunCLI(io.Discard, args); code != 0 {
					b.Fatalf("RunCLI upsert exit %d", code)
				}
			}
		})
	}
}

// BenchmarkGet measures a "get" command: the read path (load tasks.json, look up
// one task by slug). No render, no write.
func BenchmarkGet(b *testing.B) {
	b.Setenv("WIKI_SKIP_GIT", "1")
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			dir := seedWiki(b, n)
			args := []string{"--wiki-path", dir, "get", `{"id_or_slug":"task-0"}`}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if code := wiki.RunCLI(io.Discard, args); code != 0 {
					b.Fatalf("RunCLI get exit %d", code)
				}
			}
		})
	}
}

// BenchmarkList measures a "list" command: load all tasks, compute layers and
// has_proposal, and serialise the brief view.
func BenchmarkList(b *testing.B) {
	b.Setenv("WIKI_SKIP_GIT", "1")
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			dir := seedWiki(b, n)
			args := []string{"--wiki-path", dir, "list"}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if code := wiki.RunCLI(io.Discard, args); code != 0 {
					b.Fatalf("RunCLI list exit %d", code)
				}
			}
		})
	}
}

// BenchmarkUpsertFacade measures Wiki.UpsertTask directly, bypassing the CLI's
// flag parsing and JSON (un)marshalling. The gap to BenchmarkUpsert is the
// per-command CLI overhead.
func BenchmarkUpsertFacade(b *testing.B) {
	b.Setenv("WIKI_SKIP_GIT", "1")
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			dir := seedWiki(b, n)
			w := wiki.New(dir)
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
