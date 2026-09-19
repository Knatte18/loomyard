// selvagepane_test.go tests the seam helpers selvagepane.go owns: bottommostPaneID and
// ensureSelvagePaneLocked's create/heal path (relocated from lifecycle_test.go, unchanged), and
// planPaneTarget's split-target policy (relocated from spawn_test.go, adapted for its card 5
// signature change). All are pure/hermetic or driven through the e.tmux.execHook fake — no live tmux
// required.

package reedengine

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestPlanPaneTarget(t *testing.T) {
	tests := []struct {
		name            string
		live            []LivePane
		selvagePaneID   string
		wantSplitTarget string
		wantInsertAbove bool
		wantErr         bool
	}{
		{
			// Collapses the old FreshSession_AdoptsTheAliveInitialPane and
			// AllStrandsPaneless_AdoptsFirstAlivePane cases, which differed
			// only in the (now-deleted) strand table they supplied: a sole
			// alive pane and no Selvage both reduce to the same input once
			// the strand table stops mattering.
			name:            "FreshSession_SplitsTheAliveInitialPane",
			live:            []LivePane{{ID: "%1", Height: 50}},
			wantSplitTarget: "%1",
		},
		{
			name: "SoleCorpseUnbound_NeverAdopted_SplitOffTheCorpse",
			// A corpse is never a valid target for anything but a split — the
			// remove-last-strand aftermath: kill-pane on a session's sole
			// pane corpses it (pane_dead=1, exit 0) instead of removing it,
			// and send-keys into a corpse is silently swallowed.
			live:            []LivePane{{ID: "%1", Dead: true, Height: 50}},
			wantSplitTarget: "%1",
		},
		{
			// Collapses the old OneStrandHoldsAPane_SplitsTheTallestAlive and
			// TinyActiveBand_SplitTargetsTheTallestNotTheFirst cases, which
			// differed only in the (now-deleted) strand table they supplied:
			// a 2-row pane beside a 47-row pane and no Selvage, in both.
			name: "TinyActiveBand_SplitTargetsTheTallestNotTheFirst",
			// The session-target split defect this planner replaces: tmux
			// splits the active pane, which select-layout can leave on a
			// 1-2 row band, and a too-small split fails silently. The
			// planner must always pick the tallest alive pane instead.
			live:            []LivePane{{ID: "%1", Height: 2}, {ID: "%2", Height: 47}},
			wantSplitTarget: "%2",
		},
		{
			name:            "DeadPaneNeverTheSplitTargetWhileAnyAlive",
			live:            []LivePane{{ID: "%1", Dead: true, Height: 47}, {ID: "%2", Height: 2}},
			wantSplitTarget: "%2",
		},
		{
			name:    "NoPanesAtAll_Errors",
			live:    nil,
			wantErr: true,
		},
		{
			name: "SelvagePresentNoStrandBound_NonSelvagePaneIsTheSplitTarget",
			// A live Selvage pane plus an alive non-Selvage pane: the split
			// target must land on the non-Selvage pane, never Selvage.
			live:            []LivePane{{ID: "%selvage", Height: 1}, {ID: "%1", Height: 50}},
			selvagePaneID:   "%selvage",
			wantSplitTarget: "%1",
		},
		{
			name: "SelvagePresentWithStrand_SelvageNeverTheSplitTarget",
			// Selvage is tallest by raw Height here, but must still
			// never be chosen over a genuine (if shorter) non-Selvage
			// candidate.
			live:            []LivePane{{ID: "%selvage", Height: 90}, {ID: "%1", Height: 10}},
			selvagePaneID:   "%selvage",
			wantSplitTarget: "%1",
		},
		{
			name: "SeveralUntrackedAlivePanes_SplitsRatherThanGuessingWhichToAdopt",
			// R4 review finding R4-F5, reproduced live and the reason this
			// seam was removed: after .lyx/reed.json was scrubbed from a
			// running session, no strand held a binding and several
			// untracked alive panes remained — one of them the previous
			// header pane, still running "lyx reed header --blocking" (the
			// pre-Selvage keepalive this batch removes). Adoption picked
			// it, send-keys typed the strand's command onto
			// a blocked pane's screen where it never executed (exit 0
			// throughout), and status then reported the strand live with no
			// such process on the box. With more than one candidate there
			// was no way to tell an idle shell from a busy one, so the
			// planner always splits a guaranteed-idle new pane instead — off
			// the tallest, %2 here.
			live:            []LivePane{{ID: "%selvage", Height: 1}, {ID: "%stale", Height: 12}, {ID: "%2", Height: 37}},
			selvagePaneID:   "%selvage",
			wantSplitTarget: "%2",
		},
		{
			name: "SeveralAlivePanesButOnlyOneNonSelvageAlive_StillSplits",
			// A fresh boot's Selvage plus the sole new-session pane, with a
			// dead corpse also present. Exactly one alive non-Selvage pane —
			// it is the split target regardless.
			live:            []LivePane{{ID: "%selvage", Height: 1}, {ID: "%corpse", Dead: true, Height: 12}, {ID: "%1", Height: 37}},
			selvagePaneID:   "%selvage",
			wantSplitTarget: "%1",
		},
		{
			name: "SelvageIsSolePane_SplitTargetFallsBackToSelvage",
			// Every strand has been removed: only Selvage remains. Selvage
			// must become the split target so a subsequent add still
			// has something to split (Selvage survives the split).
			live:            []LivePane{{ID: "%selvage", Height: 21}},
			selvagePaneID:   "%selvage",
			wantSplitTarget: "%selvage",
			wantInsertAbove: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &ReedState{SelvagePaneID: tt.selvagePaneID}
			splitTarget, insertAbove, err := planPaneTarget(st, tt.live)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("planPaneTarget(%+v, %q): expected error, got nil", tt.live, tt.selvagePaneID)
				}
				return
			}
			if err != nil {
				t.Fatalf("planPaneTarget: unexpected error: %v", err)
			}
			if splitTarget != tt.wantSplitTarget {
				t.Errorf("planPaneTarget(%+v, %q) splitTargetID = %q, want %q",
					tt.live, tt.selvagePaneID, splitTarget, tt.wantSplitTarget)
			}
			if insertAbove != tt.wantInsertAbove {
				t.Errorf("planPaneTarget(%+v, %q) insertAbove = %v, want %v",
					tt.live, tt.selvagePaneID, insertAbove, tt.wantInsertAbove)
			}
		})
	}
}

