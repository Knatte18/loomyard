//go:build tmux

// smoke_drivingsurface_test.go pins the live-substrate properties behind a run's driving surface.
// On the start side, an llm-seeded start leaves exactly one status strand.
// On the step side, "lyx loom step" and "lyx shed step" bring reed up and never add, replace or remove a status strand, whatever driver the run was seeded with.
// ensureStatusStrand's branches are pinned at Tier 1 through resolveStatusStrandAction;
// what only a real tmux server can show is that a step leaves reed's strand table without a status strand, and leaves a pre-existing one alone.
//
// Zero real LLM subprocesses: the fixtures are reused from smoke_helpers_test.go,
// and the shuttle config is the providerless one,
// so the producer a step reaches bounces at launch rather than starting a provider.
package loomcli

import (
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/testkit/lyxbin"
)

// verbSmokeTimeout bounds one step or `start --no-attach` invocation.
// The providerless shuttle config makes a step's producer launch and a start's llm driver launch fail within seconds.
const verbSmokeTimeout = 60 * time.Second

// TestSmokeStatusStrandAcrossDriverSeeds drives the step verbs, and an llm-seeded start, over one pair per driver seed, in this order.
// A run's seed cannot be rewritten to the other driver, so each driver gets its own pair, and the steps of one pair share its reed session.
// The step that adds a status strand builds on the reed session the step before it brought up, and the strand it adds stays for the steps after it.
func TestSmokeStatusStrandAcrossDriverSeeds(t *testing.T) {
	tmuxBinaryPath(t)
	exe := lyxbin.Build(t)

	requireNoStatusStrandAfter := func(t *testing.T, loc *lyxcwd.Location, worktree, verb string, args ...string) {
		t.Helper()
		out, _, err := runLoomCLINoFatal(exe, worktree, verbSmokeTimeout, args...)
		if err != nil {
			t.Fatalf("lyx %s: %v; output: %s", verb, err, out)
		}

		eng := probeReedEngine(t, loc)
		if _, err := eng.Status(); err != nil {
			t.Fatalf("reed status after %s: %v; want reed up; output: %s", verb, err, out)
		}
		if count := statusStrandCount(t, eng, statusStrandDisplayName); count != 0 {
			t.Errorf("status strands after %s = %d; want 0", verb, count)
		}
	}

	t.Run("go driver", func(t *testing.T) {
		_, loc, worktree, _ := newWiredPairFixture(t)
		registerBootstrapTeardown(t, loc, worktree)
		seedGoDriverRun(t, loc)

		t.Run("loom step adds no status strand", func(t *testing.T) {
			requireNoStatusStrandAfter(t, loc, worktree, "loom step", "loom", "step")
		})

		t.Run("shed step adds no status strand", func(t *testing.T) {
			requireNoStatusStrandAfter(t, loc, worktree, "shed step", "shed", "step")
		})

		t.Run("loom step leaves a pre-existing status strand untouched", func(t *testing.T) {
			eng := probeReedEngine(t, loc)
			if _, err := eng.Up(); err != nil {
				t.Fatalf("reed up: %v", err)
			}
			added, err := eng.AddStrand(statusStrandAddSpec("sleep 3600"))
			if err != nil {
				t.Fatalf("add status strand: %v", err)
			}

			out, _, err := runLoomCLINoFatal(exe, worktree, verbSmokeTimeout, "loom", "step")
			if err != nil {
				t.Fatalf("lyx loom step: %v; output: %s", err, out)
			}

			status, err := eng.Status()
			if err != nil {
				t.Fatalf("reed status: %v", err)
			}
			var guids []string
			for _, s := range status.Strands {
				if agentname.Matches(s.Name, statusStrandDisplayName) {
					guids = append(guids, s.GUID)
				}
			}
			if len(guids) != 1 || guids[0] != added.GUID {
				t.Errorf("status strand guids after step = %v; want exactly [%s], the one added before the step", guids, added.GUID)
			}
		})
	})

	t.Run("llm driver", func(t *testing.T) {
		_, loc, worktree, _ := newWiredPairFixture(t)
		registerBootstrapTeardown(t, loc, worktree)
		seedLLMDriver(t, loc)

		t.Run("loom step adds no status strand", func(t *testing.T) {
			requireNoStatusStrandAfter(t, loc, worktree, "loom step", "loom", "step")
		})

		t.Run("shed step adds no status strand", func(t *testing.T) {
			requireNoStatusStrandAfter(t, loc, worktree, "shed step", "shed", "step")
		})

		// An llm-seeded start leaves exactly one status strand, as a go-seeded one does.
		// Reed refuses a second strand under the same agent name, so a duplicate can no longer be built here.
		// The providerless shuttle config makes the driver launch fail, which the step ignores: the status strand is ensured before the driver spawn.
		t.Run("start keeps one status strand", func(t *testing.T) {
			eng := probeReedEngine(t, loc)
			if _, err := eng.Up(); err != nil {
				t.Fatalf("reed up: %v", err)
			}
			if _, err := eng.AddStrand(statusStrandAddSpec("sleep 3600")); err != nil {
				t.Fatalf("add status strand: %v", err)
			}
			if count := statusStrandCount(t, eng, statusStrandDisplayName); count != 1 {
				t.Fatalf("status strands before start = %d; want 1", count)
			}

			out, _, err := runLoomCLINoFatal(exe, worktree, verbSmokeTimeout, "loom", "start", "--no-attach")
			if err != nil {
				t.Fatalf("lyx loom start --no-attach: %v; output: %s", err, out)
			}

			if count := statusStrandCount(t, eng, statusStrandDisplayName); count != 1 {
				t.Errorf("status strands after an llm-seeded start = %d; want exactly 1; output: %s", count, out)
			}
		})
	})
}
