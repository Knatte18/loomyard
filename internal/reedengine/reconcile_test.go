// reconcile_test.go table-tests planReconcile's pure decision logic.
// The cases run against saved strand tables and fake list-panes results.
// The cases cover pane_dead=1 rows and the Selvage exemption.
// It also exercises reconcileLocked's real-record mutation on the no-dead-panes path.
// That path never touches tmux, so it stays hermetic.
// It pins reconcileLocked's reap log line via logcapture.CaptureVerbose.

package reedengine

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// ownWindowTestStrand returns a strand anchored own-window and bound to paneID.
func ownWindowTestStrand(guid, paneID string) Strand {
	return Strand{GUID: guid, PaneID: paneID, Display: render.Display{Anchor: render.AnchorOwnWindow}}
}

func TestPlanReconcile(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name                     string
		strands                  []Strand
		live                     []LivePane
		windows                  []windowPane
		elsewhere                map[string]string
		selvagePaneID            string
		wantCleared              []string
		wantDeadPanesToKill      []string
		wantUntrackedPanesToKill []string
		wantSolePane             string
		wantOwnWindowsToEnd      []string
	}{
		{
			name:        "GoneStrandClearedRecordKept",
			strands:     []Strand{{GUID: "g1", PaneID: "%1"}},
			live:        nil,
			wantCleared: []string{"g1"},
		},
		{
			name:        "PresentLiveStrandKeepsBinding",
			strands:     []Strand{{GUID: "g1", PaneID: "%1"}},
			live:        []LivePane{{ID: "%1", Dead: false}},
			wantCleared: nil,
		},
		{
			name:                "NonSoleDeadPaneScheduledForKillAndBindingCleared",
			strands:             []Strand{{GUID: "g1", PaneID: "%1"}, {GUID: "g2", PaneID: "%2"}},
			live:                []LivePane{{ID: "%1", Dead: true}, {ID: "%2", Dead: false}},
			wantCleared:         []string{"g1"},
			wantDeadPanesToKill: []string{"%1"},
		},
		{
			name:         "SoleRemainingDeadPaneKeptAndNotScheduledForKill",
			strands:      []Strand{{GUID: "g1", PaneID: "%1"}},
			live:         []LivePane{{ID: "%1", Dead: true}},
			wantCleared:  nil,
			wantSolePane: "%1",
		},
		{
			// Every pane is dead: one must be spared, because killing the last pane ends the session.
			// The first dead pane is kept with its binding, and the rest are killed and their bindings cleared.
			name: "AllDeadKeepsFirstPaneAndKillsTheRest",
			strands: []Strand{
				{GUID: "g1", PaneID: "%1"},
				{GUID: "g2", PaneID: "%2"},
			},
			live:                []LivePane{{ID: "%1", Dead: true}, {ID: "%2", Dead: true}},
			wantCleared:         []string{"g2"},
			wantDeadPanesToKill: []string{"%2"},
			wantSolePane:        "%1",
		},
		{
			name:        "StrandWithNoPaneIDIgnored",
			strands:     []Strand{{GUID: "hidden", PaneID: ""}},
			live:        nil,
			wantCleared: nil,
		},
		{
			name: "OnlyRemoveStrandDeletesRecordsPlanReconcileNeverDrops",
			strands: []Strand{
				{GUID: "gone", PaneID: "%9"},
				{GUID: "present", PaneID: "%1"},
			},
			live:        []LivePane{{ID: "%1", Dead: false}},
			wantCleared: []string{"gone"},
		},
		{
			// A live pane no strand owns is killed deterministically while a strand is bound to a present pane.
			// Such a pane is an operator split or a mid-op-crash orphan.
			// It must not be left to select-layout's positional reaping, which can destroy a tracked pane instead.
			name:                     "UntrackedAlivePaneKilledWhileBoundContentPresent",
			strands:                  []Strand{{GUID: "g1", PaneID: "%1"}},
			live:                     []LivePane{{ID: "%1", Dead: false}, {ID: "%7", Dead: false}},
			wantCleared:              nil,
			wantUntrackedPanesToKill: []string{"%7"},
		},
		{
			// No strand is bound to a present pane, and no Selvage is alive.
			// selvagePaneID is unset, so policy.authorizesReap() is false.
			// reed has nothing to lay out and leaves foreign panes alone; the apply is skipped too (anyPlacedStrand).
			name:        "UntrackedPanesUntouchedWhenNoSelvageAndNothingBound",
			strands:     []Strand{{GUID: "cleared", PaneID: ""}},
			live:        []LivePane{{ID: "%7", Dead: false}, {ID: "%8", Dead: false}},
			wantCleared: nil,
		},
		{
			// Selvage is never reaped as an untracked pane, even while a strand is bound and anyBoundPresent is true.
			// exemptPaneIDs (boundPaneIDs plus Selvage) protects it.
			// boundPaneIDs itself stays strand-only, so a merely live Selvage never inflates anyBoundPresent.
			// The reap already fires via anyBoundPresent in this case, so policy.authorizesReap() adds nothing here.
			name:          "SelvageNeverReapedAsUntrackedWhileStrandBound",
			strands:       []Strand{{GUID: "g1", PaneID: "%1"}},
			live:          []LivePane{{ID: "%1", Dead: false}, {ID: "%selvage", Dead: false}, {ID: "%7", Dead: false}},
			selvagePaneID: "%selvage",
			wantCleared:   nil,
			// %7 is a genuine foreign pane and is still reaped; %selvage is
			// exempt and must not appear here.
			wantUntrackedPanesToKill: []string{"%7"},
		},
		{
			// Selvage is alive and no strand is bound to a present pane.
			// anyBoundPresent stays false, since it derives from boundPaneIDs alone, and folding Selvage in would wrongly flip it.
			// policy.authorizesReap() is true, so the untracked reap fires from that disjunct alone.
			// %7 is killed and Selvage itself stays exempt.
			name:                     "SelvageAloneNeverMakesAnyBoundPresentTrue",
			strands:                  []Strand{{GUID: "cleared", PaneID: ""}},
			live:                     []LivePane{{ID: "%selvage", Dead: false}, {ID: "%7", Dead: false}},
			selvagePaneID:            "%selvage",
			wantCleared:              nil,
			wantUntrackedPanesToKill: []string{"%7"},
		},
		{
			// An alive Selvage with zero strands and one untracked alive pane: that pane is killed as an untracked kill.
			// It is not a dead-pane kill, and Selvage itself is spared.
			name:                     "AliveSelvageNoStrandsReapsOneUntrackedPane",
			strands:                  nil,
			live:                     []LivePane{{ID: "%selvage", Dead: false}, {ID: "%orphan", Dead: false}},
			selvagePaneID:            "%selvage",
			wantCleared:              nil,
			wantUntrackedPanesToKill: []string{"%orphan"},
		},
		{
			// An alive Selvage with zero strands and several untracked panes: all of them are killed.
			// The untracked panes include an old Selvage pane and an orphaned strand pane.
			// The current Selvage is spared.
			name:    "AliveSelvageNoStrandsReapsSeveralUntrackedPanes",
			strands: nil,
			live: []LivePane{
				{ID: "%selvage", Dead: false},
				{ID: "%oldselvage", Dead: false},
				{ID: "%orphanstrand", Dead: false},
			},
			selvagePaneID:            "%selvage",
			wantCleared:              nil,
			wantUntrackedPanesToKill: []string{"%oldselvage", "%orphanstrand"},
		},
		{
			// Selvage is present but Dead: true, with no strand bound and one alive untracked pane.
			// policy.authorizesReap() is false, because the Selvage entry is present but dead, so nothing is reaped.
			name:          "PresentButDeadSelvageDoesNotAuthorizeReap",
			strands:       nil,
			live:          []LivePane{{ID: "%selvage", Dead: true}, {ID: "%orphan", Dead: false}},
			selvagePaneID: "%selvage",
			wantCleared:   nil,
		},
		{
			// A non-empty selvagePaneID naming no entry in live at all, with no strand bound and one alive untracked pane:
			// policy.authorizesReap() is false, a third way distinct from the empty id and the present-but-dead case.
			// The add path reaches this state once an operator kills Selvage outright: no verb but up/resume rebuilds it.
			name:          "SelvageIDNamingNoLivePaneDoesNotAuthorizeReap",
			strands:       nil,
			live:          []LivePane{{ID: "%orphan", Dead: false}},
			selvagePaneID: "%selvage",
			wantCleared:   nil,
		},
		{
			// A dead Selvage alongside a strand bound to a present pane: the reap fires anyway via anyBoundPresent,
			// and the Selvage corpse is still spared.
			name:                     "DeadSelvageAlongsideBoundStrandStillReapsViaAnyBoundPresent",
			strands:                  []Strand{{GUID: "g1", PaneID: "%1"}},
			live:                     []LivePane{{ID: "%selvage", Dead: true}, {ID: "%1", Dead: false}, {ID: "%7", Dead: false}},
			selvagePaneID:            "%selvage",
			wantCleared:              nil,
			wantUntrackedPanesToKill: []string{"%7"},
		},
		{
			// A dead Selvage pane must not be scheduled for killing either:
			// the dead-pane kill loop, not only the untracked reap, spares it.
			// Nothing outside up/resume rebuilds Selvage.
			// Killing the corpse would leave every intermediate add/remove without a Selvage, holding a stale SelvagePaneID.
			// The kept corpse stays enumerable; ensureSelvagePaneLocked heals it at the next boot.
			name:          "DeadSelvagePaneKeptNotKilled",
			strands:       []Strand{{GUID: "g1", PaneID: "%1"}},
			live:          []LivePane{{ID: "%selvage", Dead: true}, {ID: "%1", Dead: false}},
			selvagePaneID: "%selvage",
			wantCleared:   nil,
		},
		{
			// A dead Selvage alongside a dead strand pane:
			// the strand corpse is still killable business-as-usual (an alive pane remains),
			// while the Selvage corpse stays exempt.
			name:                "DeadSelvageExemptWhileDeadStrandPaneStillKilled",
			strands:             []Strand{{GUID: "g1", PaneID: "%1"}, {GUID: "g2", PaneID: "%2"}},
			live:                []LivePane{{ID: "%selvage", Dead: true}, {ID: "%1", Dead: true}, {ID: "%2", Dead: false}},
			selvagePaneID:       "%selvage",
			wantCleared:         []string{"g1"},
			wantDeadPanesToKill: []string{"%1"},
		},
		{
			name:        "OwnWindowStrandPresentAliveKeepsBinding",
			strands:     []Strand{ownWindowTestStrand("o", "%5")},
			windows:     []windowPane{{WindowID: "@3", Own: true, PaneID: "%5"}},
			wantCleared: nil,
		},
		{
			name:        "OwnWindowStrandPresentDeadKeepsBindingAndIsNeverKilled",
			strands:     []Strand{{GUID: "g1", PaneID: "%1"}, ownWindowTestStrand("o", "%5")},
			live:        []LivePane{{ID: "%1", Dead: false}},
			windows:     []windowPane{{WindowID: "@0", PaneID: "%1"}, {WindowID: "@3", Own: true, PaneID: "%5", Dead: true}},
			wantCleared: nil,
		},
		{
			name:        "OwnWindowStrandAbsentIsCleared",
			strands:     []Strand{ownWindowTestStrand("o", "%5")},
			wantCleared: []string{"o"},
		},
		{
			name:                "AliveOwnWindowPaneDoesNotSatisfyTheSurvivalRule",
			strands:             []Strand{{GUID: "g1", PaneID: "%1"}, {GUID: "g2", PaneID: "%2"}, ownWindowTestStrand("o", "%5")},
			live:                []LivePane{{ID: "%1", Dead: true}, {ID: "%2", Dead: true}},
			windows:             []windowPane{{WindowID: "@3", Own: true, PaneID: "%5"}},
			wantCleared:         []string{"g2"},
			wantDeadPanesToKill: []string{"%2"},
			wantSolePane:        "%1",
		},
		{
			name:                "OwnWindowWithNoBoundPaneIsEnded",
			windows:             []windowPane{{WindowID: "@4", Own: true, PaneID: "%8"}},
			wantOwnWindowsToEnd: []string{"@4"},
		},
		{
			name:    "WindowWithoutTheMarkerIsNeverEnded",
			windows: []windowPane{{WindowID: "@4", PaneID: "%8"}},
		},
		{
			name:    "OwnWindowHoldingABoundPaneIsKept",
			strands: []Strand{ownWindowTestStrand("o", "%5")},
			windows: []windowPane{{WindowID: "@3", Own: true, PaneID: "%5"}, {WindowID: "@3", Own: true, PaneID: "%6"}},
		},
		{
			name:      "OwnWindowStrandFoundInAnotherSessionKeepsItsBinding",
			strands:   []Strand{ownWindowTestStrand("o", "%5")},
			elsewhere: map[string]string{"%5": "other:2"},
		},
		{
			name:        "OwnWindowStrandAbsentFromEveryListingIsCleared",
			strands:     []Strand{ownWindowTestStrand("o", "%5")},
			elsewhere:   map[string]string{"%9": "other:2"},
			wantCleared: []string{"o"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := newReapPolicy(&ReedState{SelvagePaneID: tt.selvagePaneID}, tt.live)
			gotPlan := planReconcile(tt.strands, tt.live, tt.windows, policy, tt.elsewhere)
			if !slices.Equal(gotPlan.ownWindowsToEnd, tt.wantOwnWindowsToEnd) {
				t.Errorf("planReconcile() ownWindowsToEnd = %v, want %v", gotPlan.ownWindowsToEnd, tt.wantOwnWindowsToEnd)
			}
			if !slices.Equal(gotPlan.clearedGUIDs, tt.wantCleared) {
				t.Errorf("planReconcile() clearedGUIDs = %v, want %v", gotPlan.clearedGUIDs, tt.wantCleared)
			}
			if !slices.Equal(gotPlan.deadPanesToKill, tt.wantDeadPanesToKill) {
				t.Errorf("planReconcile() deadPanesToKill = %v, want %v", gotPlan.deadPanesToKill, tt.wantDeadPanesToKill)
			}
			if !slices.Equal(gotPlan.untrackedPanesToKill, tt.wantUntrackedPanesToKill) {
				t.Errorf("planReconcile() untrackedPanesToKill = %v, want %v", gotPlan.untrackedPanesToKill, tt.wantUntrackedPanesToKill)
			}
			if gotPlan.keptDeadPane != tt.wantSolePane {
				t.Errorf("planReconcile() keptDeadPane = %q, want %q", gotPlan.keptDeadPane, tt.wantSolePane)
			}
		})
	}
}

