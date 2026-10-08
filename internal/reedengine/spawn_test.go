// spawn_test.go covers launchStrandLocked's reap-before-allocate ordering (TestLaunchStrandLocked_*,
// invoking it directly through the e.tmux.execHook fake) and loadOrInitStateLocked's fresh-worktree
// bootstrap. Both are pure/hermetic, no live tmux required. The composed live behavior against a real
// tmux is covered by the smoke tests. planPaneTarget's own table-driven test moved to
// selvagepane_test.go alongside the function it exercises.
// It also pins two regression guards for the pane-binary prelude wiring: that launchStrandLocked's send-keys payload is the launch script's source statement and the script holds the composed prelude rather than the bare command, and that the split-window argv it issues still carries no trailing shell-command argument.

package reedengine

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// TestLaunchStrandLocked_ReapsUntrackedPanesBeforeChoosingASplitTarget pins the reap-before-allocate
// chokepoint at the unit tier: launchStrandLocked must reconcile before it plans a split target, so
// an untracked alive pane is never eligible to become the split target and is instead reaped first.
//
// The fixture is a ReedState with an alive Selvage pane, zero strands bound to a present pane, and one
// untracked alive pane; the strand being launched has PaneID == "", mirroring how addStrandLocked
// appends a fresh strand before calling launchStrandLocked. The alive Selvage — not any strand
// binding — is what authorizes the untracked reap here (see reconcile.go's policy.authorizesReap() disjunct).
func TestLaunchStrandLocked_ReapsUntrackedPanesBeforeChoosingASplitTarget(t *testing.T) {
	e := newTestEngine(t)

	const selvagePaneID = "%selvage"
	const untrackedPaneID = "%untracked"
	preReap := selvagePaneID + " 0 0 100 3 4321\n" + untrackedPaneID + " 0 3 100 20 4322\n"
	postReap := selvagePaneID + " 0 0 100 3 4321\n"

	fake := installFakeTmux(t, e)
	fake.answerFunc("list-panes", func([]string) (string, error) {
		if fake.Count("list-panes") == 1 {
			return preReap, nil
		}
		return postReap, nil
	})
	fake.answer("split-window", "%new\n", nil)

	st := &ReedState{SelvagePaneID: selvagePaneID}
	st.Strands = append(st.Strands, Strand{GUID: "new"})
	s := &st.Strands[0]

	if err := e.launchStrandLocked(st, s, "echo hi"); err != nil {
		t.Fatalf("launchStrandLocked: %v", err)
	}
	verbs := fake.Sequence()
	splitArgs := fake.LastArgv("split-window")

	killIdx, splitIdx, secondListIdx := -1, -1, -1
	listPanesSeen := 0
	for i, v := range verbs {
		switch v {
		case "kill-pane":
			if killIdx == -1 {
				killIdx = i
			}
		case "split-window":
			if splitIdx == -1 {
				splitIdx = i
			}
		case "list-panes":
			listPanesSeen++
			if listPanesSeen == 2 {
				secondListIdx = i
			}
		}
	}
	if killIdx == -1 {
		t.Fatalf("verbs %v: expected a kill-pane reaping the untracked pane, got none", verbs)
	}
	if splitIdx == -1 {
		t.Fatalf("verbs %v: expected a split-window, got none", verbs)
	}
	if killIdx > splitIdx {
		t.Errorf("verbs %v: kill-pane at %d, split-window at %d; want kill-pane before split-window", verbs, killIdx, splitIdx)
	}
	if secondListIdx == -1 {
		t.Fatalf("verbs %v: expected a second list-panes (re-enumeration after reap), got none", verbs)
	}
	if !(killIdx < secondListIdx && secondListIdx < splitIdx) {
		t.Errorf("verbs %v: want kill-pane(%d) < second list-panes(%d) < split-window(%d)", verbs, killIdx, secondListIdx, splitIdx)
	}

	if len(splitArgs) == 0 {
		t.Fatal("split-window was never called")
	}
	for i, arg := range splitArgs {
		if arg == "-t" && i+1 < len(splitArgs) && splitArgs[i+1] == untrackedPaneID {
			t.Errorf("split-window target = %q, want it not to be the reaped pane", untrackedPaneID)
		}
	}
}