// TestBottommostPaneID asserts the Selvage split target is chosen by pane_top rather than by
// list-panes order, which tmux does not guarantee is top-to-bottom.
func TestBottommostPaneID(t *testing.T) {
	tests := []struct {
		name string
		live []LivePane
		want string
	}{
		{"sole pane", []LivePane{{ID: "%0", Top: 0}}, "%0"},
		{"already last", []LivePane{{ID: "%0", Top: 0}, {ID: "%1", Top: 2}}, "%1"},
		{"not last in list order", []LivePane{{ID: "%1", Top: 26}, {ID: "%0", Top: 0}}, "%1"},
		{"three panes, tallest-top listed first", []LivePane{{ID: "%2", Top: 30}, {ID: "%0", Top: 10}, {ID: "%1", Top: 0}}, "%2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bottommostPaneID(tt.live); got != tt.want {
				t.Errorf("bottommostPaneID(%v) = %q; want %q", tt.live, got, tt.want)
			}
		})
	}
}

// TestEnsureSelvagePaneLocked_SplitsWithPaneCwdNotAnchorPath pins that the Selvage split-window call
// pins its pane to Geometry.PaneCwd, not Geometry.AnchorPath — the two are distinct on newTestEngine's
// fixture (lock_test.go), so this assertion cannot pass by coincidence.
// This covers only the Selvage split site: the new-session spawn site is not reachable from this
// seam, since it builds its argv and runs it through the os/exec package's Command function
// directly rather than through e.tmux — that half of the same change is covered by the tagged reed
// suites this batch's verify: also runs (contract_integration_test.go,
// mouse_boot_integration_test.go).
func TestEnsureSelvagePaneLocked_SplitsWithPaneCwdNotAnchorPath(t *testing.T) {
	e := newTestEngine(t)

	const existingPaneID = "%0"
	const newPaneID = "%1"
	listPanesOut := existingPaneID + " 0 0 100 20 4321\n"

	var splitArgs []string
	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
		switch args[0] {
		case "list-panes":
			return listPanesOut, nil
		case "split-window":
			splitArgs = append([]string{}, args...)
			// A genuinely new pane id, distinct from the pre-split live set, so
			// the silent-split guard (validateSplitCreatedNewPane) does not
			// reject the call.
			return newPaneID + "\n", nil
		default:
			return "", nil
		}
	}

	st := &ReedState{Socket: e.Socket(), Session: e.SessionName()}
	if err := e.ensureSelvagePaneLocked(st); err != nil {
		t.Fatalf("ensureSelvagePaneLocked: %v", err)
	}

	found := false
	for i, arg := range splitArgs {
		if arg != "-c" {
			continue
		}
		if i+1 >= len(splitArgs) {
			t.Fatalf("split-window argv %v has a trailing -c with no value", splitArgs)
		}
		found = true
		if splitArgs[i+1] != e.geom.PaneCwd {
			t.Errorf("split-window -c value = %q, want %q (Geometry.PaneCwd)", splitArgs[i+1], e.geom.PaneCwd)
		}
		if splitArgs[i+1] == e.geom.AnchorPath {
			t.Errorf("split-window -c value = %q, want it to differ from AnchorPath %q on this fixture", splitArgs[i+1], e.geom.AnchorPath)
		}
	}
	if !found {
		t.Fatalf("split-window argv %v has no -c flag", splitArgs)
	}
}

