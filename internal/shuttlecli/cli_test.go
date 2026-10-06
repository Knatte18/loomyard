// cli_test.go covers the shuttlecli cobra seam through RunCLI: run's flag-shape validation, and interrupt/send's exact-args validation.
// No live tmux/claude session is required by any test in this file;
// the full run/interrupt/send round-trip against a live agent lives in smoke tests (batch 6) and
// the sandbox suite.

package shuttlecli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
)

// TestRunCLI_Run_FlagValidation drives runCmd directly rather than through RunCLI, so no parent
// PersistentPreRunE runs and no abort is in play — this is the flag-shape check on its own terms,
// which is what these cases are actually about. Its companion below covers what happens when a
// pre-run abort and a flag error coincide.
// Each case asserts the whole buffer parses as EXACTLY ONE JSON object: the module's contract is one
// envelope per invocation, and a substring check cannot see a second one.
func TestRunCLI_Run_FlagValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "MissingOutputFile",
			args:    []string{"--prompt", "do the thing"},
			wantErr: "--output-file",
		},
		{
			name:    "BothPromptAndPromptFile",
			args:    []string{"--prompt", "do the thing", "--prompt-file", "task.md", "--output-file", "out.md"},
			wantErr: "mutually exclusive",
		},
		{
			name:    "NeitherPromptNorPromptFile",
			args:    []string{"--output-file", "out.md"},
			wantErr: "exactly one of --prompt or --prompt-file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// A nil runner is deliberate: every case here must be refused by the flag-shape checks
			// before c.runner is ever dereferenced, and a nil runner is what proves it.
			c := &shuttleCLI{}
			cmd := c.runCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("cmd.Execute() error: %v; output: %s", err, out.String())
			}

			envelope := parseSingleEnvelope(t, out.Bytes())
			if ok, _ := envelope["ok"].(bool); ok {
				t.Errorf("envelope ok = true; want false for a flag error; output: %s", out.String())
			}
			if got, _ := envelope["error"].(string); !strings.Contains(got, tt.wantErr) {
				t.Errorf("envelope error = %q; want substring %q", got, tt.wantErr)
			}
		})
	}
}

// TestRunCLI_Run_PreRunAbort_EmitsExactlyOneEnvelope is R2-F1's regression guard.
//
// runCmd's flag-shape validation used to sit AHEAD of its clihelp.ShouldAbort check, on the
// reasoning that a bad flag combination should be reported as its own error rather than swallowed by
// the abort's already-recorded exit code. But clihelp.Abort only records an exit code — cobra still
// runs RunE — so when geometry resolution failed AND a flag was bad, one invocation emitted TWO JSON
// objects. That breaks any caller unmarshalling the output as a single object, this package's own
// smoke tests included (they do json.Unmarshal over the whole buffer), and it reported the secondary
// problem after the primary one with nothing saying which to fix first.
//
// Running from a non-git temp dir makes lyxcwd.Resolve fail, which is the abort this pins. The
// original intent is untouched and still covered: TestRunCLI_Run_FlagValidation above proves a flag
// error is reported on its own whenever the pre-run did NOT abort.
func TestRunCLI_Run_PreRunAbort_EmitsExactlyOneEnvelope(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"AbortPlusMissingPrompt", []string{"run", "--output-file", "out.md"}},
		{"AbortPlusMissingOutputFile", []string{"run", "--prompt", "do the thing"}},
		{"AbortPlusMutuallyExclusivePrompts", []string{"run", "--prompt", "a", "--prompt-file", "b", "--output-file", "out.md"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			var out bytes.Buffer
			exitCode := RunCLI(&out, tt.args)

			if exitCode != 1 {
				t.Errorf("RunCLI(%v) = %d; want 1", tt.args, exitCode)
			}
			envelope := parseSingleEnvelope(t, out.Bytes())
			if ok, _ := envelope["ok"].(bool); ok {
				t.Errorf("envelope ok = true; want false; output: %s", out.String())
			}
			// The pre-run failure is the one the operator must fix first, so it is the one reported.
			if got, _ := envelope["error"].(string); !strings.Contains(got, "not a git repository") {
				t.Errorf("envelope error = %q; want the pre-run failure (%q), not the secondary flag error", got, "not a git repository")
			}
		})
	}
}

// parseSingleEnvelope decodes out as exactly one JSON object and fails the test if it holds none,
// more than one, or trailing junk.
// json.Unmarshal alone would not do: it rejects a second object with an opaque "invalid character
// '{' after top-level value", which reads as malformed JSON rather than as the extra envelope it
// actually is.
func parseSingleEnvelope(t *testing.T, out []byte) map[string]any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(out))
	var envelope map[string]any
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatalf("parse first envelope: %v; output: %s", err, out)
	}
	var extra map[string]any
	if err := decoder.Decode(&extra); err == nil {
		t.Fatalf("a second JSON envelope followed the first (%v) — one invocation must emit exactly one; output: %s", extra, out)
	} else if !errors.Is(err, io.EOF) {
		t.Fatalf("trailing junk after the envelope: %v; output: %s", err, out)
	}
	return envelope
}

