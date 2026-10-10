// start_test.go drives the start verb's RunE over a fake strandOps, session starter and watcher spawn, asserting the recorded calls and the saved state without a spawn.

package orchcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// fakeStarter records the specs it is asked to start and answers a fixed guid and warning.
type fakeStarter struct {
	specs   []shuttleengine.Spec
	guid    string
	warning string
	err     error
}

func (f *fakeStarter) StartSession(spec shuttleengine.Spec) (string, string, error) {
	f.specs = append(f.specs, spec)
	return f.guid, f.warning, f.err
}

// startHarness bundles a receiver with its fakes.
type startHarness struct {
	cli     *orchCLI
	strands *fakeStrands
	starter *fakeStarter
	spawns  int
	// sleeps counts the waits start asked for;
	// onSleep, when set, runs on each.
	sleeps  int
	onSleep func()
}

func newStartHarness(t *testing.T, strands ...reedengine.StrandStatus) *startHarness {
	t.Helper()
	fake := &fakeStrands{strands: strands}
	c := newTestCLI(t, fake)
	c.paths.StartLockPath = filepath.Join(c.paths.Dir, "start.lock")
	c.paths.HandoffsDir = filepath.Join(c.paths.Dir, "handoffs")
	c.paths.RolePath = filepath.Join(c.paths.Dir, "role.md")
	c.paths.NoteTemplatePath = filepath.Join(c.paths.Dir, "note-template.md")
	c.cfg.Model, c.cfg.Effort = "opus", "high"
	c.stencilsDir = stencilkit.Seed(t)
	c.location = locationkit.Location(t.TempDir(), "main", "")
	h := &startHarness{cli: c, strands: fake, starter: &fakeStarter{guid: "new-guid"}}
	c.starter = h.starter
	c.reedUp = func() error { return nil }
	c.spawnWatcher = func() error { h.spawns++; return nil }
	c.sleep = func(time.Duration) {
		h.sleeps++
		if h.onSleep != nil {
			h.onSleep()
		}
	}
	return h
}

// run executes start with args and decodes the envelope.
func (h *startHarness) run(t *testing.T, args ...string) (int, map[string]any) {
	t.Helper()
	var out bytes.Buffer
	code := clihelp.Execute(h.cli.startCmd(), &out, args)
	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("output %q is not one JSON object: %v", out.String(), err)
	}
	return code, env
}

