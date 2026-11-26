// remotecontrol_test.go covers remote control without a live session:
// the name a launch passes and the pane it marks once the session connects, and the RC click over a fake inject.

package orchcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// remoteControlTemplate is orch.yaml's template value of remote_control_name.
const remoteControlTemplate = "{machine}::{name}"

// TestStart_LaunchPassesAndMarksRemoteControl pins the name a relaunch passes on the orch spec.
// It also pins the pane mark the relaunch asks for:
// the new strand's guid once the session records its connection past the pre-launch checkpoint, and none otherwise.
func TestStart_LaunchPassesAndMarksRemoteControl(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name     string
		switchOn bool
		// noRemoteControl runs the orch on an engine set whose default has no remote control.
		noRemoteControl bool
		// codexDefault runs the orch on an engine set whose default engine is codex and that also holds claude.
		codexDefault bool
		// unlisted launches with no listed orch strand, so the name forms from the worktree's fixed strand name.
		// The harness hub is no git repository, so that name cannot be formed and the launch goes without remote control.
		unlisted bool
		// machineErr fails the machine name the remote session's name carries.
		machineErr error
		// markErr fails the pane mark, which must never fail the launch.
		markErr error
		// recordedSession seeds the orch state with this session id, so the launch resumes it.
		recordedSession string
		// resumeErr fails the resume of recordedSession, so the launch falls back to a fresh session.
		resumeErr error
		// notConnected makes the session record no remote-control connection, as when the provider refuses it.
		notConnected bool
		// checkpointErr fails the read of the record's end before a resume.
		checkpointErr error
		wantName      string
		// wantAwait is the session and checkpoint the connection was awaited from, empty for no wait.
		wantAwait string
		wantMark  bool
	}{
		{name: "SwitchOn", switchOn: true, wantName: "neve::tst:orch", wantAwait: "minted-id@0", wantMark: true},
		{name: "RefusedConnectionLeavesThePaneUnmarked", switchOn: true, notConnected: true, wantName: "neve::tst:orch", wantAwait: "minted-id@0"},
		{name: "FailedMarkStillLaunches", switchOn: true, markErr: errors.New("tmux is gone"), wantName: "neve::tst:orch", wantAwait: "minted-id@0", wantMark: true},
		{name: "SwitchOff"},
		{name: "EngineWithoutRemoteControl", switchOn: true, noRemoteControl: true},
		{name: "CodexDefaultEngineStillTakesTheOrchsPinnedEngine", switchOn: true, codexDefault: true, wantName: "neve::tst:orch", wantAwait: "minted-id@0", wantMark: true},
		{name: "NoListedStrandWithoutAFixedName", switchOn: true, unlisted: true},
		{name: "MachineNameFails", switchOn: true, machineErr: errors.New("no hostname")},
		{name: "ResumedSessionCountsOnlyPastTheCheckpoint", switchOn: true, recordedSession: "sess-1", wantName: "neve::tst:orch", wantAwait: "sess-1@42", wantMark: true},
		{
			name: "ResumeFallingBackToAFreshLaunchCountsTheWholeTranscript", switchOn: true, recordedSession: "sess-1", resumeErr: noTranscriptErr,
			wantName: "neve::tst:orch", wantAwait: "minted-id@0", wantMark: true,
		},
		{name: "UnreadableCheckpointLeavesThePaneUnmarked", switchOn: true, recordedSession: "sess-1", checkpointErr: errors.New("transcript unreadable"), wantName: "neve::tst:orch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var listed []reedengine.StrandStatus
			if !tt.unlisted {
				listed = append(listed, reedengine.StrandStatus{GUID: "old-guid", Name: "tst:orch"})
			}
			h := newStartHarness(t, listed...)
			h.starter.resumeErr = tt.resumeErr
			h.cli.cfg.RemoteControl, h.cli.cfg.RemoteControlName = tt.switchOn, remoteControlTemplate
			h.cli.machineName = func() (string, error) { return "neve", tt.machineErr }
			var marked, awaited []string
			h.cli.markRemoteControl = func(guid string) error {
				marked = append(marked, guid)
				return tt.markErr
			}
			h.cli.remoteControlCheckpoint = func(string) (int64, error) { return 42, tt.checkpointErr }
			h.cli.awaitRemoteControl = func(sessionID string, checkpoint int64) (bool, error) {
				awaited = append(awaited, fmt.Sprintf("%s@%d", sessionID, checkpoint))
				return !tt.notConnected, nil
			}
			if tt.noRemoteControl {
				h.cli.engines = shuttleengine.NewEngineSet("none", nil)
			}
			if tt.codexDefault {
				h.cli.engines = shuttleengine.NewEngineSet("codex", map[string]shuttleengine.Engine{"claude": claudeengine.New()})
			}
			if tt.recordedSession != "" {
				saveSeedState(t, h, orchengine.State{Phase: orchengine.PhaseIdle, Strand: "old-guid", SessionID: tt.recordedSession})
			}

			if code, env := h.run(t); code != 0 {
				t.Fatalf("beamUp exit %d, env %v; want 0", code, env)
			}

			wantStarts, wantResume := 1, tt.recordedSession
			if tt.resumeErr != nil {
				wantStarts, wantResume = 2, ""
			}
			if len(h.starter.specs) != wantStarts {
				t.Fatalf("started %d sessions; want %d", len(h.starter.specs), wantStarts)
			}
			spec := h.starter.specs[len(h.starter.specs)-1]
			if spec.RemoteControlName != tt.wantName {
				t.Errorf("orch spec RemoteControlName = %q; want %q", spec.RemoteControlName, tt.wantName)
			}
			if spec.ResumeSessionID != wantResume {
				t.Errorf("orch spec ResumeSessionID = %q; want %q", spec.ResumeSessionID, wantResume)
			}
			if got := strings.Join(awaited, "; "); got != tt.wantAwait {
				t.Errorf("awaited the connection from %q; want %q", got, tt.wantAwait)
			}
			var wantMarked []string
			if tt.wantMark {
				wantMarked = []string{"new-guid"}
			}
			if !slices.Equal(marked, wantMarked) {
				t.Errorf("marked strands = %v; want %v", marked, wantMarked)
			}
		})
	}
}

