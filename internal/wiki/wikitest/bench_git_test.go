// bench_git_test.go — git-backed write benchmarks (integration-gated).
//
// Measures an upsert command with the full git round-trip (pull → commit → push
// to the dummy wiki at testRepoURL) and with push skipped (WIKI_SKIP_PUSH=1), so
// the cost of git — the dominant part of a real write — is visible. Hits the
// network and pushes throwaway commits, hence the integration build tag.

//go:build integration

package wikitest

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Knatte18/mhgo/internal/wiki"
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
	run("git", "-C", repoPath, "config", "user.email", "bench@mhgo.dev")
	run("git", "-C", repoPath, "config", "user.name", "MHGo Bench")
	return repoPath
}

// benchmarkUpsertGit upserts a task once per iteration against a real clone of
// the dummy wiki. The title varies each iteration so tasks.json actually changes
// and a real commit (and push, unless skipped) is forced — CommitPush is a no-op
// when nothing changed.
func benchmarkUpsertGit(b *testing.B) {
	repo := cloneBenchWiki(b)
	w := wiki.New(repo)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := w.UpsertTask(map[string]any{
			"slug":  "bench-task",
			"title": "Updated " + strconv.Itoa(i),
		})
		if err != nil {
			b.Fatalf("UpsertTask: %v", err)
		}
	}
}

// BenchmarkUpsertGit measures a write command with the full git round-trip:
// pull --ff-only → render → commit → push to the remote.
func BenchmarkUpsertGit(b *testing.B) {
	benchmarkUpsertGit(b)
}

// BenchmarkUpsertGitNoPush measures a write command with git pull + commit but
// no network push (WIKI_SKIP_PUSH=1). The gap to BenchmarkUpsertGit is the push
// cost; the gap to the no-git BenchmarkUpsert is pull + local commit.
func BenchmarkUpsertGitNoPush(b *testing.B) {
	b.Setenv("WIKI_SKIP_PUSH", "1")
	benchmarkUpsertGit(b)
}
