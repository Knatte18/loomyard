// cli_test.go covers the orchcli seam without a live session: the prime refusal decision, the shuttle-config override, and the status, cycle and stop verbs over hand-built receivers with a fake strandOps.

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
	// An empty, non-nil slice keeps cobra from reading the test binary's own flags as arguments.
	code := clihelp.Execute(cmd, &out, []string{})
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

func TestStatus_Envelope(t *testing.T) {
	t.Parallel()

	const tooShort = "orch pane too short for the idle probe; resize or use the larger client"
	liveOrch := []reedengine.StrandStatus{{GUID: "g1", Name: "orch", Live: true}}
	cases := []struct {
		name    string
		strands []reedengine.StrandStatus
		// state is the saved state; nil leaves none on disk.
		state *orchengine.State
		// want maps an envelope key to its value, nil meaning a present JSON null.
		want func(c *orchCLI) map[string]any
	}{
		{
			name:    "populated state",
			strands: liveOrch,
			state: &orchengine.State{
				Strand: "g1", Phase: orchengine.PhaseClearing, LastContextTokens: 999, LastContextKnown: true,
				CycleCount: 4, LastHandoff: "h.md", LastAbortReason: "why", Stuck: "busy", WatcherExit: "gone",
				PhaseEnteredAt: time.Unix(0, 0), CycleTrigger: orchengine.TriggerSoft,
				LastDeferral: time.Date(2026, 10, 1, 12, 30, 0, 0, time.UTC),
			},
			want: func(c *orchCLI) map[string]any {
				return map[string]any{
					"strand": "g1", "strand_live": true, "watcher_live": false, "context_tokens": float64(999),
					"threshold_tokens": float64(1234), "phase": "clearing", "cycle_count": float64(4),
					"last_handoff": "h.md", "last_abort_reason": "why", "stuck": "busy", "watcher_exit": "gone",
					"soft_threshold_tokens": float64(c.cfg.SoftThreshold()), "cycle_trigger": "soft",
					"last_deferral": "2026-10-01T12:30:00Z",
				}
			},
		},
		{
			name:    "too short pane under stuck",
			strands: liveOrch,
			state:   &orchengine.State{Strand: "g1", Phase: orchengine.PhaseIdle, Stuck: tooShort},
			want: func(*orchCLI) map[string]any {
				return map[string]any{"stuck": tooShort, "phase": "idle"}
			},
		},
		{
			name: "unknown tokens are null",
			want: func(*orchCLI) map[string]any {
				return map[string]any{"context_tokens": nil, "strand_live": false, "last_deferral": nil, "cycle_trigger": ""}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := newTestCLI(t, &fakeStrands{strands: tc.strands})
			if tc.state != nil {
				if err := orchengine.SaveState(c.paths, *tc.state); err != nil {
					t.Fatal(err)
				}
			}

			code, env := runVerb(t, c.statusCmd())
			if code != 0 {
				t.Fatalf("exit = %d; env %v", code, env)
			}
			for k, v := range tc.want(c) {
				if got, present := env[k]; !present || got != v {
					t.Errorf("status[%q] = %v (present %v); want %v", k, got, present, v)
				}
			}
		})
	}
}