// findStrandPaneID returns the PaneID of the strand with the given GUID, or
// "" if no strand with that GUID is present.
func findStrandPaneID(strands []Strand, guid string) string {
	for _, s := range strands {
		if s.GUID == guid {
			return s.PaneID
		}
	}
	return ""
}

//testtiming:keep pins reconcileLocked mutating the real state record, clearing a gone strand's binding and keeping a present one's, with no tmux call; its covering tests run this code without asserting it
func TestReconcileLocked_NoDeadPanes_ClearsGoneBindingsWithoutTouchingTmux(t *testing.T) {
	tiergate.Require(t, 0)
	// cfg.Tmux points at a nonexistent binary (newTestEngine's fixture).
	// If reconcileLocked ever shelled out here, this test would fail loudly, not silently pass against a stray real server.
	e := newTestEngine(t)

	st := &ReedState{Strands: []Strand{
		{GUID: "gone", PaneID: "%9"},
		{GUID: "present", PaneID: "%1"},
	}}
	live := []LivePane{{ID: "%1", Dead: false}}

	killed, err := e.reconcileLocked(st, live, nil)
	if err != nil {
		t.Fatalf("reconcileLocked: %v", err)
	}
	if len(killed) != 0 {
		t.Errorf("killed = %v, want none (no dead panes in this fixture)", killed)
	}
	if got := findStrandPaneID(st.Strands, "gone"); got != "" {
		t.Errorf("gone strand PaneID = %q, want cleared", got)
	}
	if got := findStrandPaneID(st.Strands, "present"); got != "%1" {
		t.Errorf("present strand PaneID = %q, want kept", got)
	}
}

