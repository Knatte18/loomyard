// run_test.go covers the generic run body's hook ordering, both busy treatments, and the
// producer-hard-error path, driven against the fake Shed testsupport_test.go builds.

package shedverbs

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// runTexts returns a minimal, non-blank VerbTexts for run alone -- the other three verbs' text
// fields are irrelevant to run_test.go and left at their zero value.
func runTexts() VerbTexts {
	return VerbTexts{Run: VerbText{Use: "run", Short: "run the fake shed"}}
}

// TestRunCmd_SuccessEnvelope covers a successful run's four-key envelope.
func TestRunCmd_SuccessEnvelope(t *testing.T) {
	paths := newTestPaths(t)
	seedStatus(t, paths, "Only")

	spec := &Spec{
		BuildShed: func() (*shedengine.Shed, error) {
			return newFakeShed(paths, []shedengine.ProducerDef{stubRow("Only")}), nil
		},
	}

	env, code := execEnvelope(t, runCmd(runTexts(), spec), nil)
	if code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}
	if env["outcome"] != string(shedengine.RunDone) {
		t.Errorf("outcome = %v; want %q", env["outcome"], shedengine.RunDone)
	}
	if env["halted_producer"] != "Only" {
		t.Errorf("halted_producer = %v; want %q", env["halted_producer"], "Only")
	}
	if env["reason"] != "" {
		t.Errorf("reason = %v; want empty", env["reason"])
	}
	if _, ok := env["history_length"]; !ok {
		t.Errorf("envelope missing history_length key: %v", env)
	}
	for _, key := range []string{"outcome", "halted_producer", "reason", "history_length", "ok"} {
		if _, ok := env[key]; !ok {
			t.Errorf("envelope missing key %q: %v", key, env)
		}
	}
}

// TestRunCmd_BusyTreatment covers both told treatments of an ErrShedBusy run: an empty
// RunBusyMessage reports err.Error() verbatim, and a filled one reports the told text instead.
func TestRunCmd_BusyTreatment(t *testing.T) {
	tests := []struct {
		name           string
		runBusyMessage string
	}{
		{name: "Passthrough", runBusyMessage: ""},
		{name: "ToldMessage", runBusyMessage: "run: another driver already holds the lock"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := newTestPaths(t)
			seedStatus(t, paths, "Only")

			// Hold the run lock so shed.Run's own acquisition fails with ErrShedBusy.
			held, locked, err := lock.TryAcquireWriteLock(paths.LockPath)
			if err != nil {
				t.Fatalf("acquire run lock: %v", err)
			}
			if !locked {
				t.Fatal("run lock was not free at test start")
			}
			defer held.Release()

			spec := &Spec{
				RunBusyMessage: tt.runBusyMessage,
				BuildShed: func() (*shedengine.Shed, error) {
					return newFakeShed(paths, []shedengine.ProducerDef{stubRow("Only")}), nil
				},
			}

			env, code := execEnvelope(t, runCmd(runTexts(), spec), nil)
			if code != 1 {
				t.Fatalf("exit code = %d; want 1", code)
			}
			gotErr, _ := env["error"].(string)
			if tt.runBusyMessage == "" {
				if gotErr == "" {
					t.Errorf("error = %q; want a non-empty passthrough message", gotErr)
				}
			} else if gotErr != tt.runBusyMessage {
				t.Errorf("error = %q; want told message %q", gotErr, tt.runBusyMessage)
			}
		})
	}
}

// TestRunCmd_ProducerHardError covers a producer hard error reaching the error envelope.
func TestRunCmd_ProducerHardError(t *testing.T) {
	paths := newTestPaths(t)
	seedStatus(t, paths, "Bad")

	wantErr := errors.New("boom")
	spec := &Spec{
		BuildShed: func() (*shedengine.Shed, error) {
			return newFakeShed(paths, []shedengine.ProducerDef{erroringRow("Bad", wantErr)}), nil
		},
	}

	env, code := execEnvelope(t, runCmd(runTexts(), spec), nil)
	if code != 1 {
		t.Fatalf("exit code = %d; want 1", code)
	}
	if env["error"] != wantErr.Error() {
		t.Errorf("error = %v; want %q", env["error"], wantErr.Error())
	}
}

// TestRunCmd_PreRunErrorSkipsBuildShed covers a PreRun returning a non-nil error: it reports on
// the error envelope and leaves BuildShed uncalled.
func TestRunCmd_PreRunErrorSkipsBuildShed(t *testing.T) {
	paths := newTestPaths(t)
	seedStatus(t, paths, "Only")

	build, calls := countingBuildShed(func() (*shedengine.Shed, error) {
		return newFakeShed(paths, []shedengine.ProducerDef{stubRow("Only")}), nil
	})

	wantErr := errors.New("preflight refused")
	spec := &Spec{
		BuildShed: build,
		Hooks: Hooks{
			PreRun: func(ctx context.Context) error { return wantErr },
		},
	}

	env, code := execEnvelope(t, runCmd(runTexts(), spec), nil)
	if code != 1 {
		t.Fatalf("exit code = %d; want 1", code)
	}
	if env["error"] != wantErr.Error() {
		t.Errorf("error = %v; want %q", env["error"], wantErr.Error())
	}
	if *calls != 0 {
		t.Errorf("BuildShed call count = %d; want 0", *calls)
	}
}

