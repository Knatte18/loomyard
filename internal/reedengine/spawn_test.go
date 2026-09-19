// spawn_test.go covers launchStrandLocked's reap-before-allocate ordering (TestLaunchStrandLocked_*,
// invoking it directly through the e.tmux.execHook fake) and loadOrInitStateLocked's fresh-worktree
// bootstrap. Both are pure/hermetic, no live tmux required. The composed live behavior against a real
// tmux is covered by the smoke tests. planPaneTarget's own table-driven test moved to
// selvagepane_test.go alongside the function it exercises.

package reedengine

import (
	"testing"
)

// TestLaunchStrandLocked_ReapsUntrackedPanesBeforeChoosingASplitTarget pins the reap-before-allocate
// chokepoint at the unit tier: launchStrandLocked must reconcile before it plans a split target, so
// an untracked alive pane is never eligible to become the split target and is instead reaped first.
//
// The fixture is a ReedState with an alive Selvage pane, zero strands bound to a present pane, and one
// untracked alive pane; the strand being launched has PaneID == "", mirroring how addStrandLocked
// appends a fresh strand before calling launchStrandLocked. The alive Selvage — not any strand
// binding — is what authorizes the untracked reap here (see reconcile.go's selvageAlive disjunct).
func TestLaunchStrandLocked_ReapsUntrackedPanesBeforeChoosingASplitTarget(t *testing.T) {
	e := newTestEngine(t)

	const selvagePaneID = "%selvage"
	const untrackedPaneID = "%untracked"
	preReap := selvagePaneID + " 0 0 100 3 4321\n" + untrackedPaneID + " 0 3 100 20 4322\n"
	postReap := selvagePaneID + " 0 0 100 3 4321\n"

	var verbs []string
	var listPanesCalls int
	var splitArgs []string
	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
		verbs = append(verbs, args[0])
		switch args[0] {
		case "list-panes":
			listPanesCalls++
			if listPanesCalls == 1 {
				return preReap, nil
			}
			return postReap, nil
		case "split-window":
			splitArgs = append([]string{}, args...)
			return "%new\n", nil
		default:
			return "", nil
		}
	}

	st := &ReedState{SelvagePaneID: selvagePaneID}
	st.Strands = append(st.Strands, Strand{GUID: "new"})
	s := &st.Strands[0]

	if err := e.launchStrandLocked(st, s, "echo hi"); err != nil {
		t.Fatalf("launchStrandLocked: %v", err)
	}

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
func TestLaunchStrandLocked_SkipsTheRedundantReEnumerationWhenNothingIsReaped(t *testing.T) {
	e := newTestEngine(t)

	const selvagePaneID = "%selvage"
	const boundPaneID = "%bound"
	live := selvagePaneID + " 0 0 100 3 4321\n" + boundPaneID + " 0 3 100 20 4322\n"

	var verbs []string
	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
		verbs = append(verbs, args[0])
		switch args[0] {
		case "list-panes":
			return live, nil
		case "split-window":
			return "%new\n", nil
		default:
			return "", nil
		}
	}

	st := &ReedState{SelvagePaneID: selvagePaneID}
	st.Strands = append(st.Strands, Strand{GUID: "bound", PaneID: boundPaneID}, Strand{GUID: "new"})
	s := &st.Strands[1]

	if err := e.launchStrandLocked(st, s, "echo hi"); err != nil {
		t.Fatalf("launchStrandLocked: %v", err)
	}

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
	if verbs[0] != "list-panes" || splitIdx <= 0 {
		t.Errorf("verbs %v: want the single list-panes to precede split-window", verbs)
	}
}

func TestLoadOrInitStateLocked_AbsentFileInitializesFromEngineIdentity(t *testing.T) {
	e := newTestEngine(t)

	st, err := e.loadOrInitStateLocked()
	if err != nil {
		t.Fatalf("loadOrInitStateLocked: %v", err)
	}
	if st == nil {
		t.Fatal("loadOrInitStateLocked() = nil, want a fresh ReedState")
	}
	if st.Socket != e.Socket() {
		t.Errorf("fresh state Socket = %q, want %q", st.Socket, e.Socket())
	}
	if st.Session != e.SessionName() {
		t.Errorf("fresh state Session = %q, want %q", st.Session, e.SessionName())
	}
	if len(st.Strands) != 0 {
		t.Errorf("fresh state Strands = %v, want empty", st.Strands)
	}
}

// TestLoadOrInitStateLocked_ExistingFileLoadsStrandsAndRestampsIdentity pins the R3 review's R3-F2
// contract: strand data loads verbatim from the persisted file, while the Socket/Session identity
// diagnostic is re-stamped from the engine's told geometry on every load — a renamed worktree
// carries its .lyx state along, but its session name changes with the directory, and a diagnostic
// recording an identity reed no longer drives is worse than none.
func TestLoadOrInitStateLocked_ExistingFileLoadsStrandsAndRestampsIdentity(t *testing.T) {
	e := newTestEngine(t)

	persisted := &ReedState{
		Socket:  "stale-server-from-before-a-rename",
		Session: "stale-session-from-before-a-rename",
		Strands: []Strand{{GUID: "g1", PaneID: "%1"}},
	}
	if err := SaveState(e.stateDir(), persisted); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	st, err := e.loadOrInitStateLocked()
	if err != nil {
		t.Fatalf("loadOrInitStateLocked: %v", err)
	}
	if st.Socket != e.Socket() {
		t.Errorf("loadOrInitStateLocked() Socket = %q, want the re-stamped %q, not the stale persisted value", st.Socket, e.Socket())
	}
	if st.Session != e.SessionName() {
		t.Errorf("loadOrInitStateLocked() Session = %q, want the re-stamped %q, not the stale persisted value", st.Session, e.SessionName())
	}
	if len(st.Strands) != 1 || st.Strands[0].GUID != "g1" {
		t.Errorf("loadOrInitStateLocked() Strands = %+v, want the persisted strand", st.Strands)
	}
}

// TestSendKeysLiteralArg pins the dash-escape rule for tmux send-keys -l: tmux parses a '-'-leading
// literal argument as flags and silently drops it (exit 0, nothing typed; '--' does not stop the
// parsing), so a dash-leading opaque cmd must be sent with one leading space — which the pane shell
// ignores — while every other text passes through verbatim.
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
			e.tmux.execHook = func(capture bool, args ...string) (string, error) {
				switch args[0] {
				case "display-message":
					return liveAnswer, nil
				case "list-sessions":
					return "worktree\n", nil
				case "list-panes":
					// Both panes present and alive, so a binding that survives the repair reads as
					// live and one that does not reads as not-live.
					return selvagePane + " 0 0 100 3 4322\n" + firstStrandPane + " 0 3 100 20 4323\n", nil
				default:
					return "", nil
				}
			}

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
