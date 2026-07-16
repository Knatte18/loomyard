// reconcile_test.go table-tests planReconcile's pure decision logic against
// saved strand tables and fake list-panes results (including pane_dead=1
// rows and the header-pane exemption), and exercises reconcileLocked's
// real-record mutation for the no-dead-panes path, which never touches
// tmux and so stays hermetic.

package muxengine

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
		name            string
		strands         []Strand
		live            []LivePane
		headerPaneID    string
		wantCleared     []string
		wantPanesToKill []string
		wantSolePane    string
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
			name:            "NonSoleDeadPaneScheduledForKillAndBindingCleared",
			strands:         []Strand{{GUID: "g1", PaneID: "%1"}, {GUID: "g2", PaneID: "%2"}},
			live:            []LivePane{{ID: "%1", Dead: true}, {ID: "%2", Dead: false}},
			wantCleared:     []string{"g1"},
			wantPanesToKill: []string{"%1"},
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
			live:            []LivePane{{ID: "%1", Dead: true}, {ID: "%2", Dead: true}},
			wantCleared:     []string{"g2"},
			wantPanesToKill: []string{"%2"},
			wantSolePane:    "%1",
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
			name:            "UntrackedAlivePaneKilledWhileBoundContentPresent",
			strands:         []Strand{{GUID: "g1", PaneID: "%1"}},
			live:            []LivePane{{ID: "%1", Dead: false}, {ID: "%7", Dead: false}},
			wantCleared:     nil,
			wantPanesToKill: []string{"%7"},
		},
		{
			// With NO strand bound to any present pane, mux has nothing to
			// lay out and leaves foreign panes strictly alone (the apply is
			// skipped too — anyPlacedStrand).
			name:        "UntrackedPanesUntouchedWhenNothingBound",
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
			// live header).
			name:         "HeaderPaneNeverReapedAsUntrackedWhileStrandBound",
			strands:      []Strand{{GUID: "g1", PaneID: "%1"}},
			live:         []LivePane{{ID: "%1", Dead: false}, {ID: "%header", Dead: false}, {ID: "%7", Dead: false}},
			headerPaneID: "%header",
			wantCleared:  nil,
			// %7 is a genuine foreign pane and is still reaped; %header is
			// exempt and must not appear here.
			wantPanesToKill: []string{"%7"},
		},
		{
			// With the header live but NO strand bound to any present pane,
			// anyBoundPresent must stay false (derived from boundPaneIDs
			// alone, never the header) so foreign panes are left untouched —
			// folding the header into boundPaneIDs would wrongly flip this.
			name:         "HeaderAloneNeverMakesAnyBoundPresentTrue",
			strands:      []Strand{{GUID: "cleared", PaneID: ""}},
			live:         []LivePane{{ID: "%header", Dead: false}, {ID: "%7", Dead: false}},
			headerPaneID: "%header",
			wantCleared:  nil,
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
			name:            "DeadHeaderExemptWhileDeadStrandPaneStillKilled",
			strands:         []Strand{{GUID: "g1", PaneID: "%1"}, {GUID: "g2", PaneID: "%2"}},
			live:            []LivePane{{ID: "%header", Dead: true}, {ID: "%1", Dead: true}, {ID: "%2", Dead: false}},
			headerPaneID:    "%header",
			wantCleared:     []string{"g1"},
			wantPanesToKill: []string{"%1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCleared, gotKill, gotSole := planReconcile(tt.strands, tt.live, tt.headerPaneID)
			if !equalStringSlices(gotCleared, tt.wantCleared) {
				t.Errorf("planReconcile() clearedGUIDs = %v, want %v", gotCleared, tt.wantCleared)
			}
			if !equalStringSlices(gotKill, tt.wantPanesToKill) {
				t.Errorf("planReconcile() panesToKill = %v, want %v", gotKill, tt.wantPanesToKill)
			}
			if gotSole != tt.wantSolePane {
				t.Errorf("planReconcile() solePane = %q, want %q", gotSole, tt.wantSolePane)
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

	st := &MuxState{Strands: []Strand{
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
