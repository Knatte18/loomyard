//go:build integration

// weftskippush_integration_test.go covers the fabric CLI's content-sync verbs ("push", "sync") against one real hub with FABRIC_SKIP_PUSH set, so the detached push child does no network work:
// the env-to-SyncOptions mapping at the CLI edge, and the sync pathspec still covering _lyx when the repo-wide config names only another path.
// Package fabriccli_test, sharing the single TestMain in testmain_test.go.

package fabriccli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// TestRunCLI_WeftSkipPushScenario runs the push and sync checks over one hub.
// It stays serial (no t.Parallel): t.Setenv("FABRIC_SKIP_PUSH", "1") panics under t.Parallel, and the env var is process-global state.
// Steps run serially in this order, because both rewrite the weft placeholder file and each commit must differ from the last.
func TestRunCLI_WeftSkipPushScenario(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	// Prevent the detached push child from doing any real network work;
	// SpawnDetachedPush itself checks this env var before spawning.
	t.Setenv("FABRIC_SKIP_PUSH", "1")

	weftConfigFile := filepath.Join(h.WeftBase, lyxdirs.LyxDirName, "placeholder")

	if !t.Run("EnvMapToOption", func(t *testing.T) {
		// The CLI edge maps FABRIC_SKIP_PUSH to SyncOptions on the push verb.
		// Fabric config is a repo-wide fact read from the board dir: fabriccli.CloneAndWire already materializes it with the plain registered template as part of building h, so nothing further needs seeding here.
		if err := os.WriteFile(weftConfigFile, []byte("modified"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		weftBranch := strings.TrimSpace(gitOutputCLI(t, h.PrimeRecords(), "rev-parse", "--abbrev-ref", "HEAD"))
		weftBareBefore := gitOutputCLI(t, h.RecordsBare, "for-each-ref", "refs/heads/"+weftBranch)
		warpBareBefore := gitkit.RevParse(t, h.CodeBare, "HEAD")

		code, output := runFabric(t, h.PrimeWorktree(), "push")
		if code != 0 {
			t.Errorf("RunCLI push returned %d; want 0", code)
			t.Logf("output: %s", output)
		}

		envelope.RequireOK(t, output)

		if got := gitOutputCLI(t, h.RecordsBare, "for-each-ref", "refs/heads/"+weftBranch); got != weftBareBefore {
			t.Errorf("weft bare %s = %s; want %s (FABRIC_SKIP_PUSH must push nothing)", weftBranch, got, weftBareBefore)
		}
		if got := gitkit.RevParse(t, h.CodeBare, "HEAD"); got != warpBareBefore {
			t.Errorf("warp bare HEAD = %s; want %s (FABRIC_SKIP_PUSH must push nothing)", got, warpBareBefore)
		}
	}) {
		return
	}

	t.Run("SyncStillCommitsLyx_WhenRepoWidePathspecNamesOnlyAnotherPath", func(t *testing.T) {
		// With the repo-wide fabric.yaml's pathspec naming only "_extra" (a single non-_lyx name), "lyx fabric sync" must still commit _lyx content, because the sync pathspec is built from fabricengine.PathspecNames — the routing set, which always contains "_lyx" structurally — never from a raw, unfiltered Config.Dirs() that would silently drop it.
		// A miss here is silent, not loud.
		hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: _extra\n")

		if err := os.WriteFile(weftConfigFile, []byte("modified for sync regression"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		code, output := runFabric(t, h.PrimeWorktree(), "sync")
		if code != 0 {
			t.Fatalf("RunCLI(sync) = %d; want 0\noutput: %s", code, output)
		}

		result := envelope.RequireOK(t, output)

		// In the prime sync hands the detached child the records side alone,
		// so the record holds a single push_spawned entry.
		if got := pushSpawnedTargets(result); len(got) != 1 {
			t.Errorf("push_spawned targets = %v; want exactly the records side\noutput: %s", got, output)
		}

		tracked := strings.TrimSpace(gitOutputCLI(t, h.PrimeRecords(), "log", "-1", "--name-only", "--pretty=format:"))
		if !strings.Contains(tracked, filepath.ToSlash(filepath.Join(lyxdirs.LyxDirName, "placeholder"))) {
			t.Errorf("HEAD commit on %s does not touch %s; want the sync-built pathspec to still cover _lyx even though the repo-wide config names only _extra\nfiles: %s", h.PrimeRecords(), lyxdirs.LyxDirName, tracked)
		}
	})

	t.Run("SyncInATaskPairSpawnsBothSides", func(t *testing.T) {
		// In a task pair sync hands the detached child both sides, one push_spawned entry each.
		const slug = "sync-both-sides-pair"
		if code, output := runFabric(t, h.PrimeWorktree(), "add", slug); code != 0 {
			t.Fatalf("RunCLI(add) = %d; want 0\noutput: %s", code, output)
		}
		pairLyx := filepath.Join(h.PairRecordsSibling(slug), lyxdirs.LyxDirName)
		if err := os.MkdirAll(pairLyx, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(pairLyx, "placeholder"), []byte("modified in the pair"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		code, output := runFabric(t, h.PairCodeWorktree(slug), "sync")
		if code != 0 {
			t.Fatalf("RunCLI(sync) = %d; want 0\noutput: %s", code, output)
		}

		if got := pushSpawnedTargets(envelope.RequireOK(t, output)); len(got) != 2 {
			t.Errorf("push_spawned targets = %v; want one entry per side (code and records)\noutput: %s", got, output)
		}
	})
}

// pushSpawnedTargets returns the targets of the push_spawned entries in an envelope's mutation record.
func pushSpawnedTargets(result envelope.Envelope) map[string]bool {
	targets := map[string]bool{}
	mutations, _ := result.Raw["mutations"].([]any)
	for _, raw := range mutations {
		entry, _ := raw.(map[string]any)
		if entry["kind"] == string(fabricengine.KindPushSpawned) {
			target, _ := entry["target"].(string)
			targets[target] = true
		}
	}
	return targets
}
