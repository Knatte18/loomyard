// pushteardownlock_integration_test.go covers `lym fabric push` meeting an exclusive hold of the pair's teardown lock:
// the verb pushes nothing and reports an ok envelope with push_skipped.
// Package fabriccli_test, sharing the single TestMain in testmain_test.go.

package fabriccli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRunCLI_PushSkippedWhileTeardownLockHeld holds a pair's teardown lock exclusive, as a removal would.
// It runs `lym fabric push` from the pair with a new records commit.
// The add verb's own pair push runs in the foreground here (see testmain_test.go).
// Origin therefore holds the pair's weft branch before the commit.
func TestRunCLI_PushSkippedWhileTeardownLockHeld(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	traceSinkMu.RLock()
	t.Cleanup(traceSinkMu.RUnlock)

	const slug = "push-skipped-pair"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	if code, output := runFabric(t, h.PrimeWorktree(), "add", slug); code != 0 {
		t.Fatalf("RunCLI(add) = %d; want 0\noutput: %s", code, output)
	}
	weftBranch := fabricengine.RecordsBranchName(slug)
	tipBefore := gitkit.RevParse(t, h.RecordsBare, weftBranch)
	gitkit.CommitFile(t, h.PairRecordsSibling(slug), "held.txt", "held\n", "records work while the lock is held")

	lockPath := fabricengine.TeardownLockPath(h.Path, slug)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("mkdir the teardown lock dir: %v", err)
	}
	held, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("hold the teardown lock exclusive: %v", err)
	}
	t.Cleanup(func() { _ = held.Release() })

	code, output := runFabric(t, h.PairCodeWorktree(slug), "push")
	if code != 0 {
		t.Fatalf("RunCLI(push) = %d; want 0\noutput: %s", code, output)
	}
	result := envelope.RequireOK(t, output)
	if skipped, _ := result.Raw["push_skipped"].(string); skipped == "" {
		t.Errorf("push_skipped = %v; want the reason text\noutput: %s", result.Raw["push_skipped"], output)
	}
	if got := gitkit.RevParse(t, h.RecordsBare, weftBranch); got != tipBefore {
		t.Errorf("origin weft tip = %s; want unchanged %s", got, tipBefore)
	}
}