// TestRunCmd_PreRunAndPostRunNilOrFilled covers each of PreRun and PostRun nil and filled, over
// the success path.
func TestRunCmd_PreRunAndPostRunNilOrFilled(t *testing.T) {
	tests := []struct {
		name        string
		fillPreRun  bool
		fillPostRun bool
	}{
		{name: "BothNil", fillPreRun: false, fillPostRun: false},
		{name: "PreRunOnly", fillPreRun: true, fillPostRun: false},
		{name: "PostRunOnly", fillPreRun: false, fillPostRun: true},
		{name: "BothFilled", fillPreRun: true, fillPostRun: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := newTestPaths(t)
			seedStatus(t, paths, "Only")

			preRunCalled := false
			postRunCalled := false

			hooks := Hooks{}
			if tt.fillPreRun {
				hooks.PreRun = func(ctx context.Context) error {
					preRunCalled = true
					return nil
				}
			}
			if tt.fillPostRun {
				hooks.PostRun = func(ctx context.Context, result shedengine.Result, runErr error) map[string]any {
					postRunCalled = true
					return map[string]any{"extra": "value"}
				}
			}

			spec := &Spec{
				Hooks: hooks,
				BuildShed: func() (*shedengine.Shed, error) {
					return newFakeShed(paths, []shedengine.ProducerDef{stubRow("Only")}), nil
				},
			}

			env, code := execEnvelope(t, runCmd(runTexts(), spec), nil)
			if code != 0 {
				t.Fatalf("exit code = %d; want 0", code)
			}
			if tt.fillPreRun != preRunCalled {
				t.Errorf("preRunCalled = %v; want %v", preRunCalled, tt.fillPreRun)
			}
			if tt.fillPostRun != postRunCalled {
				t.Errorf("postRunCalled = %v; want %v", postRunCalled, tt.fillPostRun)
			}
			_, hasExtra := env["extra"]
			if tt.fillPostRun && !hasExtra {
				t.Errorf("envelope missing PostRun's extra key: %v", env)
			}
			if !tt.fillPostRun && hasExtra {
				t.Errorf("envelope carries an extra key with PostRun nil: %v", env)
			}
		})
	}
}

// TestRunCmd_PostRunRunsOnErrorPathBeforeEnvelope is the property most easily lost in the
// extraction: PostRun is called with the non-nil runErr, called BEFORE the error envelope is
// written, and its returned map does not appear on that envelope. Ordering is asserted by having
// PostRun write into the same buffer the envelope is written to and checking the buffer's
// ordering, not by a call counter alone -- a counter cannot distinguish "called before" from
// "called after".
func TestRunCmd_PostRunRunsOnErrorPathBeforeEnvelope(t *testing.T) {
	paths := newTestPaths(t)
	seedStatus(t, paths, "Bad")

	wantErr := errors.New("boom")
	var postRunErr error
	var postRunCalled bool

	var buf bytes.Buffer
	spec := &Spec{
		BuildShed: func() (*shedengine.Shed, error) {
			return newFakeShed(paths, []shedengine.ProducerDef{erroringRow("Bad", wantErr)}), nil
		},
		Hooks: Hooks{
			PostRun: func(ctx context.Context, result shedengine.Result, runErr error) map[string]any {
				postRunCalled = true
				postRunErr = runErr
				buf.WriteString("POSTRUN-MARKER\n")
				return map[string]any{"should_not_appear": true}
			},
		},
	}

	code := clihelp.Execute(runCmd(runTexts(), spec), &buf, nil)
	if code != 1 {
		t.Fatalf("exit code = %d; want 1", code)
	}
	if !postRunCalled {
		t.Fatal("PostRun was not called on the error path")
	}
	if postRunErr == nil || postRunErr.Error() != wantErr.Error() {
		t.Errorf("PostRun's runErr = %v; want %v", postRunErr, wantErr)
	}

	output := buf.String()
	markerIdx := strings.Index(output, "POSTRUN-MARKER")
	envelopeIdx := strings.Index(output, `"error"`)
	if markerIdx < 0 || envelopeIdx < 0 || markerIdx > envelopeIdx {
		t.Fatalf("PostRun's marker did not precede the error envelope in output: %q", output)
	}
	if strings.Contains(output, "should_not_appear") {
		t.Errorf("PostRun's returned map leaked onto the error envelope: %q", output)
	}
}
