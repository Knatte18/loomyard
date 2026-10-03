//go:build integration

// circling_integration_test.go drives the circling group through the real cobra tree against a real hub pair.
// It pins what circling_test.go's fakes cannot: the group's pre-run resolves a task worktree with and without a slug,
// and the loom parent's pre-run never arms the group, which would read the slug as a run-id and refuse for the missing seed instead.

package loomcli

import (
	"bytes"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

func TestCirclingCLI_PreRunResolvesTarget(t *testing.T) {
	hub := hubforge.NewHub(t, ".")
	const slug = "circling"
	hubforge.AddPair(t, hub, slug)

	run := func(cwd string, args ...string) (int, string) {
		t.Helper()
		var out bytes.Buffer
		code := RunCLIIn(cwd, &out, args)
		return code, out.String()
	}

	t.Run("task worktree without a slug", func(t *testing.T) {
		code, out := run(hub.PairWarpWorktree(slug), "circling", "accept")
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
