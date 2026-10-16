// cli_test.go covers the shuttlecli cobra seam through RunCLI: run's flag-shape validation, and interrupt/send's exact-args validation.
// It also pins, over a fake reed and engine, the message tail `send` appends.
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
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/spf13/cobra"
)

// fakeClock is a shuttleengine.Clock whose Sleep advances it, so a runner's poll sleeps cost no wall-clock time.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Sleep(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// newFakeClockRunner is shuttleengine.NewRunner with a fake clock installed, at the floor poll interval.
func newFakeClockRunner(reed shuttleengine.ReedOps, engine shuttleengine.Engine, anchorPath, worktreeRoot string) *shuttleengine.Runner {
	runner := shuttleengine.NewRunner(reed, shuttlefake.EngineSet(engine), anchorPath, worktreeRoot, shuttleengine.Config{RunTimeoutMin: 30, PollIntervalMS: 1000, LivenessEveryNPolls: 1, StartupTimeoutS: 1})
	runner.SetClock(&fakeClock{now: time.Now()})
	return runner
}

// TestRunCLI_Run_FlagValidation drives runCmd, and openCmd for its OpenMissingRole and OpenInvalidModelSpec rows, directly rather than through RunCLI, so no parent
// PersistentPreRunE runs and no abort is in play — this is the flag-shape check on its own terms,
// which is what these cases are actually about. Its companion below covers what happens when a
// pre-run abort and a flag error coincide.
// Each case asserts the whole buffer parses as EXACTLY ONE JSON object: the module's contract is one
// envelope per invocation, and a substring check cannot see a second one.
func TestRunCLI_Run_FlagValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// build constructs the command under test; nil means c.runCmd.
		build   func(c *shuttleCLI) *cobra.Command
		args    []string
		wantErr string
	}{
		{
			name:    "OpenMissingRole",
			build:   (*shuttleCLI).openCmd,
			wantErr: "--role is required",
		},
		{
			name:    "OpenInvalidModelSpec",
			build:   (*shuttleCLI).openCmd,
			args:    []string{"--role", "explore", "--model", "opus[nope"},
			wantErr: "--model",
		},
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
		{
			name:    "AnchorRetiredBelowSpelling",
			args:    []string{"--prompt", "do the thing", "--output-file", "out.md", "--anchor", "below-parent"},
			wantErr: "own-window",
		},
		{
			name:    "AnchorRetiredTopSpelling",
			args:    []string{"--prompt", "do the thing", "--output-file", "out.md", "--anchor", "top"},
			wantErr: "own-window",
		},
		{
			name:    "AnchorOverview",
			args:    []string{"--prompt", "do the thing", "--output-file", "out.md", "--anchor", "overview"},
			wantErr: "own-window",
		},
		{
			name:    "AnchorUnknown",
			args:    []string{"--prompt", "do the thing", "--output-file", "out.md", "--anchor", "sideways"},
			wantErr: "own-window",
		},
		{
			name:    "AnchorEmpty",
			args:    []string{"--prompt", "do the thing", "--output-file", "out.md", "--anchor", ""},
			wantErr: "own-window",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// A nil runner is deliberate: every case here must be refused by the flag-shape checks
			// before c.runner is ever dereferenced, and a nil runner is what proves it.
			c := &shuttleCLI{}
			build := tt.build
			if build == nil {
				build = (*shuttleCLI).runCmd
			}
			cmd := build(c)
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

// TestRunCLI_PositionalArgValidation verifies that "lyx shuttle interrupt" and "lyx shuttle send" enforce their exact positional arguments (<guid>, and <guid> <text>), and "lyx shuttle state" accepts none, via cobra's Args validation, which runs before PersistentPreRunE — so this fires even against a non-git directory with no config to resolve.
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
		{"StateStrayArgument", []string{"state", "stray"}},
		{"AwaitNoArgs", []string{"await"}},
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

// TestAwaitVerb runs await from a temp directory outside any git repository and asserts its answer, exit code and envelope for each way a watched path can exist, be absent or be unreadable, and that the verb writes nothing.
// A live poll waits out no interval here; the engine's poll test pins it.
func TestAwaitVerb(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// seed names the files that exist before the verb runs.
		seed []string
		args []string
		// wantPath is the name under the directory the envelope's path carries on a ready answer.
		wantPath string
		// wantWatched are the names under the directory the envelope lists as watched.
		wantWatched []string
		wantReady   bool
		wantErr     bool
		// wantEntries are the names the directory holds afterwards.
		wantEntries []string
	}{
		{
			name:        "existing path is ready at once",
			seed:        []string{"a"},
			args:        []string{"a"},
			wantPath:    "a",
			wantWatched: []string{"a"},
			wantReady:   true,
			wantEntries: []string{"a"},
		},
		{
			name:        "the second path alone existing is the one named",
			seed:        []string{"b"},
			args:        []string{"a", "b"},
			wantPath:    "b",
			wantWatched: []string{"a", "b"},
			wantReady:   true,
			wantEntries: []string{"b"},
		},
		{
			name:        "absent path answers not ready after the cap",
			args:        []string{"a", "--cap", "50ms"},
			wantWatched: []string{"a"},
		},
		{
			name:    "path under a regular file is a stat error",
			seed:    []string{"blocker"},
			args:    []string{filepath.Join("blocker", "a"), "--cap", "50ms"},
			wantErr: true,
			// the directory still holds the blocker alone.
			wantEntries: []string{"blocker"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			for _, name := range tt.seed {
				if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
					t.Fatalf("seed %q: %v", name, err)
				}
			}

			var out bytes.Buffer
			code := RunCLIIn(dir, &out, append([]string{"await"}, tt.args...))

			var envelope map[string]any
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("envelope %q is not JSON: %v", out.String(), err)
			}
			if tt.wantErr {
				if code != 1 || envelope["ok"] != false {
					t.Errorf("await = exit %d, envelope %v; want exit 1, ok false", code, envelope)
				}
			} else {
				if code != 0 || envelope["ok"] != true || envelope["ready"] != tt.wantReady {
					t.Fatalf("await = exit %d, envelope %v; want exit 0, ok true, ready %v", code, envelope, tt.wantReady)
				}
				if tt.wantReady {
					if envelope["path"] != filepath.Join(dir, tt.wantPath) {
						t.Errorf("path = %v; want %q", envelope["path"], filepath.Join(dir, tt.wantPath))
					}
				} else if envelope["message"] != "none of the watched paths exists yet; call this command again" {
					t.Errorf("message = %v; want the call-again message", envelope["message"])
				}
				watched, _ := envelope["watched"].([]any)
				if len(watched) != len(tt.wantWatched) {
					t.Fatalf("watched = %v; want %v under %q", watched, tt.wantWatched, dir)
				}
				for i, name := range tt.wantWatched {
					if watched[i] != filepath.Join(dir, name) {
						t.Errorf("watched[%d] = %v; want %q", i, watched[i], filepath.Join(dir, name))
					}
				}
			}

			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatalf("ReadDir() error = %v", err)
			}
			var got []string
			for _, entry := range entries {
				got = append(got, entry.Name())
			}
			if strings.Join(got, ",") != strings.Join(tt.wantEntries, ",") {
				t.Errorf("directory entries = %v; want %v (the verb writes nothing)", got, tt.wantEntries)
			}
		})
	}
}

