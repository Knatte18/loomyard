// lifecycle_test.go drives the lifecycle ops' planning seams — the parts that decide what would run
// without needing a live tmux server: planUpLaunches (Up never launches anything) and
// planResumeLaunches across the three states the discussion calls out (server dead, server-up/
// CLI-restarted, a single strand's pane died).
// Any real-tmux round trip (ensureServerAndSessionLocked, and Up/Resume/Down/Status themselves) is
// out of hermetic reach and is not exercised here.

package reedengine

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shell"
)

func guids(strands []Strand) []string {
	out := make([]string, len(strands))
	for i, s := range strands {
		out[i] = s.GUID
	}
	return out
}

// TestUp_BootValidation pins the eager boot validation of Up.
// An over-long socket path, an unresolvable shell, a segment color outside the palette, an invalid watchdog value, an invalid window_split or a negative window_share_max each fail with an error naming it before any tmux round trip (validation ORDER, not just existence),
// while "on" and "off" do not trip the watchdog check (the fixture's nonexistent tmux binary is expected to fail Up() past this point,
// so the assertion is only that the error is NOT the watchdog validation error).
// Each refusal is asserted at three boot entries: Up and Resume, whose op-lock brackets refuse before their zoom read, and ensureServerAndSessionLocked, which every other boot path reaches without that bracket.
func TestUp_BootValidation(t *testing.T) {
	tests := []struct {
		name      string
		configure func(cfg *Config)
		wantErr   string // the validation error Up must fail with before any tmux contact; empty when the value must pass the check
		notErr    string // an error text Up must not fail with
		setup     func(t *testing.T)
	}{
		{
			name: "SocketPathTooLong",
			setup: func(t *testing.T) {
				if runtime.GOOS == "windows" {
					t.Skip("psmux keeps no socket file")
				}
				t.Setenv("TMUX_TMPDIR", filepath.Join(t.TempDir(), strings.Repeat("a", unixSocketPathLimit("linux"))))
			},
			configure: func(cfg *Config) {},
			wantErr:   "TMUX_TMPDIR",
		},
		{
			name: "ShellUnresolvable",
			setup: func(t *testing.T) {
				if runtime.GOOS == "windows" {
					t.Skip("Windows keeps its current shell handling")
				}
			},
			configure: func(cfg *Config) { cfg.Shell = "lyx-no-such-shell-for-this-test" },
			wantErr:   "key shell",
		},
		{
			name: "TmuxConfRelative",
			setup: func(t *testing.T) {
				if runtime.GOOS == "windows" {
					t.Skip("Windows parses tmux_conf and validates nothing")
				}
			},
			configure: func(cfg *Config) {
				cfg.Shell = "sh"
				cfg.TmuxConf = "tmux.conf"
			},
			wantErr: "tmux_conf",
		},
		{
			name: "TmuxConfUnvalidatedOnWindows",
			setup: func(t *testing.T) {
				if runtime.GOOS != "windows" {
					t.Skip("only Windows leaves tmux_conf unvalidated")
				}
			},
			configure: func(cfg *Config) { cfg.TmuxConf = "tmux.conf" },
			notErr:    "tmux_conf",
		},
		{
			name:      "SegmentColorOutsidePalette",
			configure: func(cfg *Config) { cfg.SegmentColors = map[string]string{"review": "crimson"} },
			wantErr:   `segment_colors.review: color "crimson" is not in the palette`,
		},
		{name: "InvalidWatchdog_Empty", configure: func(cfg *Config) { cfg.Watchdog = "" }, wantErr: "invalid watchdog value"},
		{name: "InvalidWatchdog_1", configure: func(cfg *Config) { cfg.Watchdog = "1" }, wantErr: "invalid watchdog value"},
		{name: "InvalidWatchdog_Yes", configure: func(cfg *Config) { cfg.Watchdog = "yes" }, wantErr: "invalid watchdog value"},
		{name: "ValidWatchdog_On", configure: func(cfg *Config) { cfg.Watchdog = "on" }, notErr: "invalid watchdog value"},
		{name: "ValidWatchdog_Off", configure: func(cfg *Config) { cfg.Watchdog = "off" }, notErr: "invalid watchdog value"},
		{name: "WindowSplitInvalid", configure: func(cfg *Config) { cfg.WindowSplit = "diagonal" }, wantErr: "window_split"},
		{name: "WindowShareMaxNegative", configure: func(cfg *Config) { cfg.WindowShareMax = -1 }, wantErr: "window_share_max"},
		{name: "WindowSplitHeightPasses", configure: func(cfg *Config) { cfg.WindowSplit = "height" }, notErr: "window_split"},
		{name: "WindowShareMaxZeroPasses", configure: func(cfg *Config) { cfg.WindowShareMax = 0 }, notErr: "window_share_max"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t)
			}
			e := newTestEngine(t)
			e.cfg.DebugLog = "0"
			e.cfg.Mouse = "off"
			tt.configure(&e.cfg)
			var fake *fakeTmux
			if tt.wantErr != "" {
				fake = installFakeTmux(t, e)
			}

			_, err := e.Up()

			if tt.wantErr != "" {
				_, _, ensureErr := e.ensureServerAndSessionLocked()
				_, resumeErr := e.Resume()
				for _, entry := range []struct {
					name string
					err  error
				}{{"Up()", err}, {"ensureServerAndSessionLocked()", ensureErr}, {"Resume()", resumeErr}} {
					if entry.err == nil {
						t.Fatalf("%s = nil error, want the eager validation error containing %q", entry.name, tt.wantErr)
					}
					if !strings.Contains(entry.err.Error(), tt.wantErr) {
						t.Errorf("%s error = %q, want it to contain %q; any other error means validation ran after tmux contact", entry.name, entry.err, tt.wantErr)
					}
				}
				if calls := fake.Calls(); len(calls) != 0 {
					t.Errorf("Up(), ensureServerAndSessionLocked() and Resume() issued %d tmux calls before failing, want zero: %v", len(calls), calls)
				}
				return
			}
			if err != nil && strings.Contains(err.Error(), tt.notErr) {
				t.Errorf("Up() error = %q, want the check to pass", err)
			}
		})
	}
}

