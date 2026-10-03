// step_test.go covers the generic step body's sixteen-key envelope closure, the five-kind closed
// vocabulary, PreStep's kind threading, and PostStep's success-only, before-the-envelope ordering.

package shedverbs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

func stepTexts() VerbTexts {
	return VerbTexts{Step: VerbText{Use: "step", Short: "step the fake shed"}}
}

// TestStepEnvelope_KeySetIsExactlySixteen asserts the envelope's key set is exactly the sixteen
// documented keys and no larger.
func TestStepEnvelope_KeySetIsExactlySixteen(t *testing.T) {
	res := shedengine.StepResult{
		Producer: "P",
		Outcome:  shedengine.Done,
		Output:   "out",
		Next:     "Q",
		State:    shedengine.StateRunning,
		Reason:   "",
		History:  []shedengine.HistoryEntry{{Producer: "P", Outcome: shedengine.Done}},
	}
	env := StepEnvelope(res, "policy", "/status.json", StepLocations{}, nil)

	wantKeys := map[string]bool{
		"producer": true, "outcome": true, "output": true, "next": true, "state": true,
		"reason": true, "continue": true, "history_length": true, "next_interrupt_policy": true,
		"status_file": true, "trace_file": true, "friction_dir": true, "scratch_dir": true,
		"trace_id": true, "run_id": true, "progress": true,
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
		env := StepEnvelope(shedengine.StepResult{State: tt.state}, "", "", StepLocations{}, nil)
		if env["continue"] != tt.want {
			t.Errorf("state %q: continue = %v; want %v", tt.state, env["continue"], tt.want)
		}
	}
}

// TestStepEnvelope_FieldMapping tables the routing shapes, asserting each field's mapping onto the
// envelope.
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
				Reason:   shedengine.ReasonBounceBudgetExhausted,
				History:  []shedengine.HistoryEntry{{Producer: "Discussion-Bouncer", Outcome: shedengine.Stuck}},
			},
			wantProducer: "Discussion-Bouncer",
			wantOutcome:  string(shedengine.Stuck),
			wantNext:     "Discussion-Bouncer",
			wantState:    string(shedengine.StateBlocked),
			wantContinue: false,
			wantReason:   shedengine.ReasonBounceBudgetExhausted,
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
			env := StepEnvelope(tt.res, "", "/some/status.json", StepLocations{}, nil)

			if got := env["producer"]; got != tt.wantProducer {
				t.Errorf("envelope[\"producer\"] = %v; want %v", got, tt.wantProducer)
			}
			if got := env["outcome"]; got != tt.wantOutcome {
				t.Errorf("envelope[\"outcome\"] = %v; want %v", got, tt.wantOutcome)
			}
			if got := env["next"]; got != tt.wantNext {
				t.Errorf("envelope[\"next\"] = %v; want %v", got, tt.wantNext)
			}
			if got := env["state"]; got != tt.wantState {
				t.Errorf("envelope[\"state\"] = %v; want %v", got, tt.wantState)
			}
			if got := env["continue"]; got != tt.wantContinue {
				t.Errorf("envelope[\"continue\"] = %v; want %v", got, tt.wantContinue)
			}
			if got := env["reason"]; got != tt.wantReason {
				t.Errorf("envelope[\"reason\"] = %v; want %v", got, tt.wantReason)
			}
			if got := env["history_length"]; got != len(tt.res.History) {
				t.Errorf("envelope[\"history_length\"] = %v; want %v", got, len(tt.res.History))
			}
			if got := env["status_file"]; got != "/some/status.json" {
				t.Errorf("envelope[\"status_file\"] = %v; want %v", got, "/some/status.json")
			}
		})
	}
}

