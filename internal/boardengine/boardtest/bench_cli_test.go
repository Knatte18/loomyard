//go:build integration

// bench_cli_test.go — integration-tier benchmarks for the board CLI commands.
//
// Upsert/Get/List drive boardcli.RunCLI, whose cwd-authoritative config
// resolution calls hubgeometry.Resolve (a `git rev-parse`), so the seeded temp
// dir must be a real git repo. That makes these benchmarks spawn git, so they
// belong behind `//go:build integration` (Tier 2), not in the default offline
// loop — and the git-init helper below uses exec.Command, a token the
// tier-purity guard bans from untagged test files. Run them with:
//
//	go test -tags integration -run '^$' -bench 'Upsert|Get|List' -benchmem ./internal/boardengine/boardtest
//
// BenchmarkRender and BenchmarkUpsertFacade stay untagged in bench_test.go:
// they never touch the CLI/git path and so still run in the offline tier.

package boardtest

import (
	"fmt"
	"io"
	"os/exec"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardcli"
)

// seedWikiRepo seeds a board (via seedWiki) and initialises a git repo at its
// root so hubgeometry.Resolve can find the hub — the CLI entrypoint's config
// resolution requires a repository, which a bare temp dir is not. Kept out of
// the untagged seedWiki so the Tier-1 concurrency test that shares seedWiki
// never spawns git.
func seedWikiRepo(tb testing.TB, n int) string {
	tb.Helper()
	dir := seedWiki(tb, n)
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "bench@lyx.test"},
		{"config", "user.name", "bench"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			tb.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return dir
}

// BenchmarkUpsert measures a full "upsert" command through the CLI entrypoint:
// JSON parse → dispatch → lock → load → mutate → render all tasks → write files.
// It updates an existing task so the per-op work is stable across iterations.
// CLI-bench numbers include the os.Getwd() + LoadConfig cost from cwd-based config.
func BenchmarkUpsert(b *testing.B) {
	b.Setenv("BOARD_SKIP_GIT", "1")
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			dir := seedWikiRepo(b, n)
			b.Chdir(dir)
			args := []string{"upsert", `{"slug":"task-0","title":"Updated"}`}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if code := boardcli.RunCLI(io.Discard, args); code != 0 {
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
			dir := seedWikiRepo(b, n)
			b.Chdir(dir)
			args := []string{"get", `{"slug":"task-0"}`}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if code := boardcli.RunCLI(io.Discard, args); code != 0 {
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
			dir := seedWikiRepo(b, n)
			b.Chdir(dir)
			args := []string{"list"}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if code := boardcli.RunCLI(io.Discard, args); code != 0 {
					b.Fatalf("RunCLI list exit %d", code)
				}
			}
		})
	}
}
