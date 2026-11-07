// bench_cli_test.go holds the benchmarks for the board CLI commands.
//
// They drive boardcli.RunCLI, whose cwd-authoritative config resolution calls lymcwd.Resolve (a `git rev-parse`).
// The seeded temp dir must therefore be a real git repo, so these benchmarks spawn git.
// The git-init helper below uses exec.Command, a token the tier-purity guard bans from untagged test files.
//
// BenchmarkRender and BenchmarkUpsertFacade stay untagged in bench_test.go:
// they never touch the CLI or git path, so they run in the offline tier.

package boardtest

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardcli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// seedWikiRepo seeds a board and initialises a git repo at its root, so lymcwd.Resolve can find the hub.
// The CLI entrypoint's config resolution requires a repository, which a bare temp dir is not.
// Only this helper spawns git, so a board seeded without it never does.
func seedWikiRepo(tb testing.TB, n int) string {
	tb.Helper()
	dir := seedWiki(tb, n)
	// The CLI reads the board from the hub's Bolt checkout, and the hub is the repo root's parent.
	if err := os.Rename(filepath.Join(dir, "board"), filepath.Join(filepath.Dir(dir), fabricengine.BoltDirName)); err != nil {
		tb.Fatalf("move board to hub: %v", err)
	}
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "bench@lym.test"},
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

// BenchmarkUpsert measures a full "upsert" command through the CLI entrypoint.
// The measured path is JSON parse, dispatch, lock, load, mutate, render all tasks, then write files.
// It updates an existing task so the per-op work is stable across iterations.
// CLI-bench numbers include the os.Getwd() and LoadConfig cost from cwd-based config.
func BenchmarkUpsert(b *testing.B) {
	tiergate.Require(b, 1)
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

// BenchmarkGet measures a "get" command: the read path (load tasks.json, look up one task by slug).
// No render, no write.
func BenchmarkGet(b *testing.B) {
	tiergate.Require(b, 1)
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

// BenchmarkList measures a "list" command.
// It loads all tasks, computes layers and has_proposal, and serialises the brief view.
func BenchmarkList(b *testing.B) {
	tiergate.Require(b, 1)
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
