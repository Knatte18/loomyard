// step_test.go covers step's two pure decisions -- stepEnvelope's key set and field mapping, and
// stepKindForBootstrapStage's classification -- plus the closed refusal-kind vocabulary itself, and
// drives the busy refusal end-to-end against a hand-populated receiver, per the
// new-tests-stay-untagged-and-pure Shared Decision: step's success envelope cannot be reached in an
// untagged test without a real fabric and a real reed session, so these pure decisions are tested
// directly instead.

package loomcli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// TestStepEnvelope_KeySetIsExactlyTen asserts stepEnvelope's returned map carries exactly the ten
// documented keys, no more and no fewer, built and compared in both directions so a key added
// without a test fails here.
func TestStepEnvelope_KeySetIsExactlyTen(t *testing.T) {
	res := shedengine.StepResult{
		Producer: "Discussion-Write",
		Outcome:  shedengine.Done,
		Output:   "some/output.md",
		Next:     "Discussion-Bouncer",
		State:    shedengine.StateRunning,
		Reason:   "",
		History:  []shedengine.HistoryEntry{{Producer: "Discussion-Write", Outcome: shedengine.Done}},
	}
	envelope := stepEnvelope(res, "reinvoke", "/tmp/status.json")

	want := []string{
		"producer", "outcome", "output", "next", "state", "reason",
		"continue", "history_length", "next_interrupt_policy", "status_file",
	}

	if len(envelope) != len(want) {
		t.Fatalf("len(stepEnvelope(...)) = %d; want %d", len(envelope), len(want))
	}
	for _, key := range want {
		if _, ok := envelope[key]; !ok {
			t.Errorf("stepEnvelope(...) missing key %q", key)
		}
	}
	for key := range envelope {
		found := false
		for _, w := range want {
			if key == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("stepEnvelope(...) carries undocumented key %q", key)
		}
	}
}

// TestStepEnvelope_ContinueTracksRunningState drives "continue" across all five State values and
// asserts it is true exactly when res.State is shedengine.StateRunning.
func TestStepEnvelope_ContinueTracksRunningState(t *testing.T) {
	states := []shedengine.State{
		shedengine.StateRunning,
		shedengine.StatePaused,
		shedengine.StateDone,
		shedengine.StateBlocked,
		shedengine.StateFailed,
	}
	for _, state := range states {
		t.Run(string(state), func(t *testing.T) {
			envelope := stepEnvelope(shedengine.StepResult{State: state}, "", "")
			want := state == shedengine.StateRunning
			if envelope["continue"] != want {
				t.Errorf("stepEnvelope(state=%q)[\"continue\"] = %v; want %v", state, envelope["continue"], want)
			}
		})
	}
}

