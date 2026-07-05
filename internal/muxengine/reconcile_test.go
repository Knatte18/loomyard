// reconcile_test.go table-tests planReconcile's pure decision logic against
// saved strand tables and fake list-panes results (including pane_dead=1
// rows), and exercises reconcileLocked's real-record mutation for the
// no-dead-panes path, which never touches psmux and so stays hermetic.

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCleared, gotKill, gotSole := planReconcile(tt.strands, tt.live)
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

func TestReconcileLocked_NoDeadPanes_ClearsGoneBindingsWithoutTouchingPsmux(t *testing.T) {
	// cfg.Psmux points at a nonexistent binary (newTestEngine's fixture): if
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
