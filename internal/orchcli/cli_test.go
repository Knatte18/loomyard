// cli_test.go covers the orchcli seam without a live session: the prime refusal decision, the shuttle-config override, and the status, cycle and stop verbs over hand-built receivers with a fake strandOps.

package orchcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/spf13/cobra"
)

// fakeStrands is a strandOps over a fixed strand list that records removals.
type fakeStrands struct {
	strands []reedengine.StrandStatus
	removed []string
}

func (f *fakeStrands) Strands() ([]reedengine.StrandStatus, error) { return f.strands, nil }

func (f *fakeStrands) RemoveStrand(guid string) error {
	f.removed = append(f.removed, guid)
	return nil
}

// newTestCLI builds a receiver over a temporary Paths and the given fake.
func newTestCLI(t *testing.T, fake *fakeStrands) *orchCLI {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "orch")
	return &orchCLI{
		cfg:     orchengine.Config{ThresholdTokens: 1234},
		strands: fake,
		paths: orchengine.Paths{
			Dir:              dir,
			StatePath:        filepath.Join(dir, "state.json"),
			StateLockPath:    filepath.Join(dir, "state.json.lock"),
			WatchLockPath:    filepath.Join(dir, "watch.lock"),
			CycleRequestPath: filepath.Join(dir, "cycle-request"),
		},
	}
}

// runVerb executes cmd as a root command and decodes its single JSON envelope.
func runVerb(t *testing.T, cmd *cobra.Command) (int, map[string]any) {
	t.Helper()
	var out bytes.Buffer
	code := clihelp.Execute(cmd, &out, nil)
	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("output %q is not one JSON object: %v", out.String(), err)
	}
	return code, env
}

func TestRefuseNonPrime(t *testing.T) {
	t.Parallel()

	if err := refuseNonPrime("main", "main", nil); err != nil {
		t.Errorf("prime worktree refused: %v", err)
	}
	err := refuseNonPrime("task", "main", nil)
	if err == nil || !strings.Contains(err.Error(), "prime worktree only") ||
		!strings.Contains(err.Error(), `"task"`) || !strings.Contains(err.Error(), `"main"`) {
		t.Errorf("non-prime refusal = %v; want prime-only wording naming both worktrees", err)
	}
	err = refuseNonPrime("main", "", errors.New("boom"))
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("unresolvable prime refusal = %v; want it to wrap the cause", err)
	}
}

func TestOrchShuttleConfig_ClearsOnlyAgentDeny(t *testing.T) {
	t.Parallel()

	in := shuttleengine.Config{
		RunDir: "runs", PollIntervalMS: 7, LivenessEveryNPolls: 3, RunTimeoutMin: 9, StartupTimeoutS: 11,
		Claude: "claude", ClaudeDenyAgentTool: true, ClaudeDenyAskUserQuestion: true,
	}
	got := orchShuttleConfig(in)

	want := in
	want.ClaudeDenyAgentTool = false
	if got != want {
		t.Errorf("orchShuttleConfig = %+v; want %+v", got, want)
	}
	if !in.ClaudeDenyAgentTool {
		t.Error("orchShuttleConfig mutated its argument")
	}
}

func TestStatus_ReportsPopulatedState(t *testing.T) {
	t.Parallel()

	fake := &fakeStrands{strands: []reedengine.StrandStatus{{GUID: "g1", Name: "orch", Live: true}}}
	c := newTestCLI(t, fake)
	err := orchengine.SaveState(c.paths, orchengine.State{
		Strand: "g1", Phase: orchengine.PhaseClearing, LastContextTokens: 999, LastContextKnown: true,
		CycleCount: 4, LastHandoff: "h.md", LastAbortReason: "why", WatcherExit: "gone",
		PhaseEnteredAt: time.Unix(0, 0),
	})
	if err != nil {
		t.Fatal(err)
	}

	code, env := runVerb(t, c.statusCmd())
	if code != 0 {
		t.Fatalf("exit = %d; env %v", code, env)
	}
	want := map[string]any{
		"strand": "g1", "strand_live": true, "watcher_live": false, "context_tokens": float64(999),
		"threshold_tokens": float64(1234), "phase": "clearing", "cycle_count": float64(4),
		"last_handoff": "h.md", "last_abort_reason": "why", "watcher_exit": "gone",
	}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("status[%q] = %v; want %v", k, env[k], v)
		}
	}
}

func TestStatus_UnknownTokensAreNull(t *testing.T) {
	t.Parallel()

	c := newTestCLI(t, &fakeStrands{})
	code, env := runVerb(t, c.statusCmd())
	if code != 0 {
		t.Fatalf("exit = %d; env %v", code, env)
	}
	if v, present := env["context_tokens"]; !present || v != nil {
		t.Errorf("context_tokens = %v (present %v); want null", v, present)
	}
	if env["strand_live"] != false {
		t.Errorf("strand_live = %v; want false", env["strand_live"])
	}
}

func TestCycle_WritesRequest(t *testing.T) {
	t.Parallel()

	c := newTestCLI(t, &fakeStrands{})
	code, env := runVerb(t, c.cycleCmd())
	if code != 0 {
		t.Fatalf("exit = %d; env %v", code, env)
	}
	if env["requested"] != true || env["watcher_live"] != false {
		t.Errorf("cycle envelope = %v; want requested true, watcher_live false", env)
	}
	pending, err := orchengine.CycleRequested(c.paths)
	if err != nil || !pending {
		t.Errorf("CycleRequested = %v, %v; want true, nil", pending, err)
	}
}

func TestStop_RemovesTrackedStrand(t *testing.T) {
	t.Parallel()

	fake := &fakeStrands{strands: []reedengine.StrandStatus{{GUID: "g1", Name: "orch"}}}
	c := newTestCLI(t, fake)
	if err := orchengine.SaveState(c.paths, orchengine.State{Strand: "g1", Phase: orchengine.PhaseIdle}); err != nil {
		t.Fatal(err)
	}

	_, env := runVerb(t, c.stopCmd())
	if env["removed"] != true || env["strand"] != "g1" {
		t.Errorf("stop envelope = %v; want removed true, strand g1", env)
	}
	if len(fake.removed) != 1 || fake.removed[0] != "g1" {
		t.Errorf("removed = %v; want [g1]", fake.removed)
	}
}

func TestStop_UntrackedStrandReportsNotRemoved(t *testing.T) {
	t.Parallel()

	fake := &fakeStrands{}
	c := newTestCLI(t, fake)
	if err := orchengine.SaveState(c.paths, orchengine.State{Strand: "gone", Phase: orchengine.PhaseIdle}); err != nil {
		t.Fatal(err)
	}

	code, env := runVerb(t, c.stopCmd())
	if code != 0 || env["removed"] != false {
		t.Errorf("stop = exit %d, %v; want exit 0, removed false", code, env)
	}
	if len(fake.removed) != 0 {
		t.Errorf("removed = %v; want none", fake.removed)
	}

	// An unrecorded strand behaves the same.
	c2 := newTestCLI(t, fake)
	_, env = runVerb(t, c2.stopCmd())
	if env["removed"] != false {
		t.Errorf("unrecorded stop envelope = %v; want removed false", env)
	}
}

func TestRunCLI_NoArgsListsSubcommands(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if code := RunCLI(&out, nil); code != 0 {
		t.Fatalf("RunCLI(nil) = %d; want 0", code)
	}
	for _, sub := range []string{"status", "cycle", "stop"} {
		if !strings.Contains(out.String(), sub) {
			t.Errorf("bare lyx orch output missing %q; got:\n%s", sub, out.String())
		}
	}
}
