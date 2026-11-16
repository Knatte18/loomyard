// remotecontrol_test.go covers the orch's remote control without a live session:
// the name a launch passes and records, the RC click over a fake strandOps and inject, and the RC button's state word.

package orchcli

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// remoteControlTemplate is orch.yaml's template value of remote_control_name.
const remoteControlTemplate = "{machine}::{name}"

// TestStart_LaunchPassesAndRecordsRemoteControl pins the name a relaunch passes on the orch spec.
// It also pins the remote control the relaunch records for the new strand, as the RC button's state word shows it.
func TestStart_LaunchPassesAndRecordsRemoteControl(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name     string
		switchOn bool
		// noRemoteControl runs the orch on an engine set whose default has no remote control.
		noRemoteControl bool
		// unlisted launches with no listed orch strand, so the name forms from the worktree's fixed strand name.
		// The harness hub is no git repository, so that name cannot be formed and the launch goes without remote control.
		unlisted bool
		// machineErr fails the machine name the remote session's name carries.
		machineErr error
		// recordedSession seeds the orch state with this session id, so the launch resumes it.
		recordedSession string
		wantName        string
		wantWord        string
	}{
		{name: "SwitchOn", switchOn: true, wantName: "neve::tst:orch", wantWord: reedengine.RemoteControlLit},
		{name: "SwitchOff", wantWord: reedengine.RemoteControlUnlit},
		{name: "EngineWithoutRemoteControl", switchOn: true, noRemoteControl: true},
		{name: "NoListedStrandWithoutAFixedName", switchOn: true, unlisted: true, wantWord: reedengine.RemoteControlUnlit},
		{name: "MachineNameFails", switchOn: true, machineErr: errors.New("no hostname"), wantWord: reedengine.RemoteControlUnlit},
		{name: "ResumedSession", switchOn: true, recordedSession: "sess-1", wantName: "neve::tst:orch", wantWord: reedengine.RemoteControlLit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var listed []reedengine.StrandStatus
			if !tt.unlisted {
				listed = append(listed, reedengine.StrandStatus{GUID: "old-guid", Name: "tst:orch"})
			}
			h := newStartHarness(t, listed...)
			h.cli.cfg.RemoteControl, h.cli.cfg.RemoteControlName = tt.switchOn, remoteControlTemplate
			h.cli.machineName = func() (string, error) { return "neve", tt.machineErr }
			if tt.noRemoteControl {
				h.cli.engines = shuttleengine.NewEngineSet("none", nil)
			}
			if tt.recordedSession != "" {
				saveSeedState(t, h, orchengine.State{Phase: orchengine.PhaseIdle, Strand: "old-guid", SessionID: tt.recordedSession})
			}

			if code, env := h.run(t); code != 0 {
				t.Fatalf("beamUp exit %d, env %v; want 0", code, env)
			}

			if len(h.starter.specs) != 1 {
				t.Fatalf("started %d sessions; want 1", len(h.starter.specs))
			}
			spec := h.starter.specs[0]
			if spec.RemoteControlName != tt.wantName {
				t.Errorf("orch spec RemoteControlName = %q; want %q", spec.RemoteControlName, tt.wantName)
			}
			if spec.ResumeSessionID != tt.recordedSession {
				t.Errorf("orch spec ResumeSessionID = %q; want %q", spec.ResumeSessionID, tt.recordedSession)
			}
			if got := remoteControlBarWordAt(h.cli.paths); got != tt.wantWord {
				t.Errorf("remoteControlBarWordAt() = %q; want %q", got, tt.wantWord)
			}
		})
	}
}

