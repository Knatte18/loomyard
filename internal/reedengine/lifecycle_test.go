// lifecycle_test.go drives the lifecycle ops' planning seams — the parts that decide what would run
// without needing a live tmux server: planUpLaunches (Up never launches anything) and
// planResumeLaunches across the three states the discussion calls out (server dead, server-up/
// CLI-restarted, a single strand's pane died).
// Any real-tmux round trip (ensureServerAndSessionLocked, and Up/Resume/Down/Status themselves) is
// out of hermetic reach and is not exercised here.

package reedengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

func guids(strands []Strand) []string {
	out := make([]string, len(strands))
	for i, s := range strands {
		out[i] = s.GUID
	}
	return out
}

// TestUp_BadHeaderTemplateFailsBeforeAnyTmuxContact pins that header validation runs before any
// tmux contact (validates validation ORDER, not just existence).
func TestUp_BadHeaderTemplateFailsBeforeAnyTmuxContact(t *testing.T) {
	e := newTestEngine(t)
	e.cfg.DebugLog = "0"
	e.cfg.Mouse = "off"
	e.cfg.StatusLine.Template = "{{.bogus}}"

	_, err := e.Up()
	if err == nil {
		t.Fatal("Up() with a bad header template = nil error, want the eager validation error")
	}
	if !strings.Contains(err.Error(), "unfilled top-level marker") {
		t.Errorf("Up() error = %q, want the stencil unfilled-marker error — any other error (e.g. the nonexistent tmux binary's) means validation ran after tmux contact", err)
	}
}

// TestUp_InvalidWatchdogFailsBeforeAnyTmuxContact pins the boot-path watchdog validation this batch
// adds: an invalid Config.Watchdog fails ensureServerAndSessionLocked (and therefore Up) with an
// error naming the offending value, before any tmux round trip — following
// TestUp_BadHeaderTemplateFailsBeforeAnyTmuxContact's shape, the file's existing sibling validation
// test for debug_log/mouse/header.
func TestUp_InvalidWatchdogFailsBeforeAnyTmuxContact(t *testing.T) {
	invalid := []string{"", "1", "yes"}
	for _, watchdog := range invalid {
		t.Run("Invalid_"+watchdog, func(t *testing.T) {
			e := newTestEngine(t)
			e.cfg.DebugLog = "0"
			e.cfg.Mouse = "off"
			e.cfg.Watchdog = watchdog

			var calls [][]string
			e.tmux.execHook = func(capture bool, args ...string) (string, error) {
				calls = append(calls, append([]string{}, args...))
				return "", nil
			}

			_, err := e.Up()
			if err == nil {
				t.Fatal("Up() with an invalid watchdog value = nil error, want the eager validation error")
			}
			if !strings.Contains(err.Error(), "invalid watchdog value") {
				t.Errorf("Up() error = %q, want it to name the invalid watchdog value", err)
			}
			if len(calls) != 0 {
				t.Errorf("Up() issued %d tmux calls before failing, want zero: %v", len(calls), calls)
			}
		})
	}
}

// TestUp_ValidWatchdogValuesPassTheBootCheck pins the negative case: "on" and "off" do not trip the
// watchdog validation (the fixture's nonexistent tmux binary is expected to fail Up() past this
// point, so the assertion is only that the error is NOT the watchdog validation error).
func TestUp_ValidWatchdogValuesPassTheBootCheck(t *testing.T) {
	for _, watchdog := range []string{"on", "off"} {
		t.Run(watchdog, func(t *testing.T) {
			e := newTestEngine(t)
			e.cfg.DebugLog = "0"
			e.cfg.Mouse = "off"
			e.cfg.Watchdog = watchdog

			_, err := e.Up()
			if err != nil && strings.Contains(err.Error(), "invalid watchdog value") {
				t.Errorf("Up() error = %q, want the watchdog check to pass for %q", err, watchdog)
			}
		})
	}
}