// TestEnsureSelvagePaneLocked_RebuildRejectsSilentSplitFailure pins the validateSplitCreatedNewPane
// guard at its call site (against regression).
func TestEnsureSelvagePaneLocked_RebuildRejectsSilentSplitFailure(t *testing.T) {
	e := newTestEngine(t)

	// One alive, non-Selvage pane (%0) — the new-session initial pane a fresh
	// boot leaves before Selvage exists. It is the only pane, so it is both
	// the bottommost split target and the id psmux's silent-split shape re-prints.
	const existingPaneID = "%0"
	listPanesOut := existingPaneID + " 0 0 100 20 4321\n"

	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
		switch args[0] {
		case "list-panes":
			return listPanesOut, nil
		case "split-window":
			// psmux silent failure: exit 0, no new pane, an EXISTING pane's id
			// printed on stdout. Trusting it would bind Selvage to %0.
			return existingPaneID + "\n", nil
		default:
			// send-keys / kill-pane etc. — only reached if the guard is
			// (wrongly) bypassed; succeed so the missing-guard regression
			// returns nil and this test's error assertion catches it.
			return "", nil
		}
	}

	st := &ReedState{Socket: e.Socket(), Session: e.SessionName()}
	err := e.ensureSelvagePaneLocked(st)
	if err == nil {
		t.Fatalf("ensureSelvagePaneLocked accepted a silent-split failure (bound Selvage to pre-existing pane %q); the validateSplitCreatedNewPane guard at this call site is missing or bypassed", existingPaneID)
	}
	if !strings.Contains(err.Error(), "split Selvage pane") {
		t.Errorf("error = %v, want it to name the Selvage split failure", err)
	}
	if st.SelvagePaneID != "" {
		t.Errorf("SelvagePaneID = %q, want unchanged (never bound to the pre-existing strand pane on a rejected rebuild)", st.SelvagePaneID)
	}
}