// TestStepKinds_IsExactlyFive asserts the closed refusal-kind vocabulary is exactly the five
// declared constants and no larger, each non-empty and distinct.
func TestStepKinds_IsExactlyFive(t *testing.T) {
	want := []string{KindBusy, KindUnseeded, KindOwnership, KindBootstrap, KindProducer}
	if len(StepKinds) != len(want) {
		t.Fatalf("StepKinds = %v; want exactly %v", StepKinds, want)
	}
	seen := map[string]bool{}
	for _, k := range StepKinds {
		if k == "" {
			t.Error("StepKinds contains an empty entry")
		}
		if seen[k] {
			t.Errorf("StepKinds contains a duplicate entry %q", k)
		}
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

// TestStepCmd_ErrorEnvelopesCarryLocations covers every error path: each envelope carries the three
// location keys, scratch_dir and friction_dir echo the Spec, and kind is unchanged.
func TestStepCmd_ErrorEnvelopesCarryLocations(t *testing.T) {
	paths := newTestPaths(t)
	seedStatus(t, paths, "Bad")

	tests := []struct {
		name     string
		spec     func() *Spec
		wantKind string
	}{
		{"PreStepError", func() *Spec {
			return &Spec{Hooks: Hooks{PreStep: func(ctx context.Context) (string, error) { return KindUnseeded, errors.New("x") }}}
		}, KindUnseeded},
		{"NilBuildShed", func() *Spec { return &Spec{} }, KindBootstrap},
		{"BuildShedError", func() *Spec {
			return &Spec{BuildShed: func() (*shedengine.Shed, error) { return nil, errors.New("build") }}
		}, KindBootstrap},
		{"Busy", func() *Spec {
			return &Spec{StepBusyKind: KindBusy, BuildShed: func() (*shedengine.Shed, error) {
				return newFakeShed(paths, []shedengine.ProducerDef{stubRow("Bad")}), nil
			}}
		}, KindBusy},
		{"ProducerError", func() *Spec {
			return &Spec{BuildShed: func() (*shedengine.Shed, error) {
				return newFakeShed(paths, []shedengine.ProducerDef{erroringRow("Bad", errors.New("boom"))}), nil
			}}
		}, KindProducer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "Busy" {
				held, locked, err := lock.TryAcquireWriteLock(paths.LockPath)
				if err != nil || !locked {
					t.Fatalf("acquire run lock: locked=%v err=%v", locked, err)
				}
				defer held.Release()
			}
			spec := tt.spec()
			spec.ScratchDir = "/scratch"
			spec.FrictionDir = "/friction"
			spec.RunID = "my-slug"

			env, code := execEnvelope(t, stepCmd(stepTexts(), spec), nil)
			if code != 1 {
				t.Fatalf("exit code = %d; want 1", code)
			}
			if env["kind"] != tt.wantKind {
				t.Errorf("kind = %v; want %q", env["kind"], tt.wantKind)
			}
			if env["scratch_dir"] != "/scratch" {
				t.Errorf("scratch_dir = %v; want /scratch", env["scratch_dir"])
			}
			if env["friction_dir"] != "/friction" {
				t.Errorf("friction_dir = %v; want /friction", env["friction_dir"])
			}
			if _, ok := env["trace_file"]; !ok {
				t.Errorf("envelope missing trace_file: %v", env)
			}
			if env["run_id"] != "my-slug" {
				t.Errorf("run_id = %v; want my-slug", env["run_id"])
			}
			if env["trace_id"] != logger.TraceID() {
				t.Errorf("trace_id = %v; want %q", env["trace_id"], logger.TraceID())
			}
			if v, ok := env["transient"]; !ok || v != "" {
				t.Errorf("transient = %v (present=%v); want present and empty", v, ok)
			}
			wantKeys := map[string]bool{
				"ok": true, "error": true, "kind": true, "transient": true, "trace_file": true,
				"friction_dir": true, "scratch_dir": true, "trace_id": true, "run_id": true,
			}
			if len(env) != len(wantKeys) {
				t.Errorf("error envelope key count = %d; want %d (%v)", len(env), len(wantKeys), env)
			}
			for k := range env {
				if !wantKeys[k] {
					t.Errorf("error envelope has unexpected key %q", k)
				}
			}
		})
	}
}

// TestStepCmd_ErrorEnvelopesCarryTransientClass covers a PreStep, a BuildShed and a producer failure each carrying its transient class on the envelope while the kind is unchanged.
// It also covers the busy refusal reporting an empty class even when the classifier would mark its error.
func TestStepCmd_ErrorEnvelopesCarryTransientClass(t *testing.T) {
	paths := newTestPaths(t)
	seedStatus(t, paths, "Bad")

	tests := []struct {
		name          string
		spec          func() *Spec
		wantKind      string
		wantTransient string
	}{
		{"PreStep", func() *Spec {
			return &Spec{Hooks: Hooks{PreStep: func(ctx context.Context) (string, error) {
				return KindUnseeded, shedengine.MarkTransient(shedengine.TransientGitTransport, errors.New("push"))
			}}}
		}, KindUnseeded, string(shedengine.TransientGitTransport)},
		{"BuildShed", func() *Spec {
			return &Spec{BuildShed: func() (*shedengine.Shed, error) {
				return nil, shedengine.MarkTransient(shedengine.TransientAgentStart, errors.New("start"))
			}}
		}, KindBootstrap, string(shedengine.TransientAgentStart)},
		{"Producer", func() *Spec {
			return &Spec{BuildShed: func() (*shedengine.Shed, error) {
				shed := newFakeShed(paths, []shedengine.ProducerDef{erroringRow("Bad", errors.New("api"))})
				shed.Transient = func(error) shedengine.TransientClass { return shedengine.TransientGitHubAPI }
				return shed, nil
			}}
		}, KindProducer, string(shedengine.TransientGitHubAPI)},
		{"Busy", func() *Spec {
			return &Spec{StepBusyKind: KindBusy, BuildShed: func() (*shedengine.Shed, error) {
				shed := newFakeShed(paths, []shedengine.ProducerDef{stubRow("Bad")})
				shed.Transient = func(error) shedengine.TransientClass { return shedengine.TransientGitHubAPI }
				return shed, nil
			}}
		}, KindBusy, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "Busy" {
				held, locked, err := lock.TryAcquireWriteLock(paths.LockPath)
				if err != nil || !locked {
					t.Fatalf("acquire run lock: locked=%v err=%v", locked, err)
				}
				defer held.Release()
			}
			env, code := execEnvelope(t, stepCmd(stepTexts(), tt.spec()), nil)
			if code != 1 {
				t.Fatalf("exit code = %d; want 1", code)
			}
			if env["kind"] != tt.wantKind {
				t.Errorf("kind = %v; want %q", env["kind"], tt.wantKind)
			}
			if env["transient"] != tt.wantTransient {
				t.Errorf("transient = %v; want %q", env["transient"], tt.wantTransient)
			}
		})
	}
}