func TestRefreshAndDistill_RequestTheirOwnMode(t *testing.T) {
	t.Parallel()

	verbs := []struct {
		name string
		verb func(*orchCLI) *cobra.Command
		want string
	}{
		{"refresh clears", (*orchCLI).refreshCmd, orchengine.CycleClear},
		{"distill compacts", (*orchCLI).distillCmd, orchengine.CycleCompact},
	}
	for _, tc := range verbs {
		t.Run(tc.name+" with a live watcher", func(t *testing.T) {
			t.Parallel()

			c := newTestCLI(t, &fakeStrands{})
			// The configured mode is the opposite of the verb's, which must not matter.
			c.cfg.CycleMode = orchengine.CycleClear
			if tc.want == orchengine.CycleClear {
				c.cfg.CycleMode = orchengine.CycleCompact
			}
			if err := os.MkdirAll(c.paths.Dir, 0o755); err != nil {
				t.Fatal(err)
			}
			held, acquired, err := lock.TryAcquireWriteLock(c.paths.WatchLockPath)
			if err != nil || !acquired {
				t.Fatalf("hold the watch lock: acquired %v, err %v", acquired, err)
			}
			defer held.Release()

			code, env := runVerb(t, tc.verb(c))
			if code != 0 {
				t.Fatalf("exit = %d; env %v", code, env)
			}
			if env["requested"] != true || env["watcher_live"] != true {
				t.Errorf("envelope = %v; want requested true, watcher_live true", env)
			}
			req, pending, err := orchengine.CycleRequested(c.paths)
			if err != nil || !pending {
				t.Fatalf("CycleRequested = %v, %v; want pending, nil", pending, err)
			}
			if req.Mode != tc.want || req.RequestedAt.IsZero() {
				t.Errorf("request = %+v; want mode %q and a request time", req, tc.want)
			}
		})

		t.Run(tc.name+" without a watcher leaves no marker", func(t *testing.T) {
			t.Parallel()

			c := newTestCLI(t, &fakeStrands{})
			code, env := runVerb(t, tc.verb(c))
			if code != 0 {
				t.Fatalf("exit = %d; env %v", code, env)
			}
			if env["watcher_live"] != false {
				t.Errorf("envelope = %v; want watcher_live false", env)
			}
			if _, pending, err := orchengine.CycleRequested(c.paths); err != nil || pending {
				t.Errorf("CycleRequested = %v, %v; want no marker, nil", pending, err)
			}
		})
	}
}

func TestStop(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		strands []reedengine.StrandStatus
		// recorded is the strand saved in state; empty leaves no state on disk.
		recorded    string
		wantRemoved bool
		wantStrand  string
		wantCalls   []string
	}{
		{"removes the tracked strand", []reedengine.StrandStatus{{GUID: "g1", Name: "orch"}}, "g1", true, "g1", []string{"g1"}},
		{"reports a recorded but untracked strand as not removed", nil, "gone", false, "", nil},
		{"reports an unrecorded strand as not removed", nil, "", false, "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeStrands{strands: tc.strands}
			c := newTestCLI(t, fake)
			if tc.recorded != "" {
				if err := orchengine.SaveState(c.paths, orchengine.State{Strand: tc.recorded, Phase: orchengine.PhaseIdle}); err != nil {
					t.Fatal(err)
				}
			}

			code, env := runVerb(t, c.stopCmd())
			if code != 0 || env["removed"] != tc.wantRemoved {
				t.Errorf("stop = exit %d, %v; want exit 0, removed %v", code, env, tc.wantRemoved)
			}
			if tc.wantStrand != "" && env["strand"] != tc.wantStrand {
				t.Errorf("stop envelope strand = %v; want %s", env["strand"], tc.wantStrand)
			}
			if !slices.Equal(fake.removed, tc.wantCalls) {
				t.Errorf("removed = %v; want %v", fake.removed, tc.wantCalls)
			}
		})
	}
}

// TestOrchStrands_MatchesFullAndLegacyNames pins that the orchestrator lookup finds a full-name strand and a legacy exact-name strand, and nothing else.
func TestOrchStrands_MatchesFullAndLegacyNames(t *testing.T) {
	strands := []reedengine.StrandStatus{
		{GUID: "g1", Name: "ly:orch"},
		{GUID: "g2", Name: "orch"},
		{GUID: "g3", Name: "ly:orch-2"},
		{GUID: "g4", Name: "ly:task:driver"},
	}
	got := orchStrands(strands)
	if len(got) != 2 || got[0].GUID != "g1" || got[1].GUID != "g2" {
		t.Errorf("orchStrands = %+v; want g1 and g2 only", got)
	}
}