// TestEnsureSelvagePaneLocked_RecoversWhenTheBottomPaneIsTooSmallToSplit is the regression guard for
// the R4 review's R4-F4: an untracked one-row Selvage band at the physical bottom of the window made
// every Selvage rebuild impossible, wedging up and resume permanently with "no space for new pane"
// while status kept reporting the session healthy.
//
// Reproduced live before the fix: with a session up and the default one-row Selvage band laid out,
// removing .lyx/reed.json — a never-tracked machine-local tree, exactly what `git clean -xdf` in the
// worktree deletes — left `lyx reed up` and `lyx reed resume` failing identically on every
// subsequent invocation, with `lyx reed down` the only (unnamed) escape.
//
// The scripted substrate below is the shape that produced it: a one-row pane at the largest pane_top
// that tmux refuses to split, and a tall pane above it. The assertions are that the even-vertical
// re-tile is actually issued and that the retried split's pane becomes Selvage — a fix that only
// improved the error message fails both.
func TestEnsureSelvagePaneLocked_RecoversWhenTheBottomPaneIsTooSmallToSplit(t *testing.T) {
	e := newTestEngine(t)

	const oneRowBottomPaneID = "%1"
	const tallPaneID = "%0"
	const rebuiltSelvagePaneID = "%7"
	// pane_id pane_dead pane_top pane_width pane_height pane_pid
	wedged := tallPaneID + " 0 0 100 48 4321\n" + oneRowBottomPaneID + " 0 48 100 1 4322\n"
	retiled := tallPaneID + " 0 0 100 24 4321\n" + oneRowBottomPaneID + " 0 25 100 25 4322\n"

	reTiled := false
	splitAttempts := 0
	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
		switch args[0] {
		case "list-panes":
			if reTiled {
				return retiled, nil
			}
			return wedged, nil
		case "select-layout":
			if len(args) < 2 || args[len(args)-1] != "even-vertical" {
				return "", fmt.Errorf("unexpected select-layout args %v; want the built-in even-vertical layout", args)
			}
			reTiled = true
			return "", nil
		case "split-window":
			splitAttempts++
			if !reTiled {
				// tmux's real refusal against a one-row pane: exit 1, no pane.
				return "", errors.New("exit status 1: no space for new pane")
			}
			return rebuiltSelvagePaneID + "\n", nil
		default:
			return "", nil
		}
	}

	st := &ReedState{Socket: e.Socket(), Session: e.SessionName()}
	if err := e.ensureSelvagePaneLocked(st); err != nil {
		t.Fatalf("ensureSelvagePaneLocked() = %v; want nil (the Selvage rebuild must recover from a one-row bottom pane, not wedge the worktree)", err)
	}
	if !reTiled {
		t.Errorf("ensureSelvagePaneLocked never issued the even-vertical re-tile; without it the retried split has no room either")
	}
	if splitAttempts != 2 {
		t.Errorf("split-window attempts = %d; want exactly 2 (one refused, one retried behind the re-tile)", splitAttempts)
	}
	if st.SelvagePaneID != rebuiltSelvagePaneID {
		t.Errorf("SelvagePaneID = %q; want %q (the pane the retried split created)", st.SelvagePaneID, rebuiltSelvagePaneID)
	}
}