func (h *startHarness) state(t *testing.T) orchengine.State {
	t.Helper()
	st, err := orchengine.LoadState(h.cli.paths)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestStart_NoStrandLaunchesAndSpawnsWatcher(t *testing.T) {
	t.Parallel()

	h := newStartHarness(t)
	h.cli.cfg.PermissionMode = "bypass"
	code, env := h.run(t)

	if code != 0 {
		t.Fatalf("exit = %d; env = %v", code, env)
	}
	if len(h.starter.specs) != 1 || h.spawns != 1 {
		t.Fatalf("starts = %d, spawns = %d; want 1 and 1", len(h.starter.specs), h.spawns)
	}
	spec := h.starter.specs[0]
	if !strings.Contains(spec.Prompt, h.cli.paths.RolePath) {
		t.Errorf("prompt = %q; want the start pointer at the role file", spec.Prompt)
	}
	if _, err := os.Stat(h.cli.paths.RolePath); err != nil {
		t.Errorf("role file not rendered before launch: %v", err)
	}
	if st := h.state(t); st.Strand != "new-guid" || st.Phase != orchengine.PhaseIdle {
		t.Errorf("state = %+v; want strand new-guid in idle", st)
	}
	if env["action"] != actionRelaunched || env["strand"] != "new-guid" || env["prompt_source"] != orchengine.SourceFresh {
		t.Errorf("envelope = %v", env)
	}

	if spec.PermissionMode != "bypass" {
		t.Errorf("PermissionMode = %q; want bypass", spec.PermissionMode)
	}
	if !spec.AllowAgentTool || !spec.ForkSubagents {
		t.Errorf("AllowAgentTool/ForkSubagents = %v/%v; want both true", spec.AllowAgentTool, spec.ForkSubagents)
	}
	if !spec.Interactive || spec.NameOverride != "orch" || spec.Role != "orch" || !spec.Display.Focus {
		t.Errorf("spec = %+v; want interactive, focused, named orch", spec)
	}
	if spec.Segment != segmentcolor.Coordinator || !spec.ColorByCaller {
		t.Errorf("Segment/ColorByCaller = %q/%v; want %q/true", spec.Segment, spec.ColorByCaller, segmentcolor.Coordinator)
	}
	if !slices.Equal(spec.Skills, []string{"scribe:prose", "scribe:conversation", "ly:board"}) {
		t.Errorf("Skills = %v; want the three orch skills", spec.Skills)
	}
	if spec.SkillLoadTimeout != h.cli.cfg.HandoffTimeout() {
		t.Errorf("SkillLoadTimeout = %v; want the handoff timeout %v", spec.SkillLoadTimeout, h.cli.cfg.HandoffTimeout())
	}
	if want := "cd '" + h.cli.location.AnchorPath() + "'; lyx orch resume-context"; spec.ContextAfterCompaction != want {
		t.Errorf("ContextAfterCompaction = %q; want %q", spec.ContextAfterCompaction, want)
	}
	if spec.Model != "opus" || spec.Effort != "high" {
		t.Errorf("model/effort = %q/%q; want opus/high", spec.Model, spec.Effort)
	}
	if len(spec.OutputFiles) != 1 || filepath.Dir(spec.OutputFiles[0]) != h.cli.paths.Dir || !strings.HasSuffix(spec.OutputFiles[0], ".never") {
		t.Errorf("output files = %v; want one session-*.never sentinel under %s", spec.OutputFiles, h.cli.paths.Dir)
	}
}

func TestStart_LiveStrand(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		// watcher holds watch.lock;
		// stopping records the watcher stopping;
		// releases frees the lock on the first sleep.
		watcher, stopping, releases bool
		wantAction                  string
		wantSpawns                  int
		wantSleeps                  int
		wantStopping                bool
	}{
		{name: "with a watcher does nothing", watcher: true, wantAction: actionAlreadyRunning},
		{name: "without a watcher spawns the watcher only", wantAction: actionSpawnedWatcher, wantSpawns: 1},
		{name: "a stopping watcher that exits is replaced", watcher: true, stopping: true, releases: true, wantAction: actionSpawnedWatcher, wantSpawns: 1, wantSleeps: 1},
		{name: "a stopping watcher that never exits is reported live", watcher: true, stopping: true, wantAction: actionAlreadyRunning, wantSleeps: 3, wantStopping: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			h := newStartHarness(t, reedengine.StrandStatus{GUID: "g1", Name: "orch", Live: true})
			if c.stopping {
				if err := orchengine.SaveState(h.cli.paths, orchengine.State{Phase: orchengine.PhaseIdle, Strand: "g1", WatcherStopping: true}); err != nil {
					t.Fatal(err)
				}
			}
			if c.watcher {
				if err := os.MkdirAll(h.cli.paths.Dir, 0o755); err != nil {
					t.Fatal(err)
				}
				l, err := lock.AcquireWriteLock(h.cli.paths.WatchLockPath)
				if err != nil {
					t.Fatal(err)
				}
				released := false
				release := func() {
					if !released {
						released = true
						l.Release()
					}
				}
				defer release()
				if c.releases {
					h.onSleep = release
				}
			}

			code, env := h.run(t)
			if code != 0 || env["action"] != c.wantAction {
				t.Fatalf("exit = %d; env = %v", code, env)
			}
			if len(h.starter.specs) != 0 || h.spawns != c.wantSpawns {
				t.Errorf("starts = %d, spawns = %d; want 0 and %d", len(h.starter.specs), h.spawns, c.wantSpawns)
			}
			if h.sleeps != c.wantSleeps {
				t.Errorf("sleeps = %d; want %d", h.sleeps, c.wantSleeps)
			}
			if c.wantStopping {
				if env["watcher_stopping"] != true || env["hint"] != stoppingWatcherHint {
					t.Errorf("envelope = %v; want watcher_stopping true and the hint", env)
				}
			} else if _, has := env["watcher_stopping"]; has {
				t.Errorf("envelope = %v; want no watcher_stopping", env)
			}
			if !c.watcher || c.releases {
				if st := h.state(t); st.Strand != "g1" {
					t.Errorf("state strand = %q; want the adopted g1", st.Strand)
				}
			}
		})
	}
}