// TestStepCmd_SuccessEnvelopeEchoesLocations asserts the success envelope echoes the Spec's two
// told locations and reports an empty trace_file with no sink armed.
func TestStepCmd_SuccessEnvelopeEchoesLocations(t *testing.T) {
	paths := newTestPaths(t)
	seedStatus(t, paths, "Only")
	spec := &Spec{
		ScratchDir:  "/scratch",
		FrictionDir: "/friction",
		BuildShed: func() (*shedengine.Shed, error) {
			return newFakeShed(paths, []shedengine.ProducerDef{stubRow("Only")}), nil
		},
	}

	env, code := execEnvelope(t, stepCmd(stepTexts(), spec), nil)
	if code != 0 {
		t.Fatalf("exit code = %d; want 0 (%v)", code, env)
	}
	if env["scratch_dir"] != "/scratch" || env["friction_dir"] != "/friction" {
		t.Errorf("scratch_dir/friction_dir = %v/%v; want /scratch and /friction", env["scratch_dir"], env["friction_dir"])
	}
	if env["trace_file"] != "" {
		t.Errorf("trace_file = %v; want empty with no sink armed", env["trace_file"])
	}
}

// TestStepCmd_SuccessEnvelopeCarriesIdentityAndProgress asserts the success envelope carries the
// told run_id, the process trace id, and progress matching ProgressAt(res.Next), and reports a null
// progress for an unarmed Routing.
func TestStepCmd_SuccessEnvelopeCarriesIdentityAndProgress(t *testing.T) {
	routing := shedengine.Routing{
		Entry: "A",
		Producers: []shedengine.ProducerDef{
			{Name: "A", OnDone: "B"},
			{Name: "B"},
		},
	}
	build := func(paths testPaths) func() (*shedengine.Shed, error) {
		return func() (*shedengine.Shed, error) {
			return newFakeShed(paths, []shedengine.ProducerDef{
				{Name: "A", Producer: stubRow("A").Producer, OnDone: "B"},
				stubRow("B"),
			}), nil
		}
	}

	t.Run("Armed", func(t *testing.T) {
		paths := newTestPaths(t)
		seedStatus(t, paths, "A")
		spec := &Spec{RunID: "my-slug", Routing: routing, BuildShed: build(paths)}

		env, code := execEnvelope(t, stepCmd(stepTexts(), spec), nil)
		if code != 0 {
			t.Fatalf("exit code = %d; want 0 (%v)", code, env)
		}
		if env["run_id"] != "my-slug" {
			t.Errorf("run_id = %v; want my-slug", env["run_id"])
		}
		if env["trace_id"] != logger.TraceID() {
			t.Errorf("trace_id = %v; want %q", env["trace_id"], logger.TraceID())
		}
		want := routing.ProgressAt("B")
		got, ok := env["progress"].(map[string]any)
		if !ok {
			t.Fatalf("progress = %v; want an object", env["progress"])
		}
		if got["step"] != float64(want.Step) || got["steps"] != float64(want.Steps) || got["name"] != want.Name {
			t.Errorf("progress = %v; want %+v", got, want)
		}
	})

	t.Run("Unarmed", func(t *testing.T) {
		paths := newTestPaths(t)
		seedStatus(t, paths, "A")
		spec := &Spec{BuildShed: build(paths)}

		env, code := execEnvelope(t, stepCmd(stepTexts(), spec), nil)
		if code != 0 {
			t.Fatalf("exit code = %d; want 0 (%v)", code, env)
		}
		if v, ok := env["progress"]; !ok || v != nil {
			t.Errorf("progress = %v (present=%v); want null", v, ok)
		}
	})
}