// errSpecCaptured is the sentinel the spec-capturing engine's Prepare returns,
// so the test can tell "Prepare ran and recorded the spec" apart from any other failure mode.
var errSpecCaptured = errors.New("specCapturingEngine: spec captured")

// startupPending answers every capture as a pane that never becomes ready.
func startupPending(string) shuttleengine.StartupState { return shuttleengine.StartupPending }

// TestRunCmd_EffortAndFocusFlags proves --effort and --focus land in the shuttleengine.Spec run builds, mirroring how --model is wired: a real *shuttleengine.Runner over a spec-capturing Engine fake and an inert reed fake lets the test drive runCmd()'s RunE directly and inspect the Spec the engine's Prepare was actually called with, without a live tmux/claude session.
// --focus=false keeps the viewer where it is, and the default takes focus.
// Prepare fails before Runner.Start reaches reed.AddStrand, so the reed fake is never exercised.
// That failure happens before any strand exists, so the error envelope must carry none of the run-identity fields (guid, sessionId, runDir) rather than three empty strings.
func TestRunCmd_EffortAndFocusFlags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		args           []string
		wantEffort     string
		wantFocus      bool
		wantKeepViewer bool
	}{
		{
			name:       "EffortFlagSet",
			args:       []string{"--prompt", "do the thing", "--output-file", "out.md", "--effort", "high"},
			wantEffort: "high",
			wantFocus:  true,
		},
		{
			name:       "EffortAndFocusFlagsOmitted",
			args:       []string{"--prompt", "do the thing", "--output-file", "out.md"},
			wantEffort: "",
			wantFocus:  true,
		},
		{
			name:           "FocusFalseKeepsTheViewer",
			args:           []string{"--prompt", "do the thing", "--output-file", "out.md", "--focus=false"},
			wantKeepViewer: true,
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
			runner := shuttleengine.NewRunner(&shuttlefake.Reed{}, shuttlefake.EngineSet(engine), anchorPath, worktreeRoot, shuttleengine.Config{RunTimeoutMin: 30})

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
			if display := engine.LastSpec.Display; display.Focus != tt.wantFocus || display.KeepViewer != tt.wantKeepViewer {
				t.Errorf("Spec.Display = %+v; want Focus %v and KeepViewer %v", display, tt.wantFocus, tt.wantKeepViewer)
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
// strand registered still names the strand, session, and run directory in its envelope.
// The failing status checks tear the run down as not started, so the envelope reports a died outcome with the run's identity.
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
	runner := newFakeClockRunner(reed, engine, anchorPath, worktreeRoot)

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
	if got, _ := envelope["outcome"].(string); got != string(shuttleengine.OutcomeDied) {
		t.Errorf("envelope outcome = %q; want %q; output: %s", got, shuttleengine.OutcomeDied, out.String())
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

// TestSendCmd_AppendsMessageTail drives sendCmd over a started run on a fake reed and engine and asserts the one text typed into the pane,
// so the verb is what is pinned, not the helper it calls.
func TestSendCmd_AppendsMessageTail(t *testing.T) {
	t.Parallel()
	tail := shuttleengine.MessageTail
	tests := []struct {
		name    string
		text    string
		want    string
		covered bool
	}{
		{"plain text gains the tail", "read the plan", "read the plan " + tail, false},
		{"text already ending with the tail is typed once with it", "read the plan " + tail, "read the plan " + tail, false},
		{"a strand a live pause covers is refused and types nothing", "read the plan", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			var pane strings.Builder
			reed := &shuttlefake.Reed{
				SendTextFn: func(_, text string, _ bool) error {
					pane.WriteString(text + "\n")
					return nil
				},
				CapturePaneFn: func(string) (string, error) { return pane.String(), nil },
			}
			engine := &shuttlefake.Engine{
				PrepareLaunch: &shuttleengine.Launch{Cmd: "launch", ResumeCmd: "resume", SessionID: "session-1"},
				ComposeSendFn: func(text string) []shuttleengine.PaneInput {
					return []shuttleengine.PaneInput{{Text: text, Submit: true}}
				},
			}
			runner := newFakeClockRunner(reed, engine, root, root)
			run, err := runner.Start(shuttleengine.Spec{Prompt: "do the thing", OutputFiles: []string{filepath.Join(root, "out.md")}})
			if err != nil {
				t.Fatalf("runner.Start: %v", err)
			}

			cli := &shuttleCLI{runner: runner}
			if tt.covered {
				cli.pause = pauserecord.Geometry{RecordPath: filepath.Join(root, "pause", pauserecord.RecordFileName), Slug: "s1"}
				now := time.Now()
				if _, err := pauserecord.Update(cli.pause.RecordPath, now, func(record *pauserecord.Record) error {
					record.Live = append(record.Live, pauserecord.Entry{Scope: "s1", PausedAt: now, Caller: "EXTERNAL"})
					return nil
				}); err != nil {
					t.Fatalf("pauserecord.Update: %v", err)
				}
			}
			cmd := cli.sendCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{run.StrandGUID(), tt.text})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("cmd.Execute() error: %v; output: %s", err, out.String())
			}

			envelope := parseSingleEnvelope(t, out.Bytes())
			if tt.covered {
				if ok, _ := envelope["ok"].(bool); ok {
					t.Fatalf("envelope ok = true; want a refusal; output: %s", out.String())
				}
				if msg, _ := envelope["error"].(string); !strings.Contains(msg, "lyx resume s1") {
					t.Errorf("refusal = %q; want it to name %q", msg, "lyx resume s1")
				}
				if len(reed.SendTextCalls) != 0 {
					t.Errorf("typed texts = %+v; want none", reed.SendTextCalls)
				}
				return
			}
			if ok, _ := envelope["ok"].(bool); !ok {
				t.Fatalf("envelope ok = false; output: %s", out.String())
			}
			if len(reed.SendTextCalls) != 1 || reed.SendTextCalls[0].Text != tt.want {
				t.Errorf("typed texts = %+v; want exactly one, %q", reed.SendTextCalls, tt.want)
			}
		})
	}
}

// TestInterruptAndSendCmd_FailWithTheGuardsMessage drives interruptCmd and sendCmd over a started run whose pane shows no running turn or the rewind picker.
// Each verb's error envelope carries the engine's message, and only the picker's one closing key is pressed.
func TestInterruptAndSendCmd_FailWithTheGuardsMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		pickerOpen bool
		send       bool
		wantError  string
		wantKeys   []string
	}{
		{"interrupt on a pane with no running turn fails naming lyx shuttle send", false, false, "lyx shuttle send", nil},
		{"interrupt on an open picker closes it once and fails", true, false, "rewind picker was open", []string{"Escape"}},
		{"send on an open picker closes it once, fails and types nothing", true, true, "rewind picker was open", []string{"Escape"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			var pickerOpen bool
			reed := &shuttlefake.Reed{
				CapturePaneFn: func(string) (string, error) {
					if pickerOpen {
						return "PICKER", nil
					}
					return "idle", nil
				},
				SendKeyFn: func(_, key string) error {
					if key == "Escape" {
						pickerOpen = false
					}
					return nil
				},
			}
			engine := &shuttlefake.Engine{
				PrepareLaunch:       &shuttleengine.Launch{Cmd: "launch", ResumeCmd: "resume", SessionID: "session-1"},
				InterruptSequenceFn: func() []shuttleengine.PaneInput { return []shuttleengine.PaneInput{{Key: "Escape"}} },
				RewindPickerFn:      func(capture string) bool { return capture == "PICKER" },
				RunningTurnFn:       func(string) bool { return false },
			}
			runner := newFakeClockRunner(reed, engine, root, root)
			run, err := runner.Start(shuttleengine.Spec{Prompt: "do the thing", OutputFiles: []string{filepath.Join(root, "out.md")}})
			if err != nil {
				t.Fatalf("runner.Start: %v", err)
			}
			pickerOpen = tt.pickerOpen
			reed.SendKeyCalls = nil

			cli := &shuttleCLI{runner: runner}
			cmd := cli.interruptCmd()
			args := []string{run.StrandGUID()}
			if tt.send {
				cmd = cli.sendCmd()
				args = append(args, "read the plan")
			}
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("cmd.Execute() error: %v; output: %s", err, out.String())
			}

			envelope := parseSingleEnvelope(t, out.Bytes())
			if ok, _ := envelope["ok"].(bool); ok {
				t.Fatalf("envelope ok = true; want a refusal; output: %s", out.String())
			}
			if msg, _ := envelope["error"].(string); !strings.Contains(msg, tt.wantError) {
				t.Errorf("error = %q; want it to contain %q", msg, tt.wantError)
			}
			var keys []string
			for _, call := range reed.SendKeyCalls {
				keys = append(keys, call.Key)
			}
			if strings.Join(keys, ",") != strings.Join(tt.wantKeys, ",") {
				t.Errorf("keys pressed = %v; want %v", keys, tt.wantKeys)
			}
			if len(reed.SendTextCalls) != 0 || len(reed.PasteSubmitCalls) != 0 {
				t.Errorf("typed texts %+v and pastes %+v; want none", reed.SendTextCalls, reed.PasteSubmitCalls)
			}
		})
	}
}

