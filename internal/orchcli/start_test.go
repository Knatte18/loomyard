// start_test.go drives the start verb's RunE with --no-attach over a fake strandOps, session starter and watcher spawn, asserting the recorded calls and the saved state without a spawn.

package orchcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
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

// seedStartStencils writes every shipped stencil default into a temporary directory.
func seedStartStencils(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	reg := stencils.Registry()
	for _, name := range reg.Names() {
		def, _ := reg.Default(name)
		path := stencilstore.Path(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, def, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// startHarness bundles a receiver with its fakes.
type startHarness struct {
	cli     *orchCLI
	strands *fakeStrands
	starter *fakeStarter
	spawns  int
}

func newStartHarness(t *testing.T, strands ...reedengine.StrandStatus) *startHarness {
	t.Helper()
	fake := &fakeStrands{strands: strands}
	c := newTestCLI(t, fake)
	c.paths.StartLockPath = filepath.Join(c.paths.Dir, "start.lock")
	c.paths.HandoffsDir = filepath.Join(c.paths.Dir, "handoffs")
	c.cfg.Model, c.cfg.Effort = "opus", "high"
	c.stencilsDir = seedStartStencils(t)
	h := &startHarness{cli: c, strands: fake, starter: &fakeStarter{guid: "new-guid"}}
	c.starter = h.starter
	c.reedUp = func() error { return nil }
	c.spawnWatcher = func() error { h.spawns++; return nil }
	return h
}

// run executes start with --no-attach plus args and decodes the envelope.
func (h *startHarness) run(t *testing.T, args ...string) (int, map[string]any) {
	t.Helper()
	var out bytes.Buffer
	code := clihelp.Execute(h.cli.startCmd(), &out, append([]string{"--no-attach"}, args...))
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
	h := newStartHarness(t)
	code, env := h.run(t)

	if code != 0 {
		t.Fatalf("exit = %d; env = %v", code, env)
	}
	if len(h.starter.specs) != 1 || h.spawns != 1 {
		t.Fatalf("starts = %d, spawns = %d; want 1 and 1", len(h.starter.specs), h.spawns)
	}
	if !strings.Contains(h.starter.specs[0].Prompt, "hub orchestrator") {
		t.Errorf("prompt = %q; want the start stencil", h.starter.specs[0].Prompt)
	}
	if st := h.state(t); st.Strand != "new-guid" || st.Phase != orchengine.PhaseIdle {
		t.Errorf("state = %+v; want strand new-guid in idle", st)
	}
	if env["action"] != actionRelaunched || env["strand"] != "new-guid" || env["prompt_source"] != orchengine.SourceFresh || env["attached"] != false {
		t.Errorf("envelope = %v", env)
	}
}

func TestStart_LiveStrandAndWatcherDoesNothing(t *testing.T) {
	h := newStartHarness(t, reedengine.StrandStatus{GUID: "g1", Name: "orch", Live: true})
	if err := os.MkdirAll(h.cli.paths.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	l, err := lock.AcquireWriteLock(h.cli.paths.WatchLockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()

	code, env := h.run(t)
	if code != 0 || env["action"] != actionAttachOnly {
		t.Fatalf("exit = %d; env = %v", code, env)
	}
	if len(h.starter.specs) != 0 || h.spawns != 0 {
		t.Errorf("starts = %d, spawns = %d; want none", len(h.starter.specs), h.spawns)
	}
}

func TestStart_LiveStrandNoWatcherSpawnsWatcherOnly(t *testing.T) {
	h := newStartHarness(t, reedengine.StrandStatus{GUID: "g1", Name: "orch", Live: true})
	code, env := h.run(t)

	if code != 0 || env["action"] != actionSpawnedWatcher {
		t.Fatalf("exit = %d; env = %v", code, env)
	}
	if len(h.starter.specs) != 0 || h.spawns != 1 {
		t.Errorf("starts = %d, spawns = %d; want 0 and 1", len(h.starter.specs), h.spawns)
	}
	if st := h.state(t); st.Strand != "g1" {
		t.Errorf("state strand = %q; want the adopted g1", st.Strand)
	}
}

func TestStart_DeadStrandRemovedBeforeStart(t *testing.T) {
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

func TestStart_DeadStrandWithLastHandoffResumes(t *testing.T) {
	h := newStartHarness(t, reedengine.StrandStatus{GUID: "corpse", Name: "orch", Live: false})
	last := filepath.Join(t.TempDir(), "last.md")
	if err := os.WriteFile(last, []byte("h"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := orchengine.SaveState(h.cli.paths, orchengine.State{Phase: orchengine.PhaseIdle, Strand: "corpse", LastHandoff: last}); err != nil {
		t.Fatal(err)
	}

	code, env := h.run(t)
	if code != 0 || env["prompt_source"] != orchengine.SourceLastHandoff {
		t.Fatalf("exit = %d; env = %v", code, env)
	}
	if !strings.Contains(h.starter.specs[0].Prompt, last) {
		t.Errorf("prompt = %q; want it naming %s", h.starter.specs[0].Prompt, last)
	}
}

func TestStart_HandoffFlagResumesFromFlagFile(t *testing.T) {
	h := newStartHarness(t)
	flagFile := filepath.Join(t.TempDir(), "flag.md")
	if err := os.WriteFile(flagFile, []byte("h"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, env := h.run(t, "--handoff", flagFile)
	if code != 0 || env["prompt_source"] != orchengine.SourceFlag {
		t.Fatalf("exit = %d; env = %v", code, env)
	}
	if !strings.Contains(h.starter.specs[0].Prompt, flagFile) {
		t.Errorf("prompt = %q; want it naming %s", h.starter.specs[0].Prompt, flagFile)
	}
}

func TestStart_HandoffWithLiveStrandRefuses(t *testing.T) {
	h := newStartHarness(t, reedengine.StrandStatus{GUID: "g1", Name: "orch", Live: true})
	flagFile := filepath.Join(t.TempDir(), "flag.md")
	if err := os.WriteFile(flagFile, []byte("h"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, env := h.run(t, "--handoff", flagFile)
	if code == 0 || !strings.Contains(env["error"].(string), "fresh launch only") {
		t.Fatalf("exit = %d; env = %v; want the live-strand refusal", code, env)
	}
	if len(h.starter.specs) != 0 || h.spawns != 0 {
		t.Errorf("starts = %d, spawns = %d; want none", len(h.starter.specs), h.spawns)
	}
}

func TestStart_TwoOrchStrandsRefuse(t *testing.T) {
	h := newStartHarness(t,
		reedengine.StrandStatus{GUID: "g1", Name: "orch", Live: true},
		reedengine.StrandStatus{GUID: "g2", Name: "orch", Live: false},
	)
	code, env := h.run(t)
	msg, _ := env["error"].(string)
	if code == 0 || !strings.Contains(msg, "g1") || !strings.Contains(msg, "g2") {
		t.Fatalf("exit = %d; env = %v; want a refusal naming both strands", code, env)
	}
	if len(h.starter.specs) != 0 || h.spawns != 0 {
		t.Errorf("starts = %d, spawns = %d; want none", len(h.starter.specs), h.spawns)
	}
}

func TestStart_FreshLaunchResetsAbandonedPhase(t *testing.T) {
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

func TestStart_SpecShape(t *testing.T) {
	for _, mode := range []string{"bypass", "prompt"} {
		t.Run(mode, func(t *testing.T) {
			h := newStartHarness(t)
			h.cli.cfg.PermissionMode = mode
			if code, env := h.run(t); code != 0 {
				t.Fatalf("exit = %d; env = %v", code, env)
			}
			spec := h.starter.specs[0]
			if spec.PermissionMode != mode {
				t.Errorf("PermissionMode = %q; want %q", spec.PermissionMode, mode)
			}
			if !spec.AllowAgentTool || !spec.ForkSubagents {
				t.Errorf("AllowAgentTool/ForkSubagents = %v/%v; want both true", spec.AllowAgentTool, spec.ForkSubagents)
			}
		})
	}

	h := newStartHarness(t)
	if code, env := h.run(t); code != 0 {
		t.Fatalf("exit = %d; env = %v", code, env)
	}
	spec := h.starter.specs[0]
	if !spec.Interactive || !spec.AwaitOperator || spec.NameOverride != "orch" || spec.Role != "orch" || !spec.Display.Focus {
		t.Errorf("spec = %+v; want interactive, await-operator, focused, named orch", spec)
	}
	if spec.Model != "opus" || spec.Effort != "high" {
		t.Errorf("model/effort = %q/%q; want opus/high", spec.Model, spec.Effort)
	}
	if len(spec.OutputFiles) != 1 || filepath.Dir(spec.OutputFiles[0]) != h.cli.paths.Dir || !strings.HasSuffix(spec.OutputFiles[0], ".never") {
		t.Errorf("output files = %v; want one session-*.never sentinel under %s", spec.OutputFiles, h.cli.paths.Dir)
	}
}

func TestStart_StartSessionFailureReportsAndSavesNothing(t *testing.T) {
	h := newStartHarness(t)
	h.starter.err = errors.New("provider never came up")

	code, env := h.run(t)
	if code == 0 || !strings.Contains(env["error"].(string), "provider never came up") {
		t.Fatalf("exit = %d; env = %v", code, env)
	}
	if h.spawns != 0 {
		t.Errorf("spawns = %d; want none after a failed launch", h.spawns)
	}
}

const adoptTestSessionID = "11111111-2222-3333-4444-555555555555"

func TestStart_AdoptWithHandoffRefuses(t *testing.T) {
	h := newStartHarness(t)
	code, env := h.run(t, "--adopt", adoptTestSessionID, "--handoff", filepath.Join(t.TempDir(), "h.md"))
	if msg, _ := env["error"].(string); code == 0 || !strings.Contains(msg, "exclusive") {
		t.Fatalf("exit = %d; env = %v; want the exclusive refusal", code, env)
	}
	if len(h.starter.specs) != 0 || h.spawns != 0 {
		t.Errorf("starts = %d, spawns = %d; want none", len(h.starter.specs), h.spawns)
	}
}

func TestStart_AdoptWithLiveStrandRefuses(t *testing.T) {
	h := newStartHarness(t, reedengine.StrandStatus{GUID: "g1", Name: "orch", Live: true})
	code, env := h.run(t, "--adopt", adoptTestSessionID)
	if msg, _ := env["error"].(string); code == 0 || !strings.Contains(msg, "lyx orch stop") {
		t.Fatalf("exit = %d; env = %v; want the refusal naming lyx orch stop", code, env)
	}
	if len(h.starter.specs) != 0 || h.spawns != 0 {
		t.Errorf("starts = %d, spawns = %d; want none", len(h.starter.specs), h.spawns)
	}
}

func TestStart_AdoptLaunchesResumeSpec(t *testing.T) {
	cases := map[string][]reedengine.StrandStatus{
		"dead strand": {{GUID: "corpse", Name: "orch", Live: false}},
		"no strand":   nil,
	}
	for name, strands := range cases {
		t.Run(name, func(t *testing.T) {
			h := newStartHarness(t, strands...)
			h.cli.cfg.PermissionMode = "bypass"
			if err := orchengine.SaveState(h.cli.paths, orchengine.State{Phase: orchengine.PhaseIdle, LastHandoff: "/keep/me.md"}); err != nil {
				t.Fatal(err)
			}

			code, env := h.run(t, "--adopt", adoptTestSessionID)
			if code != 0 || env["prompt_source"] != orchengine.SourceAdopt || env["action"] != actionRelaunched {
				t.Fatalf("exit = %d; env = %v", code, env)
			}
			if _, has := env["warning"]; has {
				t.Errorf("envelope = %v; want no warning", env)
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

func TestStart_StarterWarningReachesEnvelope(t *testing.T) {
	h := newStartHarness(t)
	h.starter.warning = "registry unreadable"
	code, env := h.run(t, "--adopt", adoptTestSessionID)
	if code != 0 || env["warning"] != "registry unreadable" {
		t.Fatalf("exit = %d; env = %v; want the warning on the envelope", code, env)
	}
}

func TestStart_AdoptStarterErrorFailsAndSavesNoState(t *testing.T) {
	h := newStartHarness(t)
	h.starter.err = errors.New("no transcript for that session")

	code, env := h.run(t, "--adopt", adoptTestSessionID)
	if msg, _ := env["error"].(string); code == 0 || !strings.Contains(msg, "no transcript for that session") {
		t.Fatalf("exit = %d; env = %v", code, env)
	}
	if h.spawns != 0 {
		t.Errorf("spawns = %d; want none", h.spawns)
	}
	if st := h.state(t); st.Strand != "" {
		t.Errorf("state = %+v; want none saved", st)
	}
}
