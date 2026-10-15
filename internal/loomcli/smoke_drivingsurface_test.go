//go:build tmux

// smoke_drivingsurface_test.go pins the live-substrate properties behind a run's driving surface.
// On the start side, an llm-seeded start keeps exactly one status strand.
// On the step side, "lyx loom step" and "lyx shed step" bring reed up and never add, replace or remove a status strand, whatever driver the run was seeded with.
// A start re-arms the session's follow state and selects the run's relevant window, and a resume leaves a follow-off session off.
// ensureStatusStrand's branches are pinned at Tier 1 through resolveStatusStrandAction;
// what only a real tmux server can show is that a step leaves reed's strand table without a status strand, and leaves a pre-existing one alone.
//
// Zero real LLM subprocesses: the fixtures are reused from smoke_helpers_test.go,
// and the shuttle config is the providerless one,
// so the producer a step reaches bounces at launch rather than starting a provider.
package loomcli

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/lyxbin"
)

// verbSmokeTimeout bounds one step or `start --no-attach` invocation.
// The providerless shuttle config makes a step's producer launch and a start's llm driver launch fail within seconds.
const verbSmokeTimeout = 60 * time.Second

// readFollow returns the session option @lyx_follow of eng's session, empty when unset.
// It reads through the tmux binary on the engine's socket, so the helper sees what a viewer's session shows.
func readFollow(t *testing.T, eng *reedengine.Engine) string {
	t.Helper()
	out, err := exec.Command(tmuxBinaryPath(t), "-L", eng.Socket(), "display-message", "-p", "-t", "="+eng.SessionName()+":", "#{@lyx_follow}").Output()
	if err != nil {
		t.Fatalf("read @lyx_follow: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// setFollow sets the session option @lyx_follow of eng's session to value.
func setFollow(t *testing.T, eng *reedengine.Engine, value string) {
	t.Helper()
	if out, err := exec.Command(tmuxBinaryPath(t), "-L", eng.Socket(), "set-option", "-t", "="+eng.SessionName(), "@lyx_follow", value).CombinedOutput(); err != nil {
		t.Fatalf("set @lyx_follow %s: %v; output: %s", value, err, out)
	}
}

// windowID returns the id of the window target names, a pane id or the session's current window "=<session>:".
func windowID(t *testing.T, eng *reedengine.Engine, target string) string {
	t.Helper()
	out, err := exec.Command(tmuxBinaryPath(t), "-L", eng.Socket(), "display-message", "-p", "-t", target, "#{window_id}").Output()
	if err != nil {
		t.Fatalf("read the window of %s: %v", target, err)
	}
	return strings.TrimSpace(string(out))
}

// statusStrandPane returns the pane id of eng's status strand.
func statusStrandPane(t *testing.T, eng *reedengine.Engine) string {
	t.Helper()
	status, err := eng.Status()
	if err != nil {
		t.Fatalf("reed status: %v", err)
	}
	for _, s := range status.Strands {
		if agentname.Matches(s.Name, statusStrandDisplayName) {
			return s.PaneID
		}
	}
	t.Fatalf("reed status holds no %s strand", statusStrandDisplayName)
	return ""
}

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

		// The step subtests leave the go-seeded run halted with no driver, so a resume takes the wake path and spawns one.
		t.Run("loom resume leaves a follow-off session off", func(t *testing.T) {
			eng := probeReedEngine(t, loc)
			setFollow(t, eng, "off")
			current := "=" + eng.SessionName() + ":"
			before := windowID(t, eng, current)

			out, exit, err := runLoomCLINoFatal(exe, worktree, verbSmokeTimeout, "loom", "resume")
			if err != nil || exit != 0 {
				t.Fatalf("lyx loom resume: exit %d, err %v; output: %s", exit, err, out)
			}
			if !strings.Contains(out, "a Go driver was spawned") {
				t.Fatalf("lyx loom resume output = %s; want the Go driver spawn, which proves the wake path rather than an early refusal", out)
			}

			if got := readFollow(t, eng); got != "off" {
				t.Errorf("@lyx_follow after the resume = %q; want off", got)
			}
			if got := windowID(t, eng, current); got != before {
				t.Errorf("current window after the resume = %s; want %s left as it was", got, before)
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

		// An llm-seeded start ensures the status strand, like a go-seeded one.
		// The providerless shuttle config makes the driver launch fail, which the step ignores: the status strand is ensured before the driver spawn.
		t.Run("start leaves exactly one status strand", func(t *testing.T) {
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
			// A second run in a reused pair starts over a session whose viewer turned follow off.
			setFollow(t, eng, "off")

			out, _, err := runLoomCLINoFatal(exe, worktree, verbSmokeTimeout, "loom", "start", "--no-attach")
			if err != nil {
				t.Fatalf("lyx loom start --no-attach: %v; output: %s", err, out)
			}

			if count := statusStrandCount(t, eng, statusStrandDisplayName); count != 1 {
				t.Errorf("status strands after an llm-seeded start = %d; want 1; output: %s", count, out)
			}
			if got := readFollow(t, eng); got != "on" {
				t.Errorf("@lyx_follow after the start = %q; want on; output: %s", got, out)
			}
			// The providerless driver launch leaves no live agent, so the relevant window is the overview holding the status strand.
			if got, want := windowID(t, eng, "="+eng.SessionName()+":"), windowID(t, eng, statusStrandPane(t, eng)); got != want {
				t.Errorf("current window after the start = %s; want the status strand's window %s; output: %s", got, want, out)
			}
		})
	})
}
