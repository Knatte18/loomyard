// circling_integration_test.go drives the circling group through the real cobra tree against a real hub pair.
// It pins what circling_test.go's fakes cannot:
// the group's pre-run resolves a task worktree with and without a slug,
// and the loom parent's pre-run never arms the group.
// Arming it there would read the slug as a run-id and refuse for the missing seed.

package loomcli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/spf13/cobra"
)

// It sets cobra.EnableTraverseRunHooks, a process global, as cmd/lym does, so it does not call t.Parallel.
func TestLoomVerbs_StaleStampReconcilesBeforeACirclingVerbLoads(t *testing.T) {
	tiergate.Require(t, 1)
	traverse := cobra.EnableTraverseRunHooks
	cobra.EnableTraverseRunHooks = true
	t.Cleanup(func() { cobra.EnableTraverseRunHooks = traverse })

	hub := hubforge.CopyHub(t, pairHubShape)
	const slug = hubforge.TemplatePairSlug
	pair, err := lymcwd.ResolveWorktree(hub.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("resolve the pair: %v", err)
	}

	const oldKeys = "code_max_bounces: 2\n"
	gitkit.CommitFile(t, hub.BoardDir(), configengine.ConfigFileRel("loom"), oldKeys, "fixture: old loom key")
	localPath := configengine.LocalFile(pair.AnchorPath(), "loom")
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		t.Fatalf("create the local config directory: %v", err)
	}
	if err := os.WriteFile(localPath, []byte(oldKeys), 0o644); err != nil {
		t.Fatalf("write the local loom config: %v", err)
	}
	statusLock := shedrun.StatusLock(pair, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(statusLock), 0o755); err != nil {
		t.Fatalf("create the status lock directory: %v", err)
	}
	awaiting := shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: loomshed.NameCodeBouncer}
	if err := state.WriteJSON(shedrun.StatusFile(pair, shedrun.SelfRunID), statusLock, awaiting); err != nil {
		t.Fatalf("seed the run status: %v", err)
	}
	// The verb stats the Code-Bouncer's run directory before it loads the hard cap.
	if err := os.MkdirAll(filepath.Join(loomengine.LoomReviewsDir(pair), "code"), 0o755); err != nil {
		t.Fatalf("create the review run directory: %v", err)
	}

	var outside bytes.Buffer
	if code := clihelp.ExecuteIn(Command(), t.TempDir(), &outside, []string{"circling", "continue"}); code != 1 || !strings.Contains(outside.String(), "not a git repository") {
		t.Errorf("circling continue outside a repository = %d, %s; want 1 and the not-a-git-repository refusal", code, outside.String())
	}

	var out bytes.Buffer
	code := clihelp.ExecuteIn(Command(), pair.WorktreePath(), &out, []string{"circling", "continue"})
	// No round is escalated, so the verb refuses at the record step, past the strict load the hard cap needs.
	if code != 1 || !strings.Contains(out.String(), "is not escalated") {
		t.Errorf("circling continue = %d, %s; want 1 and the not-escalated refusal", code, out.String())
	}
	cfg, err := loomengine.LoadConfig(fabricengine.ConfigDirs(pair), "loom")
	if err != nil {
		t.Fatalf("LoadConfig after the verb = %v; want nil", err)
	}
	if got := cfg.Caps(loomengine.PerchCode).Soft; got != 2 {
		t.Errorf("code soft cap = %d; want 2", got)
	}
}

// The rows share one hub copy and run in order, since each runs a CLI verb over it.
func TestCirclingCLI_PreRunResolvesTarget(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	hub := hubforge.CopyHub(t, pairHubShape)
	const slug = hubforge.TemplatePairSlug

	run := func(cwd string, args ...string) (int, string) {
		t.Helper()
		var out bytes.Buffer
		code := clihelp.ExecuteIn(Command(), cwd, &out, args)
		return code, out.String()
	}

	t.Run("task worktree without a slug", func(t *testing.T) {
		code, out := run(hub.PairCodeWorktree(slug), "circling", "accept")
		if code != 1 {
			t.Fatalf("exit = %d, out %s; want 1", code, out)
		}
		envelope.RequireErr(t, out, "loom: circling accept: the run has no status file")
	})
	t.Run("slug from the prime", func(t *testing.T) {
		code, out := run(hub.PrimeWorktree(), "circling", "continue", slug)
		if code != 1 {
			t.Fatalf("exit = %d, out %s; want 1", code, out)
		}
		envelope.RequireErr(t, out, "loom: circling continue: the run has no status file")
	})
	t.Run("slug required from the prime", func(t *testing.T) {
		code, out := run(hub.PrimeWorktree(), "circling", "accept")
		if code != 1 {
			t.Fatalf("exit = %d, out %s; want 1", code, out)
		}
		envelope.RequireErr(t, out, "loom: circling accept: slug required from the prime")
	})
	t.Run("unknown slug", func(t *testing.T) {
		code, out := run(hub.PrimeWorktree(), "circling", "accept", "nope")
		if code != 1 {
			t.Fatalf("exit = %d, out %s; want 1", code, out)
		}
		envelope.RequireErr(t, out, `loom: circling accept: unknown slug "nope"`)
	})
}