// TestReconcileLocked_KeepsABindingFoundInAnotherSession pins that reconcile keeps a strand's binding.
// When the server-wide listing finds an own-window strand's pane in another session, reconcile kills nothing.
// It clears the binding when that lookup fails.
func TestReconcileLocked_KeepsABindingFoundInAnotherSession(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name        string
		serverList  string
		serverErr   error
		wantPaneID  string
		wantLookups int
	}{
		{name: "FoundInAnotherSession", serverList: "%5 other 2\n%1 worktree 0\n", wantPaneID: "%5", wantLookups: 1},
		{name: "LookupFails", serverErr: errors.New("list-panes -a failed"), wantPaneID: "", wantLookups: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newTestEngine(t)
			fake := installFakeTmux(t, e)
			fake.answer(serverListVerb, tt.serverList, tt.serverErr)

			st := &ReedState{Strands: []Strand{ownWindowTestStrand("o", "%5")}}
			killed, err := e.reconcileLocked(st, nil, nil)
			if err != nil {
				t.Fatalf("reconcileLocked: %v", err)
			}
			if len(killed) != 0 {
				t.Errorf("killed = %v, want none", killed)
			}
			if got := findStrandPaneID(st.Strands, "o"); got != tt.wantPaneID {
				t.Errorf("strand PaneID = %q, want %q", got, tt.wantPaneID)
			}
			if got := fake.Count(serverListVerb); got != tt.wantLookups {
				t.Errorf("server-wide lookups = %d, want %d", got, tt.wantLookups)
			}
			if got := fake.Count("kill-pane"); got != 0 {
				t.Errorf("kill-pane calls = %d, want none", got)
			}
		})
	}
}

