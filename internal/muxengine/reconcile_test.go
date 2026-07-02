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
		name           string
		strands        []Strand
		live           []LivePane
		wantCleared    []string
		wantDeadToKill []string
		wantSolePane   string
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
			name:           "NonSoleDeadPaneScheduledForKillAndBindingCleared",
			strands:        []Strand{{GUID: "g1", PaneID: "%1"}, {GUID: "g2", PaneID: "%2"}},
			live:           []LivePane{{ID: "%1", Dead: true}, {ID: "%2", Dead: false}},
			wantCleared:    []string{"g1"},
			wantDeadToKill: []string{"%1"},
		},
		{
			name:         "SoleRemainingDeadPaneKeptAndNotScheduledForKill",
			strands:      []Strand{{GUID: "g1", PaneID: "%1"}},
			live:         []LivePane{{ID: "%1", Dead: true}},
			wantCleared:  nil,
			wantSolePane: "%1",
		},
		{
			name: "TwoDeadPanesNeitherIsSole",
			strands: []Strand{
				{GUID: "g1", PaneID: "%1"},
				{GUID: "g2", PaneID: "%2"},
			},
			live:           []LivePane{{ID: "%1", Dead: true}, {ID: "%2", Dead: true}},
			wantCleared:    []string{"g1", "g2"},
			wantDeadToKill: []string{"%1", "%2"},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCleared, gotDead, gotSole := planReconcile(tt.strands, tt.live)
			if !equalStringSlices(gotCleared, tt.wantCleared) {
				t.Errorf("planReconcile() clearedGUIDs = %v, want %v", gotCleared, tt.wantCleared)
			}
			if !equalStringSlices(gotDead, tt.wantDeadToKill) {
				t.Errorf("planReconcile() deadToKill = %v, want %v", gotDead, tt.wantDeadToKill)
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