// TestStepCmd_TraceFileHoldsBoundaryRecords arms the durable sink and asserts trace_file names a
// file holding the step boundary records. It never runs in parallel: the sink override is
// package-level state.
func TestStepCmd_TraceFileHoldsBoundaryRecords(t *testing.T) {
	logger.SetDurableSinkDir(t.TempDir())
	t.Cleanup(func() { logger.SetDurableSinkDir("") })

	paths := newTestPaths(t)
	seedStatus(t, paths, "Only")
	spec := &Spec{BuildShed: func() (*shedengine.Shed, error) {
		return newFakeShed(paths, []shedengine.ProducerDef{stubRow("Only")}), nil
	}}

	env, _ := execEnvelope(t, stepCmd(stepTexts(), spec), nil)
	traceFile, _ := env["trace_file"].(string)
	body := readTrace(t, traceFile)
	for _, want := range []string{`msg="shed: step"`, `msg="shed: step done"`, "producer="} {
		if !strings.Contains(body, want) {
			t.Errorf("trace file lacks %s:\n%s", want, body)
		}
	}

	refused := &Spec{Hooks: Hooks{PreStep: func(ctx context.Context) (string, error) { return KindUnseeded, errors.New("nope") }}}
	env, _ = execEnvelope(t, stepCmd(stepTexts(), refused), nil)
	traceFile, _ = env["trace_file"].(string)
	body = readTrace(t, traceFile)
	if !strings.Contains(body, `msg="shed: step refused"`) || !strings.Contains(body, "kind="+KindUnseeded) {
		t.Errorf("trace file lacks the refusal record with its kind:\n%s", body)
	}
}

// readTrace reads the trace file at path, failing the test when the path is empty or unreadable.
func readTrace(t *testing.T, path string) string {
	t.Helper()
	if path == "" || !filepath.IsAbs(path) {
		t.Fatalf("trace_file = %q; want an absolute path", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace file: %v", err)
	}
	return string(data)
}