// stateEngine is a shuttlefake.Engine that also reads session signals: a line STOP@<time> is a turn end stamped with that time.
// Its process always reads alive, so the state read depends on the run's files alone.
type stateEngine struct {
	*shuttlefake.Engine
}

func (stateEngine) ParseSessionSignals(data []byte) ([]shuttleengine.SessionSignal, int) {
	var signals []shuttleengine.SessionSignal
	for _, line := range strings.Split(string(data), "\n") {
		if stamp, ok := strings.CutPrefix(line, "STOP@"); ok {
			at, _ := time.Parse(time.RFC3339, stamp)
			signals = append(signals, shuttleengine.SessionSignal{Kind: shuttleengine.SessionSignalTurnEnd, At: at, SessionID: "session-1", Raw: []byte(line)})
		}
	}
	return signals, len(data)
}

func (stateEngine) ProcessLiveness(string) shuttleengine.Liveness { return shuttleengine.LivenessAlive }

func (stateEngine) TurnStartInterrupt(shuttleengine.SessionSignal) (time.Time, bool) {
	return time.Time{}, false
}

// TestStateCmd_ListsRunningRunsAndNarrowsByStrand drives stateCmd over a run-directory root holding one running record with a stamped turn end and its output file,
// and asserts the envelope's run, state, cause and history, then that --strand with a name matching no running run prints an empty list.
func TestStateCmd_ListsRunningRunsAndNarrowsByStrand(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runDir := filepath.Join(root, "run-1")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(runDir, "report.md")
	if err := os.WriteFile(output, []byte("done"), 0o644); err != nil {
		t.Fatal(err)
	}
	eventsPath := filepath.Join(runDir, "events.jsonl")
	if err := os.WriteFile(eventsPath, []byte("STOP@2026-10-08T07:05:00Z\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	record, err := json.Marshal(shuttleengine.RunState{
		RunID: "run-1", StrandGUID: "guid-1", StrandName: "hub:task:driver", SessionID: "session-1",
		OutputFiles: []string{output}, EventsPath: eventsPath, Outcome: "running",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "run.json"), record, 0o644); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) map[string]any {
		c := &shuttleCLI{cfg: shuttleengine.Config{RunDir: root}, anchorPath: t.TempDir(), engines: shuttlefake.EngineSet(stateEngine{&shuttlefake.Engine{}})}
		cmd := c.stateCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("cmd.Execute() error: %v; output: %s", err, out.String())
		}
		return parseSingleEnvelope(t, out.Bytes())
	}

	envelope := run()
	runs, _ := envelope["runs"].([]any)
	if len(runs) != 1 {
		t.Fatalf("runs = %v; want exactly the one running record", envelope["runs"])
	}
	got, _ := runs[0].(map[string]any)
	for key, want := range map[string]string{
		"strand": "hub:task:driver", "guid": "guid-1", "state": "idle-done", "cause": "done", "since": "2026-10-08T07:05:00Z",
	} {
		if got[key] != want {
			t.Errorf("run[%q] = %v; want %q", key, got[key], want)
		}
	}
	history, _ := got["history"].([]any)
	if len(history) == 0 {
		t.Fatalf("history is empty; want the states passed through")
	}
	if last, _ := history[len(history)-1].(map[string]any); last["state"] != "idle-done" || last["cause"] != "done" {
		t.Errorf("last history entry = %v; want idle-done/done", last)
	}

	for _, name := range []string{"hub:task:driver", "guid-1"} {
		if runs, _ := run("--strand", name)["runs"].([]any); len(runs) != 1 {
			t.Errorf("--strand %q runs = %v; want the one matching run", name, runs)
		}
	}
	empty := run("--strand", "no-such-strand")
	if runs, ok := empty["runs"].([]any); !ok || len(runs) != 0 || empty["ok"] != true {
		t.Errorf("--strand with no match = %v; want ok with an empty runs list", empty)
	}
}