// TestServerSpawnArgv pins the argv of the server-starting invocation: the debug flags first, `-f /dev/null` ahead of `-L` everywhere but Windows, then new-session with the configured shell, and off Windows the three chained session pins last.
func TestServerSpawnArgv(t *testing.T) {
	e := newTestEngine(t)
	session := []string{
		"-L", e.Socket(),
		"new-session", "-d", "-s", e.SessionName(),
		"-c", e.geom.PaneCwd,
		"-x", strconv.Itoa(e.cfg.Width),
		"-y", strconv.Itoa(e.cfg.Height),
		e.cfg.Shell,
	}
	// The chained pins of the new session: not destroyed when unattached, tagged as reed's, and following the run.
	posixTail := []string{
		";", "set-option", "-t", "=" + e.SessionName() + ":", "destroy-unattached", "off",
		";", "set-option", "-t", "=" + e.SessionName() + ":", "@lyx_reed_session", "on",
		";", "set-option", "-t", "=" + e.SessionName() + ":", "@lyx_follow", "on",
	}
	tests := []struct {
		name      string
		goos      string
		debugArgs []string
		want      []string
	}{
		{"LinuxWithoutDebugFlags", "linux", nil, slices.Concat([]string{"-f", os.DevNull}, session, posixTail)},
		{"LinuxWithDebugFlags", "linux", []string{"-vv"}, slices.Concat([]string{"-vv", "-f", os.DevNull}, session, posixTail)},
		{"WindowsHasNoConfigFlag", "windows", nil, session},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := e.serverSpawnArgv(tt.goos, tt.debugArgs); !slices.Equal(got, tt.want) {
				t.Errorf("serverSpawnArgv(%q, %v) = %v, want %v", tt.goos, tt.debugArgs, got, tt.want)
			}
		})
	}
}

