// cli_test.go covers the shuttlecli cobra seam through RunCLI: bare-group
// listing, the unknown-subcommand JSON envelope, run's flag-shape
// validation, and interrupt/send's exact-args validation. No live
// tmux/claude session is required by any test in this file; the full
// run/interrupt/send round-trip against a live agent lives in smoke tests
// (batch 6) and the sandbox suite.

package shuttlecli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// TestRunCLI_NoArgs verifies that "lyx shuttle" with no subcommand lists the
// run verb and exits 0 — no git repo is needed, since the PersistentPreRunE
// guard skips layout/config/engine resolution for the group command itself.
func TestRunCLI_NoArgs(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	exitCode := RunCLI(&out, nil)

	if exitCode != 0 {
		t.Errorf("RunCLI(nil) = %d; want 0", exitCode)
	}

	got := out.String()
	wantSubs := []string{"run", "interrupt", "send"}
	for _, sub := range wantSubs {
		if !strings.Contains(got, sub) {
			t.Errorf("RunCLI(nil) no-arg listing missing subcommand %q; got:\n%s", sub, got)
		}
	}
}

// TestRunCLI_UnknownSubcommand verifies that an unknown subcommand exits 1
// and emits a JSON error envelope with ok=false, without needing a git repo
// (the PersistentPreRunE guard for cmd.Name() == "shuttle" fires before
// layout resolution).
func TestRunCLI_UnknownSubcommand(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"bogus"})

	if exitCode != 1 {
		t.Errorf("RunCLI(bogus) = %d; want 1", exitCode)
	}

	got := out.String()
	if !strings.Contains(got, `"ok":false`) {
		t.Errorf("RunCLI(bogus) output missing ok:false envelope; got: %q", got)
	}
	if !strings.Contains(got, "unknown") {
		t.Errorf("RunCLI(bogus) output missing \"unknown\"; got: %q", got)
	}
}

// TestRunCLI_Run_FlagValidation exercises run's flag-shape validation
// (missing --output-file, both --prompt and --prompt-file, neither) against
// an uninitialized (non-git) directory. Config resolution aborts first in
// that directory, but run's RunE validates flag shape before ever touching
// c.runner, so each case's flag-specific error still surfaces in the
// captured output alongside the PersistentPreRunE abort's own error line.
func TestRunCLI_Run_FlagValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "MissingOutputFile",
			args:    []string{"run", "--prompt", "do the thing"},
			wantErr: "--output-file",
		},
		{
			name:    "BothPromptAndPromptFile",
			args:    []string{"run", "--prompt", "do the thing", "--prompt-file", "task.md", "--output-file", "out.md"},
			wantErr: "mutually exclusive",
		},
		{
			name:    "NeitherPromptNorPromptFile",
			args:    []string{"run", "--output-file", "out.md"},
			wantErr: "exactly one of --prompt or --prompt-file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			var out bytes.Buffer
			exitCode := RunCLI(&out, tt.args)

			if exitCode != 1 {
				t.Errorf("RunCLI(%v) = %d; want 1", tt.args, exitCode)
			}
			if !strings.Contains(out.String(), tt.wantErr) {
				t.Errorf("RunCLI(%v) output = %q; want substring %q", tt.args, out.String(), tt.wantErr)
			}
		})
	}
}

// TestRunCLI_Interrupt_ArgValidation verifies that "lyx shuttle interrupt"
// enforces exactly one positional <guid> argument via cobra's Args
// validation, which runs before PersistentPreRunE — so this fires even
// against a non-git directory with no config to resolve.
func TestRunCLI_Interrupt_ArgValidation(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"NoArgs", []string{"interrupt"}},
		{"TooManyArgs", []string{"interrupt", "guid-1", "guid-2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			var out bytes.Buffer
			exitCode := RunCLI(&out, tt.args)

			if exitCode != 1 {
				t.Errorf("RunCLI(%v) = %d; want 1", tt.args, exitCode)
			}
			if !strings.Contains(out.String(), `"ok":false`) {
				t.Errorf("RunCLI(%v) output missing ok:false envelope; got: %q", tt.args, out.String())
			}
		})
	}
}

// TestRunCLI_Send_ArgValidation verifies that "lyx shuttle send" enforces
// exactly two positional arguments (<guid> <text>) via cobra's Args
// validation, for the same reason as TestRunCLI_Interrupt_ArgValidation.
// specCapturingEngine is a hermetic shuttleengine.Engine double whose only
// job is to record the Spec it was handed and then fail Prepare — the test
// only needs to inspect the flag-to-Spec mapping run.go's RunE builds, never
// a live pane launch, so failing fast at Prepare (before Runner.Start ever
// touches mux.AddStrand) keeps the test hermetic without needing a working
// fakeMux beyond satisfying the interface.
type specCapturingEngine struct {
	gotSpec shuttleengine.Spec
}