// TestLaunchStrandLocked_SkipsTheRedundantReEnumerationWhenNothingIsReaped is the companion to
// TestLaunchStrandLocked_ReapsUntrackedPanesBeforeChoosingASplitTarget: when reconcile kills nothing,
// launchStrandLocked must not pay for a second list-panes round trip it does not need.
//
// The fixture has nothing to reap: an alive Selvage plus a strand already bound to a present alive
// pane. The strand being launched is a second one, again with PaneID == "".
//
//testtiming:keep pins launchStrandLocked paying for exactly one list-panes and issuing no kill-pane when reconcile reaps nothing; its covering tests run this code without asserting it
func TestLaunchStrandLocked_SkipsTheRedundantReEnumerationWhenNothingIsReaped(t *testing.T) {
	e := newTestEngine(t)

	const selvagePaneID = "%selvage"
	const boundPaneID = "%bound"
	live := selvagePaneID + " 0 0 100 3 4321\n" + boundPaneID + " 0 3 100 20 4322\n"

	fake := installFakeTmux(t, e)
	fake.answer("list-panes", live, nil)
	fake.answer("split-window", "%new\n", nil)

	st := &ReedState{SelvagePaneID: selvagePaneID}
	st.Strands = append(st.Strands, Strand{GUID: "bound", PaneID: boundPaneID}, Strand{GUID: "new"})
	s := &st.Strands[1]

	if err := e.launchStrandLocked(st, s, "echo hi"); err != nil {
		t.Fatalf("launchStrandLocked: %v", err)
	}
	verbs := fake.Sequence()

	for _, v := range verbs {
		if v == "kill-pane" {
			t.Fatalf("verbs %v: expected no kill-pane when nothing is reaped", verbs)
		}
	}
	listPanesCount, splitIdx := 0, -1
	for i, v := range verbs {
		if v == "list-panes" {
			listPanesCount++
		}
		if v == "split-window" && splitIdx == -1 {
			splitIdx = i
		}
	}
	if listPanesCount != 1 {
		t.Errorf("verbs %v: list-panes called %d times, want exactly 1 (no redundant re-enumeration)", verbs, listPanesCount)
	}
	if splitIdx == -1 {
		t.Fatalf("verbs %v: expected a split-window, got none", verbs)
	}
	if verbs[0] != sessionListVerb || verbs[1] != "list-panes" || splitIdx <= 1 {
		t.Errorf("verbs %v: want the window resolution, then the single list-panes, to precede split-window", verbs)
	}
}

// TestLoadOrInitStateLocked pins the R3 review's R3-F2 contract: an absent file initializes a fresh state from the engine's identity,
// while strand data loads verbatim from an existing persisted file and the Socket/Session identity diagnostic is re-stamped
// from the engine's told geometry on every load — a renamed worktree carries its .lyx state along,
// but its session name changes with the directory, and a diagnostic recording an identity reed no longer drives is worse than none.
func TestLoadOrInitStateLocked(t *testing.T) {
	tests := []struct {
		name        string
		persisted   *ReedState
		wantStrands []string
	}{
		{name: "AbsentFileInitializesFromEngineIdentity"},
		{
			name: "ExistingFileLoadsStrandsAndRestampsIdentity",
			persisted: &ReedState{
				Socket:  "stale-server-from-before-a-rename",
				Session: "stale-session-from-before-a-rename",
				Strands: []Strand{{GUID: "g1", PaneID: "%1"}},
			},
			wantStrands: []string{"g1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			if tt.persisted != nil {
				if err := SaveState(e.stateDir(), tt.persisted); err != nil {
					t.Fatalf("SaveState: %v", err)
				}
			}

			st, err := e.loadOrInitStateLocked()
			if err != nil {
				t.Fatalf("loadOrInitStateLocked: %v", err)
			}

			if st == nil {
				t.Fatal("loadOrInitStateLocked() = nil, want a state")
			}
			if st.Socket != e.Socket() {
				t.Errorf("Socket = %q, want the engine's %q, not a stale persisted value", st.Socket, e.Socket())
			}
			if st.Session != e.SessionName() {
				t.Errorf("Session = %q, want the engine's %q, not a stale persisted value", st.Session, e.SessionName())
			}
			if got := guids(st.Strands); !slices.Equal(got, tt.wantStrands) {
				t.Errorf("Strands = %v, want %v", got, tt.wantStrands)
			}
		})
	}
}