// TestPinBootOptionsLocked_InstallsTheSignalEntry pins that a fresh boot's pins end in a zero-pin rebuild of the window-resized hook array when the watchdog is on, the clear and the signal entry alone,
// and that watchdog off leaves the one unset and installs no entry.
// Both steps also see the unseen-clear hook pinned once.
// Windows issues no hook call and cannot be faked, so both steps skip there.
func TestPinBootOptionsLocked_InstallsTheSignalEntry(t *testing.T) {
	bootPins := func(t *testing.T, watchdog string) (*Engine, *fakeTmux) {
		t.Helper()
		if runtime.GOOS == "windows" {
			t.Skip("the window-resized hook is never installed on Windows")
		}
		e := newTestEngine(t)
		e.cfg.Watchdog = watchdog
		fake := installFakeTmux(t, e)
		if err := e.pinBootOptionsLocked("off", ""); err != nil {
			t.Fatalf("pinBootOptionsLocked() = %v, want nil", err)
		}
		return e, fake
	}

	t.Run("WatchdogOn", func(t *testing.T) {
		e, fake := bootPins(t, "on")
		target := exactSessionWindowTarget(e.SessionName())

		var hooks [][]string
		lastOption, firstHook := -1, -1
		for i, call := range fake.Calls() {
			switch {
			case call[0] == "set-option":
				lastOption = i
			case call[0] == "set-hook" && containsArg(call, windowResizedHookName):
				hooks = append(hooks, call)
				if firstHook == -1 {
					firstHook = i
				}
			}
		}
		assertUnseenClearHookPinned(t, e, fake)
		want := [][]string{
			{"set-hook", "-u", "-w", "-t", target, windowResizedHookName},
			{"set-hook", "-w", "-t", target, windowResizedHookName, resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath())},
		}
		if !slices.EqualFunc(hooks, want, slices.Equal[[]string]) {
			t.Errorf("set-hook calls = %v, want the clear and the signal entry alone: %v", hooks, want)
		}
		if firstHook < lastOption {
			t.Errorf("first set-hook call at %d precedes the last set-option call at %d, want the install after the geometry pins", firstHook, lastOption)
		}
	})

	t.Run("WatchdogOff", func(t *testing.T) {
		e, fake := bootPins(t, "off")

		hooks := fake.SetHookCalls(windowResizedHookName)
		assertUnseenClearHookPinned(t, e, fake)
		want := [][]string{{"set-hook", "-u", "-t", exactSessionWindowTarget(e.SessionName()), windowResizedHookName}}
		if !slices.EqualFunc(hooks, want, slices.Equal[[]string]) {
			t.Errorf("set-hook calls = %v, want the one unset %v", hooks, want)
		}
	})
}

// TestStatus_ReportsSegmentColor pins that Status carries each strand's resolved segment color, and none for a strand recorded without a segment.
func TestStatus_ReportsSegmentColor(t *testing.T) {
	e := newTestEngine(t)
	fake := installFakeTmux(t, e)
	fake.answer("display-message", "$0|4321|1787000000", nil)
	fake.answer("list-sessions", "worktree\n", nil)
	fake.answer("list-panes", "%1 0 0 100 3 4322\n%2 0 3 100 20 4323\n", nil)
	st := &ReedState{
		SelvagePaneID:  "%1",
		PaneGeneration: PaneGeneration{SessionName: "worktree", TmuxSessionID: "$0", ServerPID: "4321", Created: "1787000000"},
		Strands: []Strand{
			{GUID: "colored", Name: "colored", PaneID: "%2", Segment: "review"},
			{GUID: "plain", Name: "plain"},
		},
	}
	if err := SaveState(e.stateDir(), st); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	result, err := e.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got := result.Strands[0].Color; got != segmentcolor.Orange {
		t.Errorf("Status color of the review strand = %q, want orange", got)
	}
	if got := result.Strands[1].Color; got != "" {
		t.Errorf("Status color of the strand without a segment = %q, want none", got)
	}
}

