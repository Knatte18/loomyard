// cli_test.go covers the shuttlecli cobra seam through RunCLI: run's flag-shape validation and the positional-argument validation of the remaining verbs.
// No live tmux/claude session is required by any test in this file;
// the full run round-trip against a live agent lives in smoke tests (batch 6) and
// the sandbox suite.

package shuttlecli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/google/go-cmp/cmp"
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
	tiergate.Require(t, 0)
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
			name:    "AnchorHidden",
			args:    []string{"--prompt", "do the thing", "--output-file", "out.md", "--anchor", "hidden"},
			wantErr: `invalid --anchor "hidden"; want own-window: reed starts no command for a hidden strand, so its provider never starts, and --focus=false keeps the viewer where it is`,
		},
		{
			name:    "AnchorUnknown",
			args:    []string{"--prompt", "do the thing", "--output-file", "out.md", "--anchor", "sideways"},
			wantErr: `invalid --anchor "sideways"; want own-window`,
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
// Running from a non-git temp dir makes lymcwd.Resolve fail, which is the abort this pins. The
// original intent is untouched and still covered: TestRunCLI_Run_FlagValidation above proves a flag
// error is reported on its own whenever the pre-run did NOT abort.
func TestRunCLI_Run_PreRunAbort_EmitsExactlyOneEnvelope(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestRunCLI_PositionalArgValidation verifies that "lym shuttle state" accepts no argument and "lym shuttle await" needs one, via cobra's Args validation, which runs before PersistentPreRunE — so this fires even against a non-git directory with no config to resolve.
// It also asserts the command tree registers no `send` or `interrupt` child, so a verb registered again fails.
// Each subtest that runs the CLI changes the process working directory, so none runs in parallel.
func TestRunCLI_PositionalArgValidation(t *testing.T) {
	tiergate.Require(t, 0)
	for _, removed := range []string{"send", "interrupt"} {
		t.Run("NoChild_"+removed, func(t *testing.T) {
			for _, child := range Command().Commands() {
				if child.Name() == removed {
					t.Errorf("Command() registers a %q child; want none", removed)
				}
			}
		})
	}

	t.Run("Registers_await-forks", func(t *testing.T) {
		found := false
		for _, child := range Command().Commands() {
			found = found || child.Name() == "await-forks"
		}
		if !found {
			t.Errorf("Command() registers no await-forks child; want one")
		}
	})

	tests := []struct {
		name string
		args []string
	}{
		{"StateStrayArgument", []string{"state", "stray"}},
		{"AwaitNoArgs", []string{"await"}},
		{"AwaitForksStrayArgument", []string{"await-forks", "stray"}},
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
	tiergate.Require(t, 0)
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

// fakeResolver is a strandResolver answering a fixed guid or error.
type fakeResolver struct {
	guid string
	err  error
}

func (r fakeResolver) ResolveStrandGUID(string) (string, error) { return r.guid, r.err }

// forkListEngine is a shuttlefake.Engine with the OutstandingForkReader capability, answering a fixed fork list.
type forkListEngine struct {
	shuttlefake.Engine

	forks []shuttleengine.BackgroundTask
}

func (e *forkListEngine) OutstandingForks(string, string) ([]shuttleengine.BackgroundTask, error) {
	return e.forks, nil
}

// TestAwaitForksVerb pins the await-forks error envelopes for a caller outside a shuttle strand, for a strand reed cannot resolve and for a strand with no run record.
// It also pins the ok envelope with ready true once no fork runs.
// It also pins the ok envelope with ready false and the forks still running when the cap elapses.
// It is not parallel: it sets the strand-name environment variable.
func TestAwaitForksVerb(t *testing.T) {
	tiergate.Require(t, 0)
	fork := shuttleengine.BackgroundTask{Kind: shuttleengine.BackgroundFork, ID: "a1", Label: "lens a"}

	tests := []struct {
		name string
		// strand is the value of the strand-name environment variable; empty leaves it unset.
		strand   string
		resolver fakeResolver
		forks    []shuttleengine.BackgroundTask
		// recorded writes a run record for guid-1 before the verb runs.
		recorded    bool
		wantErr     string
		wantReady   bool
		wantPending []any
	}{
		{name: "no strand name is an error", wantErr: "no shuttle strand"},
		{name: "unresolvable strand is an error", strand: "hub:task:driver", resolver: fakeResolver{err: errors.New("no strand named")}, wantErr: "resolve the calling strand"},
		{name: "strand without a run record is an error", strand: "hub:task:driver", resolver: fakeResolver{guid: "guid-1"}, wantErr: "guid-1"},
		{name: "no fork running is ready", strand: "hub:task:driver", resolver: fakeResolver{guid: "guid-1"}, recorded: true, wantReady: true},
		{
			name:        "forks still running at the cap are listed",
			strand:      "hub:task:driver",
			resolver:    fakeResolver{guid: "guid-1"},
			forks:       []shuttleengine.BackgroundTask{fork},
			recorded:    true,
			wantPending: []any{map[string]any{"kind": "fork", "id": "a1", "label": "lens a"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(agentname.StrandNameEnv, tt.strand)

			anchor := t.TempDir()
			if tt.recorded {
				runDir := filepath.Join(anchor, lymdirs.DotLymDirName, "shuttle", "run-1")
				if err := os.MkdirAll(runDir, 0o755); err != nil {
					t.Fatal(err)
				}
				record, err := json.Marshal(shuttleengine.RunState{RunID: "run-1", StrandGUID: "guid-1", SessionID: "session-1", Outcome: "running"})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(runDir, "run.json"), record, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			engine := &forkListEngine{forks: tt.forks}
			c := &shuttleCLI{reed: tt.resolver, runner: newFakeClockRunner(nil, engine, anchor, anchor)}
			cmd := c.awaitForksCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{"--cap", "3s"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("cmd.Execute() error: %v; output: %s", err, out.String())
			}

			envelope := parseSingleEnvelope(t, out.Bytes())
			if tt.wantErr != "" {
				if got, _ := envelope["error"].(string); envelope["ok"] != false || !strings.Contains(got, tt.wantErr) {
					t.Errorf("envelope = %v; want ok false with an error containing %q", envelope, tt.wantErr)
				}
				return
			}
			if envelope["ok"] != true || envelope["ready"] != tt.wantReady {
				t.Fatalf("envelope = %v; want ok true, ready %v", envelope, tt.wantReady)
			}
			if tt.wantReady {
				return
			}
			if diff := cmp.Diff(tt.wantPending, envelope["outstanding"]); diff != "" {
				t.Errorf("outstanding mismatch (-want +got):\n%s", diff)
			}
			if envelope["message"] != "forks are still running; call this command again" {
				t.Errorf("message = %v; want the call-again message", envelope["message"])
			}
		})
	}
}

// errSpecCaptured is the sentinel the spec-capturing engine's Prepare returns,
// so the test can tell "Prepare ran and recorded the spec" apart from any other failure mode.
var errSpecCaptured = errors.New("specCapturingEngine: spec captured")

// startupPending answers every capture as a pane that never becomes ready.
func startupPending(string) shuttleengine.StartupState { return shuttleengine.StartupPending }

// TestRunCmd_FocusFlag proves --focus lands in the shuttleengine.Spec run builds: a real *shuttleengine.Runner over a spec-capturing Engine fake and an inert reed fake lets the test drive runCmd()'s RunE directly and inspect the Spec the engine's Prepare was actually called with, without a live tmux/claude session.
// --focus=false keeps the viewer where it is, and the default takes focus.
// Prepare fails before Runner.Start reaches reed.AddStrand, so the reed fake is never exercised.
// That failure happens before any strand exists, so the error envelope must carry none of the run-identity fields (guid, sessionId, runDir) rather than three empty strings.
func TestRunCmd_FocusFlag(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name           string
		args           []string
		wantFocus      bool
		wantKeepViewer bool
	}{
		{
			name:      "FocusFlagOmitted",
			args:      []string{"--prompt", "do the thing", "--output-file", "out.md"},
			wantFocus: true,
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

// TestRunCmd_ModelSpec pins that run's --model takes a model spec.
// An alias or an escape-form spec picks the engine whose Prepare sees the run's spec, with the resolved model and effort.
// No --model leaves the set's default engine.
// A spec the registry cannot resolve is refused before any engine prepares.
func TestRunCmd_ModelSpec(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// model is the --model value; empty passes no flag.
		model string
		// wantEngine is the engine whose Prepare must see the spec; empty means none may.
		wantEngine string
		wantChoice shuttleengine.ModelChoice
		// wantErr is a text the error envelope must hold when no engine prepares.
		wantErr string
	}{
		{name: "CodexAliasWithEffort", model: "luna[high]", wantEngine: "codex", wantChoice: shuttleengine.ModelChoice{Engine: "codex", Model: "gpt-6-luna", Effort: "high"}},
		{name: "ClaudeAlias", model: "sonnet", wantEngine: "claude", wantChoice: shuttleengine.ModelChoice{Engine: "claude", Model: "sonnet"}},
		{name: "EscapeForm", model: "claude:claude-opus-5-5[max]", wantEngine: "claude", wantChoice: shuttleengine.ModelChoice{Engine: "claude", Model: "claude-opus-5-5", Effort: "max"}},
		{name: "NoModel", wantEngine: shuttlefake.EngineName},
		{name: "UnknownAlias", model: "bogus[high]", wantErr: `unknown alias "bogus"`},
		{name: "MalformedSpec", model: "sonnet[", wantErr: "--model"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			engines := map[string]*shuttlefake.Engine{}
			set := map[string]shuttleengine.Engine{}
			for _, name := range []string{shuttlefake.EngineName, "claude", "codex"} {
				engines[name] = &shuttlefake.Engine{PrepareErr: errSpecCaptured, StartupFn: startupPending}
				set[name] = engines[name]
			}
			worktreeRoot := t.TempDir()
			runner := shuttleengine.NewRunner(&shuttlefake.Reed{}, shuttleengine.NewEngineSet(shuttlefake.EngineName, set), worktreeRoot, worktreeRoot, shuttleengine.Config{RunTimeoutMin: 30})
			c := &shuttleCLI{runner: runner, configDirs: configengine.Dirs{Anchor: t.TempDir()}}
			cmd := c.runCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			args := []string{"--prompt", "do the thing", "--output-file", "out.md"}
			if tt.model != "" {
				args = append(args, "--model", tt.model)
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("cmd.Execute() error: %v; output: %s", err, out.String())
			}

			for name, engine := range engines {
				if prepared := engine.LastSpec.Prompt != ""; prepared != (name == tt.wantEngine) {
					t.Errorf("engine %s prepared = %v; want %v; output: %s", name, prepared, name == tt.wantEngine, out.String())
				}
			}
			if tt.wantEngine != "" {
				if got := engines[tt.wantEngine].LastSpec.ModelChoice; got != tt.wantChoice {
					t.Errorf("Spec.ModelChoice = %+v; want %+v", got, tt.wantChoice)
				}
				return
			}
			envelope := parseSingleEnvelope(t, out.Bytes())
			if got, _ := envelope["error"].(string); envelope["ok"] != false || !strings.Contains(got, tt.wantErr) {
				t.Errorf("envelope = %v; want ok false with an error containing %q", envelope, tt.wantErr)
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
	tiergate.Require(t, 0)
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
		StatusErr: errors.New(`no reed session; run "lym reed up"`),
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

// apiErrorEngine is a shuttlefake.Engine that reads session signals: a line REJECT:<text> is an API-error turn end carrying <text> as its error.
// Its process always reads alive.
type apiErrorEngine struct {
	*shuttlefake.Engine
}

func (apiErrorEngine) ParseSessionSignals(data []byte) ([]shuttleengine.SessionSignal, int) {
	var signals []shuttleengine.SessionSignal
	for _, line := range strings.Split(string(data), "\n") {
		if text, ok := strings.CutPrefix(line, "REJECT:"); ok {
			signals = append(signals, shuttleengine.SessionSignal{Kind: shuttleengine.SessionSignalAPIErrorTurnEnd, ErrorText: text, ModelRejected: true, SessionID: "session-1", Raw: []byte(line)})
		}
	}
	return signals, len(data)
}

func (apiErrorEngine) ProcessLiveness(string) shuttleengine.Liveness {
	return shuttleengine.LivenessAlive
}

func (apiErrorEngine) TurnStartInterrupt(shuttleengine.SessionSignal) (time.Time, bool) {
	return time.Time{}, false
}

// TestRunCmd_APIErrorEnvelope pins that run's envelope carries apiError when the run ends api-error, and omits it on a done outcome.
func TestRunCmd_APIErrorEnvelope(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// events is what the engine's Prepare writes into the run's events file.
		events string
		// writeOutput makes Prepare write the run's output file.
		writeOutput bool
		wantOutcome shuttleengine.Outcome
		wantError   string
	}{
		{name: "RejectedModelCarriesTheError", events: "START\nREJECT:model_not_found\n", wantOutcome: shuttleengine.OutcomeAPIError, wantError: "model_not_found"},
		// The engine reads no turn end, so the run ends done at its deadline with the output file on disk.
		{name: "DoneCarriesNone", events: "START\n", writeOutput: true, wantOutcome: shuttleengine.OutcomeDone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			anchorPath := t.TempDir()
			worktreeRoot := filepath.Dir(anchorPath)
			output := filepath.Join(anchorPath, "out.md")
			fake := &shuttlefake.Engine{
				StartupFn:     func(string) shuttleengine.StartupState { return shuttleengine.StartupReady },
				ParseEventsFn: func(data []byte) ([]shuttleengine.Event, error) { return nil, nil },
				PrepareFn: func(runDir string, _ shuttleengine.Spec, _ shuttleengine.Config) (shuttleengine.Launch, error) {
					if err := os.WriteFile(filepath.Join(runDir, "events.jsonl"), []byte(tt.events), 0o644); err != nil {
						return shuttleengine.Launch{}, err
					}
					if tt.writeOutput {
						if err := os.WriteFile(output, []byte("done"), 0o644); err != nil {
							return shuttleengine.Launch{}, err
						}
					}
					return shuttleengine.Launch{Cmd: "launch", ResumeCmd: "resume", SessionID: "session-1"}, nil
				},
			}
			reed := &shuttlefake.Reed{
				AddStrandFn: func(reedengine.AddSpec) (reedengine.Strand, error) {
					return reedengine.Strand{GUID: "strand-1"}, nil
				},
				Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%0", Live: true}},
			}
			runner := newFakeClockRunner(reed, apiErrorEngine{fake}, anchorPath, worktreeRoot)

			c := &shuttleCLI{runner: runner}
			cmd := c.runCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"--prompt", "do the thing", "--output-file", output})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("cmd.Execute() error: %v; output: %s", err, out.String())
			}

			envelope := parseSingleEnvelope(t, out.Bytes())
			if got, _ := envelope["outcome"].(string); got != string(tt.wantOutcome) {
				t.Fatalf("envelope outcome = %q; want %q; output: %s", got, tt.wantOutcome, out.String())
			}
			got, present := envelope["apiError"].(string)
			if got != tt.wantError || present != (tt.wantError != "") {
				t.Errorf("envelope apiError = %q (present %v); want %q; output: %s", got, present, tt.wantError, out.String())
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
	tiergate.Require(t, 0)
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

func (e *openRecorderEngine) OpenLaunch(dir string, spec shuttleengine.OpenSpec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	e.specs = append(e.specs, spec)
	return shuttleengine.Launch{Cmd: "open-cmd", ResumeCmd: "resume-cmd", SessionID: "session-1"}, e.err
}

// TestOpenCmd_ModelAliasEngine pins the engine and permission mode `open` hands Runner.Open, the engine, model and effort its envelope reports, and that a refusal from Open reaches the error envelope.
func TestOpenCmd_ModelAliasEngine(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		args []string
		// wantEngine is the engine whose recorder must see the one spec, and the engine the envelope names.
		wantEngine string
		wantModel  string
		wantEffort string
		wantMode   string
		wantSkills []string
		// openErr scripts the wanted engine's refusal.
		openErr error
	}{
		{
			name: "skills are passed in the order given", args: []string{"--role", "board", "--skill", "ly:board-policy", "--skill", "ly:board"},
			wantEngine: shuttlefake.EngineName, wantSkills: []string{"ly:board-policy", "ly:board"},
		},
		{name: "an alias of the claude engine", args: []string{"--role", "explore", "--model", "opus"}, wantEngine: "claude", wantModel: "opus"},
		{name: "an alias with an effort", args: []string{"--role", "explore", "--model", "opus[high]"}, wantEngine: "claude", wantModel: "opus", wantEffort: "high"},
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
			if tt.openErr == nil {
				if !ok {
					t.Errorf("envelope = %v; want ok", envelope)
				}
				if envelope["engine"] != tt.wantEngine || envelope["model"] != tt.wantModel || envelope["effort"] != tt.wantEffort {
					t.Errorf("envelope = %v; want engine %q, model %q, effort %q", envelope, tt.wantEngine, tt.wantModel, tt.wantEffort)
				}
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
				if len(recorder.specs) != 1 || recorder.specs[0].PermissionMode != tt.wantMode || !slices.Equal(recorder.specs[0].Skills, tt.wantSkills) {
					t.Errorf("engine %s saw specs %+v; want exactly one with PermissionMode %q and Skills %v", name, recorder.specs, tt.wantMode, tt.wantSkills)
				}
			}
		})
	}
}

// TestOpenCmd_ListModels pins that --list-models, with no --role, prints the loaded registry sorted by alias with each alias's engine, model and default effort, and opens nothing.
func TestOpenCmd_ListModels(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	anchor := t.TempDir()
	if err := os.MkdirAll(configengine.ConfigDir(anchor), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	models := "zephyr: {engine: claude, model: claude-zephyr-1, defaults: {effort: low}}\n"
	if err := os.WriteFile(configengine.ConfigFile(anchor, "models"), []byte(models), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	recorder := &openRecorderEngine{}
	engines := shuttleengine.NewEngineSet(shuttlefake.EngineName, map[string]shuttleengine.Engine{shuttlefake.EngineName: recorder})
	worktreeRoot := t.TempDir()
	runner := shuttleengine.NewRunner(&shuttlefake.Reed{}, engines, worktreeRoot, worktreeRoot, shuttleengine.Config{RunTimeoutMin: 30})
	c := &shuttleCLI{runner: runner, configDirs: configengine.Dirs{Anchor: anchor}}
	cmd := c.openCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--list-models"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error: %v; output: %s", err, out.String())
	}

	envelope := parseSingleEnvelope(t, out.Bytes())
	if ok, _ := envelope["ok"].(bool); !ok {
		t.Fatalf("envelope = %v; want ok", envelope)
	}
	rows, _ := envelope["models"].([]any)
	byAlias := map[string]map[string]any{}
	var aliases []string
	for _, row := range rows {
		fields, _ := row.(map[string]any)
		alias, _ := fields["alias"].(string)
		aliases = append(aliases, alias)
		byAlias[alias] = fields
	}
	if !sort.StringsAreSorted(aliases) {
		t.Errorf("aliases = %v; want them sorted", aliases)
	}
	wantRows := map[string]map[string]any{
		"zephyr": {"alias": "zephyr", "engine": "claude", "model": "claude-zephyr-1", "effort": "low"},
		"opus":   {"alias": "opus", "engine": "claude", "model": "opus", "effort": ""},
	}
	for alias, want := range wantRows {
		if diff := cmp.Diff(want, byAlias[alias]); diff != "" {
			t.Errorf("--list-models row %q mismatch (-want +got):\n%s", alias, diff)
		}
	}
	if len(recorder.specs) != 0 {
		t.Errorf("OpenLaunch saw specs %+v; want none", recorder.specs)
	}
}

// TestOpenCmd_ListModelsRegistryLoadFailure pins that --list-models over an unparsable models config returns an error envelope naming the flag and opens nothing.
func TestOpenCmd_ListModelsRegistryLoadFailure(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	anchor := t.TempDir()
	if err := os.MkdirAll(configengine.ConfigDir(anchor), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(configengine.ConfigFile(anchor, "models"), []byte("zephyr: [unclosed\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	recorder := &openRecorderEngine{}
	engines := shuttleengine.NewEngineSet(shuttlefake.EngineName, map[string]shuttleengine.Engine{shuttlefake.EngineName: recorder})
	worktreeRoot := t.TempDir()
	runner := shuttleengine.NewRunner(&shuttlefake.Reed{}, engines, worktreeRoot, worktreeRoot, shuttleengine.Config{RunTimeoutMin: 30})
	c := &shuttleCLI{runner: runner, configDirs: configengine.Dirs{Anchor: anchor}}
	cmd := c.openCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--list-models"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error: %v; output: %s", err, out.String())
	}

	envelope := parseSingleEnvelope(t, out.Bytes())
	if ok, _ := envelope["ok"].(bool); ok {
		t.Fatalf("envelope = %v; want an error envelope", envelope)
	}
	if message, _ := envelope["error"].(string); !strings.Contains(message, "--list-models") {
		t.Errorf("error = %q; want it to name --list-models", message)
	}
	if len(recorder.specs) != 0 {
		t.Errorf("OpenLaunch saw specs %+v; want none", recorder.specs)
	}
}

// TestAwaitCap pins where a call's cap comes from: a given --cap wins and calls neither function, a found worktree answers the resolver's cap, and no worktree answers the default without asking the resolver.
func TestAwaitCap(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const strand = "sh:slug:implementer"
	worktree := t.TempDir()
	location := &lymcwd.Location{HubPath: worktree, WorktreeName: "wt"}
	wantDirs := fabricengine.ConfigDirs(location)

	tests := []struct {
		name        string
		flagCap     time.Duration
		flagGiven   bool
		resolveErr  error
		answer      shuttleengines.KeepWarmAnswer
		want        time.Duration
		wantKeep    bool
		wantResolve bool
	}{
		{name: "found worktree answers the resolver's cap", answer: shuttleengines.KeepWarmAnswer{Cap: 50 * time.Millisecond}, want: 50 * time.Millisecond, wantKeep: true, wantResolve: true},
		{name: "a fallback answer still answers its cap", answer: shuttleengines.KeepWarmAnswer{Cap: 50 * time.Millisecond, Fallback: errors.New("no engine")}, want: 50 * time.Millisecond, wantKeep: true, wantResolve: true},
		{name: "no git worktree answers the default", resolveErr: lymcwd.ErrNotAGitRepo, want: shuttleengine.DefaultAwaitCap, wantResolve: true},
		{name: "a given cap wins and asks nothing", flagCap: 30 * time.Millisecond, flagGiven: true, answer: shuttleengines.KeepWarmAnswer{Cap: 50 * time.Millisecond}, want: 30 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var keepCalls, resolveCalls int
			var gotDirs configengine.Dirs
			var gotStrand string
			keepWarm := func(dirs configengine.Dirs, strand string) shuttleengines.KeepWarmAnswer {
				keepCalls++
				gotDirs, gotStrand = dirs, strand
				return tt.answer
			}
			resolveWorktree := func(string) (*lymcwd.Location, error) {
				resolveCalls++
				if tt.resolveErr != nil {
					return nil, tt.resolveErr
				}
				return location, nil
			}

			got := awaitCap(worktree, strand, tt.flagCap, tt.flagGiven, keepWarm, resolveWorktree)
			if got != tt.want {
				t.Errorf("awaitCap() = %v; want %v", got, tt.want)
			}
			if (keepCalls == 1) != tt.wantKeep || (resolveCalls == 1) != tt.wantResolve {
				t.Errorf("awaitCap() made %d keep-warm and %d resolve calls; want keep-warm %v, resolve %v", keepCalls, resolveCalls, tt.wantKeep, tt.wantResolve)
			}
			if tt.wantKeep && (gotDirs != wantDirs || gotStrand != strand) {
				t.Errorf("keepWarm called with %+v, %q; want %+v, %q", gotDirs, gotStrand, wantDirs, strand)
			}
		})
	}
}
