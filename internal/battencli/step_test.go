// step_test.go covers battenPreStep and the step verb's whole new path end to end, since none of
// run_test.go's coverage touches it: seeding an absent status before shed.Step reads it, each of
// the three refusal-kind mappings, and BuildShed being non-nil for verb == "step". Every fixture
// here is newFakeReceiver (run_test.go), so no real I/O beyond t.TempDir(), no git spawn, and no
// process spawn.

package battencli

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/state"
)

// TestBattenPreStep_SeedsAbsentStatus asserts battenPreStep seeds a fresh status file -- current
// producer Worktree-Create, state running -- when none is persisted yet, before shed.Step ever
// reads it. Without this, stepLocked's own read gate would hit an absent status and hard-error,
// which shedverbs/step.go reports as kind: "producer" -- the one kind ly-drive retries, looping a
// fresh slug's first step forever.
func TestBattenPreStep_SeedsAbsentStatus(t *testing.T) {
	c := newFakeReceiver(t, nil)

	if _, found, err := state.ReadJSONStrict[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath); err != nil || found {
		t.Fatalf("precondition: status read = (found=%v, err=%v); want (false, nil)", found, err)
	}

	kind, err := c.battenPreStep(context.Background())
	if err != nil {
		t.Fatalf("battenPreStep() = (%q, %v); want (\"\", nil)", kind, err)
	}
	if kind != "" {
		t.Errorf("battenPreStep() kind = %q; want empty on success", kind)
	}

	st, found, err := state.ReadJSONStrict[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
	if err != nil || !found {
		t.Fatalf("status after battenPreStep = (found=%v, err=%v); want (true, nil)", found, err)
	}
	if st.CurrentProducer != battenrecipe.NameWorktreeCreate {
		t.Errorf("st.CurrentProducer = %q; want %q", st.CurrentProducer, battenrecipe.NameWorktreeCreate)
	}
	if st.State != shedengine.StateRunning {
		t.Errorf("st.State = %q; want %q", st.State, shedengine.StateRunning)
	}
}

// TestBattenPreStep_RunLockHeld_KindBusy asserts battenPreStep returns shedverbs.KindBusy, with a
// non-nil error, when the run lock is already held.
func TestBattenPreStep_RunLockHeld_KindBusy(t *testing.T) {
	c := newFakeReceiver(t, nil)

	held, err := lock.AcquireWriteLock(c.shedPaths.LockPath)
	if err != nil {
		t.Fatalf("acquire run lock in test: %v", err)
	}
	t.Cleanup(func() { _ = held.Release() })

	kind, err := c.battenPreStep(context.Background())
	if err == nil {
		t.Fatal("battenPreStep() = nil error; want a refusal -- the run lock is already held")
	}
	if kind != shedverbs.KindBusy {
		t.Errorf("battenPreStep() kind = %q; want %q", kind, shedverbs.KindBusy)
	}
}

// TestBattenPreStep_DecodeFailure_KindUnseeded asserts battenPreStep returns
// shedverbs.KindUnseeded when the status file exists but fails to decode.
func TestBattenPreStep_DecodeFailure_KindUnseeded(t *testing.T) {
	c := newFakeReceiver(t, nil)

	if err := os.WriteFile(c.shedPaths.StatusPath, []byte("not valid json{{{"), 0o644); err != nil {
		t.Fatalf("seed corrupt status file: %v", err)
	}

	kind, err := c.battenPreStep(context.Background())
	if err == nil {
		t.Fatal("battenPreStep() = nil error; want a refusal -- the status file is corrupt")
	}
	if kind != shedverbs.KindUnseeded {
		t.Errorf("battenPreStep() kind = %q; want %q", kind, shedverbs.KindUnseeded)
	}
}

// TestBattenPreStep_DoneSlug_KindBootstrap asserts battenPreStep returns shedverbs.KindBootstrap
// for a status already in StateDone -- "any other pre-producer failure" in the batch's own
// refusal-kind mapping, distinct from both KindBusy and KindUnseeded.
func TestBattenPreStep_DoneSlug_KindBootstrap(t *testing.T) {
	c := newFakeReceiver(t, nil)
	writeStatus(t, c, shedengine.Status{
		CurrentProducer: battenrecipe.NameWorktreeTeardown,
		State:           shedengine.StateDone,
	})

	kind, err := c.battenPreStep(context.Background())
	if err == nil {
		t.Fatal("battenPreStep() = nil error; want a refusal -- the slug has already completed")
	}
	if kind != shedverbs.KindBootstrap {
		t.Errorf("battenPreStep() kind = %q; want %q", kind, shedverbs.KindBootstrap)
	}
	// The remedy must be the whole abandon path: a torn-down pair keeps its branch and both remote
	// copies, so deleting the run directory alone leads straight into the create row's
	// leftover-branch refusal.
	for _, want := range []string{"delete its run directory", "local and remote", "to run it again"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("battenPreStep() error = %q; want it to contain %q", err.Error(), want)
		}
	}
}

// TestSpecFor_StepBuildShedNonNil asserts c.specFor("step").BuildShed is non-nil, mirroring "run"'s
// own fill.
func TestSpecFor_StepBuildShedNonNil(t *testing.T) {
	c := &battenCLI{}
	if c.specFor("step").BuildShed == nil {
		t.Error("specFor(\"step\").BuildShed = nil; want a non-nil constructor")
	}
}

// TestStepCmd_FreshSlugNeverSurfacesKindProducer drives the step verb end to end, against a fresh
// slug with no persisted status at all, and asserts it never surfaces kind: "producer" -- the
// likeliest regression here, since conflating the unseeded and producer cases both read as
// "nothing here yet". battenPreStep's own seed lets shed.Step's Worktree-Create row run to
// completion against newFakeReceiver's fakes.
func TestStepCmd_FreshSlugNeverSurfacesKindProducer(t *testing.T) {
	c := newFakeReceiver(t, nil)

	var out bytes.Buffer
	exitCode := clihelp.Execute(battenVerbCommand(c, "step"), &out, []string{c.slug})

	if strings.Contains(out.String(), `"kind":"producer"`) {
		t.Errorf("step() output = %q; must never surface kind: \"producer\" for a fresh slug", out.String())
	}
	if exitCode != 0 {
		t.Errorf("step() exit code = %d; want 0; output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"ok":true`) {
		t.Errorf("step() output missing ok:true envelope; got: %q", out.String())
	}
}
