// step_test.go covers loom's own step wiring: the interrupt policy it threads onto the envelope, the bootstrap-stage classification, and the busy refusal driven end-to-end against a hand-populated receiver, per the new-tests-stay-untagged-and-pure Shared Decision.
// step's success envelope cannot be reached in an untagged test without a real fabric and a real reed session,
// so these pure decisions are tested directly instead.
// The generic envelope and the refusal-kind vocabulary are tested in internal/shedverbs.

package loomcli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
)

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
			envelope := shedverbs.StepEnvelope(shedengine.StepResult{Next: tt.next}, nextPolicy, "", "", shedverbs.StepLocations{}, nil)
			if got := envelope["next_interrupt_policy"]; got != tt.want {
				t.Errorf("envelope[\"next_interrupt_policy\"] = %v; want %v", got, tt.want)
			}
		})
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
		{"Seed", bootstrapStageSeed, shedverbs.KindUnseeded},
		{"Ownership", bootstrapStageOwnership, shedverbs.KindOwnership},
		{"Origin", bootstrapStageOrigin, shedverbs.KindBootstrap},
		{"Commit", bootstrapStageCommit, shedverbs.KindBootstrap},
		{"None", bootstrapStageNone, shedverbs.KindBootstrap},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stepKindForBootstrapStage(tt.stage); got != tt.want {
				t.Errorf("stepKindForBootstrapStage(%v) = %q; want %q", tt.stage, got, tt.want)
			}
		})
	}
}

// TestStepCmd_BusyRefusal_BeforeBootstrap drives stepCmd()'s RunE against a hand-populated receiver whose shedPaths.LockPath points into a t.TempDir() and whose lock this test acquires first,
// mirroring the in-process capture idiom TestVerbRefusals (cli_test.go) already uses for run/pause.
// It confirms the busy refusal reaches the envelope with its remedy text,
// and that the refusal happened before the bootstrap and the entry observation.
// seedAndCommitBootstrap would have written the loom seed, so its absence afterwards proves the early probe fired first.
// The status file is seeded to match the handoff voucher,
// so an observation taken before the probe would consume that voucher and write a crash-resume note.
func TestStepCmd_BusyRefusal_BeforeBootstrap(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "run.lock")

	held, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("lock.AcquireWriteLock(%q) = %v; want nil", lockPath, err)
	}
	t.Cleanup(func() { _ = held.Release() })

	c := &loomCLI{
		location:    &lyxcwd.Location{HubPath: dir, WorktreeName: "pair", AnchorRel: "."},
		frictionDir: filepath.Join(dir, "friction"),
		shedPaths: shedbuild.ShedPaths{
			LockPath:       lockPath,
			StatusPath:     filepath.Join(dir, "status.json"),
			StatusLockPath: filepath.Join(dir, "status.json.lock"),
		},
	}
	handoffPath := loomengine.LoomHandoffVoucher(c.location)
	if err := os.MkdirAll(filepath.Dir(handoffPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(handoffPath), err)
	}
	recordHandoffVoucher(handoffPath, loomengine.LoomHandoffVoucherLock(c.location), 1, shedengine.StateRunning)
	writeStatusFixture(t, c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, shedengine.Status{
		CurrentProducer: loomshed.NameWebster,
		State:           shedengine.StateRunning,
		History:         make([]shedengine.HistoryEntry, 1),
	})

	var out bytes.Buffer
	exitCode := clihelp.Execute(loomVerbCommand(c, "step"), &out, nil)

	if exitCode != 1 {
		t.Errorf("stepCmd() exit code = %d; want 1", exitCode)
	}
	if !strings.Contains(out.String(), `"ok":false`) {
		t.Errorf("stepCmd() output missing ok:false envelope; got: %q", out.String())
	}
	if !strings.Contains(out.String(), `"kind":"busy"`) {
		t.Errorf("stepCmd() output missing kind:busy; got: %q", out.String())
	}
	if !strings.Contains(out.String(), `"transient":""`) {
		t.Errorf("stepCmd() output missing transient:\"\"; got: %q", out.String())
	}
	if !strings.Contains(out.String(), "lyx loom pause") {
		t.Errorf("stepCmd() output missing the lyx loom pause remedy; got: %q", out.String())
	}

	if _, found, err := shedrun.ReadSeed(c.location, shedrun.SelfRunID); err != nil || found {
		t.Errorf("shedrun.ReadSeed after a busy refusal = found %v, %v; want no seed (the bootstrap must not have run)", found, err)
	}
	if _, err := os.Stat(handoffPath); err != nil {
		t.Errorf("handoff voucher %q = %v after a busy refusal; want it left in place", handoffPath, err)
	}
	notePath := filepath.Join(c.frictionDir, "loom-crash-resume.md")
	if _, err := os.Stat(notePath); !os.IsNotExist(err) {
		t.Errorf("crash-resume note %q = %v after a busy refusal; want none written", notePath, err)
	}
}
