// bench_git_test.go — git-backed sync benchmarks (integration-gated).
//
// In the async model a write only touches the filesystem; the git cost lives in
// the background sync. These benchmarks measure Sync directly: dirty the working
// tree, then time commit + push to the dummy wiki at testRepoURL (and the
// commit-only path with BOARD_SKIP_PUSH=1). Hits the network and pushes throwaway
// commits, hence the integration build tag.

//go:build integration

package boardtest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/board"
)

// cloneBenchWiki clones the dummy wiki into a temp dir and configures a commit
// identity, returning the working-copy path. The clone is part of benchmark
// setup; call b.ResetTimer afterwards so it is excluded from the measurement.
func cloneBenchWiki(b *testing.B) string {
	b.Helper()

	repoPath := filepath.Join(b.TempDir(), "wiki")
	run := func(args ...string) {
		b.Helper()
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			b.Fatalf("%v failed: %s", args, out)
		}
	}

	run("git", "clone", testRepoURL, repoPath)
	run("git", "-C", repoPath, "config", "user.email", "bench@loomyard.dev")
	run("git", "-C", repoPath, "config", "user.name", "Loomyard Bench")
	return repoPath
}

// benchmarkSync dirties tasks.json with a unique change (not timed) and then
// times one Sync, which commits and pushes it. The file write is excluded so the
// measurement is the git round-trip only.
func benchmarkSync(b *testing.B) {
	repo := cloneBenchWiki(b)
	cfg := board.DefaultConfig()
	cfg.Path = repo
	w := board.New(cfg)
	tasksPath := filepath.Join(repo, "tasks.json")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		content := fmt.Sprintf(`[{"id":0,"slug":"bench","title":"u%d","depends_on":[]}]`, i)
		if err := os.WriteFile(tasksPath, []byte(content), 0o644); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()

		if err := w.Sync(); err != nil {
			b.Fatalf("Sync: %v", err)
		}
	}
}

// BenchmarkSyncGit measures a full background sync: commit + push to the remote.
func BenchmarkSyncGit(b *testing.B) {
	benchmarkSync(b)
}

// BenchmarkSyncGitNoPush measures the commit-only leg of a sync (BOARD_SKIP_PUSH=1).
// The gap to BenchmarkSyncGit is the network push cost.
func BenchmarkSyncGitNoPush(b *testing.B) {
	b.Setenv("BOARD_SKIP_PUSH", "1")
	benchmarkSync(b)
}