// TestStatus_ReportsOwnWindowStrandsFromTheSessionListing pins that Status reads an own-window strand's pane from the session-wide listing: alive is live, dead is present but not live, and one in no listing is neither.
// It also pins the pane pid: the listed pid for a live pane, 0 for a dead pane and for one in no listing.
func TestStatus_ReportsOwnWindowStrandsFromTheSessionListing(t *testing.T) {
	e := newTestEngine(t)
	fake := installFakeTmux(t, e)
	fake.answer("display-message", "$0|4321|1787000000", nil)
	fake.answer("list-sessions", "worktree\n", nil)
	fake.answer("list-panes", "%1 0 0 100 3 4322\n%2 0 3 100 20 4323\n", nil)
	fake.answerWindowPanes("@0 0 - %1 0 4322 t\n@0 0 - %2 0 4323 t\n@3 1 - %5 0 4325 t\n@3 1 - %6 1 4326 t\n", nil)
	st := &ReedState{
		SelvagePaneID:  "%1",
		PaneGeneration: PaneGeneration{SessionName: "worktree", TmuxSessionID: "$0", ServerPID: "4321", Created: "1787000000"},
		Strands: []Strand{
			{GUID: "w", Name: "w", PaneID: "%2"},
			{GUID: "alive", Name: "alive", PaneID: "%5", Display: render.Display{Anchor: render.AnchorOwnWindow}},
			{GUID: "dead", Name: "dead", PaneID: "%6", Display: render.Display{Anchor: render.AnchorOwnWindow}},
			{GUID: "gone", Name: "gone", PaneID: "%9", Display: render.Display{Anchor: render.AnchorOwnWindow}},
		},
	}
	if err := SaveState(e.stateDir(), st); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	result, err := e.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	type shape struct{ live, present, own bool }
	want := map[string]shape{
		"w":     {live: true, present: true},
		"alive": {live: true, present: true, own: true},
		"dead":  {present: true, own: true},
		"gone":  {own: true},
	}
	for _, s := range result.Strands {
		if got := (shape{s.Live, s.Present, s.OwnWindow}); got != want[s.GUID] {
			t.Errorf("Status of strand %q = %+v, want %+v", s.GUID, got, want[s.GUID])
		}
	}
	wantPanePID := map[string]int{"w": 4323, "alive": 4325, "dead": 0, "gone": 0}
	for _, s := range result.Strands {
		if s.PanePID != wantPanePID[s.GUID] {
			t.Errorf("Status PanePID of strand %q = %d, want %d", s.GUID, s.PanePID, wantPanePID[s.GUID])
		}
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

//testtiming:keep pins the pane env dropping exactly LYX_STRAND_NAME and LYX_PARENT and keeping a longer key sharing the prefix; its covering tests run this code without asserting it
func TestStripAgentNameEnv_DropsNameAndParentOnly(t *testing.T) {
	env := []string{"LYX_STRAND_NAME=ly:task:driver", "LYX_PARENT=ly:orch", "LYX_PARENTAL=keep", "PATH=/bin"}

	got := stripAgentNameEnv(env)

	want := []string{"LYX_PARENTAL=keep", "PATH=/bin"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("stripAgentNameEnv(%v) = %v, want %v", env, got, want)
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
	withDoneWhen := func(s Strand, paths ...string) Strand {
		s.DoneWhen = paths
		return s
	}
	existing := map[string]bool{"/out/one": true, "/out/two": true}
	exists := func(path string) bool { return existing[path] }

	tests := []struct {
		name       string
		strands    []Strand
		liveIDs    map[string]bool
		paused     bool
		wantLaunch []string
		wantDrop   []string
		wantHeld   []string
	}{
		{
			// A pause leaves the not-live strand down; the live and the hidden one are untouched.
			name:     "PausedPlanHoldsTheNotLiveStrandAndLaunchesNothing",
			strands:  []Strand{notLive, stillLive, hidden},
			liveIDs:  map[string]bool{"%2": true},
			paused:   true,
			wantHeld: []string{"a"},
		},
		{
			name:     "PausedPlanStillDropsAFinishedStrand",
			strands:  []Strand{withDoneWhen(notLive, "/out/one", "/out/two")},
			liveIDs:  map[string]bool{},
			paused:   true,
			wantDrop: []string{"a"},
		},
		{
			// Server dead (reboot): list-panes reports nothing live at all,
			// so every not-hidden strand — even ones with a stale PaneID —
			// must be relaunched.
			name:       "ServerDead_EveryNonHiddenStrandRelaunched",
			strands:    []Strand{notLive, stillLive, hidden},
			liveIDs:    map[string]bool{},
			wantLaunch: []string{"a", "b"},
		},
		{
			// Server up, CLI restarted (the normal one-shot case): every
			// strand's pane is still alive, so nothing needs relaunching.
			name:    "ServerUpCLIRestarted_NothingRelaunched",
			strands: []Strand{notLive, stillLive, hidden},
			liveIDs: map[string]bool{"%1": true, "%2": true},
		},
		{
			// A single strand's pane died: only that strand's pane id is
			// missing from liveIDs, so only it gets relaunched;
			// already-live strands are left untouched.
			name:       "SingleStrandPaneDied_OnlyThatStrandRelaunched",
			strands:    []Strand{notLive, stillLive, hidden},
			liveIDs:    map[string]bool{"%2": true},
			wantLaunch: []string{"a"},
		},
		{
			name:    "HiddenStrandNeverRelaunched",
			strands: []Strand{hidden},
			liveIDs: map[string]bool{},
		},
		{
			name:       "NoDoneWhenListRelaunches",
			strands:    []Strand{notLive},
			liveIDs:    map[string]bool{},
			wantLaunch: []string{"a"},
		},
		{
			name:       "PartlyMissingDoneWhenListRelaunches",
			strands:    []Strand{withDoneWhen(notLive, "/out/one", "/out/missing")},
			liveIDs:    map[string]bool{},
			wantLaunch: []string{"a"},
		},
		{
			name:     "FullyPresentDoneWhenListDrops",
			strands:  []Strand{withDoneWhen(notLive, "/out/one", "/out/two")},
			liveIDs:  map[string]bool{},
			wantDrop: []string{"a"},
		},
		{
			name:    "LiveStrandWithFullyPresentDoneWhenListIsUntouched",
			strands: []Strand{withDoneWhen(stillLive, "/out/one")},
			liveIDs: map[string]bool{"%2": true},
		},
		{
			name:     "HiddenStrandWithFullyPresentDoneWhenListDrops",
			strands:  []Strand{withDoneWhen(hidden, "/out/one")},
			liveIDs:  map[string]bool{},
			wantDrop: []string{"c"},
		},
		{
			name:    "HiddenStrandWithPartlyMissingDoneWhenListIsNeitherLaunchedNorDropped",
			strands: []Strand{withDoneWhen(hidden, "/out/one", "/out/missing")},
			liveIDs: map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			launch, drop, held := planResumeLaunches(tt.strands, tt.liveIDs, exists, tt.paused)
			if got := guids(launch); !slices.Equal(got, tt.wantLaunch) {
				t.Errorf("planResumeLaunches() launch guids = %v, want %v", got, tt.wantLaunch)
			}
			if got := guids(drop); !slices.Equal(got, tt.wantDrop) {
				t.Errorf("planResumeLaunches() drop guids = %v, want %v", got, tt.wantDrop)
			}
			if got := guids(held); !slices.Equal(got, tt.wantHeld) {
				t.Errorf("planResumeLaunches() held guids = %v, want %v", got, tt.wantHeld)
			}
		})
	}
}

// TestDown_ListsEveryWindowsPanesOverACorruptState pins that Down, the escape that works over a corrupt reed.json, still lists the session's panes in every window for its reap before killing the session, and deletes the state.
func TestDown_ListsEveryWindowsPanesOverACorruptState(t *testing.T) {
	e := newTestEngine(t)
	statePath := filepath.Join(e.stateDir(), reedStateFileName)
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := installFakeTmux(t, e)
	// A dead pane is listed but never reaped, so the test touches no real process.
	fake.answer("list-panes", "%1 1 0 80 24 4242\n", nil)
	// A sibling session keeps Down off the server teardown path.
	fake.answer("list-sessions", "sibling-session\n", nil)

	if _, err := e.Down(); err != nil {
		t.Fatalf("Down() error = %v; want nil", err)
	}
	if got := fake.Sequence(sessionListVerb, "kill-session"); !slices.Equal(got, []string{sessionListVerb, "kill-session"}) {
		t.Errorf("Down() tmux sequence = %v; want the session-wide pane listing, then kill-session", got)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Errorf("stat %s error = %v; want the state file deleted", statePath, err)
	}
}

// TestWithRevivalFirst_RevivesEarlierWorktreesThenRerunsTheStep pins the revival helper's sequence for a told list of fake revive functions:
// the predecessors run in list order, the booter's own entry and later entries are never called, a failing predecessor is skipped, and the step runs again once with the skip set.
func TestWithRevivalFirst_RevivesEarlierWorktreesThenRerunsTheStep(t *testing.T) {
	e := newTestEngine(t)
	e.geom.WorktreeName = "booter"
	var revived []string
	entry := func(name string, err error) ReviveEntry {
		return ReviveEntry{Worktree: name, Revive: func() (bool, error) {
			revived = append(revived, name)
			return err == nil, err
		}}
	}
	e.geom.SpawnOrder = func() ([]ReviveEntry, error) {
		return []ReviveEntry{entry("first", nil), entry("broken", errors.New("boom")), entry("second", nil), entry(e.geom.WorktreeName, nil), entry("later", nil)}, nil
	}

	var skipsSeen []bool
	err := e.withRevivalFirst(e.withOpLock, func() error {
		skipsSeen = append(skipsSeen, e.skipRevival)
		if e.skipRevival {
			return nil
		}
		return errReviveFirst
	})

	if err != nil {
		t.Fatalf("withRevivalFirst() = %v, want nil", err)
	}
	if want := []string{"first", "broken", "second"}; !slices.Equal(revived, want) {
		t.Errorf("revived = %v, want %v", revived, want)
	}
	if want := []bool{false, true}; !slices.Equal(skipsSeen, want) {
		t.Errorf("step ran with skips %v, want %v", skipsSeen, want)
	}
	if e.skipRevival {
		t.Error("the skip is still set after the step, want it cleared")
	}
}
