// windowtarget_test.go covers the strand-window seam: which window it resolves for each shape of recorded state and session listing, against the scripted multiplexer fake.

package reedengine

import (
	"errors"
	"slices"
	"testing"
)

func TestStrandWindowTarget(t *testing.T) {
	t.Parallel()
	const session = "worktree"
	current := exactSessionWindowTarget(session)

	tests := []struct {
		name          string
		state         *ReedState
		listing       string
		listErr       error
		markers       string
		markersErr    error
		want          string
		wantErr       bool
		wantLists     int
		wantMarkLists int
	}{
		{
			name:      "SelvageWindowWins",
			state:     &ReedState{SelvagePaneID: "%1", Strands: []Strand{{PaneID: "%2"}}},
			listing:   "%2 @5\n%1 @4\n%9 @7\n",
			want:      "@4",
			wantLists: 1,
		},
		{
			name:      "DeadSelvageCorpseStillResolves",
			state:     &ReedState{SelvagePaneID: "%1"},
			listing:   "%1 @4\n%9 @7\n",
			want:      "@4",
			wantLists: 1,
		},
		{
			name:          "OwnWindowStrandPaneIsNotTheStrandsWindowAndReadsTheMarker",
			state:         &ReedState{Strands: []Strand{ownWindowTestStrand("o", "%2")}},
			listing:       "%2 @5\n",
			want:          current,
			wantLists:     0,
			wantMarkLists: 1,
		},
		{
			name:      "SelvageWindowWinsOverAnOwnWindowStrand",
			state:     &ReedState{SelvagePaneID: "%1", Strands: []Strand{ownWindowTestStrand("o", "%2")}},
			listing:   "%2 @5\n%1 @4\n",
			want:      "@4",
			wantLists: 1,
		},
		{
			name:          "NothingRecordedReadsTheMarkerAndFallsBackToTheCurrentWindow",
			state:         &ReedState{Strands: []Strand{{PaneID: ""}}},
			want:          current,
			wantMarkLists: 1,
		},
		{
			name:      "NilStateIsTheCurrentWindowWithoutARoundTrip",
			want:      current,
			wantLists: 0,
		},
		{
			name:          "RecordedPaneOfAnotherSessionCountsAsAbsent",
			state:         &ReedState{SelvagePaneID: "%1", Strands: []Strand{{PaneID: "%2"}}},
			listing:       "%7 @4\n",
			want:          current,
			wantLists:     1,
			wantMarkLists: 1,
		},
		{
			name:          "NoSelvageRecordedResolvesTheMarkedWindow",
			state:         &ReedState{Strands: []Strand{ownWindowTestStrand("o", "%2")}},
			markers:       "@2 0\n@4 1\n@6 1\n",
			want:          "@4",
			wantMarkLists: 1,
		},
		{
			name:          "UnlistedSelvageResolvesTheMarkedWindow",
			state:         &ReedState{SelvagePaneID: "%1"},
			listing:       "%7 @9\n",
			markers:       "@2 0\n@4 1\n",
			want:          "@4",
			wantLists:     1,
			wantMarkLists: 1,
		},
		{
			name:          "NoMarkedWindowFallsBackToTheCurrentWindow",
			state:         &ReedState{},
			markers:       "@2 0\n@4 0\n",
			want:          current,
			wantMarkLists: 1,
		},
		{
			name:          "MarkerListingFailureIsReturned",
			state:         &ReedState{},
			markersErr:    errors.New("no server running"),
			wantErr:       true,
			wantMarkLists: 1,
		},
		{
			name:      "ListingFailureIsReturned",
			state:     &ReedState{SelvagePaneID: "%1"},
			listErr:   errors.New("no server running"),
			wantErr:   true,
			wantLists: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newTestEngine(t)
			fake := installFakeTmux(t, e)
			fake.answer(sessionListVerb, tt.listing, tt.listErr)
			fake.answer(markerListVerb, tt.markers, tt.markersErr)

			got, err := e.tmux.strandWindowTarget(session, tt.state)
			if (err != nil) != tt.wantErr {
				t.Fatalf("strandWindowTarget() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("strandWindowTarget() = %q, want %q", got, tt.want)
			}
			if lists := fake.Count(sessionListVerb); lists != tt.wantLists {
				t.Errorf("session listings = %d, want %d", lists, tt.wantLists)
			}
			if lists := fake.Count(markerListVerb); lists != tt.wantMarkLists {
				t.Errorf("marker listings = %d, want %d", lists, tt.wantMarkLists)
			}
			if tt.wantMarkLists > 0 {
				want := []string{"list-windows", "-t", exactSessionTarget(session), "-F", strandsMarkerFormat}
				if argv := fake.LastArgv(markerListVerb); !slices.Equal(argv, want) {
					t.Errorf("marker listing argv = %v, want %v", argv, want)
				}
			}
			if tt.wantLists == 0 {
				return
			}
			want := []string{"list-panes", "-s", "-t", exactSessionTarget(session), "-F", sessionPaneWindowFormat}
			if argv := fake.LastArgv(sessionListVerb); !slices.Equal(argv, want) {
				t.Errorf("listing argv = %v, want %v", argv, want)
			}
		})
	}
}