// TestServerBootEnv_ExcludesTraceID pins that LYX_TRACE_ID is stripped before the tmux server
// inherits the boot env (long-lived singleton).
func TestServerBootEnv_ExcludesTraceID(t *testing.T) {
	t.Setenv("LYX_TRACE_ID", "somevalue")

	clean, _ := CleanClaudeEnv(os.Environ())
	sawBeforeStrip := false
	for _, entry := range clean {
		if strings.HasPrefix(entry, "LYX_TRACE_ID=") {
			sawBeforeStrip = true
			break
		}
	}
	if !sawBeforeStrip {
		// CleanClaudeEnv is not expected to strip this on its own — if it's
		// already gone here, the fixture assumption (LYX_TRACE_ID surviving
		// CleanClaudeEnv) is broken and the assertion below proves nothing.
		t.Fatalf("CleanClaudeEnv(os.Environ()) does not contain LYX_TRACE_ID; test fixture assumption broken")
	}

	got := stripTraceID(clean)
	for _, entry := range got {
		if strings.HasPrefix(entry, "LYX_TRACE_ID=") {
			t.Errorf("stripTraceID(clean) = %v, want no LYX_TRACE_ID entry", got)
		}
	}
}

func TestPlanUpLaunches_NeverLaunchesAnyStrand(t *testing.T) {
	tables := [][]Strand{
		nil,
		{{GUID: "a", Display: render.Display{Anchor: render.AnchorBelowParent}}},
		{
			{GUID: "a", Display: render.Display{Anchor: render.AnchorHidden}},
			{GUID: "b", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
		},
	}
	for _, strands := range tables {
		if got := planUpLaunches(strands); got != nil {
			t.Errorf("planUpLaunches(%+v) = %v, want nil (Up never launches a strand command)", strands, got)
		}
	}
}

func TestNoSessionMessage_StrandCountVariants(t *testing.T) {
	tests := []struct {
		name          string
		strandCount   int
		stateReadable bool
		want          string
	}{
		{
			// Zero strands persisted (or no reed.json at all): nothing for
			// resume to rebuild, so today's bare "up" pointer is unchanged.
			name:          "ZeroStrands_BareUpPointer",
			strandCount:   0,
			stateReadable: true,
			want:          `no reed session; run "lyx reed up"`,
		},
		{
			name:          "OneStrand_ResumePointer",
			strandCount:   1,
			stateReadable: true,
			want:          `no reed session (1 strands persisted); run "lyx reed resume" to rebuild, or "lyx reed up" for a bare substrate`,
		},
		{
			name:          "ThreeStrands_ResumePointer",
			strandCount:   3,
			stateReadable: true,
			want:          `no reed session (3 strands persisted); run "lyx reed resume" to rebuild, or "lyx reed up" for a bare substrate`,
		},
		{
			// R5 review finding R5-F8: an unreadable reed.json yields a strand count of zero, and
			// reporting that as "nothing is persisted" sends the operator to an `up` that then
			// fails with the corrupt-file error. Say reed could not read it instead.
			name:          "UnreadableState_SaysSoInsteadOfClaimingZeroStrands",
			strandCount:   0,
			stateReadable: false,
			want:          `no reed session, and reed's persisted state could not be read; run "lyx reed down" to clear it, or "lyx reed up" for the full diagnosis`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := noSessionMessage(tt.strandCount, tt.stateReadable); got != tt.want {
				t.Errorf("noSessionMessage(%d, %v) = %q, want %q", tt.strandCount, tt.stateReadable, got, tt.want)
			}
		})
	}
}