// TestEnsureSelvagePaneLocked_LaunchesTheCommandOnTheSplitNotViaSendKeys is P1: it pins that
// Selvage is booted by handing split-window e.cfg.Shell as its own trailing shell-command
// argument, not by typing it into an interactive shell afterwards via send-keys. Both halves matter
// — a fix that carries the command on the argv but still sends keys, or vice versa, must fail this.
//
// No #{pane_current_command} assertion is added: that value is shell-dependent and this fake-tmux
// substrate never runs a real shell.
func TestEnsureSelvagePaneLocked_LaunchesTheCommandOnTheSplitNotViaSendKeys(t *testing.T) {
	e := newTestEngine(t)

	const existingPaneID = "%0"
	const newPaneID = "%1"
	listPanesOut := existingPaneID + " 0 0 100 20 4321\n"

	var splitArgs []string
	sendKeysCalls := 0
	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
		switch args[0] {
		case "list-panes":
			return listPanesOut, nil
		case "split-window":
			splitArgs = append([]string{}, args...)
			// A genuinely new pane id, distinct from the pre-split live set, so
			// the silent-split guard (validateSplitCreatedNewPane) does not
			// reject the call.
			return newPaneID + "\n", nil
		case "send-keys":
			sendKeysCalls++
			return "", nil
		default:
			return "", nil
		}
	}

	st := &ReedState{Socket: e.Socket(), Session: e.SessionName()}
	if err := e.ensureSelvagePaneLocked(st); err != nil {
		t.Fatalf("ensureSelvagePaneLocked: %v", err)
	}

	fIndex := -1
	for i, arg := range splitArgs {
		if arg == "-F" {
			fIndex = i
			break
		}
	}
	if fIndex == -1 {
		t.Fatalf("split-window argv %v has no -F flag", splitArgs)
	}
	if fIndex+1 >= len(splitArgs) {
		t.Fatalf("split-window argv %v has a trailing -F with no value", splitArgs)
	}
	if fIndex+2 >= len(splitArgs) {
		t.Fatalf("split-window argv %v carries no trailing command argument after the -F value; want the launch line appended as split-window's own trailing shell-command argument", splitArgs)
	}
	launchArg := splitArgs[fIndex+2]
	if launchArg != e.cfg.Shell {
		t.Errorf("split-window trailing command argument = %q, want %q (e.cfg.Shell, launched the same way new-session launches the session's first pane)", launchArg, e.cfg.Shell)
	}
	if sendKeysCalls != 0 {
		t.Errorf("send-keys calls = %d, want 0 (Selvage must launch its own command on the split, not be typed into via send-keys)", sendKeysCalls)
	}
}

// TestEnsureSelvagePaneLocked_RecordsThePaneIDAfterLaunch pins that the split pane's id is recorded
// onto state even under go test's fake-tmux substrate, which never runs a real shell — recording
// must not depend on anything the launched command actually does. This used to also pin a
// suppressed, commandless launch under go test; that suppression mechanism is gone along with the
// header pane's re-exec Selvage replaces, so the launch itself is covered by
// TestEnsureSelvagePaneLocked_LaunchesTheCommandOnTheSplitNotViaSendKeys and this test narrows to the
// recording half.
func TestEnsureSelvagePaneLocked_RecordsThePaneIDAfterLaunch(t *testing.T) {
	e := newTestEngine(t)

	const existingPaneID = "%0"
	const newPaneID = "%1"
	listPanesOut := existingPaneID + " 0 0 100 20 4321\n"

	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
		switch args[0] {
		case "list-panes":
			return listPanesOut, nil
		case "split-window":
			return newPaneID + "\n", nil
		default:
			return "", nil
		}
	}

	st := &ReedState{Socket: e.Socket(), Session: e.SessionName()}
	if err := e.ensureSelvagePaneLocked(st); err != nil {
		t.Fatalf("ensureSelvagePaneLocked: %v", err)
	}

	if st.SelvagePaneID != newPaneID {
		t.Errorf("SelvagePaneID = %q, want %q", st.SelvagePaneID, newPaneID)
	}
}