// TestRunCLI_PositionalArgValidation verifies that "lyx shuttle interrupt" and "lyx shuttle send" enforce their exact positional arguments (<guid>, and <guid> <text>) via cobra's Args validation, which runs before PersistentPreRunE — so this fires even against a non-git directory with no config to resolve.
// Each subtest changes the process working directory, so none runs in parallel.
func TestRunCLI_PositionalArgValidation(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"InterruptNoArgs", []string{"interrupt"}},
		{"InterruptTooManyArgs", []string{"interrupt", "guid-1", "guid-2"}},
		{"SendNoArgs", []string{"send"}},
		{"SendOnlyGuid", []string{"send", "guid-1"}},
		{"SendTooManyArgs", []string{"send", "guid-1", "text", "extra"}},
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

// errSpecCaptured is the sentinel the spec-capturing engine's Prepare returns,
// so the test can tell "Prepare ran and recorded the spec" apart from any other failure mode.
var errSpecCaptured = errors.New("specCapturingEngine: spec captured")

// startupPending answers every capture as a pane that never becomes ready.
func startupPending(string) shuttleengine.StartupState { return shuttleengine.StartupPending }

// TestRunCmd_EffortFlag proves --effort lands in the shuttleengine.Spec run builds, mirroring how --model is wired: a real *shuttleengine.Runner over a spec-capturing Engine fake and an inert reed fake lets the test drive runCmd()'s RunE directly and inspect the Spec the engine's Prepare was actually called with, without a live tmux/claude session.
// Prepare fails before Runner.Start reaches reed.AddStrand, so the reed fake is never exercised.
// That failure happens before any strand exists, so the error envelope must carry none of the run-identity fields (guid, sessionId, runDir) rather than three empty strings.
func TestRunCmd_EffortFlag(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			engine := &shuttlefake.Engine{PrepareErr: errSpecCaptured, StartupFn: startupPending}
			// Distinct, but in the geometric relation NewRunner validates: the
			// anchor is always the worktree root or a subdirectory of it.
			worktreeRoot := t.TempDir()
			anchorPath := filepath.Join(worktreeRoot, "sub", "dir")
			if err := os.MkdirAll(anchorPath, 0o755); err != nil {
				t.Fatalf("mkdir anchor path: %v", err)
			}
			runner := shuttleengine.NewRunner(&shuttlefake.Reed{}, engine, anchorPath, worktreeRoot, shuttleengine.Config{RunTimeoutMin: 30})

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

			if engine.LastSpec.Prompt == "" {
				t.Fatalf("Engine.Prepare was never called; want it invoked with the built Spec; output: %s", out.String())
			}
			if engine.LastSpec.Effort != tt.wantEffort {
				t.Errorf("Spec.Effort = %q; want %q", engine.LastSpec.Effort, tt.wantEffort)
			}

			envelope := parseSingleEnvelope(t, out.Bytes())
			for _, key := range []string{"guid", "sessionId", "runDir"} {
				if _, present := envelope[key]; present {
					t.Errorf("envelope carries %q before any strand existed; output: %s", key, out.String())
				}
			}
		})
	}
}

// TestRunCmd_MechanismFailure_EnvelopeCarriesRunIdentity pins that a run which fails after its
// strand registered still names the strand, session, and run directory in its error envelope.
// Reproduced live before this: tearing the reed session down under an in-flight run answered with
// the bare error and no handle at all, while the run directory was still on disk and the strand
// possibly still live — nothing left for the operator to attach to or tear down.
func TestRunCmd_MechanismFailure_EnvelopeCarriesRunIdentity(t *testing.T) {
	t.Parallel()
	anchorPath := t.TempDir()
	worktreeRoot := filepath.Dir(anchorPath)
	// The engine Prepares successfully and never becomes ready,
	// so the Runner surfaces the mechanism failure from inside its startup step.
	// The reed registers a strand, then fails every Status the way a torn-down reed session does.
	engine := &shuttlefake.Engine{
		PrepareLaunch: &shuttleengine.Launch{Cmd: "launch", ResumeCmd: "resume", SessionID: "session-1"},
		StartupFn:     startupPending,
	}
	reed := &shuttlefake.Reed{
		AddStrandFn: func(reedengine.AddSpec) (reedengine.Strand, error) {
			return reedengine.Strand{GUID: "strand-1"}, nil
		},
		StatusErr: errors.New(`no reed session; run "lyx reed up"`),
	}
	runner := shuttleengine.NewRunner(reed, engine, anchorPath, worktreeRoot, shuttleengine.Config{RunTimeoutMin: 30, PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 1})

	c := &shuttleCLI{runner: runner}
	cmd := c.runCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--prompt", "do the thing", "--output-file", filepath.Join(anchorPath, "never-written.md")})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error: %v; output: %s", err, out.String())
	}

	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("parse envelope: %v; output: %s", err, out.String())
	}
	if ok, _ := envelope["ok"].(bool); ok {
		t.Fatalf("envelope ok = true; want false for a mechanism failure; output: %s", out.String())
	}
	if got, _ := envelope["guid"].(string); got != "strand-1" {
		t.Errorf("envelope guid = %q; want %q; output: %s", got, "strand-1", out.String())
	}
	if got, _ := envelope["sessionId"].(string); got != "session-1" {
		t.Errorf("envelope sessionId = %q; want %q; output: %s", got, "session-1", out.String())
	}
	if got, _ := envelope["runDir"].(string); got == "" {
		t.Errorf("envelope runDir is empty; want the run dir that is still on disk; output: %s", out.String())
	}
}
