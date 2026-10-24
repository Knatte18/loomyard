//go:build tier0

// bench_test.go — offline (tier 0) benchmarks for the core board commands.
//
// Benchmarks the pure Render and the Board facade (Board.UpsertTask) across board sizes of
// 10/100/1000 tasks, with git skipped (BOARD_SKIP_GIT=1) so they measure board logic + file I/O
// only and stay in the default offline loop.
// The CLI-driven upsert/get/list benchmarks require a real git repo (config resolution) and so live
// behind `//go:build integration` in bench_cli_test.go.

package boardtest

import (
	"fmt"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

// BenchmarkRender measures the pure Render function (tasks → markdown content), no I/O.
// A quarter of the tasks have a body so proposal-*.md generation is exercised.
// Render runs once inside every write (boardCriticalSection).
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
					Kind:      boardengine.KindTask,
					Labels:    []string{"enhancement"},
					DependsOn: []string{},
					Brief:     "brief for task " + strconv.Itoa(i),
					Body:      body,
				}
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := boardengine.Render(tasks, "", boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-"}, time.Now()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// The CLI-driven command benchmarks (Upsert/Get/List) live in the
// integration-tagged bench_cli_test.go: they drive boardcli.RunCLI, whose config
// resolution requires a real git repo, so they spawn git and belong in Tier 2.

// BenchmarkUpsertFacade measures Board.UpsertTask directly, bypassing the CLI's flag parsing and
// JSON (un)marshalling.
// The gap to BenchmarkUpsert is the per-command CLI overhead.
func BenchmarkUpsertFacade(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			dir := seedWiki(b, n)
			cfg := boardengine.Config{Path: filepath.Join(dir, "board"), Readme: "Home.md", DesignPrefix: "proposal-", Types: []boardengine.Label{{Name: "bug"}, {Name: "enhancement"}}, Labels: []boardengine.Label{{Name: "undecided"}}, SkipGit: true}
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
