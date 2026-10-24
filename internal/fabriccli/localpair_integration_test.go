//go:build tier1

// localpair_integration_test.go drives the local-only pair verbs of the fabric CLI over one real hub: `add --local`, the push that skips a local pair's branches, and the local add's refusal over an ordinary pair.
// Package fabriccli_test, sharing the single TestMain in testmain_test.go.

package fabriccli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// TestRunCLI_LocalPairScenario adds a local pair, pushes from it and tries a local add over an ordinary pair's kept branch.
// Steps run serially in this order over one hub.
func TestRunCLI_LocalPairScenario(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const localSlug = "localpair"
	const ordinarySlug = "ordinarypair"

	if !t.Run("AddLocal", func(t *testing.T) {
		code, output := runFabric(t, h.PrimeWorktree(), "add", "--local", localSlug)
		if code != 0 {
			t.Fatalf("add --local exit = %d; output: %s", code, output)
		}
		result := envelope.RequireOK(t, output)
		if result.Raw["local"] != true {
			t.Errorf("local = %v; want true\noutput: %s", result.Raw["local"], output)
		}
		if targets := pushSpawnedTargets(result); len(targets) != 0 {
			t.Errorf("push_spawned targets = %v; want none for a local pair", targets)
		}
	}) {
		return
	}

	if !t.Run("PushSkipsLocalBranches", func(t *testing.T) {
		code, output := runFabric(t, h.PairCodeWorktree(localSlug), "push")
		if code != 0 {
			t.Fatalf("push exit = %d; output: %s", code, output)
		}
		requireLocalSkipped(t, output, localSlug)
		if gitkit.BranchExists(t, h.CodeBare, localSlug) {
			t.Errorf("origin code bare carries %q; want a local pair never pushed", localSlug)
		}
	}) {
		return
	}

	if !t.Run("SyncSpawnsNothingForALocalPair", func(t *testing.T) {
		syncNote := filepath.Join(h.PairCodeWorktree(localSlug), "_lyx", "sync-note.txt")
		if err := os.WriteFile(syncNote, []byte("a local record to sync\n"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		code, output := runFabric(t, h.PairCodeWorktree(localSlug), "sync")
		if code != 0 {
			t.Fatalf("sync exit = %d; output: %s", code, output)
		}
		result := envelope.RequireOK(t, output)
		requireLocalSkipped(t, output, localSlug)
		if targets := pushSpawnedTargets(result); len(targets) != 0 {
			t.Errorf("push_spawned targets = %v; want none for a local pair\noutput: %s", targets, output)
		}
	}) {
		return
	}

	t.Run("AddLocalRefusesOrdinaryPairBranch", func(t *testing.T) {
		if code, output := runFabric(t, h.PrimeWorktree(), "add", ordinarySlug); code != 0 {
			t.Fatalf("add %s exit = %d; output: %s", ordinarySlug, code, output)
		}
		gitkit.MustRun(t, h.PrimeWorktree(), "git", "worktree", "remove", "--force", h.PairCodeWorktree(ordinarySlug))
		gitkit.MustRun(t, h.PrimeRecords(), "git", "worktree", "remove", "--force", h.PairRecordsSibling(ordinarySlug))

		code, output := runFabric(t, h.PrimeWorktree(), "add", "--local", ordinarySlug)
		if code == 0 {
			t.Fatalf("add --local over an ordinary pair's branch exit = 0; want a refusal\noutput: %s", output)
		}
		envelope.RequireErr(t, output, "lyx fabric checkout")
		envelope.RequireErr(t, output, "git branch -D")
	})
}

// requireLocalSkipped asserts the envelope in output names both branches of slug's local pair under local_skipped.
func requireLocalSkipped(t *testing.T, output, slug string) {
	t.Helper()

	result := envelope.RequireOK(t, output)
	skipped, _ := result.Raw["local_skipped"].([]any)
	got := map[string]bool{}
	for _, branch := range skipped {
		name, _ := branch.(string)
		got[name] = true
	}
	for _, want := range []string{slug, fabricengine.RecordsBranchName(slug)} {
		if !got[want] {
			t.Errorf("local_skipped = %v; want it to name %q\noutput: %s", skipped, want, output)
		}
	}
}