// TestStepEnvelope_FieldMapping tables the routing shapes batch 1 pins, asserting each field's
// mapping onto the envelope.
func TestStepEnvelope_FieldMapping(t *testing.T) {
	tests := []struct {
		name         string
		res          shedengine.StepResult
		wantProducer string
		wantOutcome  string
		wantNext     string
		wantState    string
		wantContinue bool
		wantReason   string
	}{
		{
			name: "DoneWithOnDone",
			res: shedengine.StepResult{
				Producer: "Discussion-Write",
				Outcome:  shedengine.Done,
				Next:     "Discussion-Bouncer",
				State:    shedengine.StateRunning,
				History:  []shedengine.HistoryEntry{{Producer: "Discussion-Write", Outcome: shedengine.Done}},
			},
			wantProducer: "Discussion-Write",
			wantOutcome:  string(shedengine.Done),
			wantNext:     "Discussion-Bouncer",
			wantState:    string(shedengine.StateRunning),
			wantContinue: true,
		},
		{
			name: "DoneTerminal",
			res: shedengine.StepResult{
				Producer: "Finalize",
				Outcome:  shedengine.Done,
				Next:     "Finalize",
				State:    shedengine.StateDone,
				History:  []shedengine.HistoryEntry{{Producer: "Finalize", Outcome: shedengine.Done}},
			},
			wantProducer: "Finalize",
			wantOutcome:  string(shedengine.Done),
			wantNext:     "Finalize",
			wantState:    string(shedengine.StateDone),
			wantContinue: false,
		},
		{
			name: "StuckWithinBudget",
			res: shedengine.StepResult{
				Producer: "Discussion-Bouncer",
				Outcome:  shedengine.Stuck,
				Next:     "Discussion-Burler",
				State:    shedengine.StateRunning,
				History:  []shedengine.HistoryEntry{{Producer: "Discussion-Bouncer", Outcome: shedengine.Stuck}},
			},
			wantProducer: "Discussion-Bouncer",
			wantOutcome:  string(shedengine.Stuck),
			wantNext:     "Discussion-Burler",
			wantState:    string(shedengine.StateRunning),
			wantContinue: true,
		},
		{
			name: "Blocked",
			res: shedengine.StepResult{
				Producer: "Discussion-Bouncer",
				Outcome:  shedengine.Stuck,
				Next:     "Discussion-Bouncer",
				State:    shedengine.StateBlocked,
				Reason:   "bounce budget exhausted",
				History:  []shedengine.HistoryEntry{{Producer: "Discussion-Bouncer", Outcome: shedengine.Stuck}},
			},
			wantProducer: "Discussion-Bouncer",
			wantOutcome:  string(shedengine.Stuck),
			wantNext:     "Discussion-Bouncer",
			wantState:    string(shedengine.StateBlocked),
			wantContinue: false,
			wantReason:   "bounce budget exhausted",
		},
		{
			name: "AlreadyDoneShortCircuit",
			res: shedengine.StepResult{
				Next:    "Finalize",
				State:   shedengine.StateDone,
				History: []shedengine.HistoryEntry{{Producer: "Finalize", Outcome: shedengine.Done}},
			},
			wantProducer: "",
			wantOutcome:  "",
			wantNext:     "Finalize",
			wantState:    string(shedengine.StateDone),
			wantContinue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envelope := stepEnvelope(tt.res, "", "/some/status.json")

			if got := envelope["producer"]; got != tt.wantProducer {
				t.Errorf("envelope[\"producer\"] = %v; want %v", got, tt.wantProducer)
			}
			if got := envelope["outcome"]; got != tt.wantOutcome {
				t.Errorf("envelope[\"outcome\"] = %v; want %v", got, tt.wantOutcome)
			}
			if got := envelope["next"]; got != tt.wantNext {
				t.Errorf("envelope[\"next\"] = %v; want %v", got, tt.wantNext)
			}
			if got := envelope["state"]; got != tt.wantState {
				t.Errorf("envelope[\"state\"] = %v; want %v", got, tt.wantState)
			}
			if got := envelope["continue"]; got != tt.wantContinue {
				t.Errorf("envelope[\"continue\"] = %v; want %v", got, tt.wantContinue)
			}
			if got := envelope["reason"]; got != tt.wantReason {
				t.Errorf("envelope[\"reason\"] = %v; want %v", got, tt.wantReason)
			}
			if got := envelope["history_length"]; got != len(tt.res.History) {
				t.Errorf("envelope[\"history_length\"] = %v; want %v", got, len(tt.res.History))
			}
			if got := envelope["status_file"]; got != "/some/status.json" {
				t.Errorf("envelope[\"status_file\"] = %v; want %v", got, "/some/status.json")
			}
		})
	}
}

// TestStepEnvelope_NextInterruptPolicyMatchesTable asserts next_interrupt_policy equals
// loomshed.InterruptPolicyFor(res.Next), driven over a reinvoke row, the one handback row
// (loomshed.NameWebster), and an empty res.Next.
func TestStepEnvelope_NextInterruptPolicyMatchesTable(t *testing.T) {
	tests := []struct {
		name string
		next string
		want string
	}{
		{"ReinvokeRow", loomshed.NamePlanWrite, loomshed.InterruptPolicyReinvoke},
		{"HandbackRow", loomshed.NameWebster, loomshed.InterruptPolicyHandback},
		{"EmptyNext", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextPolicy := loomshed.InterruptPolicyFor(tt.next)
			envelope := stepEnvelope(shedengine.StepResult{Next: tt.next}, nextPolicy, "")
			if got := envelope["next_interrupt_policy"]; got != tt.want {
				t.Errorf("envelope[\"next_interrupt_policy\"] = %v; want %v", got, tt.want)
			}
		})
	}
}

