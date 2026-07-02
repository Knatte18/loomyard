// spawn_test.go table-tests planLaunch's adopt-vs-split decision, and
// verifies loadOrInitStateLocked's fresh-worktree bootstrap — both pure/
// hermetic, no live psmux required. launchStrandLocked itself always makes
// a real psmux round trip (activePaneID/split-window + send-keys), so it is
// exercised only through this decision seam, not invoked directly here.

package muxengine

import "testing"

func TestPlanLaunch(t *testing.T) {
	tests := []struct {
		name    string
		strands []Strand
		want    bool
	}{
		{
			name:    "EmptyTable_AdoptsNewSessionPane",
			strands: nil,
			want:    true,
		},
		{
			name:    "AllStrandsPaneless_AdoptsNewSessionPane",
			strands: []Strand{{GUID: "a"}, {GUID: "b"}},
			want:    true,
		},
		{
			name:    "OneStrandHoldsAPane_SplitsInstead",
			strands: []Strand{{GUID: "a", PaneID: "%1"}, {GUID: "b"}},
			want:    false,
		},
		{
			name:    "EveryStrandHoldsAPane_SplitsInstead",
			strands: []Strand{{GUID: "a", PaneID: "%1"}, {GUID: "b", PaneID: "%2"}},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &MuxState{Strands: tt.strands}
			if got := planLaunch(st); got != tt.want {
				t.Errorf("planLaunch(%+v) = %v, want %v", tt.strands, got, tt.want)
			}
		})
	}
}

func TestLoadOrInitStateLocked_AbsentFileInitializesFromEngineIdentity(t *testing.T) {
	e := newTestEngine(t)

	st, err := e.loadOrInitStateLocked()
	if err != nil {
		t.Fatalf("loadOrInitStateLocked: %v", err)
	}
	if st == nil {
		t.Fatal("loadOrInitStateLocked() = nil, want a fresh MuxState")
	}
	if st.Server != e.Socket() || st.Socket != e.Socket() {
		t.Errorf("fresh state Server/Socket = %q/%q, want %q", st.Server, st.Socket, e.Socket())
	}
	if st.Session != e.SessionName() {
		t.Errorf("fresh state Session = %q, want %q", st.Session, e.SessionName())
	}
	if len(st.Strands) != 0 {
		t.Errorf("fresh state Strands = %v, want empty", st.Strands)
	}
}

func TestLoadOrInitStateLocked_ExistingFileLoadsVerbatim(t *testing.T) {
	e := newTestEngine(t)

	want := &MuxState{
		Server:  "some-other-server",
		Socket:  "some-other-server",
		Session: "some-other-session",
		Strands: []Strand{{GUID: "g1", PaneID: "%1"}},
	}
	if err := SaveState(e.layout.DotLyxDir(), want); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	st, err := e.loadOrInitStateLocked()
	if err != nil {
		t.Fatalf("loadOrInitStateLocked: %v", err)
	}
	if st.Server != want.Server || st.Session != want.Session {
		t.Errorf("loadOrInitStateLocked() = %+v, want the persisted state, not a freshly initialized one", st)
	}
	if len(st.Strands) != 1 || st.Strands[0].GUID != "g1" {
		t.Errorf("loadOrInitStateLocked() Strands = %+v, want the persisted strand", st.Strands)
	}
}