// TestStart_NeverAttaches is not parallel: it sets the process-global TMUX with t.Setenv.
func TestStart_NeverAttaches(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1234,0")

	for _, args := range [][]string{nil, {"--no-attach"}} {
		h := newStartHarness(t)
		code, env := h.run(t, args...)
		if code != 0 || env["ok"] != true || env["action"] != actionRelaunched || env["strand"] != "new-guid" {
			t.Errorf("start %v with TMUX set: exit = %d; env = %v; want exit 0 and the success envelope", args, code, env)
		}
	}
}

func TestStart_DeadStrandRemovedBeforeStart(t *testing.T) {
	t.Parallel()

	h := newStartHarness(t, reedengine.StrandStatus{GUID: "corpse", Name: "orch", Live: false})
	removedBeforeStart := false
	h.cli.starter = starterFunc(func(spec shuttleengine.Spec) (string, string, error) {
		removedBeforeStart = len(h.strands.removed) == 1 && h.strands.removed[0] == "corpse"
		return "new-guid", "", nil
	})

	if code, env := h.run(t); code != 0 {
		t.Fatalf("exit = %d; env = %v", code, env)
	}
	if !removedBeforeStart {
		t.Error("the corpse strand was not removed before StartSession")
	}
}

// starterFunc adapts a function to sessionStarter.
type starterFunc func(shuttleengine.Spec) (string, string, error)

func (f starterFunc) StartSession(spec shuttleengine.Spec) (string, string, error) { return f(spec) }

func TestStart_ResumesFromHandoffFile(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		strands    []reedengine.StrandStatus
		fromState  bool
		wantSource string
	}{
		{"dead strand with a last handoff", []reedengine.StrandStatus{{GUID: "corpse", Name: "orch", Live: false}}, true, orchengine.SourceLastHandoff},
		{"handoff flag", nil, false, orchengine.SourceFlag},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			h := newStartHarness(t, c.strands...)
			handoff := filepath.Join(t.TempDir(), "handoff.md")
			if err := os.WriteFile(handoff, []byte("h"), 0o644); err != nil {
				t.Fatal(err)
			}
			var args []string
			if c.fromState {
				if err := orchengine.SaveState(h.cli.paths, orchengine.State{Phase: orchengine.PhaseIdle, Strand: "corpse", LastHandoff: handoff}); err != nil {
					t.Fatal(err)
				}
			} else {
				args = []string{"--handoff", handoff}
			}

			code, env := h.run(t, args...)
			if code != 0 || env["prompt_source"] != c.wantSource {
				t.Fatalf("exit = %d; env = %v", code, env)
			}
			if !strings.Contains(h.starter.specs[0].Prompt, handoff) {
				t.Errorf("prompt = %q; want it naming %s", h.starter.specs[0].Prompt, handoff)
			}
		})
	}
}

func TestStart_FreshLaunchResetsAbandonedPhase(t *testing.T) {
	t.Parallel()

	h := newStartHarness(t)
	if err := orchengine.SaveState(h.cli.paths, orchengine.State{Phase: orchengine.PhaseClearing, Strand: "old", PhaseEventsOffset: 9}); err != nil {
		t.Fatal(err)
	}

	if code, env := h.run(t); code != 0 {
		t.Fatalf("exit = %d; env = %v", code, env)
	}
	st := h.state(t)
	if st.Phase != orchengine.PhaseIdle || st.Strand != "new-guid" || st.PhaseEventsOffset != 0 ||
		!strings.Contains(st.LastAbortReason, string(orchengine.PhaseClearing)) {
		t.Errorf("state = %+v; want idle, new-guid, zero offset and the abandoned clearing recorded", st)
	}
	if len(h.starter.specs) != 1 {
		t.Errorf("starts = %d; want the launch prompt alone", len(h.starter.specs))
	}
}

const adoptTestSessionID = "11111111-2222-3333-4444-555555555555"

