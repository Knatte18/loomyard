//go:build tier0

// cli_test.go covers the orchcli seam without a live session: the prime refusal decision, the shuttle-config override, and the status, cycle and stop verbs over hand-built receivers with a fake strandOps.

package orchcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/spf13/cobra"
)

// fakeAdd is one AddStrand call a fakeStrands recorded, with the guid it answered.
type fakeAdd struct {
	spec reedengine.AddSpec
	guid string
}

// fakeReplace is one ReplaceStrand call a fakeStrands recorded, with the guid it replaced and the guid it answered.
type fakeReplace struct {
	replaced string
	spec     reedengine.AddSpec
	guid     string
}

// fakeStrands is a strandOps over a fixed strand list that records removals, adds and replaces.
type fakeStrands struct {
	strands  []reedengine.StrandStatus
	removed  []string
	added    []fakeAdd
	replaced []fakeReplace
	// addErr, when set, is what AddStrand returns instead of adding.
	addErr error
	// calls, when set, receives "add", "replace" and "remove" as they happen.
	calls *[]string
	// after, when non-nil, is the list Strands answers from its second call on.
	after        []reedengine.StrandStatus
	strandsCalls int
	// onRemove, when set, runs in RemoveStrand before it records the removal.
	onRemove func()
}

func (f *fakeStrands) Strands() ([]reedengine.StrandStatus, error) {
	f.strandsCalls++
	if f.after != nil && f.strandsCalls >= 2 {
		return f.after, nil
	}
	return f.strands, nil
}

func (f *fakeStrands) RemoveStrand(guid string) error {
	if f.onRemove != nil {
		f.onRemove()
	}
	if f.calls != nil {
		*f.calls = append(*f.calls, "remove")
	}
	f.removed = append(f.removed, guid)
	return nil
}

func (f *fakeStrands) AddStrand(spec reedengine.AddSpec) (reedengine.Strand, error) {
	if f.calls != nil {
		*f.calls = append(*f.calls, "add")
	}
	if f.addErr != nil {
		return reedengine.Strand{}, f.addErr
	}
	guid := "added-" + spec.NameOverride
	f.added = append(f.added, fakeAdd{spec: spec, guid: guid})
	return reedengine.Strand{GUID: guid}, nil
}

func (f *fakeStrands) ReplaceStrand(guid string, spec reedengine.AddSpec) (reedengine.Strand, error) {
	if f.calls != nil {
		*f.calls = append(*f.calls, "replace")
	}
	replacement := "replaced-" + spec.NameOverride
	f.replaced = append(f.replaced, fakeReplace{replaced: guid, spec: spec, guid: replacement})
	return reedengine.Strand{GUID: replacement}, nil
}

// fakeRunnerIndex is the index the fake index runner returns.
const fakeRunnerIndex = "- fake-runner-command: from a fake runner\n"

// newTestCLI builds a receiver over a temporary Paths and the given fake.
func newTestCLI(t *testing.T, fake *fakeStrands) *orchCLI {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "orch")
	return &orchCLI{
		cfg:     orchengine.Config{ThresholdTokens: 1234},
		strands: fake,
		// The record is absent until a test writes it with pauserecord.Update.
		pauseRecordPath: filepath.Join(filepath.Dir(dir), "pause", pauserecord.RecordFileName),
		engines:         shuttleengine.NewEngineSet("claude", map[string]shuttleengine.Engine{"claude": claudeengine.New()}),
		indexRunner:     func(string) (string, error) { return fakeRunnerIndex, nil },
		newCreel:        func() (orchengine.Creel, error) { return &fakeCreel{}, nil },
		spawnWatcher:    func() error { return nil },
		// A short wait keeps a stop that finds start.lock held from waiting out the production minute.
		startLockWait: 100 * time.Millisecond,
		paths: orchengine.Paths{
			Dir:              dir,
			StatePath:        filepath.Join(dir, "state.json"),
			StateLockPath:    filepath.Join(dir, "state.json.lock"),
			WatchLockPath:    filepath.Join(dir, "watch.lock"),
			StartLockPath:    filepath.Join(dir, "start.lock"),
			CycleRequestPath: filepath.Join(dir, "cycle-request"),
			PrimeRoot:        filepath.Dir(dir),
			HubRulesPath:     filepath.Join(filepath.Dir(dir), "board", "rules", "hub.md"),
			OwnRulesPath:     filepath.Join(filepath.Dir(dir), "own", "rules.md"),
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

	if err := refuseNonPrime("main", "main", nil, false); err != nil {
		t.Errorf("prime worktree refused: %v", err)
	}
	if err := refuseNonPrime("b", "main", nil, true); err != nil {
		t.Errorf("beam refused: %v", err)
	}
	err := refuseNonPrime("task", "main", nil, false)
	if err == nil || !strings.Contains(err.Error(), "runs from the hub's prime worktree or a beam") ||
		!strings.Contains(err.Error(), `"task"`) || !strings.Contains(err.Error(), `"main"`) ||
		!strings.Contains(err.Error(), "lyx beam create") || !strings.Contains(err.Error(), "lyx beam promote") {
		t.Errorf("non-prime refusal = %v; want the prime-or-beam wording naming both worktrees and both beam verbs", err)
	}
	err = refuseNonPrime("main", "", errors.New("boom"), false)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("unresolvable prime refusal = %v; want it to wrap the cause", err)
	}
}

