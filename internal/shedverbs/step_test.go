// step_test.go covers the generic step body's ten-key envelope closure, the five-kind closed
// vocabulary, PreStep's kind threading, and PostStep's success-only, before-the-envelope ordering.

package shedverbs

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

func stepTexts() VerbTexts {
	return VerbTexts{Step: VerbText{Use: "step", Short: "step the fake shed"}}
}

// TestStepEnvelope_KeySetIsExactlyTen mirrors internal/loomcli/step_test.go's own closure test:
// the envelope's key set is exactly the ten documented keys and no larger.
func TestStepEnvelope_KeySetIsExactlyTen(t *testing.T) {
	res := shedengine.StepResult{
		Producer: "P",
		Outcome:  shedengine.Done,
		Output:   "out",
		Next:     "Q",
		State:    shedengine.StateRunning,
		Reason:   "",
		History:  []shedengine.HistoryEntry{{Producer: "P", Outcome: shedengine.Done}},
	}
	env := StepEnvelope(res, "policy", "/status.json")

	wantKeys := map[string]bool{
		"producer": true, "outcome": true, "output": true, "next": true, "state": true,
		"reason": true, "continue": true, "history_length": true, "next_interrupt_policy": true,
		"status_file": true,
	}
	if len(env) != len(wantKeys) {
		t.Fatalf("StepEnvelope key count = %d; want %d (%v)", len(env), len(wantKeys), env)
	}
	for k := range env {
		if !wantKeys[k] {
			t.Errorf("StepEnvelope has unexpected key %q", k)
		}
	}
}

// TestStepEnvelope_ContinueDerivedFromState asserts continue is true only for StateRunning.
func TestStepEnvelope_ContinueDerivedFromState(t *testing.T) {
	tests := []struct {
		state shedengine.State
		want  bool
	}{
		{shedengine.StateRunning, true},
		{shedengine.StatePaused, false},
		{shedengine.StateDone, false},
		{shedengine.StateBlocked, false},
		{shedengine.StateFailed, false},
	}
	for _, tt := range tests {
		env := StepEnvelope(shedengine.StepResult{State: tt.state}, "", "")
		if env["continue"] != tt.want {
			t.Errorf("state %q: continue = %v; want %v", tt.state, env["continue"], tt.want)
		}
	}
}

// TestStepKinds_IsExactlyFive asserts the closed refusal-kind vocabulary is exactly the five
// declared constants and no larger.
func TestStepKinds_IsExactlyFive(t *testing.T) {
	want := []string{KindBusy, KindUnseeded, KindOwnership, KindBootstrap, KindProducer}
	if len(StepKinds) != len(want) {
		t.Fatalf("StepKinds = %v; want exactly %v", StepKinds, want)
	}
	seen := map[string]bool{}
	for _, k := range StepKinds {
		seen[k] = true
	}
	for _, w := range want {
		if !seen[w] {
			t.Errorf("StepKinds missing %q", w)
		}
	}
}

// TestStepCmd_PreStepErrorReachesKindAndSkipsBuildShed covers a PreStep returning (kind, err):
// the envelope's kind field carries that exact kind, and BuildShed is never called.
func TestStepCmd_PreStepErrorReachesKindAndSkipsBuildShed(t *testing.T) {
	paths := newTestPaths(t)
	seedStatus(t, paths, "Only")

	build, calls := countingBuildShed(func() (*shedengine.Shed, error) {
		return newFakeShed(paths, []shedengine.ProducerDef{stubRow("Only")}), nil
	})

	wantErr := errors.New("not seeded")
	spec := &Spec{
		BuildShed: build,
		Hooks: Hooks{
			PreStep: func(ctx context.Context) (string, error) { return KindUnseeded, wantErr },
		},
	}

	env, code := execEnvelope(t, stepCmd(stepTexts(), spec), nil)
	if code != 1 {
		t.Fatalf("exit code = %d; want 1", code)
	}
	if env["kind"] != KindUnseeded {
		t.Errorf("kind = %v; want %q", env["kind"], KindUnseeded)
	}
	if env["error"] != wantErr.Error() {
		t.Errorf("error = %v; want %q", env["error"], wantErr.Error())
	}
	if *calls != 0 {
		t.Errorf("BuildShed call count = %d; want 0", *calls)
	}
}

// TestStepCmd_BusyTreatment covers step's own ErrShedBusy mapping onto spec.StepBusyKind, with
// both the passthrough and told-message forms.
func TestStepCmd_BusyTreatment(t *testing.T) {
	paths := newTestPaths(t)
	seedStatus(t, paths, "Only")

	spec := &Spec{
		StepBusyKind:    KindBusy,
		StepBusyMessage: "step: another driver already holds the run lock",
		BuildShed: func() (*shedengine.Shed, error) {
			return &shedengine.Shed{
				Producers:      []shedengine.ProducerDef{stubRow("Only")},
				StatusPath:     paths.StatusPath,
				LockPath:       paths.LockPath,
				StatusLockPath: paths.StatusLockPath,
			}, nil
		},
	}

	held, locked, err := lock.TryAcquireWriteLock(paths.LockPath)
	if err != nil {
		t.Fatalf("acquire run lock: %v", err)
	}
	if !locked {
		t.Fatal("run lock was not free at test start")
	}
	defer held.Release()

	env, code := execEnvelope(t, stepCmd(stepTexts(), spec), nil)
	if code != 1 {
		t.Fatalf("exit code = %d; want 1", code)
	}
	if env["kind"] != KindBusy {
		t.Errorf("kind = %v; want %q", env["kind"], KindBusy)
	}
	if env["error"] != spec.StepBusyMessage {
		t.Errorf("error = %v; want %q", env["error"], spec.StepBusyMessage)
	}
}