// TestReconcileLocked_LogsTheUntrackedPanesItReaps pins reconcileLocked's reap log line.
// The line is one Info line naming the panes that were destroyed,
// never the planned plan.deadPanesToKill or plan.untrackedPanesToKill.
// It is written on both the success path and the partial-kill error path, and never when nothing was killed.
func TestReconcileLocked_LogsTheUntrackedPanesItReaps(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("KillsUntrackedPanes_LogsTheirIDs", func(t *testing.T) {
		e := newTestEngine(t)
		installFakeTmux(t, e)
		buf := logcapture.CaptureVerbose(t)

		st := &ReedState{SelvagePaneID: "%selvage"}
		live := []LivePane{{ID: "%selvage", Dead: false}, {ID: "%orphan1", Dead: false}, {ID: "%orphan2", Dead: false}}

		killed, err := e.reconcileLocked(st, live, nil)
		if err != nil {
			t.Fatalf("reconcileLocked: %v", err)
		}
		if !slices.Equal(killed, []string{"%orphan1", "%orphan2"}) {
			t.Fatalf("killed = %v, want [%%orphan1 %%orphan2]", killed)
		}
		out := buf.String()
		for _, want := range []string{"%orphan1", "%orphan2"} {
			if !strings.Contains(out, want) {
				t.Errorf("log output = %q, want it to contain %q", out, want)
			}
		}
	})

	t.Run("KillsNothing_LogsNothing", func(t *testing.T) {
		e := newTestEngine(t)
		installFakeTmux(t, e)
		buf := logcapture.CaptureVerbose(t)

		st := &ReedState{}
		killed, err := e.reconcileLocked(st, nil, nil)
		if err != nil {
			t.Fatalf("reconcileLocked: %v", err)
		}
		if len(killed) != 0 {
			t.Fatalf("killed = %v, want none", killed)
		}
		if out := buf.String(); out != "" {
			t.Errorf("log output = %q, want empty (a reconcile that killed nothing must log nothing)", out)
		}
	})

	t.Run("PartialKillFailure_LogsOnlyTheDestroyedPaneAndStillReturnsTheError", func(t *testing.T) {
		e := newTestEngine(t)
		killPaneErr := errors.New("kill-pane failed")
		installFakeTmux(t, e).answerFunc("kill-pane", func(args []string) (string, error) {
			if len(args) >= 3 && args[2] == "%orphan2" {
				return "", killPaneErr
			}
			return "", nil
		})
		buf := logcapture.CaptureVerbose(t)

		st := &ReedState{SelvagePaneID: "%selvage"}
		live := []LivePane{{ID: "%selvage", Dead: false}, {ID: "%orphan1", Dead: false}, {ID: "%orphan2", Dead: false}}

		killed, err := e.reconcileLocked(st, live, nil)
		if err == nil {
			t.Fatal("reconcileLocked() err = nil, want the kill-pane failure to propagate")
		}
		if !errors.Is(err, killPaneErr) {
			t.Errorf("reconcileLocked() err = %v, want it to wrap %v", err, killPaneErr)
		}
		if !slices.Equal(killed, []string{"%orphan1"}) {
			t.Fatalf("killed = %v, want [%%orphan1] (only the pane destroyed before the failure)", killed)
		}

		out := buf.String()
		if !strings.Contains(out, "%orphan1") {
			t.Errorf("log output = %q, want it to contain the destroyed pane %q", out, "%orphan1")
		}
		if strings.Contains(out, "%orphan2") {
			t.Errorf("log output = %q, want it to NOT contain %q (that pane was scheduled but never actually destroyed)", out, "%orphan2")
		}
	})
}