// openRecorderEngine is a shuttlefake.Engine with the SessionOpener capability: OpenLaunch appends the spec it is given to specs and answers a fixed launch beside err.
type openRecorderEngine struct {
	shuttlefake.Engine

	err   error
	specs []shuttleengine.OpenSpec
}

func (e *openRecorderEngine) OpenLaunch(spec shuttleengine.OpenSpec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	e.specs = append(e.specs, spec)
	return shuttleengine.Launch{Cmd: "open-cmd", ResumeCmd: "resume-cmd", SessionID: "session-1"}, e.err
}

// TestOpenCmd_ModelAliasEngine pins the engine and permission mode `open` hands Runner.Open, and that a refusal from Open reaches the error envelope.
func TestOpenCmd_ModelAliasEngine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
		// wantEngine is the engine whose recorder must see the one spec.
		wantEngine string
		wantMode   string
		// openErr scripts the wanted engine's refusal.
		openErr error
	}{
		{name: "an alias of the claude engine", args: []string{"--role", "explore", "--model", "opus"}, wantEngine: "claude"},
		{name: "no model leaves the set's default", args: []string{"--role", "explore"}, wantEngine: shuttlefake.EngineName},
		{name: "bypass is passed through", args: []string{"--role", "explore", "--permission-mode", "bypass"}, wantEngine: shuttlefake.EngineName, wantMode: "bypass"},
		{name: "prompt is passed through", args: []string{"--role", "explore", "--permission-mode", "prompt"}, wantEngine: shuttlefake.EngineName, wantMode: "prompt"},
		{name: "a wrong-case mode is passed unchanged", args: []string{"--role", "explore", "--permission-mode", "Bypass"}, wantEngine: shuttlefake.EngineName, wantMode: "Bypass"},
		{
			name: "an engine refusal reaches the error envelope", args: []string{"--role", "explore", "--permission-mode", "auto"},
			wantEngine: shuttlefake.EngineName, wantMode: "auto", openErr: errors.New("invalid permission mode \"auto\""),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			worktreeRoot := t.TempDir()
			recorders := map[string]*openRecorderEngine{
				shuttlefake.EngineName: {},
				"claude":               {},
			}
			recorders[tt.wantEngine].err = tt.openErr
			engines := shuttleengine.NewEngineSet(shuttlefake.EngineName, map[string]shuttleengine.Engine{
				shuttlefake.EngineName: recorders[shuttlefake.EngineName],
				"claude":               recorders["claude"],
			})
			runner := shuttleengine.NewRunner(&shuttlefake.Reed{}, engines, worktreeRoot, worktreeRoot, shuttleengine.Config{RunTimeoutMin: 30})
			c := &shuttleCLI{runner: runner, configDirs: configengine.Dirs{Anchor: t.TempDir()}}
			cmd := c.openCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("cmd.Execute() error: %v; output: %s", err, out.String())
			}

			envelope := parseSingleEnvelope(t, out.Bytes())
			ok, _ := envelope["ok"].(bool)
			if tt.openErr == nil && !ok {
				t.Errorf("envelope = %v; want ok", envelope)
			}
			if tt.openErr != nil {
				if got, _ := envelope["error"].(string); ok || !strings.Contains(got, "invalid permission mode") {
					t.Errorf("envelope = %v; want the engine's refusal in the error", envelope)
				}
			}
			for name, recorder := range recorders {
				if name != tt.wantEngine {
					if len(recorder.specs) != 0 {
						t.Errorf("engine %s saw specs %+v; want none", name, recorder.specs)
					}
					continue
				}
				if len(recorder.specs) != 1 || recorder.specs[0].PermissionMode != tt.wantMode {
					t.Errorf("engine %s saw specs %+v; want exactly one with PermissionMode %q", name, recorder.specs, tt.wantMode)
				}
			}
		})
	}
}