// TestSendKeysLiteralArg pins the dash-escape rule for tmux send-keys -l: tmux parses a '-'-leading
// literal argument as flags and silently drops it (exit 0, nothing typed; '--' does not stop the
// parsing), so a dash-leading opaque cmd must be sent with one leading space — which the pane shell
// ignores — while every other text passes through verbatim.
//
//testtiming:keep pins the dash-escape rule for send-keys -l: a dash-leading text gets one leading space and every other text, an already spaced one included, passes verbatim; its covering tests run this code without asserting it
func TestSendKeysLiteralArg(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{"claude --continue", "claude --continue"},
		{"-join('a','b')", " -join('a','b')"},
		{"--flag-first", " --flag-first"},
		{" -already-spaced", " -already-spaced"},
		{"", ""},
		{"echo one; echo Enter", "echo one; echo Enter"},
	}
	for _, tt := range tests {
		if got := sendKeysLiteralArg(tt.text); got != tt.want {
			t.Errorf("sendKeysLiteralArg(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}

// TestValidateSplitCreatedNewPane pins the genuinely-new-pane guard both split sites
// (launchStrandLocked, ensureSelvagePaneLocked) share: psmux's silent too-small-to-split failure
// exits 0 and prints an EXISTING pane's id, and trusting it would bind two owners to one pane — a
// duplicate pane number in the next select-layout string, which destroys the session's panes
// wholesale.
//
//testtiming:keep pins the genuinely-new-pane guard: a fresh pane id passes while an empty, pre-existing alive or pre-existing dead pane id errors; its covering tests run this code without asserting it
func TestValidateSplitCreatedNewPane(t *testing.T) {
	preSplitLive := []LivePane{{ID: "%0"}, {ID: "%1", Dead: true}}

	tests := []struct {
		name    string
		paneID  string
		wantErr bool
	}{
		{"genuinely new pane id passes", "%2", false},
		{"empty pane id errors", "", true},
		{"pre-existing alive pane id errors", "%0", true},
		{"pre-existing dead pane id errors", "%1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSplitCreatedNewPane(tt.paneID, preSplitLive, "%0")
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSplitCreatedNewPane(%q) error = %v, wantErr %v", tt.paneID, err, tt.wantErr)
			}
		})
	}
}

// TestStatus_NeverReportsAStrandLiveOnAPaneAnotherOwnerClaims pins R5-F3's repair at the CALL SITE
// rather than at the helper.
//
// clearConflictingPaneBindings has its own unit coverage (reconcile_test.go) and the render path has
// an independent second layer (removeDuplicatePaneCells), so deleting the call to it from
// loadOrInitStateLocked left the whole hermetic and smoke suites green while the reconcile-side layer
// was gone — the wiring gap the orchestrator's independent verification of round 5 found.
//
// Status is the observable that isolates this layer: it reads the loaded table and cross-references
// it against live panes, and never touches the render path. With the repair wired, a strand whose
// PaneID names a pane another owner already claims is cleared and reported not-live; without it, that
// pane IS alive, so status reports live:true against someone else's pane — exactly the false-healthy
// symptom the R5 review reproduced live ("status reported the strand live:true against the header
// pane running `lyx reed header --blocking`", the pre-Selvage keepalive this batch removes).
//
// The recorded generation deliberately MATCHES the one the probe answers, so the pane-generation
// guard adopts rather than clears and the only thing that can clear a binding here is the repair
// under test.
func TestStatus_NeverReportsAStrandLiveOnAPaneAnotherOwnerClaims(t *testing.T) {
	const selvagePane = "%1"
	const firstStrandPane = "%2"
	const liveAnswer = "$0|4321|1787000000"
	liveGeneration := PaneGeneration{SessionName: "worktree", TmuxSessionID: "$0", ServerPID: "4321", Created: "1787000000"}

	tests := []struct {
		name string
		// strandPaneIDs are the persisted PaneIDs, in table order, for strands named "first" and
		// "second".
		strandPaneIDs []string
		wantLive      []bool
	}{
		{
			name:          "a strand bound to Selvage's own pane is not reported live on it",
			strandPaneIDs: []string{selvagePane},
			wantLive:      []bool{false},
		},
		{
			name:          "of two strands claiming one pane, only the first owner is reported live",
			strandPaneIDs: []string{firstStrandPane, firstStrandPane},
			wantLive:      []bool{true, false},
		},
		{
			name:          "a table with no conflict is left alone",
			strandPaneIDs: []string{firstStrandPane},
			wantLive:      []bool{true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			fake := installFakeTmux(t, e)
			fake.answer("display-message", liveAnswer, nil)
			fake.answer("list-sessions", "worktree\n", nil)
			// Both panes present and alive, so a binding that survives the repair reads as
			// live and one that does not reads as not-live.
			fake.answer("list-panes", selvagePane+" 0 0 100 3 4322\n"+firstStrandPane+" 0 3 100 20 4323\n", nil)

			st := &ReedState{SelvagePaneID: selvagePane, PaneGeneration: liveGeneration}
			names := []string{"first", "second"}
			for i, paneID := range tt.strandPaneIDs {
				st.Strands = append(st.Strands, Strand{GUID: names[i], Name: names[i], PaneID: paneID})
			}
			if err := SaveState(e.stateDir(), st); err != nil {
				t.Fatalf("SaveState: %v", err)
			}

			result, err := e.Status()
			if err != nil {
				t.Fatalf("Status: %v", err)
			}
			if len(result.Strands) != len(tt.wantLive) {
				t.Fatalf("Status reported %d strands; want %d", len(result.Strands), len(tt.wantLive))
			}
			for i, want := range tt.wantLive {
				got := result.Strands[i]
				if got.Live != want {
					t.Errorf("Status strand %q live = %v on pane %q; want %v — a pane has exactly one owner, and a binding naming a pane another owner claims must be cleared at load",
						got.GUID, got.Live, got.PaneID, want)
				}
			}
		})
	}
}

// launchFake installs a fakeTmux that reports one live Selvage pane and answers split-window with a fresh pane id.
// onEnter, when non-nil, runs at the Enter submit, the last step of launchStrandLocked.
func launchFake(t *testing.T, e *Engine, onEnter func()) *fakeTmux {
	t.Helper()
	fake := installFakeTmux(t, e)
	fake.answer("list-panes", "%selvage 0 0 100 20 4321\n", nil)
	fake.answer("split-window", "%new\n", nil)
	if onEnter != nil {
		fake.answerFunc("send-keys", func(args []string) (string, error) {
			if args[len(args)-1] == "Enter" {
				onEnter()
			}
			return "", nil
		})
	}
	return fake
}

// readLaunchScript returns the content of strandGUID's launch script.
func readLaunchScript(t *testing.T, e *Engine, strandGUID string) string {
	t.Helper()
	data, err := os.ReadFile(launchScriptPath(shell.ForGOOS(), e.stateDir(), strandGUID))
	if err != nil {
		t.Fatalf("read launch script: %v", err)
	}
	return string(data)
}

// assertLaunchDirEmpty fails when the launch directory holds any file.
func assertLaunchDirEmpty(t *testing.T, e *Engine) {
	t.Helper()
	entries, err := os.ReadDir(launchScriptDir(e.stateDir()))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("ReadDir launch dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("launch dir holds %d file(s), want none", len(entries))
	}
}

// breakSaveState replaces reed.json with a non-empty directory, so SaveState's rename onto it fails.
func breakSaveState(t *testing.T, e *Engine) {
	t.Helper()
	path := filepath.Join(e.stateDir(), reedStateFileName)
	if err := os.RemoveAll(path); err != nil {
		t.Fatalf("RemoveAll reed.json: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(path, "blocker"), 0o755); err != nil {
		t.Fatalf("MkdirAll blocker: %v", err)
	}
}

// TestLaunchStrandLocked_LaunchScript pins, per launch sequence, what launchStrandLocked types and what the strand's launch script holds:
// each launch types the source statement for the script (a single line) and then Enter as a separate send-keys call,
// and the script holds the composed pane-binary prelude joined onto the strand's command, not the bare command.
// A relaunch regenerates the script, an unresolvable executable path leaves the command alone in it, and an empty command writes the prelude alone.
//
//testtiming:keep pins what a launch types and writes: the source statement then a separate Enter, and a launch script holding the prelude joined onto the command, regenerated on relaunch, the bare command when the executable is unresolvable and the prelude alone for an empty command; its covering tests run this code without asserting it
func TestLaunchStrandLocked_LaunchScript(t *testing.T) {
	const exe = "/opt/lyx/bin/lyx"
	tests := []struct {
		name       string
		executable error
		cmds       []string
		want       func(sh shell.Shell, s Strand, e *Engine) string
	}{
		{
			name: "PreludeAheadOfTheStrandCommand",
			cmds: []string{"claude --continue"},
			want: func(sh shell.Shell, s Strand, e *Engine) string {
				return composePaneLaunchLine(sh, "claude --continue", s.GUID, s.Name, e.geom.ParentName) + "\n"
			},
		},
		{
			name: "RelaunchRegeneratesTheScript",
			cmds: []string{"first cmd", "second cmd"},
			want: func(sh shell.Shell, s Strand, e *Engine) string {
				return composePaneLaunchLine(sh, "second cmd", s.GUID, s.Name, e.geom.ParentName) + "\n"
			},
		},
		{
			name:       "NoPreludeWhenTheExecutableIsUnresolvable",
			executable: errors.New("no exe"),
			cmds:       []string{"claude"},
			want:       func(shell.Shell, Strand, *Engine) string { return "claude\n" },
		},
		{
			name: "EmptyCommandWritesThePreludeAlone",
			cmds: []string{""},
			want: func(sh shell.Shell, _ Strand, _ *Engine) string { return paneBinPrelude(sh, exe) + "\n" },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			withInjectedExecutablePath(t, func() (string, error) {
				if tt.executable != nil {
					return "", tt.executable
				}
				return exe, nil
			})
			fake := launchFake(t, e, nil)
			st := &ReedState{SelvagePaneID: "%selvage", Strands: []Strand{{GUID: "new"}}}
			s := &st.Strands[0]

			for _, cmd := range tt.cmds {
				if err := e.launchStrandLocked(st, s, cmd); err != nil {
					t.Fatalf("launchStrandLocked(%q): %v", cmd, err)
				}
			}

			sh := shell.ForGOOS()
			sendKeysCalls := fake.ArgvFor("send-keys")
			if len(sendKeysCalls) != 2*len(tt.cmds) {
				t.Fatalf("send-keys called %d times, want %d (the literal payload, then Enter, per launch): %v", len(sendKeysCalls), 2*len(tt.cmds), sendKeysCalls)
			}
			path := filepath.Join(e.stateDir(), "reed", "launch", s.GUID+sh.ScriptExt())
			wantLiteral := sendKeysLiteralArg(sh.Source(path))
			if strings.Contains(wantLiteral, "\n") {
				t.Errorf("source statement payload = %q, want a single line with no newline", wantLiteral)
			}
			for i := 0; i < len(sendKeysCalls); i += 2 {
				literal, enter := sendKeysCalls[i], sendKeysCalls[i+1]
				if literal[len(literal)-1] != wantLiteral {
					t.Errorf("send-keys args = %v, want the last argument to be the source statement %q", literal, wantLiteral)
				}
				if enter[len(enter)-1] != "Enter" {
					t.Errorf("send-keys args = %v, want its last argument to be \"Enter\" (a separate submit)", enter)
				}
			}
			if got, want := readLaunchScript(t, e, s.GUID), tt.want(sh, *s, e); got != want {
				t.Errorf("launch script = %q, want %q", got, want)
			}
		})
	}
}

// TestLaunchStrandLocked_WriteFailureSendsTheFullLine pins the degrade path: a regular file where the launch directory's parent belongs makes the payload the composed line,
// and the warning names the strand GUID.
//
//testtiming:keep pins the degrade path: when the launch script cannot be written the send-keys payload is the full composed line and the warning names the strand guid; its covering tests run this code without asserting it
func TestLaunchStrandLocked_WriteFailureSendsTheFullLine(t *testing.T) {
	e := newTestEngine(t)
	withInjectedExecutablePath(t, func() (string, error) { return "/opt/lyx/bin/lyx", nil })
	fake := launchFake(t, e, nil)
	if err := os.MkdirAll(e.stateDir(), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(e.stateDir(), "reed"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	buf := logcapture.CaptureVerbose(t)

	st := &ReedState{SelvagePaneID: "%selvage", Strands: []Strand{{GUID: "guid-w"}}}
	const launchCmd = "claude"
	if err := e.launchStrandLocked(st, &st.Strands[0], launchCmd); err != nil {
		t.Fatalf("launchStrandLocked: %v", err)
	}
	want := sendKeysLiteralArg(composePaneLaunchLine(shell.ForGOOS(), launchCmd, "guid-w", st.Strands[0].Name, e.geom.ParentName))
	if first := fake.ArgvFor("send-keys")[0]; first[len(first)-1] != want {
		t.Errorf("first send-keys args = %v, want the full composed line %q", first, want)
	}
	if !strings.Contains(buf.String(), "guid-w") {
		t.Errorf("log %q does not name the strand GUID", buf.String())
	}
}

// TestLaunchScriptDoesNotSurviveAFailedLaunch pins that no launch script outlives a launch that did not complete:
// a launch whose literal send-keys fails deletes the script it already wrote,
// and a SaveState failure right after the launch deletes the never-persisted strand's script, for add and for replace alike.
func TestLaunchScriptDoesNotSurviveAFailedLaunch(t *testing.T) {
	spec := AddSpec{Role: "worker", NameOverride: "n", Cmd: "claude"}
	tests := []struct {
		name  string
		setup func(t *testing.T, e *Engine)
		run   func(e *Engine) error
	}{
		{
			name: "FailedSendOnAdd",
			setup: func(t *testing.T, e *Engine) {
				launchFake(t, e, nil).answerFunc("send-keys", func(args []string) (string, error) {
					if len(args) > 3 && args[3] == "-l" {
						return "", errors.New("send failed")
					}
					return "", nil
				})
			},
			run: func(e *Engine) error {
				_, err := e.addStrandLocked(&ReedState{SelvagePaneID: "%selvage"}, spec)
				return err
			},
		},
		{
			name:  "PersistFailureOnAdd",
			setup: func(t *testing.T, e *Engine) { launchFake(t, e, func() { breakSaveState(t, e) }) },
			run: func(e *Engine) error {
				_, err := e.AddStrand(spec)
				return err
			},
		},
		{
			name: "PersistFailureOnReplace",
			setup: func(t *testing.T, e *Engine) {
				if err := SaveState(e.stateDir(), &ReedState{Strands: []Strand{{GUID: "old", Display: render.Display{Anchor: render.AnchorHidden}}}}); err != nil {
					t.Fatalf("SaveState: %v", err)
				}
				launchFake(t, e, func() { breakSaveState(t, e) })
			},
			run: func(e *Engine) error {
				_, err := e.ReplaceStrand("old", spec)
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			withInjectedExecutablePath(t, func() (string, error) { return "/opt/lyx/bin/lyx", nil })
			tt.setup(t, e)

			if err := tt.run(e); err == nil {
				t.Fatalf("want the launch error")
			}
			assertLaunchDirEmpty(t, e)
		})
	}
}

// TestLaunchStrandLocked_SplitWindowAndPaneTitleCalls pins the two pane-start guarantees of one launch.
// The split-window argv ends with the -F flag and its #{pane_id} value, so that appending ANY trailing argument fails it, not merely a specific known-bad value:
// a trailing shell-command makes tmux hand the pane to /bin/sh -c, which execs a non-login shell that skips ~/.profile / ~/.bash_profile
// and therefore changes the pane's inherited PATH — and that pane resolves claude by bare name, so the change would stop the agent binary resolving at all.
// The two title commands then run in order between the split and the send-keys, and carry the strand's full name.
//
//testtiming:keep pins the split-window argv ending with -F #{pane_id} with no trailing shell command, which would make tmux exec a non-login shell, and the two title commands running in order between the split and the send-keys; its covering tests run this code without asserting it
func TestLaunchStrandLocked_SplitWindowAndPaneTitleCalls(t *testing.T) {
	e := newTestEngine(t)
	withInjectedExecutablePath(t, func() (string, error) { return "/opt/lyx/bin/lyx", nil })
	fake := launchFake(t, e, nil)
	st := &ReedState{SelvagePaneID: "%selvage"}
	st.Strands = append(st.Strands, Strand{GUID: "new", Name: "tst:slug:worker"})
	s := &st.Strands[0]

	if err := e.launchStrandLocked(st, s, "claude --continue"); err != nil {
		t.Fatalf("launchStrandLocked: %v", err)
	}

	splitArgs := fake.LastArgv("split-window")
	if len(splitArgs) < 2 {
		t.Fatalf("split-window argv = %v, too short to check its tail", splitArgs)
	}
	last, secondLast := splitArgs[len(splitArgs)-1], splitArgs[len(splitArgs)-2]
	if secondLast != "-F" || last != "#{pane_id}" {
		t.Errorf("split-window argv = %v, want it to end with \"-F\" \"#{pane_id}\" and nothing after -- a trailing shell-command argument would make tmux exec a non-login shell that skips the pane's profile", splitArgs)
	}

	var order []string
	for _, call := range fake.Calls() {
		switch call[0] {
		case "split-window":
			order = append(order, "split-window")
		case "set-option", "select-pane", "send-keys":
			order = append(order, strings.Join(call, " "))
		}
	}
	if len(order) < 4 {
		t.Fatalf("recorded calls = %v, want split-window, set-option, select-pane, send-keys", order)
	}
	if order[0] != "split-window" {
		t.Errorf("first call = %q, want split-window", order[0])
	}
	if want := "set-option -p -t %new allow-set-title off"; order[1] != want {
		t.Errorf("second call = %q, want %q", order[1], want)
	}
	if want := "select-pane -t %new -T tst:slug:worker"; order[2] != want {
		t.Errorf("third call = %q, want %q", order[2], want)
	}
	if !strings.HasPrefix(order[3], "send-keys") {
		t.Errorf("fourth call = %q, want send-keys after the title commands", order[3])
	}
}