// TestRemoteControlOnVerb_QueuesForTheClientsPaneWithoutMarkingIt runs the rc-on verb against a fake tmux script.
// An agent pane gets the command queued and is left unmarked, since only its session's recorded connection lights it,
// and a client whose active pane cannot be read fails the verb naming the client, with nothing queued or marked.
func TestRemoteControlOnVerb_QueuesForTheClientsPaneWithoutMarkingIt(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the fake tmux is a shell script")
	}
	tests := []struct {
		name string
		// paneReadFails makes the fake tmux fail the list-clients that reads the active pane.
		paneReadFails bool
		wantCode      int
		wantError     string
		wantInject    string
	}{
		{name: "AgentPaneIsQueuedAndLeftUnmarked", wantInject: "tst:orch neve::tst:orch"},
		{name: "UnreadablePaneNamesTheClient", paneReadFails: true, wantCode: 1, wantError: `read the active pane of client "/dev/pts/3"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			log := filepath.Join(dir, "calls.log")
			paneRead := "printf '%s\\t%s\\t%s\\t\\n' '/dev/pts/3' '%7' 'tst:orch'"
			if tt.paneReadFails {
				paneRead = "echo no server >&2; exit 1"
			}
			script := "#!/bin/sh\necho \"$*\" >> '" + log + "'\ncase \"$*\" in\n*'list-clients -F '*) " + paneRead + " ;;\nesac\n"
			tmuxPath := filepath.Join(dir, "tmux")
			if err := os.WriteFile(tmuxPath, []byte(script), 0o755); err != nil {
				t.Fatalf("write the fake tmux: %v", err)
			}
			c := newTestCLI(t, &fakeStrands{})
			c.cfg.RemoteControlName = remoteControlTemplate
			c.machineName = func() (string, error) { return "neve", nil }
			var injected []string
			c.injectRemoteControl = func(receiver, name string) error {
				injected = append(injected, receiver+" "+name)
				return nil
			}

			var out bytes.Buffer
			code := clihelp.Execute(c.remoteControlOnCmd(), &out, []string{"--socket", "/tmp/sock", "--tmux", tmuxPath, "--client", "/dev/pts/3"})

			var env map[string]any
			if err := json.Unmarshal(out.Bytes(), &env); err != nil {
				t.Fatalf("output %q is not one JSON object: %v", out.String(), err)
			}
			if code != tt.wantCode {
				t.Errorf("rc-on exit %d, env %v; want %d", code, env, tt.wantCode)
			}
			if got, _ := env["error"].(string); !strings.Contains(got, tt.wantError) {
				t.Errorf("rc-on error = %q; want it to contain %q", got, tt.wantError)
			}
			if got := strings.Join(injected, "; "); got != tt.wantInject {
				t.Errorf("injected %q; want %q", got, tt.wantInject)
			}
			calls, err := os.ReadFile(log)
			if err != nil {
				t.Fatalf("read the fake tmux's call log: %v", err)
			}
			if strings.Contains(string(calls), "set-option") {
				t.Errorf("tmux calls %q; want the pane mark left alone", calls)
			}
		})
	}
}

// TestTurnOnRemoteControl pins the RC click:
// it queues the command under the name remote_control_name forms for the clicked agent,
// and every refusal queues nothing.
func TestTurnOnRemoteControl(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	injectFailed := errors.New("creel is down")
	tests := []struct {
		name      string
		pane      reedengine.AgentPane
		injectErr error
		// wantInject is the receiver and name queued, empty for none.
		wantInject string
		wantErr    string
	}{
		{
			name:       "Queues",
			pane:       reedengine.AgentPane{PaneID: "%1", StrandName: "tst:orch"},
			wantInject: "tst:orch neve::tst:orch",
		},
		{
			name:       "ServesAnAgentOtherThanTheOrch",
			pane:       reedengine.AgentPane{PaneID: "%2", StrandName: "tst:slug:darn"},
			wantInject: "tst:slug:darn neve::tst:slug:darn",
		},
		{name: "PaneWithNoAgentRefuses", pane: reedengine.AgentPane{PaneID: "%3"}, wantErr: "runs no agent"},
		{
			name:    "PaneAlreadyMarkedRefuses",
			pane:    reedengine.AgentPane{PaneID: "%1", StrandName: "tst:orch", RemoteControlOn: true},
			wantErr: "already on",
		},
		{
			name:    "UnparseableStrandNameFails",
			pane:    reedengine.AgentPane{PaneID: "%4", StrandName: "not-an-agent-name"},
			wantErr: "does not parse",
		},
		{
			name:       "FailedInjectFails",
			pane:       reedengine.AgentPane{PaneID: "%1", StrandName: "tst:orch"},
			injectErr:  injectFailed,
			wantInject: "tst:orch neve::tst:orch", wantErr: injectFailed.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := newTestCLI(t, &fakeStrands{})
			c.cfg.RemoteControlName = remoteControlTemplate
			c.machineName = func() (string, error) { return "neve", nil }
			var injected []string
			c.injectRemoteControl = func(receiver, name string) error {
				injected = append(injected, receiver+" "+name)
				return tt.injectErr
			}

			_, err := c.turnOnRemoteControl(tt.pane)

			if tt.wantErr == "" && err != nil {
				t.Fatalf("turnOnRemoteControl() = %v; want nil", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("turnOnRemoteControl() = %v; want an error containing %q", err, tt.wantErr)
			}
			if got := strings.Join(injected, "; "); got != tt.wantInject {
				t.Errorf("injected %q; want %q", got, tt.wantInject)
			}
		})
	}
}