// TestTurnOnRemoteControl pins the RC click:
// it queues the command under the launch's name into the live orch strand and records the strand on,
// and every refusal queues nothing and leaves the record as it was.
func TestTurnOnRemoteControl(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	liveOrch := reedengine.StrandStatus{GUID: "g1", Name: "tst:orch", Live: true}
	injectFailed := errors.New("creel is down")
	tests := []struct {
		name            string
		strands         []reedengine.StrandStatus
		noRemoteControl bool
		// recordedOn seeds the record of g1 as on.
		recordedOn bool
		// blockRecord makes the record file unwritable.
		blockRecord bool
		injectErr   error
		wantInject  string
		wantErr     string
		wantOn      bool
		// wantWarning expects the success to carry a warning field.
		wantWarning bool
	}{
		{name: "QueuesAndRecords", strands: []reedengine.StrandStatus{liveOrch}, wantInject: "tst:orch neve::tst:orch", wantOn: true},
		{name: "NoLiveOrch", strands: []reedengine.StrandStatus{{GUID: "g1", Name: "tst:orch"}}, wantErr: "no live orch strand"},
		{name: "NoOrch", wantErr: "no live orch strand"},
		{name: "TwoOrchStrands", strands: []reedengine.StrandStatus{liveOrch, {GUID: "g2", Name: "tst:orch", Live: true}}, wantErr: "remove the extras with `lym reed remove`"},
		{name: "EngineWithoutRemoteControl", strands: []reedengine.StrandStatus{liveOrch}, noRemoteControl: true, wantErr: "no remote control"},
		{name: "AlreadyOn", strands: []reedengine.StrandStatus{liveOrch}, recordedOn: true, wantErr: "already on", wantOn: true},
		{name: "FailedInject", strands: []reedengine.StrandStatus{liveOrch}, injectErr: injectFailed, wantInject: "tst:orch neve::tst:orch", wantErr: injectFailed.Error()},
		{name: "FailedRecordWriteStillSucceeds", strands: []reedengine.StrandStatus{liveOrch}, blockRecord: true, wantInject: "tst:orch neve::tst:orch", wantWarning: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := newTestCLI(t, &fakeStrands{strands: tt.strands})
			c.cfg.RemoteControlName = remoteControlTemplate
			c.machineName = func() (string, error) { return "neve", nil }
			if tt.noRemoteControl {
				c.engines = shuttleengine.NewEngineSet("none", nil)
			}
			if tt.recordedOn {
				if err := orchengine.WriteRemoteControl(c.paths, orchengine.RemoteControlRecord{Strand: "g1", On: true}); err != nil {
					t.Fatalf("WriteRemoteControl: %v", err)
				}
			}
			if tt.blockRecord {
				if err := os.MkdirAll(c.paths.RemoteControlPath, 0o755); err != nil {
					t.Fatalf("block the record file: %v", err)
				}
			}
			var injected []string
			c.injectRemoteControl = func(receiver, name string) error {
				injected = append(injected, receiver+" "+name)
				return tt.injectErr
			}

			fields, err := c.turnOnRemoteControl()

			if _, hasWarning := fields["warning"]; hasWarning != tt.wantWarning {
				t.Errorf("fields = %v; want a warning field: %v", fields, tt.wantWarning)
			}
			if tt.wantErr == "" && err != nil {
				t.Fatalf("turnOnRemoteControl() = %v; want nil", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("turnOnRemoteControl() = %v; want an error containing %q", err, tt.wantErr)
			}
			if got := strings.Join(injected, "; "); got != tt.wantInject {
				t.Errorf("injected %q; want %q", got, tt.wantInject)
			}
			if tt.blockRecord {
				return
			}
			on, readErr := orchengine.RemoteControlOn(c.paths, "g1")
			if readErr != nil || on != tt.wantOn {
				t.Errorf("RemoteControlOn(g1) = %v, %v; want %v, nil", on, readErr, tt.wantOn)
			}
		})
	}
}

// TestRemoteControlBarWordAt pins the RC button's state word:
// none without a recorded strand or a record naming it.
// Lit for a strand recorded on, and unlit for a strand recorded off or whose record cannot be read.
func TestRemoteControlBarWordAt(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name   string
		state  *orchengine.State
		record *orchengine.RemoteControlRecord
		// rawRecord, when set, is written as the record file's bytes.
		rawRecord string
		want      string
	}{
		{name: "NoState"},
		{name: "NoStrand", state: &orchengine.State{Phase: orchengine.PhaseIdle}},
		{name: "NoRecord", state: &orchengine.State{Strand: "g1"}},
		{name: "RecordedOn", state: &orchengine.State{Strand: "g1"}, record: &orchengine.RemoteControlRecord{Strand: "g1", On: true}, want: reedengine.RemoteControlLit},
		{name: "RecordedOff", state: &orchengine.State{Strand: "g1"}, record: &orchengine.RemoteControlRecord{Strand: "g1"}, want: reedengine.RemoteControlUnlit},
		{name: "RecordedOnForAnOldStrand", state: &orchengine.State{Strand: "g2"}, record: &orchengine.RemoteControlRecord{Strand: "g1", On: true}},
		{name: "UnreadableRecord", state: &orchengine.State{Strand: "g1"}, rawRecord: "{not json", want: reedengine.RemoteControlUnlit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			paths := newTestCLI(t, &fakeStrands{}).paths
			if tt.state != nil {
				if err := orchengine.SaveState(paths, *tt.state); err != nil {
					t.Fatalf("SaveState: %v", err)
				}
			}
			if tt.record != nil {
				if err := orchengine.WriteRemoteControl(paths, *tt.record); err != nil {
					t.Fatalf("WriteRemoteControl: %v", err)
				}
			}
			if tt.rawRecord != "" {
				if err := os.WriteFile(paths.RemoteControlPath, []byte(tt.rawRecord), 0o644); err != nil {
					t.Fatalf("write the record file: %v", err)
				}
			}
			if got := remoteControlBarWordAt(paths); got != tt.want {
				t.Errorf("remoteControlBarWordAt() = %q; want %q", got, tt.want)
			}
		})
	}
}

// TestRemoteControlBarVerb_PrintsNothingForNoWorktree pins what the rc-bar verb prints, and it skips the orch wiring.
// It prints an empty line and exits 0 for no directory and for a directory that is no worktree.
func TestRemoteControlBarVerb_PrintsNothingForNoWorktree(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		args []string
	}{
		{"NoDir", []string{remoteControlBarVerb}},
		{"UnresolvableDir", []string{remoteControlBarVerb, "--dir", t.TempDir()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			code := clihelp.ExecuteIn(Command(), t.TempDir(), &out, tt.args)
			if code != 0 || out.String() != "\n" {
				t.Errorf("rc-bar exit %d, output %q; want exit 0 and an empty line", code, out.String())
			}
		})
	}
}