func TestStart_Refusals(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		strands []reedengine.StrandStatus
		// args builds the verb's arguments from a handoff file path that exists.
		args   func(handoff string) []string
		wantIn []string
	}{
		{
			name:    "handoff with a live strand",
			strands: []reedengine.StrandStatus{{GUID: "g1", Name: "orch", Live: true}},
			args:    func(handoff string) []string { return []string{"--handoff", handoff} },
			wantIn:  []string{"fresh launch only"},
		},
		{
			name: "two orch strands",
			strands: []reedengine.StrandStatus{
				{GUID: "g1", Name: "orch", Live: true},
				{GUID: "g2", Name: "orch", Live: false},
			},
			args:   func(string) []string { return nil },
			wantIn: []string{"g1", "g2"},
		},
		{
			name:   "adopt with a handoff",
			args:   func(handoff string) []string { return []string{"--adopt", adoptTestSessionID, "--handoff", handoff} },
			wantIn: []string{"exclusive"},
		},
		{
			name:    "adopt with a live strand",
			strands: []reedengine.StrandStatus{{GUID: "g1", Name: "orch", Live: true}},
			args:    func(string) []string { return []string{"--adopt", adoptTestSessionID} },
			wantIn:  []string{"lyx orch stop"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			h := newStartHarness(t, c.strands...)
			handoff := filepath.Join(t.TempDir(), "handoff.md")
			if err := os.WriteFile(handoff, []byte("h"), 0o644); err != nil {
				t.Fatal(err)
			}

			code, env := h.run(t, c.args(handoff)...)
			msg, _ := env["error"].(string)
			if code == 0 {
				t.Fatalf("exit = 0; env = %v; want a refusal", env)
			}
			for _, want := range c.wantIn {
				if !strings.Contains(msg, want) {
					t.Errorf("refusal %q missing %q", msg, want)
				}
			}
			if len(h.starter.specs) != 0 || h.spawns != 0 {
				t.Errorf("starts = %d, spawns = %d; want none", len(h.starter.specs), h.spawns)
			}
		})
	}
}

func TestStart_StarterErrorFailsAndSavesNoState(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		err  string
	}{
		{"fresh launch", nil, "provider never came up"},
		{"adopt", []string{"--adopt", adoptTestSessionID}, "no transcript for that session"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			h := newStartHarness(t)
			h.starter.err = errors.New(c.err)

			code, env := h.run(t, c.args...)
			if msg, _ := env["error"].(string); code == 0 || !strings.Contains(msg, c.err) {
				t.Fatalf("exit = %d; env = %v", code, env)
			}
			if h.spawns != 0 {
				t.Errorf("spawns = %d; want none after a failed launch", h.spawns)
			}
			if st := h.state(t); st.Strand != "" {
				t.Errorf("state = %+v; want none saved", st)
			}
		})
	}
}

func TestStart_AdoptLaunchesResumeSpec(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		strands []reedengine.StrandStatus
		warning string
	}{
		{"dead strand", []reedengine.StrandStatus{{GUID: "corpse", Name: "orch", Live: false}}, ""},
		{"no strand", nil, ""},
		{"starter warning reaches the envelope", nil, "registry unreadable"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			h := newStartHarness(t, c.strands...)
			h.starter.warning = c.warning
			h.cli.cfg.PermissionMode = "bypass"
			if err := orchengine.SaveState(h.cli.paths, orchengine.State{Phase: orchengine.PhaseIdle, LastHandoff: "/keep/me.md"}); err != nil {
				t.Fatal(err)
			}

			code, env := h.run(t, "--adopt", adoptTestSessionID)
			if code != 0 || env["prompt_source"] != orchengine.SourceAdopt || env["action"] != actionRelaunched {
				t.Fatalf("exit = %d; env = %v", code, env)
			}
			if c.warning == "" {
				if _, has := env["warning"]; has {
					t.Errorf("envelope = %v; want no warning", env)
				}
			} else if env["warning"] != c.warning {
				t.Errorf("warning = %v; want %q on the envelope", env["warning"], c.warning)
			}
			if len(h.starter.specs) != 1 || h.spawns != 1 {
				t.Fatalf("starts = %d, spawns = %d; want 1 and 1", len(h.starter.specs), h.spawns)
			}
			spec := h.starter.specs[0]
			if spec.ResumeSessionID != adoptTestSessionID || spec.PermissionMode != "bypass" || !spec.AllowAgentTool || !spec.ForkSubagents {
				t.Errorf("spec = %+v; want the adopted id, bypass, AllowAgentTool and ForkSubagents", spec)
			}
			if st := h.state(t); st.LastHandoff != "/keep/me.md" || st.Strand != "new-guid" {
				t.Errorf("state = %+v; want LastHandoff kept and strand new-guid", st)
			}
		})
	}
}