func TestRefuseBeamStart(t *testing.T) {
	t.Parallel()

	landing := boardengine.RunStatus("running", "Land")
	done := "done"
	tests := []struct {
		name   string
		owner  string
		status *string
		want   []string
	}{
		{"No owner and no status passes", "", nil, nil},
		{"This machine's own beam passes", "desktop", &done, nil},
		{"Another machine's beam names the owner and the takeover", "laptop", nil, []string{`"laptop"`, "lyx adopt b"}},
		{"A beam its landing holds names the landing", "", &landing, []string{"landing", "lyx batten status b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := refuseBeamStart("b", "desktop", tt.owner, tt.status)
			if tt.want == nil {
				if err != nil {
					t.Errorf("refuseBeamStart(b) = %v; want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("refuseBeamStart(b) = nil; want a refusal naming %q", tt.want)
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refuseBeamStart(b) = %q; missing %q", err, want)
				}
			}
		})
	}
}

// requestedAt is the request time requestAt writes, and requestedAtText its RFC 3339 UTC spelling in an envelope.
var requestedAt = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

const requestedAtText = "2026-10-01T12:00:00Z"

// requestAt writes a cycle request of mode made at requestedAt.
func requestAt(t *testing.T, c *orchCLI, mode string) {
	t.Helper()
	if err := orchengine.RequestCycle(c.paths, mode, requestedAt); err != nil {
		t.Fatal(err)
	}
}

// pauseHubAt writes a live hub entry into the pause record, paused at requestedAt.
func pauseHubAt(t *testing.T, c *orchCLI) {
	t.Helper()
	entry := pauserecord.Entry{Scope: pauserecord.ScopeHub, PausedAt: requestedAt, GraceEnd: requestedAt.Add(30 * time.Second)}
	_, err := pauserecord.Update(c.pauseRecordPath, requestedAt, func(r *pauserecord.Record) error {
		r.Live = append(r.Live, entry)
		return nil
	})
	if err != nil {
		t.Fatal(err)
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
		// arrange prepares the cycle request and the pause record; nil leaves both absent.
		arrange func(t *testing.T, c *orchCLI)
		// wantRequest is the envelope's cycle_request, nil meaning a present JSON null.
		wantRequest any
		// want maps an envelope key to its value, nil meaning a present JSON null.
		want func(c *orchCLI) map[string]any
	}{
		{
			name:    "populated state",
			strands: liveOrch,
			state: &orchengine.State{
				Strand: "g1", SessionID: "sess-1", Phase: orchengine.PhaseClearing, LastContextTokens: 999, LastContextKnown: true,
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
					"last_deferral": "2026-10-01T12:30:00Z", "session_id": "sess-1",
					"watcher_restarted": true, "strand_elsewhere": "",
				}
			},
		},
		{
			name:    "too short pane under stuck",
			strands: liveOrch,
			state:   &orchengine.State{Strand: "g1", Phase: orchengine.PhaseIdle, Stuck: tooShort},
			want: func(*orchCLI) map[string]any {
				return map[string]any{"stuck": tooShort, "phase": "idle", "watcher_restarted": true}
			},
		},
		{
			name: "unknown tokens are null",
			want: func(*orchCLI) map[string]any {
				return map[string]any{"context_tokens": nil, "strand_live": false, "last_deferral": nil, "cycle_trigger": "", "session_id": "", "watcher_restarted": false, "strand_elsewhere": ""}
			},
		},
		{
			name:    "a pane in another session is named",
			strands: []reedengine.StrandStatus{{GUID: "g1", Name: "orch", Live: true, Elsewhere: "other:2"}},
			state:   &orchengine.State{Strand: "g1", Phase: orchengine.PhaseIdle},
			want: func(*orchCLI) map[string]any {
				return map[string]any{"strand_elsewhere": "other:2"}
			},
		},
		{
			name:    "a compact request is reported unheld",
			arrange: func(t *testing.T, c *orchCLI) { requestAt(t, c, orchengine.CycleCompact) },
			wantRequest: map[string]any{
				"mode": orchengine.CycleCompact, "requested_at": requestedAtText, "held_by_pause": false,
			},
			want: func(*orchCLI) map[string]any { return nil },
		},
		{
			name: "a request under a live hub pause is reported held",
			arrange: func(t *testing.T, c *orchCLI) {
				requestAt(t, c, orchengine.CycleCompact)
				pauseHubAt(t, c)
			},
			wantRequest: map[string]any{
				"mode": orchengine.CycleCompact, "requested_at": requestedAtText, "held_by_pause": true,
			},
			want: func(*orchCLI) map[string]any { return nil },
		},
		{
			name: "an unreadable pause record holds nothing",
			arrange: func(t *testing.T, c *orchCLI) {
				requestAt(t, c, orchengine.CycleCompact)
				if err := os.MkdirAll(filepath.Dir(c.pauseRecordPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(c.pauseRecordPath, []byte("not json"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantRequest: map[string]any{
				"mode": orchengine.CycleCompact, "requested_at": requestedAtText, "held_by_pause": false,
			},
			want: func(*orchCLI) map[string]any { return nil },
		},
		{
			name: "an unparseable marker reports an empty mode and a null time",
			arrange: func(t *testing.T, c *orchCLI) {
				if err := os.MkdirAll(c.paths.Dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(c.paths.CycleRequestPath, []byte("cycle\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantRequest: map[string]any{"mode": "", "requested_at": nil, "held_by_pause": false},
			want:        func(*orchCLI) map[string]any { return nil },
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
			if tc.arrange != nil {
				tc.arrange(t, c)
			}

			code, env := runVerb(t, c.statusCmd())
			if code != 0 {
				t.Fatalf("exit = %d; env %v", code, env)
			}
			if got, present := env["cycle_request"]; !present || !reflect.DeepEqual(got, tc.wantRequest) {
				t.Errorf("status[cycle_request] = %v (present %v); want %v", got, present, tc.wantRequest)
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
			if _, present := env["held_by_pause"]; present {
				t.Errorf("envelope = %v; want no held_by_pause key without a watcher", env)
			}
		})

		t.Run(tc.name+" with a live watcher under a hub pause", func(t *testing.T) {
			t.Parallel()

			c := newTestCLI(t, &fakeStrands{})
			if err := os.MkdirAll(c.paths.Dir, 0o755); err != nil {
				t.Fatal(err)
			}
			held, acquired, err := lock.TryAcquireWriteLock(c.paths.WatchLockPath)
			if err != nil || !acquired {
				t.Fatalf("hold the watch lock: acquired %v, err %v", acquired, err)
			}
			defer held.Release()
			pauseHubAt(t, c)

			code, env := runVerb(t, tc.verb(c))
			if code != 0 {
				t.Fatalf("exit = %d; env %v", code, env)
			}
			if env["held_by_pause"] != true || env["note"] != "the request waits for the hub pause to be resumed" {
				t.Errorf("envelope = %v; want held_by_pause true and the note", env)
			}
			if req, pending, err := orchengine.CycleRequested(c.paths); err != nil || !pending || req.Mode != tc.want {
				t.Errorf("CycleRequested = %+v, %v, %v; want the request still pending in mode %q", req, pending, err, tc.want)
			}
		})

		t.Run(tc.name+" without a watcher under a hub pause carries no held_by_pause", func(t *testing.T) {
			t.Parallel()

			c := newTestCLI(t, &fakeStrands{})
			pauseHubAt(t, c)

			code, env := runVerb(t, tc.verb(c))
			if code != 0 {
				t.Fatalf("exit = %d; env %v", code, env)
			}
			if _, present := env["held_by_pause"]; present {
				t.Errorf("envelope = %v; want no held_by_pause key without a watcher", env)
			}
		})
	}
}

// TestWithdraw pins the withdraw verb: a pending request of either mode is removed, and none pending answers the persisted phase and, while a cycle runs, its trigger.
func TestWithdraw(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// request is the mode of the request written first; empty writes none.
		request string
		state   orchengine.State
		// want maps an envelope key to its value; wantAbsent lists keys the envelope must not carry.
		want       map[string]any
		wantAbsent []string
	}{
		{
			name: "APendingClearRequestIsRemoved", request: orchengine.CycleClear,
			state: orchengine.State{Strand: "g1", Phase: orchengine.PhaseIdle},
			want:  map[string]any{"withdrawn": true}, wantAbsent: []string{"phase"},
		},
		{
			name: "APendingCompactRequestIsRemoved", request: orchengine.CycleCompact,
			state: orchengine.State{Strand: "g1", Phase: orchengine.PhaseIdle},
			want:  map[string]any{"withdrawn": true}, wantAbsent: []string{"phase"},
		},
		{
			name:  "NoRequestInIdleAnswersNoTriggerEvenWithARecordedOne",
			state: orchengine.State{Strand: "g1", Phase: orchengine.PhaseIdle, CycleTrigger: orchengine.TriggerRequested},
			want:  map[string]any{"withdrawn": false, "phase": "idle"}, wantAbsent: []string{"cycle_trigger"},
		},
		{
			name:  "NoRequestInResumingAnswersNoTriggerEvenWithARecordedOne",
			state: orchengine.State{Strand: "g1", Phase: orchengine.PhaseResuming, CycleTrigger: orchengine.TriggerRequested},
			want:  map[string]any{"withdrawn": false, "phase": "resuming"}, wantAbsent: []string{"cycle_trigger"},
		},
		{
			name:  "NoRequestInHandoffRequestedAnswersTheRunningCyclesTrigger",
			state: orchengine.State{Strand: "g1", Phase: orchengine.PhaseHandoffRequested, CycleTrigger: orchengine.TriggerRequested},
			want:  map[string]any{"withdrawn": false, "phase": "handoff-requested", "cycle_trigger": "requested"},
		},
		{
			name: "ARequestWrittenDuringARunningCycleIsRemoved", request: orchengine.CycleClear,
			state: orchengine.State{Strand: "g1", Phase: orchengine.PhaseClearing, CycleTrigger: orchengine.TriggerRequested},
			want:  map[string]any{"withdrawn": true}, wantAbsent: []string{"phase"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := newTestCLI(t, &fakeStrands{})
			if err := orchengine.SaveState(c.paths, tt.state); err != nil {
				t.Fatal(err)
			}
			if tt.request != "" {
				requestAt(t, c, tt.request)
			}

			code, env := runVerb(t, c.withdrawCmd())
			if code != 0 {
				t.Fatalf("exit = %d; env %v", code, env)
			}
			for key, want := range tt.want {
				if got, present := env[key]; !present || got != want {
					t.Errorf("withdraw[%q] = %v (present %v); want %v", key, got, present, want)
				}
			}
			for _, key := range tt.wantAbsent {
				if _, present := env[key]; present {
					t.Errorf("withdraw envelope = %v; want no %q key", env, key)
				}
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
		{"leaves the runs strand beside the orch", []reedengine.StrandStatus{{GUID: "g1", Name: "orch"}, {GUID: "g2", Name: "runs"}}, "g1", true, "g1", []string{"g1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeStrands{strands: tc.strands}
			c := newTestCLI(t, fake)
			// The removal runs under start.lock, so a try for it fails there.
			fake.onRemove = func() {
				if l, acquired, err := lock.TryAcquireWriteLock(c.paths.StartLockPath); err != nil || acquired {
					if acquired {
						_ = l.Release()
					}
					t.Errorf("start.lock free during the removal: acquired %v, err %v", acquired, err)
				}
			}
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
			l, acquired, err := lock.TryAcquireWriteLock(c.paths.StartLockPath)
			if err != nil || !acquired {
				t.Fatalf("start.lock after stop: acquired %v, err %v; want it released", acquired, err)
			}
			_ = l.Release()
		})
	}

	t.Run("a held start.lock refuses after the wait and removes nothing", func(t *testing.T) {
		t.Parallel()

		fake := &fakeStrands{strands: []reedengine.StrandStatus{{GUID: "g1", Name: "orch"}}}
		c := newTestCLI(t, fake)
		if err := orchengine.SaveState(c.paths, orchengine.State{Strand: "g1", Phase: orchengine.PhaseIdle}); err != nil {
			t.Fatal(err)
		}
		held, acquired, err := lock.TryAcquireWriteLock(c.paths.StartLockPath)
		if err != nil || !acquired {
			t.Fatalf("hold start.lock: acquired %v, err %v", acquired, err)
		}
		defer held.Release()

		code, env := runVerb(t, c.stopCmd())
		msg, _ := env["error"].(string)
		if code == 0 || !strings.Contains(msg, "start.lock") || !strings.Contains(msg, "lyx orch stop") {
			t.Errorf("stop = exit %d, %v; want a refusal naming start.lock and the way forward", code, env)
		}
		if len(fake.removed) != 0 {
			t.Errorf("removed = %v; want nothing removed", fake.removed)
		}
	})
}

// TestStatus_RestartsADeadWatcher pins that status spawns a watcher for a live strand whose watcher is dead, and only then.
// It is not parallel: logcapture.CaptureVerbose swaps the process-global logger to read the Info line.
func TestStatus_RestartsADeadWatcher(t *testing.T) {
	liveOrch := []reedengine.StrandStatus{{GUID: "g1", Name: "orch", Live: true}}
	recorded := orchengine.State{Strand: "g1", Phase: orchengine.PhaseIdle, WatcherExit: "strand gone before the reboot"}
	tests := []struct {
		name    string
		strands []reedengine.StrandStatus
		// after, when non-nil, is the strand list the second Strands call answers.
		after []reedengine.StrandStatus
		// state is the saved state; nil leaves none on disk.
		state *orchengine.State
		// holdStart and holdWatch have the test hold start.lock and watch.lock.
		holdStart, holdWatch bool
		spawnErr             error
		wantSpawns           int
		wantRestarted        bool
		wantHint             bool
		wantError            string
	}{
		{name: "ALiveStrandWithNoWatcherSpawnsOnce", strands: liveOrch, state: &recorded, wantSpawns: 1, wantRestarted: true},
		{name: "AHeldStartLockSpawnsNothingAndHints", strands: liveOrch, state: &recorded, holdStart: true, wantHint: true},
		{name: "ALiveWatcherSpawnsNothing", strands: liveOrch, state: &recorded, holdWatch: true},
		{name: "AStrandThatIsNotLiveSpawnsNothing", strands: []reedengine.StrandStatus{{GUID: "g1", Name: "orch", Live: false}}, state: &recorded},
		{name: "NoRecordedStrandSpawnsNothing", strands: liveOrch},
		{name: "AStrandRemovedBeforeTheLockSpawnsNothing", strands: liveOrch, after: []reedengine.StrandStatus{}, state: &recorded},
		{name: "AFailingSpawnIsTheErrorEnvelopeNamingStart", strands: liveOrch, state: &recorded, spawnErr: errors.New("no binary"), wantSpawns: 1, wantError: "lyx orch start"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := logcapture.CaptureVerbose(t)
			c := newTestCLI(t, &fakeStrands{strands: tt.strands, after: tt.after})
			spawns := 0
			c.spawnWatcher = func() error {
				spawns++
				return tt.spawnErr
			}
			if tt.state != nil {
				if err := orchengine.SaveState(c.paths, *tt.state); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.MkdirAll(c.paths.Dir, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, hold := range []struct {
				held bool
				path string
			}{{tt.holdStart, c.paths.StartLockPath}, {tt.holdWatch, c.paths.WatchLockPath}} {
				if !hold.held {
					continue
				}
				l, acquired, err := lock.TryAcquireWriteLock(hold.path)
				if err != nil || !acquired {
					t.Fatalf("hold %s: acquired %v, err %v", hold.path, acquired, err)
				}
				defer l.Release()
			}

			code, env := runVerb(t, c.statusCmd())
			if spawns != tt.wantSpawns {
				t.Errorf("watcher spawns = %d; want %d", spawns, tt.wantSpawns)
			}
			if tt.wantError != "" {
				if msg, _ := env["error"].(string); code == 0 || !strings.Contains(msg, tt.wantError) {
					t.Errorf("status = exit %d, %v; want the error envelope naming %q", code, env, tt.wantError)
				}
				return
			}
			if code != 0 || env["watcher_restarted"] != tt.wantRestarted {
				t.Fatalf("status = exit %d, %v; want watcher_restarted %v", code, env, tt.wantRestarted)
			}
			if _, hinted := env["hint"]; hinted != tt.wantHint {
				t.Errorf("status hint present = %v; want %v; env %v", hinted, tt.wantHint, env)
			}
			if logged := strings.Contains(logs.String(), "orch: restarted a dead watcher"); logged != tt.wantRestarted {
				t.Errorf("restart line logged = %v; want %v; logs: %s", logged, tt.wantRestarted, logs.String())
			}
			if tt.wantRestarted && !strings.Contains(logs.String(), recorded.WatcherExit) {
				t.Errorf("logs do not carry the recorded exit %q: %s", recorded.WatcherExit, logs.String())
			}
		})
	}

	t.Run("StartSpawnsAWatcherForALiveStrandAndLogsTheRestart", func(t *testing.T) {
		logs := logcapture.CaptureVerbose(t)
		h := newStartHarness(t, reedengine.StrandStatus{GUID: "g1", Name: "orch", Live: true})
		if err := orchengine.SaveState(h.cli.paths, orchengine.State{Strand: "g1", Phase: orchengine.PhaseIdle, WatcherExit: recorded.WatcherExit}); err != nil {
			t.Fatal(err)
		}
		code, env := h.run(t)
		if code != 0 || env["action"] != actionSpawnedWatcher || h.spawns != 1 {
			t.Fatalf("start = exit %d, %v, spawns %d; want one spawned watcher", code, env, h.spawns)
		}
		if !strings.Contains(logs.String(), "orch: restarted a dead watcher") || !strings.Contains(logs.String(), recorded.WatcherExit) {
			t.Errorf("logs do not carry the restart line with the recorded exit: %s", logs.String())
		}
	})
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

func TestResumeContext_PrintsHookJSONPerPhaseAndFailsWithoutMark(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state orchengine.State
		// breakStencil removes the reload stencil, so a render of the idle pointer fails.
		breakStencil bool
		// fakeEngine swaps the receiver's engine set for a fake engine without the resume-context capability.
		fakeEngine bool
		want       func(c *orchCLI) string
		wantErr    bool
		// wantErrContains, when set, is a substring the error envelope's error text must carry.
		wantErrContains string
	}{
		{name: "resuming prints the pending pointer", state: orchengine.State{Strand: "g1", Phase: orchengine.PhaseResuming, PendingResume: "read the note"},
			want: func(*orchCLI) string { return "read the note" }},
		{name: "compacting prints the resume prompt", state: orchengine.State{Strand: "g1", Phase: orchengine.PhaseCompacting, LastHandoff: "/h/note.md"},
			want: func(c *orchCLI) string {
				text, err := orchengine.RenderResumePrompt(c.stencilsDir, c.paths.RolePath, "/h/note.md")
				if err != nil {
					t.Fatal(err)
				}
				return text
			}},
		{name: "idle prints the reload prompt", state: orchengine.State{Strand: "g1", Phase: orchengine.PhaseIdle},
			want: func(c *orchCLI) string {
				text, err := orchengine.RenderReloadPrompt(c.stencilsDir, c.paths.RolePath)
				if err != nil {
					t.Fatal(err)
				}
				return text
			}},
		{name: "a missing stencil is a JSON error and writes no mark", state: orchengine.State{Strand: "g1", Phase: orchengine.PhaseIdle}, breakStencil: true, wantErr: true},
		{name: "an engine without the capability is a JSON error and writes no mark", state: orchengine.State{Strand: "g1", Phase: orchengine.PhaseResuming, PendingResume: "read the note"},
			fakeEngine: true, wantErr: true, wantErrContains: "fake"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := newTestCLI(t, &fakeStrands{})
			c.paths.ResumeMarkPath = filepath.Join(c.paths.Dir, "resume-mark")
			c.paths.RolePath = filepath.Join(c.paths.Dir, "role.md")
			c.stencilsDir = stencilkit.Seed(t)
			if tt.fakeEngine {
				c.engines = shuttlefake.EngineSet(&shuttlefake.Engine{})
			}
			if err := orchengine.SaveState(c.paths, tt.state); err != nil {
				t.Fatal(err)
			}
			if tt.breakStencil {
				if err := os.Remove(stencilstore.Path(c.stencilsDir, "orch-template-reload")); err != nil {
					t.Fatal(err)
				}
			}

			// The verb renders the operator index of its root, so it runs under a root that has an operator command.
			root := &cobra.Command{Use: "lyx"}
			root.AddCommand(c.resumeContextCmd(), &cobra.Command{
				Use:         "probe",
				Short:       "an operator command for the index",
				Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
				Run:         func(*cobra.Command, []string) {},
			})
			var out bytes.Buffer
			code := clihelp.Execute(root, &out, []string{resumeContextVerb})
			var env map[string]any
			if err := json.Unmarshal(out.Bytes(), &env); err != nil {
				t.Fatalf("output %q is not one JSON object: %v", out.String(), err)
			}

			_, markFound, err := orchengine.ReadResumeMark(c.paths)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantErr {
				if code != 1 || env["ok"] != false || markFound {
					t.Errorf("exit = %d, env = %v, mark found = %v; want a JSON error and no mark", code, env, markFound)
				}
				if message, _ := env["error"].(string); !strings.Contains(message, tt.wantErrContains) {
					t.Errorf("error = %q; want it to contain %q", message, tt.wantErrContains)
				}
				return
			}
			hookOutput, _ := env["hookSpecificOutput"].(map[string]any)
			if code != 0 || hookOutput["hookEventName"] != "SessionStart" || hookOutput["additionalContext"] != tt.want(c) || !markFound {
				t.Errorf("exit = %d, env = %v, mark found = %v; want the SessionStart context %q and a mark", code, env, markFound, tt.want(c))
			}
			if tt.state.Phase != orchengine.PhaseResuming {
				wantIndex := clihelp.RenderIndex(root, clihelp.AudienceOperator)
				role, err := os.ReadFile(c.paths.RolePath)
				if err != nil || !strings.Contains(string(role), wantIndex) || strings.Contains(string(role), fakeRunnerIndex) {
					t.Errorf("role file = %q, %v; want it to carry the root's own index %q, not the runner's", role, err, wantIndex)
				}
			}
		})
	}
}

func TestCleanup_Envelope(t *testing.T) {
	t.Parallel()

	for _, apply := range []bool{false, true} {
		name := "dry run"
		args := []string{}
		if apply {
			name, args = "apply", []string{"--apply"}
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := newTestCLI(t, &fakeStrands{})
			c.paths.HandoffsDir = filepath.Join(c.paths.Dir, "handoffs")
			if err := orchengine.SaveState(c.paths, orchengine.State{Phase: orchengine.PhaseIdle, LastHandoff: "keep.md"}); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(c.paths.HandoffsDir, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, file := range []string{"keep.md", "old.md"} {
				if err := os.WriteFile(filepath.Join(c.paths.HandoffsDir, file), []byte("note"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			// The retired notice queue directory and notice log, which nothing writes any more.
			c.paths.NoticesDir = filepath.Join(c.paths.Dir, "notices")
			c.paths.RetiredNoticeLogPath = filepath.Join(filepath.Dir(c.paths.Dir), "notices.log")
			if err := os.MkdirAll(c.paths.NoticesDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(c.paths.NoticesDir, "a.notice"), []byte("line"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(c.paths.RetiredNoticeLogPath, []byte("log"), 0o644); err != nil {
				t.Fatal(err)
			}

			var out bytes.Buffer
			code := clihelp.Execute(c.cleanupCmd(), &out, args)
			var env map[string]any
			if err := json.Unmarshal(out.Bytes(), &env); err != nil {
				t.Fatalf("output %q is not one JSON object: %v", out.String(), err)
			}

			want := map[string]any{"ok": true, "applied": apply, "kept": "keep.md", "removed": []any{"old.md"}, "reason": "", "retired": []any{"notices", "notices.log"}}
			if code != 0 || !reflect.DeepEqual(env, want) {
				t.Errorf("exit = %d, envelope = %v; want %v", code, env, want)
			}
			for _, path := range []string{filepath.Join(c.paths.HandoffsDir, "old.md"), c.paths.NoticesDir, c.paths.RetiredNoticeLogPath} {
				_, err := os.Stat(path)
				if gone := errors.Is(err, os.ErrNotExist); gone != apply {
					t.Errorf("%s removed = %v, want %v", filepath.Base(path), gone, apply)
				}
			}
		})
	}
}