func (e *specCapturingEngine) Prepare(runDir string, spec shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	e.gotSpec = spec
	return shuttleengine.Launch{}, errSpecCaptured
}
func (e *specCapturingEngine) ParseEvents(data []byte) ([]shuttleengine.Event, error) {
	return nil, nil
}
func (e *specCapturingEngine) Startup(capture string) shuttleengine.StartupState {
	return shuttleengine.StartupPending
}
func (e *specCapturingEngine) InterruptSequence() []shuttleengine.PaneInput      { return nil }
func (e *specCapturingEngine) TrustDismissSequence() []shuttleengine.PaneInput   { return nil }
func (e *specCapturingEngine) ComposeSend(text string) []shuttleengine.PaneInput { return nil }

// AuditForks is never reached: Prepare always fails before Runner.Start could
// ever run this spec to a fork-mode done classification.
func (e *specCapturingEngine) AuditForks(sessionID, workdir string) (shuttleengine.ForkAudit, error) {
	return shuttleengine.ForkAudit{}, nil
}

// AuditForksIncremental is never reached, for the same reason as AuditForks.
func (e *specCapturingEngine) AuditForksIncremental(sessionID, workdir string, seenTranscripts map[string]bool) (shuttleengine.ForkAudit, error) {
	return shuttleengine.ForkAudit{}, nil
}

// ModelSwitchSequence is never reached: Prepare always fails before any
// model-switch choreography could ever be driven.
func (e *specCapturingEngine) ModelSwitchSequence(model string) []shuttleengine.PaneInput {
	return nil
}

var _ shuttleengine.Engine = (*specCapturingEngine)(nil)

// errSpecCaptured is the sentinel specCapturingEngine.Prepare always
// returns, so the test can tell "Prepare ran and recorded the spec" apart
// from any other failure mode.
var errSpecCaptured = errors.New("specCapturingEngine: spec captured")

// noopMux is a hermetic shuttleengine.MuxOps double whose methods are never
// actually reached in TestRunCmd_EffortFlag (specCapturingEngine.Prepare
// fails before Runner.Start ever calls AddStrand) — it exists only to
// satisfy the MuxOps interface Runner requires.
type noopMux struct{}

func (noopMux) AddStrand(spec muxengine.AddSpec) (muxengine.Strand, error) {
	return muxengine.Strand{}, nil
}
func (noopMux) RemoveStrand(guid string, recursive bool) (muxengine.Removed, error) {
	return muxengine.Removed{}, nil
}
func (noopMux) Status() (muxengine.StatusResult, error)       { return muxengine.StatusResult{}, nil }
func (noopMux) SendText(guid, text string, submit bool) error { return nil }
func (noopMux) SendKey(guid, key string) error                { return nil }
func (noopMux) CapturePane(guid string) (string, error)       { return "", nil }

var _ shuttleengine.MuxOps = noopMux{}

// TestRunCmd_EffortFlag proves --effort lands in the shuttleengine.Spec run
// builds, mirroring how --model is wired: a fake Runner (a real
// *shuttleengine.Runner over a spec-capturing Engine fake and a no-op mux
// fake) lets the test drive runCmd()'s RunE directly and inspect the Spec
// the engine's Prepare was actually called with, without a live tmux/claude
// session.
func TestRunCmd_EffortFlag(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantEffort string
	}{
		{
			name:       "EffortFlagSet",
			args:       []string{"--prompt", "do the thing", "--output-file", "out.md", "--effort", "high"},
			wantEffort: "high",
		},
		{
			name:       "EffortFlagOmitted",
			args:       []string{"--prompt", "do the thing", "--output-file", "out.md"},
			wantEffort: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := &specCapturingEngine{}
			layout := &hubgeometry.Layout{WorktreeRoot: t.TempDir()}
			runner := shuttleengine.NewRunner(noopMux{}, engine, layout, shuttleengine.Config{RunTimeoutMin: 30})

			c := &shuttleCLI{runner: runner}
			cmd := c.runCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs(tt.args)

			// The command's own RunE always returns nil (errors go through
			// output.Err), so Execute()'s error is only ever a flag-parse
			// failure — not a signal about the spec-capture path below.
			if err := cmd.Execute(); err != nil {
				t.Fatalf("cmd.Execute() error: %v; output: %s", err, out.String())
			}

			if engine.gotSpec.Prompt == "" {
				t.Fatalf("specCapturingEngine.Prepare was never called; want it invoked with the built Spec; output: %s", out.String())
			}
			if engine.gotSpec.Effort != tt.wantEffort {
				t.Errorf("Spec.Effort = %q; want %q", engine.gotSpec.Effort, tt.wantEffort)
			}
		})
	}
}

func TestRunCLI_Send_ArgValidation(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"NoArgs", []string{"send"}},
		{"OnlyGuid", []string{"send", "guid-1"}},
		{"TooManyArgs", []string{"send", "guid-1", "text", "extra"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			var out bytes.Buffer
			exitCode := RunCLI(&out, tt.args)

			if exitCode != 1 {
				t.Errorf("RunCLI(%v) = %d; want 1", tt.args, exitCode)
			}
			if !strings.Contains(out.String(), `"ok":false`) {
				t.Errorf("RunCLI(%v) output missing ok:false envelope; got: %q", tt.args, out.String())
			}
		})
	}
}
