// smoke_drivingsurface_test.go pins the live-substrate properties behind a run's driving surface.
// On the start side, an llm-seeded start keeps exactly one status strand.
// On the step side, "lym loom step" and "lym shed step" bring reed up.
// They never add, replace or remove a status strand, whatever driver the run was seeded with.
// A start re-arms the session's follow state and selects the run's relevant window,
// and a resume leaves a follow-off session off.
// A start puts the status strand in the overview window after the runs strand.
// It replaces an own-window status strand that an older build left.
// ensureStatusStrand's branches are pinned at Tier 1 through resolveStatusStrandAction.
// Only a real tmux server can show the step's effect on reed's strand table.
// A step leaves the table without a status strand, and it leaves a pre-existing one alone.
//
// Zero real LLM subprocesses: the fixtures are reused from smoke_helpers_test.go,
// and the shuttle config is the providerless one,
// so the producer a step reaches bounces at launch rather than starting a provider.
package loomcli

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/consoletitle"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/statuspane"
	"github.com/Knatte18/loomyard/internal/testkit/lymbin"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// verbSmokeTimeout bounds one step or `start --no-attach` invocation.
// The providerless shuttle config makes a step's producer launch and a start's llm driver launch fail within seconds.
const verbSmokeTimeout = 60 * time.Second

