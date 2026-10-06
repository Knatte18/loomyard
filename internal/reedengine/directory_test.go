// directory_test.go pins the name directory on a fake tmux: which strands map to which rows, and that a cold session reports every row dormant.

package reedengine

import (
	"reflect"
	"testing"
)

func TestDirectory_MapsEachStrandToItsRow(t *testing.T) {
	t.Parallel()

	strands := []Strand{
		{GUID: "g1", Name: "tc:tslug:worker", Worktree: "tslug", PaneID: "%1"},
		{GUID: "g2", Name: "tc:tslug:worker-2", Worktree: "tslug", PaneID: "%2"},
		{GUID: "g3", Name: "tc:tslug:dead", Worktree: "tslug", PaneID: "%3"},
		{GUID: "g4", Name: "tc:tslug:unbound", Worktree: "tslug"},
	}
	live := []LivePane{
		{ID: "%1", Title: "tc:tslug:worker"},
		{ID: "%2", Title: "hand-set"},
		{ID: "%3", Title: "tc:tslug:dead", Dead: true},
	}
	e, fake := newRepairTestEngine(t, strands, live)

	got, err := e.Directory()
	if err != nil {
		t.Fatalf("Directory: %v", err)
	}
	want := []DirectoryRow{
		{Name: "tc:tslug:worker", GUID: "g1", Worktree: "tslug", PaneID: "%1", Title: "tc:tslug:worker", Live: true},
		{Name: "tc:tslug:worker-2", GUID: "g2", Worktree: "tslug", PaneID: "%2", Title: "hand-set", Live: true, Drift: true},
		{Name: "tc:tslug:dead", GUID: "g3", Worktree: "tslug", PaneID: "%3"},
		{Name: "tc:tslug:unbound", GUID: "g4", Worktree: "tslug"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Directory rows:\n got %+v\nwant %+v", got, want)
	}
	// The drifted title of g2 is reported, never repaired.
	if n := fake.Count("select-pane"); n != 0 {
		t.Errorf("Directory issued %d select-pane calls; want none", n)
	}

	// A cold session has no live panes: every row is dormant.
	wantCold := make([]DirectoryRow, len(strands))
	for i, s := range strands {
		wantCold[i] = DirectoryRow{Name: s.Name, GUID: s.GUID, Worktree: s.Worktree, PaneID: s.PaneID}
	}
	if gotCold := directoryRows(strands, nil); !reflect.DeepEqual(gotCold, wantCold) {
		t.Errorf("cold rows:\n got %+v\nwant %+v", gotCold, wantCold)
	}
}
