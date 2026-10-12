// ownwindow_test.go pins the own-window group placement: the pure plan that chooses between a shared window and a window of its own, and the join of a pane into a group's shared window.

package reedengine

import (
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// TestPlanOwnWindowPlacement pins the placement and the group record planOwnWindowPlacement returns for each arrival in a group.
func TestPlanOwnWindowPlacement(t *testing.T) {
	t.Parallel()
	group := func(name string, size int) render.Display {
		return render.Display{Anchor: render.AnchorOwnWindow, Window: name, WindowSize: size}
	}
	shared2 := WindowGroup{Shared: true, Size: 2}
	tests := []struct {
		name        string
		groups      map[string]WindowGroup
		display     render.Display
		shareMax    int
		liveCount   int
		groupWindow string
		wantPlace   ownWindowPlacement
		wantRecord  WindowGroup
		wantErr     []string
	}{
		{name: "UngroupedIsApartWithNoRecord", display: group("", 0), shareMax: 2, wantPlace: placeApart},
		{name: "FirstOfAGroupOfTwoUnderTheCapStartsTheSharedWindow", display: group("pair", 2), shareMax: 2, wantPlace: placeNewShared, wantRecord: shared2},
		{name: "SecondOfAGroupOfTwoJoins", groups: map[string]WindowGroup{"pair": shared2}, display: group("pair", 2), shareMax: 2, liveCount: 1, groupWindow: "@4", wantPlace: placeJoinShared, wantRecord: shared2},
		{name: "EitherArrivalOrderEndsShared", display: group("pair", 2), shareMax: 2, liveCount: 1, groupWindow: "@4", wantPlace: placeJoinShared, wantRecord: shared2},
		{name: "GroupOfFiveIsApart", display: group("five", 5), shareMax: 2, wantPlace: placeApart, wantRecord: WindowGroup{Shared: false, Size: 5}},
		{name: "ShareMaxZeroIsApart", display: group("pair", 2), shareMax: 0, wantPlace: placeApart, wantRecord: WindowGroup{Shared: false, Size: 2}},
		{name: "ZeroSizeReadsAsOne", display: group("solo", 0), shareMax: 2, wantPlace: placeNewShared, wantRecord: WindowGroup{Shared: true, Size: 1}},
		{name: "SizeMismatchIsRefusedNamingBothSizes", groups: map[string]WindowGroup{"pair": shared2}, display: group("pair", 3), shareMax: 2, wantErr: []string{"size 2", "size 3"}},
		{name: "RecordedChoiceBeatsAChangedCap", groups: map[string]WindowGroup{"pair": shared2}, display: group("pair", 2), shareMax: 0, liveCount: 1, groupWindow: "@4", wantPlace: placeJoinShared, wantRecord: shared2},
		{name: "OverflowPastTheRecordedSizeIsApart", groups: map[string]WindowGroup{"pair": shared2}, display: group("pair", 2), shareMax: 2, liveCount: 2, groupWindow: "@4", wantPlace: placeApart, wantRecord: shared2},
		{name: "SharedRecordWithNoLiveWindowStartsIt", groups: map[string]WindowGroup{"pair": shared2}, display: group("pair", 2), shareMax: 2, wantPlace: placeNewShared, wantRecord: shared2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			place, record, err := planOwnWindowPlacement(tt.groups, tt.display, tt.shareMax, tt.liveCount, tt.groupWindow)
			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("planOwnWindowPlacement = nil error, want one containing %q", tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error = %q, want it to contain %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("planOwnWindowPlacement: %v", err)
			}
			if place != tt.wantPlace || record != tt.wantRecord {
				t.Errorf("planOwnWindowPlacement = (%d, %+v), want (%d, %+v)", place, record, tt.wantPlace, tt.wantRecord)
			}
		})
	}
}

// TestJoinGroupWindowLocked_RefusesASilentlyReusedPane pins that a split answering the group window's own pane is refused as in window 0, runs no layout, and that a genuine join splits the group window's first pane detached in the told direction.
func TestJoinGroupWindowLocked_RefusesASilentlyReusedPane(t *testing.T) {
	t.Parallel()
	windows := []windowPane{{WindowID: "@4", Own: true, Group: "pair", PaneID: "%5"}, {WindowID: "@0", PaneID: "%1"}}

	t.Run("ReusedPaneIsRefused", func(t *testing.T) {
		t.Parallel()
		e := newTestEngine(t)
		fake := installFakeTmux(t, e)
		fake.answer("split-window", "%5\n", nil)
		fake.mustNotCall("select-layout")

		if got, err := e.joinGroupWindowLocked("@4", windows, "-h"); err == nil {
			t.Errorf("joinGroupWindowLocked = %q, nil; want the split-guard error", got)
		}
	})

	t.Run("NewPaneIsReturnedFromADetachedSplit", func(t *testing.T) {
		t.Parallel()
		e := newTestEngine(t)
		fake := installFakeTmux(t, e)
		fake.answer("split-window", "%6\n", nil)

		got, err := e.joinGroupWindowLocked("@4", windows, "-h")
		if err != nil || got != "%6" {
			t.Fatalf("joinGroupWindowLocked = (%q, %v), want (%%6, nil)", got, err)
		}
		argv := fake.LastArgv("split-window")
		for _, want := range []string{"-d", "-h"} {
			if !slices.Contains(argv, want) {
				t.Errorf("split-window argv = %v, want it to carry %s", argv, want)
			}
		}
		if i := slices.Index(argv, "-t"); i == -1 || argv[i+1] != "%5" {
			t.Errorf("split-window argv = %v, want -t %%5", argv)
		}
	})
}