// readFollow returns the session option @lym_follow of eng's session, empty when unset.
// It reads through the tmux binary on the engine's socket, so the helper sees what a viewer's session shows.
func readFollow(t *testing.T, eng *reedengine.Engine) string {
	t.Helper()
	out, err := exec.Command(tmuxBinaryPath(t), "-L", eng.Socket(), "display-message", "-p", "-t", "="+eng.SessionName()+":", "#{@lym_follow}").Output()
	if err != nil {
		t.Fatalf("read @lym_follow: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// setFollow sets the session option @lym_follow of eng's session to value.
func setFollow(t *testing.T, eng *reedengine.Engine, value string) {
	t.Helper()
	if out, err := exec.Command(tmuxBinaryPath(t), "-L", eng.Socket(), "set-option", "-t", "="+eng.SessionName()+":", "@lym_follow", value).CombinedOutput(); err != nil {
		t.Fatalf("set @lym_follow %s: %v; output: %s", value, err, out)
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

// strandPane returns the pane id of eng's strand named name.
func strandPane(t *testing.T, eng *reedengine.Engine, name string) string {
	t.Helper()
	status, err := eng.Status()
	if err != nil {
		t.Fatalf("reed status: %v", err)
	}
	for _, s := range status.Strands {
		if agentname.Matches(s.Name, name) {
			return s.PaneID
		}
	}
	t.Fatalf("reed status holds no %s strand", name)
	return ""
}

// statusStrandPane returns the pane id of eng's status strand.
func statusStrandPane(t *testing.T, eng *reedengine.Engine) string {
	t.Helper()
	return strandPane(t, eng, statusStrandDisplayName)
}

// TestSmokeStatusStrandAcrossDriverSeeds drives the step verbs and an llm-seeded start over one pair per driver seed.
// It then drives a start over a pair whose session holds a runs strand.
// A run's seed cannot be rewritten to the other driver, so each driver gets its own pair.
// The steps of one pair share its reed session.
// The step that adds a status strand builds on the reed session that the step before it brought up,
// and the strand it adds stays for the steps after it.
func TestSmokeStatusStrandAcrossDriverSeeds(t *testing.T) {
	tiergate.Require(t, 2)
	tmuxBinaryPath(t)
	exe := lymbin.Build(t)

	requireNoStatusStrandAfter := func(t *testing.T, loc *lymcwd.Location, worktree, verb string, args ...string) {
		t.Helper()
		out, _, err := runLoomCLIWithEnvNoFatal(exe, worktree, skipWalkPush, verbSmokeTimeout, args...)
		if err != nil {
			t.Fatalf("lym %s: %v; output: %s", verb, err, out)
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
			added, err := eng.AddStrand(statusStrandAddSpec("sleep 3600", shedrun.RecipeTapestry))
			if err != nil {
				t.Fatalf("add status strand: %v", err)
			}

			out, _, err := runLoomCLIWithEnvNoFatal(exe, worktree, skipWalkPush, verbSmokeTimeout, "loom", "step")
			if err != nil {
				t.Fatalf("lym loom step: %v; output: %s", err, out)
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

			out, exit, err := runLoomCLIWithEnvNoFatal(exe, worktree, skipWalkPush, verbSmokeTimeout, "loom", "resume")
			if err != nil || exit != 0 {
				t.Fatalf("lym loom resume: exit %d, err %v; output: %s", exit, err, out)
			}
			if !strings.Contains(out, "a Go driver was spawned") {
				t.Fatalf("lym loom resume output = %s; want the Go driver spawn, which proves the wake path rather than an early refusal", out)
			}

			if got := readFollow(t, eng); got != "off" {
				t.Errorf("@lym_follow after the resume = %q; want off", got)
			}
			if got := windowID(t, eng, current); got != before {
				t.Errorf("current window after the resume = %s; want %s left as it was", got, before)
			}
		})
	})

	// A beam's session holds a runs strand in its first overview window.
	// An older build may have left the status strand in a window of its own, with a sidecar naming another build.
	// A start puts the status strand into the overview window after the runs strand.
	t.Run("start beside a runs strand puts an own-window status strand into the overview window after it", func(t *testing.T) {
		_, loc, worktree, _ := newWiredPairFixture(t)
		registerBootstrapTeardown(t, loc, worktree)
		seedLLMDriver(t, loc)

		eng := probeReedEngine(t, loc)
		if _, err := eng.Up(); err != nil {
			t.Fatalf("reed up: %v", err)
		}
		runsSpec := reedengine.AddSpec{
			NameOverride: orchcli.RunsStrandName,
			Cmd:          "sleep 3600",
			Display:      render.Display{Anchor: render.AnchorOverview, Order: orchcli.RunsStrandOrder, Title: "beam: smoke"},
		}
		if _, err := eng.AddStrand(runsSpec); err != nil {
			t.Fatalf("add runs strand: %v", err)
		}
		older, err := eng.AddStrand(reedengine.AddSpec{
			NameOverride: statusStrandDisplayName,
			Cmd:          "sleep 3600",
			Display:      render.Display{Anchor: render.AnchorOwnWindow, KeepViewer: true},
		})
		if err != nil {
			t.Fatalf("add the older status strand: %v", err)
		}
		olderBuild := statuspane.BuildIdentity{Path: "/older/lym", Size: 9, ModTime: 9}
		if err := statuspane.WriteSidecar(statusSidecarPath(loc), older.GUID, olderBuild); err != nil {
			t.Fatalf("write the status sidecar: %v", err)
		}

		out, _, err := runLoomCLIWithEnvNoFatal(exe, worktree, skipWalkPush, verbSmokeTimeout, "loom", "start", "--no-attach")
		if err != nil {
			t.Fatalf("lym loom start --no-attach: %v; output: %s", err, out)
		}

		if count := statusStrandCount(t, eng, statusStrandDisplayName); count != 1 {
			t.Fatalf("status strands after the start = %d; want 1; output: %s", count, out)
		}
		status, err := eng.Status()
		if err != nil {
			t.Fatalf("reed status: %v", err)
		}
		for _, s := range status.Strands {
			if agentname.Matches(s.Name, statusStrandDisplayName) && s.GUID == older.GUID {
				t.Errorf("status strand guid = %s, the older strand's; want it replaced; output: %s", s.GUID, out)
			}
		}
		statusPane := statusStrandPane(t, eng)
		statusWindow := windowID(t, eng, statusPane)
		runsPane := strandPane(t, eng, orchcli.RunsStrandName)
		if statusWindow == windowID(t, eng, runsPane) {
			t.Errorf("status strand window = %s, the runs strand's window; want a window of its own; output: %s", statusWindow, out)
		}
		display := func(pane, format string) string {
			t.Helper()
			shown, err := exec.Command(tmuxBinaryPath(t), "-L", eng.Socket(), "display-message", "-p", "-t", pane, format).Output()
			if err != nil {
				t.Fatalf("read %s of %s: %v", format, pane, err)
			}
			return strings.TrimSpace(string(shown))
		}
		if got := display(statusPane, "#{@lym_overview}"); got != "1" {
			t.Errorf("status window @lym_overview = %q; want 1; output: %s", got, out)
		}
		wantName := consoletitle.Title("", hubforge.TestShortname, "", shedrun.RecipeTapestry)
		if got := display(statusPane, "#{window_name}"); got != wantName {
			t.Errorf("status window name = %q; want %q, the hub shortname and the seeded recipe; output: %s", got, wantName, out)
		}
		statusIndex, err := strconv.Atoi(display(statusPane, "#{window_index}"))
		if err != nil {
			t.Fatalf("status window index: %v", err)
		}
		runsIndex, err := strconv.Atoi(display(runsPane, "#{window_index}"))
		if err != nil {
			t.Fatalf("runs window index: %v", err)
		}
		if statusIndex != runsIndex+1 {
			t.Errorf("status window index = %d; want %d, one above the runs window; output: %s", statusIndex, runsIndex+1, out)
		}
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
		// The providerless shuttle config makes the driver launch fail, which the step ignores.
		// The status strand is ensured before the driver spawn.
		t.Run("start leaves exactly one status strand", func(t *testing.T) {
			eng := probeReedEngine(t, loc)
			if _, err := eng.Up(); err != nil {
				t.Fatalf("reed up: %v", err)
			}
			if _, err := eng.AddStrand(statusStrandAddSpec("sleep 3600", shedrun.RecipeTapestry)); err != nil {
				t.Fatalf("add status strand: %v", err)
			}
			if count := statusStrandCount(t, eng, statusStrandDisplayName); count != 1 {
				t.Fatalf("status strands before start = %d; want 1", count)
			}
			// A second run in a reused pair starts over a session whose viewer turned follow off.
			setFollow(t, eng, "off")

			out, _, err := runLoomCLIWithEnvNoFatal(exe, worktree, skipWalkPush, verbSmokeTimeout, "loom", "start", "--no-attach")
			if err != nil {
				t.Fatalf("lym loom start --no-attach: %v; output: %s", err, out)
			}

			if count := statusStrandCount(t, eng, statusStrandDisplayName); count != 1 {
				t.Errorf("status strands after an llm-seeded start = %d; want 1; output: %s", count, out)
			}
			if got := readFollow(t, eng); got != "on" {
				t.Errorf("@lym_follow after the start = %q; want on; output: %s", got, out)
			}
			// The providerless driver launch leaves no live agent, so the relevant window is the overview holding the status strand.
			if got, want := windowID(t, eng, "="+eng.SessionName()+":"), windowID(t, eng, statusStrandPane(t, eng)); got != want {
				t.Errorf("current window after the start = %s; want the status strand's window %s; output: %s", got, want, out)
			}
		})
	})
}