// TestClearConflictingPaneBindings pins the repair of contradicting strand pane bindings in a corrupt reed.json:
// a strand naming Selvage's pane, or two strands naming one pane.
// Left in place, such bindings make `up` destroy unrelated live panes and their processes while reporting ok:true,
// and they report the strand live against a pane it does not own.
// The repair is first-writer-wins, so the table order of the cleared GUIDs is part of the contract.
//
//testtiming:keep pins the first-writer-wins repair of contradicting bindings: a strand naming Selvage's pane or sharing another strand's pane is cleared in table order, unbound strands never conflict and Selvage's own binding is never cleared; its covering tests run this code without asserting it
func TestClearConflictingPaneBindings(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name         string
		state        ReedState
		wantCleared  []string
		wantPaneByID map[string]string
	}{
		{
			name: "a healthy table is untouched",
			state: ReedState{
				SelvagePaneID: "%1",
				Strands:       []Strand{{GUID: "a", PaneID: "%2"}, {GUID: "b", PaneID: "%3"}},
			},
			wantCleared:  nil,
			wantPaneByID: map[string]string{"a": "%2", "b": "%3"},
		},
		{
			name: "a strand naming Selvage's pane is cleared",
			state: ReedState{
				SelvagePaneID: "%1",
				Strands:       []Strand{{GUID: "a", PaneID: "%1"}, {GUID: "b", PaneID: "%2"}},
			},
			wantCleared:  []string{"a"},
			wantPaneByID: map[string]string{"a": "", "b": "%2"},
		},
		{
			name: "the later of two strands sharing one pane is cleared",
			state: ReedState{
				SelvagePaneID: "%1",
				Strands:       []Strand{{GUID: "a", PaneID: "%2"}, {GUID: "b", PaneID: "%2"}},
			},
			wantCleared:  []string{"b"},
			wantPaneByID: map[string]string{"a": "%2", "b": ""},
		},
		{
			name: "unbound strands are never reported as conflicting with each other",
			state: ReedState{
				SelvagePaneID: "%1",
				Strands:       []Strand{{GUID: "a", PaneID: ""}, {GUID: "b", PaneID: ""}},
			},
			wantCleared:  nil,
			wantPaneByID: map[string]string{"a": "", "b": ""},
		},
		{
			name: "an absent Selvage claims nothing, so an empty SelvagePaneID clears no strand",
			state: ReedState{
				SelvagePaneID: "",
				Strands:       []Strand{{GUID: "a", PaneID: "%2"}, {GUID: "b", PaneID: "%3"}},
			},
			wantCleared:  nil,
			wantPaneByID: map[string]string{"a": "%2", "b": "%3"},
		},
		{
			name: "three strands on one pane clear all but the first",
			state: ReedState{
				SelvagePaneID: "%9",
				Strands:       []Strand{{GUID: "a", PaneID: "%4"}, {GUID: "b", PaneID: "%4"}, {GUID: "c", PaneID: "%4"}},
			},
			wantCleared:  []string{"b", "c"},
			wantPaneByID: map[string]string{"a": "%4", "b": "", "c": ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Copy the strand slice, not just the struct:
			// clearConflictingPaneBindings mutates elements in place,
			// and a shared backing array would let one subtest rewrite another subtest's fixture.
			st := tt.state
			st.Strands = append([]Strand(nil), tt.state.Strands...)
			got := clearConflictingPaneBindings(&st)
			if !slices.Equal(got, tt.wantCleared) {
				t.Errorf("clearConflictingPaneBindings() cleared = %v; want %v", got, tt.wantCleared)
			}
			for guid, want := range tt.wantPaneByID {
				if paneID := findStrandPaneID(st.Strands, guid); paneID != want {
					t.Errorf("strand %s PaneID = %q; want %q", guid, paneID, want)
				}
			}
			if st.SelvagePaneID != tt.state.SelvagePaneID {
				t.Errorf("SelvagePaneID = %q; want %q (the Selvage binding is never the one cleared)", st.SelvagePaneID, tt.state.SelvagePaneID)
			}
		})
	}
}