// TestEnsureSelvagePaneLocked_RetriedSplitAlsoCarriesTheLaunchCommand reuses
// TestEnsureSelvagePaneLocked_RecoversWhenTheBottomPaneIsTooSmallToSplit's wedged/retiled scripted
// substrate (a one-row bottom pane the first split-window refuses, an even-vertical re-tile, then a
// successful retry), and pins that the RETRIED split-window call — not just a hypothetical first
// one — carries the launch command too. A retry path that dropped launchCmd would boot a recovered
// Selvage as a commandless shell, silently reopening this batch's noise class on exactly the
// wedged-worktree recovery path R4-F4 exists for.
func TestEnsureSelvagePaneLocked_RetriedSplitAlsoCarriesTheLaunchCommand(t *testing.T) {
	e := newTestEngine(t)

	const oneRowBottomPaneID = "%1"
	const tallPaneID = "%0"
	const rebuiltSelvagePaneID = "%7"
	// pane_id pane_dead pane_top pane_width pane_height pane_pid
	wedged := tallPaneID + " 0 0 100 48 4321\n" + oneRowBottomPaneID + " 0 48 100 1 4322\n"
	retiled := tallPaneID + " 0 0 100 24 4321\n" + oneRowBottomPaneID + " 0 25 100 25 4322\n"

	reTiled := false
	var retriedSplitArgs []string
	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
		switch args[0] {
		case "list-panes":
			if reTiled {
				return retiled, nil
			}
			return wedged, nil
		case "select-layout":
			reTiled = true
			return "", nil
		case "split-window":
			if !reTiled {
				// tmux's real refusal against a one-row pane: exit 1, no pane.
				return "", errors.New("exit status 1: no space for new pane")
			}
			retriedSplitArgs = append([]string{}, args...)
			return rebuiltSelvagePaneID + "\n", nil
		default:
			return "", nil
		}
	}

	st := &ReedState{Socket: e.Socket(), Session: e.SessionName()}
	if err := e.ensureSelvagePaneLocked(st); err != nil {
		t.Fatalf("ensureSelvagePaneLocked() = %v; want nil", err)
	}
	if st.SelvagePaneID != rebuiltSelvagePaneID {
		t.Fatalf("SelvagePaneID = %q; want %q (the pane the retried split created)", st.SelvagePaneID, rebuiltSelvagePaneID)
	}

	if len(retriedSplitArgs) == 0 {
		t.Fatalf("the retried split-window call was never recorded")
	}
	launchArg := retriedSplitArgs[len(retriedSplitArgs)-1]
	if launchArg != e.cfg.Shell {
		t.Errorf("retried split-window trailing argument = %q, want %q (a retried Selvage must never boot commandless)", launchArg, e.cfg.Shell)
	}
}

// TestEnsureSelvagePaneLocked_SplitsBelowTheBottommostPaneWithNoBFlag pins the new split direction
// end to end: given a scripted pane list whose largest pane_top is a known id, ensureSelvagePaneLocked
// targets that id, and the split-window argv it issues carries no -b — tmux's default direction
// (new pane below target) is exactly where Selvage must land now that render.Rules emits the band
// cell last rather than first.
func TestEnsureSelvagePaneLocked_SplitsBelowTheBottommostPaneWithNoBFlag(t *testing.T) {
	e := newTestEngine(t)

	const topPaneID = "%0"
	const bottomPaneID = "%1"
	const newPaneID = "%2"
	listPanesOut := topPaneID + " 0 0 100 10 4321\n" + bottomPaneID + " 0 10 100 10 4322\n"

	var splitArgs []string
	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
		switch args[0] {
		case "list-panes":
			return listPanesOut, nil
		case "split-window":
			splitArgs = append([]string{}, args...)
			return newPaneID + "\n", nil
		default:
			return "", nil
		}
	}

	st := &ReedState{Socket: e.Socket(), Session: e.SessionName()}
	if err := e.ensureSelvagePaneLocked(st); err != nil {
		t.Fatalf("ensureSelvagePaneLocked: %v", err)
	}

	for _, arg := range splitArgs {
		if arg == "-b" {
			t.Errorf("split-window argv %v carries -b; want no -b (Selvage now splits below, not above)", splitArgs)
		}
	}

	targetFound := false
	for i, arg := range splitArgs {
		if arg != "-t" {
			continue
		}
		if i+1 >= len(splitArgs) {
			t.Fatalf("split-window argv %v has a trailing -t with no value", splitArgs)
		}
		targetFound = true
		if splitArgs[i+1] != bottomPaneID {
			t.Errorf("split-window -t value = %q, want %q (the bottommost pane)", splitArgs[i+1], bottomPaneID)
		}
	}
	if !targetFound {
		t.Fatalf("split-window argv %v has no -t flag", splitArgs)
	}
}
