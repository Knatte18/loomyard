// reconcile_test.go table-tests planReconcile's pure decision logic against saved strand tables and
// fake list-panes results (including pane_dead=1 rows and the header-pane exemption),
// and exercises reconcileLocked's real-record mutation for the no-dead-panes path, which never
// touches tmux and so stays hermetic.

package reedengine

import "testing"

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestPlanReconcile(t *testing.T) {
	tests := []struct {
		name                     string
		strands                  []Strand
		live                     []LivePane
		headerPaneID             string
		wantCleared              []string
		wantDeadPanesToKill      []string
		wantUntrackedPanesToKill []string
		wantSolePane             string
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
			// Every pane is dead: one must be spared so the session survives
			// (killing the last pane ends it). The first dead pane is kept
			// (binding stays), the rest are killed and their bindings cleared.
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
			// A live pane no strand owns (operator split / mid-op-crash
			// orphan) is killed deterministically while a strand is bound to
			// a present pane — never left to select-layout's positional
			// reaping, which can destroy a tracked pane instead.
			name:                     "UntrackedAlivePaneKilledWhileBoundContentPresent",
			strands:                  []Strand{{GUID: "g1", PaneID: "%1"}},
			live:                     []LivePane{{ID: "%1", Dead: false}, {ID: "%7", Dead: false}},
			wantCleared:              nil,
			wantUntrackedPanesToKill: []string{"%7"},
		},
		{
			// With no strand bound to any present pane and no alive header
			// (headerPaneID unset, so headerAlive is false), reed has
			// nothing to lay out and leaves foreign panes strictly alone
			// (the apply is skipped too — anyPlacedStrand). This is the
			// absent-header shape of "nothing authorizes the reap"; see
			// HeaderAloneNeverMakesAnyBoundPresentTrue and the two
			// headerAlive-false-with-a-live-header cases below for the other
			// shapes.
			name:        "UntrackedPanesUntouchedWhenNoHeaderAndNothingBound",
			strands:     []Strand{{GUID: "cleared", PaneID: ""}},
			live:        []LivePane{{ID: "%7", Dead: false}, {ID: "%8", Dead: false}},
			wantCleared: nil,
		},
		{
			// The header pane must never be reaped as an "untracked" pane
			// even while a strand is bound and anyBoundPresent is true —
			// exemptPaneIDs (boundPaneIDs plus the header) is what protects
			// it, distinct from boundPaneIDs itself (which must stay
			// strand-only so anyBoundPresent is never inflated by a merely
			// live header). This shape also covers "alive header alongside a
			// bound strand" for the headerAlive disjunct: the reap already
			// fires via anyBoundPresent here, so headerAlive contributes
			// nothing new to this case, and no separate case is needed for
			// it.
			name:         "HeaderPaneNeverReapedAsUntrackedWhileStrandBound",
			strands:      []Strand{{GUID: "g1", PaneID: "%1"}},
			live:         []LivePane{{ID: "%1", Dead: false}, {ID: "%header", Dead: false}, {ID: "%7", Dead: false}},
			headerPaneID: "%header",
			wantCleared:  nil,
			// %7 is a genuine foreign pane and is still reaped; %header is
			// exempt and must not appear here.
			wantUntrackedPanesToKill: []string{"%7"},
		},
		{
			// The header is alive and no strand is bound to any present
			// pane: anyBoundPresent stays false (derived from boundPaneIDs
			// alone, never the header — folding the header in would wrongly
			// flip it), but headerAlive is true, so the untracked reap fires
			// from that disjunct alone and %7 is killed while the header
			// itself stays exempt.
			name:                     "HeaderAloneNeverMakesAnyBoundPresentTrue",
			strands:                  []Strand{{GUID: "cleared", PaneID: ""}},
			live:                     []LivePane{{ID: "%header", Dead: false}, {ID: "%7", Dead: false}},
			headerPaneID:             "%header",
			wantCleared:              nil,
			wantUntrackedPanesToKill: []string{"%7"},
		},
		{
			// An alive header with zero strands and one untracked alive
			// pane: that pane is killed as an untracked kill (not a dead-pane
			// kill) and the header itself is spared.
			name:                     "AliveHeaderNoStrandsReapsOneUntrackedPane",
			strands:                  nil,
			live:                     []LivePane{{ID: "%header", Dead: false}, {ID: "%orphan", Dead: false}},
			headerPaneID:             "%header",
			wantCleared:              nil,
			wantUntrackedPanesToKill: []string{"%orphan"},
		},
		{
			// An alive header with zero strands and several untracked panes
			// (an old header pane plus an orphaned strand pane, M22's
			// shape): all of them are killed and the current header is
			// spared.
			name:    "AliveHeaderNoStrandsReapsSeveralUntrackedPanes",
			strands: nil,
			live: []LivePane{
				{ID: "%header", Dead: false},
				{ID: "%oldheader", Dead: false},
				{ID: "%orphanstrand", Dead: false},
			},
			headerPaneID:             "%header",
			wantCleared:              nil,
			wantUntrackedPanesToKill: []string{"%oldheader", "%orphanstrand"},
		},
		{
			// A header present but Dead: true, with no strand bound and one
			// alive untracked pane: headerAlive is false (the header entry
			// is present but dead), so nothing is reaped.
			name:         "PresentButDeadHeaderDoesNotAuthorizeReap",
			strands:      nil,
			live:         []LivePane{{ID: "%header", Dead: true}, {ID: "%orphan", Dead: false}},
			headerPaneID: "%header",
			wantCleared:  nil,
		},
		{
			// A non-empty headerPaneID naming no entry in live at all, with
			// no strand bound and one alive untracked pane: headerAlive's
			// third way of being false, distinct from the empty-id and
			// present-but-dead cases above. Reachable on the add path once
			// an operator kills the header pane outright, since no verb but
			// up/resume rebuilds it.
			name:         "HeaderIDNamingNoLivePaneDoesNotAuthorizeReap",
			strands:      nil,
			live:         []LivePane{{ID: "%orphan", Dead: false}},
			headerPaneID: "%header",
			wantCleared:  nil,
		},
		{
			// A dead header alongside a strand bound to a present pane: the
			// reap fires anyway via anyBoundPresent, and the header corpse
			// is still spared.
			name:                     "DeadHeaderAlongsideBoundStrandStillReapsViaAnyBoundPresent",
			strands:                  []Strand{{GUID: "g1", PaneID: "%1"}},
			live:                     []LivePane{{ID: "%header", Dead: true}, {ID: "%1", Dead: false}, {ID: "%7", Dead: false}},
			headerPaneID:             "%header",
			wantCleared:              nil,
			wantUntrackedPanesToKill: []string{"%7"},
		},
		{
			// A DEAD header pane must not be scheduled for killing either —
			// the dead-pane kill loop, not only the untracked reap, spares
			// it. Nothing outside up/resume rebuilds a header, so killing
			// the corpse here would leave every intermediate add/remove
			// headerless with a stale HeaderPaneID (the fable-header-r1
			// layout-scramble-then-wedged-up defect). The kept corpse stays
			// enumerable; ensureHeaderPaneLocked heals it at the next boot.
			name:         "DeadHeaderPaneKeptNotKilled",
			strands:      []Strand{{GUID: "g1", PaneID: "%1"}},
			live:         []LivePane{{ID: "%header", Dead: true}, {ID: "%1", Dead: false}},
			headerPaneID: "%header",
			wantCleared:  nil,
		},
		{
			// A dead header alongside a dead strand pane: the strand corpse
			// is still killable business-as-usual (an alive pane remains),
			// while the header corpse stays exempt.
			name:                "DeadHeaderExemptWhileDeadStrandPaneStillKilled",
			strands:             []Strand{{GUID: "g1", PaneID: "%1"}, {GUID: "g2", PaneID: "%2"}},
			live:                []LivePane{{ID: "%header", Dead: true}, {ID: "%1", Dead: true}, {ID: "%2", Dead: false}},
			headerPaneID:        "%header",
			wantCleared:         []string{"g1"},
			wantDeadPanesToKill: []string{"%1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPlan := planReconcile(tt.strands, tt.live, tt.headerPaneID)
			if !equalStringSlices(gotPlan.clearedGUIDs, tt.wantCleared) {
				t.Errorf("planReconcile() clearedGUIDs = %v, want %v", gotPlan.clearedGUIDs, tt.wantCleared)
			}
			if !equalStringSlices(gotPlan.deadPanesToKill, tt.wantDeadPanesToKill) {
				t.Errorf("planReconcile() deadPanesToKill = %v, want %v", gotPlan.deadPanesToKill, tt.wantDeadPanesToKill)
			}
			if !equalStringSlices(gotPlan.untrackedPanesToKill, tt.wantUntrackedPanesToKill) {
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

func TestReconcileLocked_NoDeadPanes_ClearsGoneBindingsWithoutTouchingTmux(t *testing.T) {
	// cfg.Tmux points at a nonexistent binary (newTestEngine's fixture): if
	// reconcileLocked ever shelled out here, this test would fail loudly
	// rather than silently passing against a stray real server.
	e := newTestEngine(t)

	st := &ReedState{Strands: []Strand{
		{GUID: "gone", PaneID: "%9"},
		{GUID: "present", PaneID: "%1"},
	}}
	live := []LivePane{{ID: "%1", Dead: false}}

	killed, err := e.reconcileLocked(st, live)
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

// TestClearConflictingPaneBindings is the regression guard for the R5 review's R5-F3: a corrupt
// reed.json whose strand pane bindings contradict each other (a strand naming the header's pane, or
// two strands naming one pane) made `up` destroy unrelated live panes and their processes while
// reporting ok:true, then report the strand live against a pane it does not own.
// The repair is first-writer-wins, so the table order of the cleared GUIDs is part of the contract,
// not an implementation detail.
func TestClearConflictingPaneBindings(t *testing.T) {
	tests := []struct {
		name         string
		state        ReedState
		wantCleared  []string
		wantPaneByID map[string]string
	}{
		{
			name: "a healthy table is untouched",
			state: ReedState{
				HeaderPaneID: "%1",
				Strands:      []Strand{{GUID: "a", PaneID: "%2"}, {GUID: "b", PaneID: "%3"}},
			},
			wantCleared:  nil,
			wantPaneByID: map[string]string{"a": "%2", "b": "%3"},
		},
		{
			name: "a strand naming the header pane is cleared",
			state: ReedState{
				HeaderPaneID: "%1",
				Strands:      []Strand{{GUID: "a", PaneID: "%1"}, {GUID: "b", PaneID: "%2"}},
			},
			wantCleared:  []string{"a"},
			wantPaneByID: map[string]string{"a": "", "b": "%2"},
		},
		{
			name: "the later of two strands sharing one pane is cleared",
			state: ReedState{
				HeaderPaneID: "%1",
				Strands:      []Strand{{GUID: "a", PaneID: "%2"}, {GUID: "b", PaneID: "%2"}},
			},
			wantCleared:  []string{"b"},
			wantPaneByID: map[string]string{"a": "%2", "b": ""},
		},
		{
			name: "unbound strands are never reported as conflicting with each other",
			state: ReedState{
				HeaderPaneID: "%1",
				Strands:      []Strand{{GUID: "a", PaneID: ""}, {GUID: "b", PaneID: ""}},
			},
			wantCleared:  nil,
			wantPaneByID: map[string]string{"a": "", "b": ""},
		},
		{
			name: "an absent header claims nothing, so an empty HeaderPaneID clears no strand",
			state: ReedState{
				HeaderPaneID: "",
				Strands:      []Strand{{GUID: "a", PaneID: "%2"}, {GUID: "b", PaneID: "%3"}},
			},
			wantCleared:  nil,
			wantPaneByID: map[string]string{"a": "%2", "b": "%3"},
		},
		{
			name: "three strands on one pane clear all but the first",
			state: ReedState{
				HeaderPaneID: "%9",
				Strands:      []Strand{{GUID: "a", PaneID: "%4"}, {GUID: "b", PaneID: "%4"}, {GUID: "c", PaneID: "%4"}},
			},
			wantCleared:  []string{"b", "c"},
			wantPaneByID: map[string]string{"a": "%4", "b": "", "c": ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Copy the strand slice, not just the struct: clearConflictingPaneBindings mutates
			// elements in place, and a shared backing array would let one subtest rewrite another
			// subtest's fixture.
			st := tt.state
			st.Strands = append([]Strand(nil), tt.state.Strands...)
			got := clearConflictingPaneBindings(&st)
			if !equalStringSlices(got, tt.wantCleared) {
				t.Errorf("clearConflictingPaneBindings() cleared = %v; want %v", got, tt.wantCleared)
			}
			for guid, want := range tt.wantPaneByID {
				if paneID := findStrandPaneID(st.Strands, guid); paneID != want {
					t.Errorf("strand %s PaneID = %q; want %q", guid, paneID, want)
				}
			}
			if st.HeaderPaneID != tt.state.HeaderPaneID {
				t.Errorf("HeaderPaneID = %q; want %q (the header binding is never the one cleared)", st.HeaderPaneID, tt.state.HeaderPaneID)
			}
		})
	}
}
