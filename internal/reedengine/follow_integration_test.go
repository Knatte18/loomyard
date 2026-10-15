//go:build tmux && linux

// follow_integration_test.go proves the follow state against a real tmux with real attached clients.
// A run start turns follow on and moves every client to the relevant window, a second attach moves the clients only while follow is on, and a resume or a failed option write changes nothing it should not.
// One cold session serves every step.

package reedengine

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// TestFollow_RealTmux runs its steps in order on one cold session with four attached clients.
// It captures the process-global logger output in one step, so it does not run in parallel.
func TestFollow_RealTmux(t *testing.T) {
	e := newColdScratchEngine(t)
	if _, err := e.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}
	session := exactSessionWindowTarget(e.SessionName())
	display := func(target, format string) string {
		t.Helper()
		out, err := e.tmux.output("display-message", "-p", "-t", target, format)
		if err != nil {
			t.Fatalf("display-message %s %s: %v", target, format, err)
		}
		return strings.TrimSpace(out)
	}
	currentWindow := func() string {
		t.Helper()
		return display(session, "#{window_id}")
	}
	selectWindow := func(window string) {
		t.Helper()
		if err := e.tmux.run("select-window", "-t", window); err != nil {
			t.Fatalf("select-window %s: %v", window, err)
		}
	}
	setFollow := func(value string) {
		t.Helper()
		if err := e.tmux.run("set-option", "-t", session, followSessionOption, value); err != nil {
			t.Fatalf("set %s %s: %v", followSessionOption, value, err)
		}
	}
	readFollow := func() string {
		t.Helper()
		return display(session, "#{"+followSessionOption+"}")
	}
	assertFollow := func(want string) {
		t.Helper()
		if got := readFollow(); got != want {
			t.Errorf("%s = %q, want %q", followSessionOption, got, want)
		}
	}
	shellWindow := func() string {
		t.Helper()
		st, err := LoadState(e.stateDir())
		if err != nil || st == nil || st.SelvagePaneID == "" {
			t.Fatalf("LoadState = (%+v, %v), want a persisted SelvagePaneID", st, err)
		}
		return windowOfPane(t, e, st.SelvagePaneID)
	}
	clientNames := func() []string {
		t.Helper()
		out, err := e.tmux.output("list-clients", "-t", exactSessionTarget(e.SessionName()), "-F", "#{client_name}")
		if err != nil {
			t.Fatalf("list-clients: %v", err)
		}
		return strings.Fields(out)
	}
	// attachClient starts one more client through AttachArgv and returns once the session lists want clients.
	// The client stays attached until the test ends.
	attachClient := func(want int) {
		t.Helper()
		startInPTY(t, append([]string{e.cfg.Tmux}, e.AttachArgv(120, 30)...), 120, 30)
		waitUntil(t, 15*time.Second, "the session never listed the attached client", func() bool { return len(clientNames()) == want })
	}
	clientWindow := func(client string) string {
		t.Helper()
		return display(client, "#{window_id}")
	}
	// waitAllClientsOn waits until every attached client shows window, since an attach's chained select-window runs after the client appears in the listing.
	waitAllClientsOn := func(window, what string) {
		t.Helper()
		waitUntil(t, 15*time.Second, what, func() bool {
			for _, client := range clientNames() {
				if clientWindow(client) != window {
					return false
				}
			}
			return true
		})
	}
	// assertAllClientsOn asserts every attached client shows window right now.
	assertAllClientsOn := func(window, what string) {
		t.Helper()
		for _, client := range clientNames() {
			if got := clientWindow(client); got != window {
				t.Errorf("%s: client %s shows window %s, want %s", what, client, got, window)
			}
		}
	}
	// assertAllClientsStayOn asserts every attached client keeps showing window for a short settling window, long enough for an attach's chained commands to have run.
	assertAllClientsStayOn := func(window, what string) {
		t.Helper()
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			assertAllClientsOn(window, what)
			time.Sleep(50 * time.Millisecond)
		}
	}
	ownDisplay := render.Display{Anchor: render.AnchorOwnWindow}
	overviewDisplay := render.Display{Anchor: render.AnchorOverview}
	addOwn := func(name string) Strand {
		t.Helper()
		s, err := e.AddStrand(AddSpec{NameOverride: name, Cmd: "sleep 300", Display: ownDisplay})
		if err != nil {
			t.Fatalf("AddStrand(%s): %v", name, err)
		}
		return s
	}
	addOverview := func() Strand {
		t.Helper()
		s, err := e.AddStrand(AddSpec{NameOverride: "status", Cmd: "sleep 300", Display: overviewDisplay})
		if err != nil {
			t.Fatalf("AddStrand(overview): %v", err)
		}
		return s
	}
	remove := func(s Strand) {
		t.Helper()
		if _, err := e.RemoveStrand(s.GUID, false); err != nil {
			t.Fatalf("RemoveStrand(%s): %v", s.Name, err)
		}
	}

	var overview, first, second Strand
	t.Run("BootSetsFollowOn", func(t *testing.T) {
		assertFollow("on")
	})

	t.Run("RunStartSelectsTheOverviewThenEachNewAgentForEveryClient", func(t *testing.T) {
		shell := shellWindow()
		attachClient(1)
		attachClient(2)
		waitAllClientsOn(shell, "the clients never both showed the shell")

		overview = addOverview()
		assertAllClientsOn(shell, "after the overview add")
		assertFollow("on")

		if err := e.FollowRun(); err != nil {
			t.Fatalf("FollowRun: %v", err)
		}
		overviewWindow := windowOfPane(t, e, overview.PaneID)
		assertAllClientsOn(overviewWindow, "after FollowRun")

		first = addOwn("first")
		assertAllClientsOn(windowOfPane(t, e, first.PaneID), "after the first agent")
		second = addOwn("second")
		assertAllClientsOn(windowOfPane(t, e, second.PaneID), "after the second agent")
	})

	t.Run("FollowRunTurnsAFollowOffSessionOnAndSelectsTheRelevantWindow", func(t *testing.T) {
		shell := shellWindow()
		setFollow("off")
		selectWindow(shell)
		if err := e.FollowRun(); err != nil {
			t.Fatalf("FollowRun: %v", err)
		}
		assertFollow("on")
		assertAllClientsOn(windowOfPane(t, e, second.PaneID), "FollowRun with live agents")

		setFollow("off")
		selectWindow(shell)
		remove(second)
		remove(first)
		if err := e.FollowRun(); err != nil {
			t.Fatalf("FollowRun: %v", err)
		}
		assertFollow("on")
		assertAllClientsOn(windowOfPane(t, e, overview.PaneID), "FollowRun with only the overview")

		setFollow("off")
		selectWindow(shell)
		remove(overview)
		if err := e.FollowRun(); err != nil {
			t.Fatalf("FollowRun: %v", err)
		}
		assertFollow("on")
		assertAllClientsOn(shell, "FollowRun with no agent or overview")
	})

	t.Run("ASecondAttachMovesTheFirstClientOnlyWhileFollowIsOn", func(t *testing.T) {
		shell := shellWindow()
		overview = addOverview()
		overviewWindow := windowOfPane(t, e, overview.PaneID)

		setFollow("on")
		selectWindow(shell)
		attachClient(3)
		waitAllClientsOn(overviewWindow, "the third attach never landed every client on the overview")
		assertFollow("on")

		setFollow("off")
		selectWindow(shell)
		attachClient(4)
		assertAllClientsStayOn(shell, "the fourth attach with follow off")
		assertFollow("off")
	})

	t.Run("AResumeOfALiveSessionChangesNoFollowState", func(t *testing.T) {
		shell := shellWindow()
		setFollow("off")
		selectWindow(shell)

		if _, err := e.Resume(); err != nil {
			t.Fatalf("Resume: %v", err)
		}

		assertFollow("off")
		if got := currentWindow(); got != shell {
			t.Errorf("current window = %s after Resume, want the viewer left on the shell %s", got, shell)
		}
	})

	t.Run("AFailedFollowWriteStillSelects", func(t *testing.T) {
		shell := shellWindow()
		setFollow("off")
		selectWindow(shell)
		real := NewTmuxCmd(e.tmux.tmuxPath, e.tmux.socket)
		fake := installFakeTmux(t, e)
		fake.forwardTo(real)
		fake.answerFunc("set-option", func(args []string) (string, error) {
			if containsArg(args, followSessionOption) {
				return "", errors.New("write refused")
			}
			return real.output(args...)
		})
		t.Cleanup(func() { e.tmux.execHook = nil })
		buf := logcapture.CaptureVerbose(t)

		if err := e.FollowRun(); err != nil {
			t.Fatalf("FollowRun = %v, want nil beside a failed option write", err)
		}

		if !strings.Contains(buf.String(), "could not turn the follow state on") {
			t.Errorf("log output = %q, want the failed follow write logged", buf.String())
		}
		if got, want := currentWindow(), windowOfPane(t, e, overview.PaneID); got != want {
			t.Errorf("current window = %s, want the relevant window %s selected anyway", got, want)
		}
	})

	socketPath := func() string {
		t.Helper()
		return display(session, "#{socket_path}")
	}

	var alpha, beta Strand
	t.Run("FollowSessionTurnsFollowOnAndSelectsTheNewestAgent", func(t *testing.T) {
		shell := shellWindow()
		setFollow("off")
		alpha = addOwn("alpha")
		beta = addOwn("beta")
		selectWindow(shell)

		if err := FollowSession(e.cfg.Tmux, socketPath(), e.SessionName()); err != nil {
			t.Fatalf("FollowSession: %v", err)
		}

		assertFollow("on")
		assertAllClientsOn(windowOfPane(t, e, beta.PaneID), "FollowSession with live agents")
	})

	t.Run("FollowSessionWithOnlyTheOverviewSelectsIt", func(t *testing.T) {
		shell := shellWindow()
		remove(beta)
		remove(alpha)
		setFollow("off")
		selectWindow(shell)

		if err := FollowSession(e.cfg.Tmux, socketPath(), e.SessionName()); err != nil {
			t.Fatalf("FollowSession: %v", err)
		}

		assertFollow("on")
		overviewWindow := windowOfPane(t, e, overview.PaneID)
		if got := display(overviewWindow, "#{"+overviewWindowOption+"}"); got != "1" {
			t.Errorf("overview window %s = %q, want the real overview marker 1", overviewWindowOption, got)
		}
		assertAllClientsOn(overviewWindow, "FollowSession with only the overview")
	})

	t.Run("FollowSessionWithNoAgentOrOverviewSelectsNothing", func(t *testing.T) {
		shell := shellWindow()
		remove(overview)
		setFollow("off")
		selectWindow(shell)

		if err := FollowSession(e.cfg.Tmux, socketPath(), e.SessionName()); err != nil {
			t.Fatalf("FollowSession: %v", err)
		}

		assertFollow("on")
		assertAllClientsOn(shell, "FollowSession with no agent or overview")
	})

	t.Run("FollowSessionOnAnUnknownSessionFailsAndChangesNothing", func(t *testing.T) {
		setFollow("off")
		before := currentWindow()

		if err := FollowSession(e.cfg.Tmux, socketPath(), "no-such-session"); err == nil {
			t.Error("FollowSession on an unknown session = nil, want an error")
		}

		assertFollow("off")
		if got := currentWindow(); got != before {
			t.Errorf("current window = %s, want %s unchanged", got, before)
		}
	})

	t.Run("NoSessionIsAnError", func(t *testing.T) {
		e.tmux.execHook = nil
		if err := e.tmux.run("kill-session", "-t", exactSessionTarget(e.SessionName())); err != nil {
			t.Fatalf("kill-session: %v", err)
		}
		fake := installFakeTmux(t, e)
		fake.forwardTo(NewTmuxCmd(e.tmux.tmuxPath, e.tmux.socket))
		fake.mustNotCall("set-option")
		t.Cleanup(func() { e.tmux.execHook = nil })

		if err := e.FollowRun(); !errors.Is(err, ErrNoSession) {
			t.Errorf("FollowRun = %v, want an error satisfying ErrNoSession", err)
		}
	})
}