// TestStepCmd_ProducerHardErrorReachesKindProducer covers a Step producer hard error mapping onto
// KindProducer.
func TestStepCmd_ProducerHardErrorReachesKindProducer(t *testing.T) {
	paths := newTestPaths(t)
	seedStatus(t, paths, "Bad")

	wantErr := errors.New("boom")
	spec := &Spec{
		BuildShed: func() (*shedengine.Shed, error) {
			return newFakeShed(paths, []shedengine.ProducerDef{erroringRow("Bad", wantErr)}), nil
		},
	}

	env, code := execEnvelope(t, stepCmd(stepTexts(), spec), nil)
	if code != 1 {
		t.Fatalf("exit code = %d; want 1", code)
	}
	if env["kind"] != KindProducer {
		t.Errorf("kind = %v; want %q", env["kind"], KindProducer)
	}
	if env["error"] != wantErr.Error() {
		t.Errorf("error = %v; want %q", env["error"], wantErr.Error())
	}
}

// TestStepCmd_InterruptPolicyThreading asserts next_interrupt_policy is empty when
// InterruptPolicyFor is nil, and threaded through when the hook is filled.
func TestStepCmd_InterruptPolicyThreading(t *testing.T) {
	tests := []struct {
		name       string
		fillPolicy bool
		want       string
	}{
		{name: "NilHook", fillPolicy: false, want: ""},
		{name: "FilledHook", fillPolicy: true, want: "auto-retry"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := newTestPaths(t)
			seedStatus(t, paths, "Only")

			spec := &Spec{
				BuildShed: func() (*shedengine.Shed, error) {
					return newFakeShed(paths, []shedengine.ProducerDef{stubRow("Only")}), nil
				},
			}
			if tt.fillPolicy {
				spec.Hooks.InterruptPolicyFor = func(row string) string { return tt.want }
			}

			env, code := execEnvelope(t, stepCmd(stepTexts(), spec), nil)
			if code != 0 {
				t.Fatalf("exit code = %d; want 0", code)
			}
			if env["next_interrupt_policy"] != tt.want {
				t.Errorf("next_interrupt_policy = %v; want %q", env["next_interrupt_policy"], tt.want)
			}
		})
	}
}

// TestStepCmd_PostStepOrderingAndScope covers PostStep's own contract: called with the returned
// StepResult on the success path before the envelope is written, and never called on either the
// PreStep-error or the Step-error path.
func TestStepCmd_PostStepOrderingAndScope(t *testing.T) {
	t.Run("SuccessPath_CalledBeforeEnvelope", func(t *testing.T) {
		paths := newTestPaths(t)
		seedStatus(t, paths, "Only")

		var postStepRes shedengine.StepResult
		var postStepCalled bool
		var out strings.Builder

		spec := &Spec{
			BuildShed: func() (*shedengine.Shed, error) {
				return newFakeShed(paths, []shedengine.ProducerDef{stubRow("Only")}), nil
			},
			Hooks: Hooks{
				// Writes into the SAME writer the envelope is written to, so ordering is provable
				// from the buffer's own contents rather than from a call counter alone.
				PostStep: func(res shedengine.StepResult) {
					postStepCalled = true
					postStepRes = res
					out.WriteString("POSTSTEP-MARKER\n")
				},
			},
		}

		code := clihelp.Execute(stepCmd(stepTexts(), spec), &out, nil)
		if code != 0 {
			t.Fatalf("exit code = %d; want 0", code)
		}
		if !postStepCalled {
			t.Fatal("PostStep was not called on the success path")
		}
		if postStepRes.Producer != "Only" {
			t.Errorf("PostStep's res.Producer = %q; want %q", postStepRes.Producer, "Only")
		}

		markerIdx := strings.Index(out.String(), "POSTSTEP-MARKER")
		envelopeIdx := strings.Index(out.String(), `"ok"`)
		if markerIdx < 0 || envelopeIdx < 0 || markerIdx > envelopeIdx {
			t.Fatalf("PostStep's marker did not precede the envelope in output: %q", out.String())
		}
	})

	t.Run("PreStepError_NeverCalled", func(t *testing.T) {
		paths := newTestPaths(t)
		seedStatus(t, paths, "Only")

		called := false
		spec := &Spec{
			BuildShed: func() (*shedengine.Shed, error) {
				return newFakeShed(paths, []shedengine.ProducerDef{stubRow("Only")}), nil
			},
			Hooks: Hooks{
				PreStep:  func(ctx context.Context) (string, error) { return KindBootstrap, errors.New("nope") },
				PostStep: func(res shedengine.StepResult) { called = true },
			},
		}

		_, code := execEnvelope(t, stepCmd(stepTexts(), spec), nil)
		if code != 1 {
			t.Fatalf("exit code = %d; want 1", code)
		}
		if called {
			t.Error("PostStep was called on the PreStep-error path")
		}
	})

	t.Run("StepError_NeverCalled", func(t *testing.T) {
		paths := newTestPaths(t)
		seedStatus(t, paths, "Bad")

		called := false
		spec := &Spec{
			BuildShed: func() (*shedengine.Shed, error) {
				return newFakeShed(paths, []shedengine.ProducerDef{erroringRow("Bad", errors.New("boom"))}), nil
			},
			Hooks: Hooks{
				PostStep: func(res shedengine.StepResult) { called = true },
			},
		}

		_, code := execEnvelope(t, stepCmd(stepTexts(), spec), nil)
		if code != 1 {
			t.Fatalf("exit code = %d; want 1", code)
		}
		if called {
			t.Error("PostStep was called on the Step-error path")
		}
	})
}