// TestPruneServerLogsLocked_ServerAndClientPrefixesPrunedIndependently pins the fix for a real
// defect found live-driving debug_log against native tmux: a debug-armed boot's -v/-vv global flag
// makes tmux log BOTH the forked server (tmux-server-<pid>.log, documented and already pruned) AND
// the client half of that same invocation (tmux-client-<pid>.log, observed live — never surfaced
// before since the original debug-logging batch was developed/reviewed against psmux on Windows,
// not native tmux).
// Without pruning the client-prefixed files too, they accumulate unbounded across repeated
// debug-armed boots/crashes while the server-prefixed files stay capped — this test seeds both
// shapes plus an unrelated file the pruner must never touch, and asserts each prefix is pruned to
// keep independently.
func TestPruneServerLogsLocked_ServerAndClientPrefixesPrunedIndependently(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	write := func(name string, age time.Duration) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		mtime := now.Add(-age)
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatalf("chtimes %s: %v", name, err)
		}
	}

	// Three server logs (oldest to newest) and three client logs (oldest to
	// newest), interleaved in age so a prefix-blind prune would not
	// accidentally produce the same result as a correct per-prefix prune.
	write("tmux-server-1.log", 6*time.Minute)
	write("tmux-client-1.log", 5*time.Minute)
	write("tmux-server-2.log", 4*time.Minute)
	write("tmux-client-2.log", 3*time.Minute)
	write("tmux-server-3.log", 2*time.Minute)
	write("tmux-client-3.log", time.Minute)
	// An unrelated file must survive untouched — the pruner only matches its
	// given prefix, never a bare glob over every file in the dir.
	write("unrelated.log", 10*time.Minute)

	if err := pruneServerLogsLocked(dir, serverLogNamePrefix, 2); err != nil {
		t.Fatalf("prune server logs: %v", err)
	}
	if err := pruneServerLogsLocked(dir, clientLogNamePrefix, 2); err != nil {
		t.Fatalf("prune client logs: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var remaining []string
	for _, e := range entries {
		remaining = append(remaining, e.Name())
	}

	wantPresent := []string{"tmux-server-2.log", "tmux-server-3.log", "tmux-client-2.log", "tmux-client-3.log", "unrelated.log"}
	wantAbsent := []string{"tmux-server-1.log", "tmux-client-1.log"}
	for _, name := range wantPresent {
		found := false
		for _, r := range remaining {
			if r == name {
				found = true
			}
		}
		if !found {
			t.Errorf("expected %s to survive pruning; remaining = %v", name, remaining)
		}
	}
	for _, name := range wantAbsent {
		for _, r := range remaining {
			if r == name {
				t.Errorf("expected %s to be pruned; remaining = %v", name, remaining)
			}
		}
	}
	if len(remaining) != len(wantPresent) {
		t.Errorf("remaining = %v; want exactly %v", remaining, wantPresent)
	}
}

func TestPlanResumeLaunches_ThreeLifecycleStates(t *testing.T) {
	notLive := Strand{GUID: "a", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}}
	stillLive := Strand{GUID: "b", PaneID: "%2", Display: render.Display{Anchor: render.AnchorBelowParent}}
	hidden := Strand{GUID: "c", Display: render.Display{Anchor: render.AnchorHidden}}

	tests := []struct {
		name    string
		strands []Strand
		liveIDs map[string]bool
		want    []string
	}{
		{
			// Server dead (reboot): list-panes reports nothing live at all,
			// so every not-hidden strand — even ones with a stale PaneID —
			// must be relaunched.
			name:    "ServerDead_EveryNonHiddenStrandRelaunched",
			strands: []Strand{notLive, stillLive, hidden},
			liveIDs: map[string]bool{},
			want:    []string{"a", "b"},
		},
		{
			// Server up, CLI restarted (the normal one-shot case): every
			// strand's pane is still alive, so nothing needs relaunching.
			name:    "ServerUpCLIRestarted_NothingRelaunched",
			strands: []Strand{notLive, stillLive, hidden},
			liveIDs: map[string]bool{"%1": true, "%2": true},
			want:    nil,
		},
		{
			// A single strand's pane died: only that strand's pane id is
			// missing from liveIDs, so only it gets relaunched;
			// already-live strands are left untouched.
			name:    "SingleStrandPaneDied_OnlyThatStrandRelaunched",
			strands: []Strand{notLive, stillLive, hidden},
			liveIDs: map[string]bool{"%2": true},
			want:    []string{"a"},
		},
		{
			name:    "HiddenStrandNeverRelaunched",
			strands: []Strand{hidden},
			liveIDs: map[string]bool{},
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := guids(planResumeLaunches(tt.strands, tt.liveIDs))
			if !equalStringSlices(got, tt.want) {
				t.Errorf("planResumeLaunches() guids = %v, want %v", got, tt.want)
			}
		})
	}
}