// TestStepKinds_ClosedSetOfFive asserts stepKinds contains exactly the five declared refusal-kind
// constants, each non-empty and distinct, so an undeclared sixth kind cannot ship silently.
func TestStepKinds_ClosedSetOfFive(t *testing.T) {
	if len(stepKinds) != 5 {
		t.Fatalf("len(stepKinds) = %d; want 5", len(stepKinds))
	}

	seen := make(map[string]bool, len(stepKinds))
	for _, kind := range stepKinds {
		if kind == "" {
			t.Error("stepKinds contains an empty entry")
		}
		if seen[kind] {
			t.Errorf("stepKinds contains a duplicate entry %q", kind)
		}
		seen[kind] = true
	}

	for _, want := range []string{stepKindBusy, stepKindUnseeded, stepKindOwnership, stepKindBootstrap, stepKindProducer} {
		if !seen[want] {
			t.Errorf("stepKinds is missing declared constant %q", want)
		}
	}
}

// TestStepKindForBootstrapStage tables the mapping from bootstrapStage to step's refusal-kind
// vocabulary: bootstrapStageSeed maps to stepKindUnseeded, bootstrapStageOwnership maps to
// stepKindOwnership, and every other stage -- bootstrapStageOrigin, bootstrapStageCommit, and
// bootstrapStageNone included -- maps to stepKindBootstrap.
func TestStepKindForBootstrapStage(t *testing.T) {
	tests := []struct {
		name  string
		stage bootstrapStage
		want  string
	}{
		{"Seed", bootstrapStageSeed, stepKindUnseeded},
		{"Ownership", bootstrapStageOwnership, stepKindOwnership},
		{"Origin", bootstrapStageOrigin, stepKindBootstrap},
		{"Commit", bootstrapStageCommit, stepKindBootstrap},
		{"None", bootstrapStageNone, stepKindBootstrap},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stepKindForBootstrapStage(tt.stage); got != tt.want {
				t.Errorf("stepKindForBootstrapStage(%v) = %q; want %q", tt.stage, got, tt.want)
			}
		})
	}
}

// TestStepCmd_BusyRefusal_BeforeBootstrap drives stepCmd()'s RunE against a hand-populated receiver
// whose shedPaths.LockPath points into a t.TempDir() and whose lock this test acquires first,
// mirroring the in-process capture idiom TestVerbRefusals (cli_test.go) already uses for drive/
// pause. It confirms the busy refusal reaches the envelope with its remedy text, and that the
// refusal happened before the bootstrap: seedAndCommitBootstrap would have seeded a status file, so
// its absence afterwards proves the early probe fired first.
func TestStepCmd_BusyRefusal_BeforeBootstrap(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "run.lock")

	held, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("lock.AcquireWriteLock(%q) = %v; want nil", lockPath, err)
	}
	t.Cleanup(func() { _ = held.Release() })

	c := &loomCLI{
		location: &lyxcwd.Location{HubPath: dir, WorktreeName: "warp", AnchorRel: "."},
		shedPaths: loomrecipe.ShedPaths{
			LockPath:       lockPath,
			StatusPath:     filepath.Join(dir, "status.json"),
			StatusLockPath: filepath.Join(dir, "status.json.lock"),
		},
	}

	var out bytes.Buffer
	exitCode := clihelp.Execute(c.stepCmd(), &out, nil)

	if exitCode != 1 {
		t.Errorf("stepCmd() exit code = %d; want 1", exitCode)
	}
	if !strings.Contains(out.String(), `"ok":false`) {
		t.Errorf("stepCmd() output missing ok:false envelope; got: %q", out.String())
	}
	if !strings.Contains(out.String(), `"kind":"busy"`) {
		t.Errorf("stepCmd() output missing kind:busy; got: %q", out.String())
	}
	if !strings.Contains(out.String(), "lyx loom pause") {
		t.Errorf("stepCmd() output missing the lyx loom pause remedy; got: %q", out.String())
	}

	if _, err := os.Stat(c.shedPaths.StatusPath); err == nil {
		t.Errorf("status file exists at %q after a busy refusal; the bootstrap must not have run", c.shedPaths.StatusPath)
	}
}
