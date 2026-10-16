// listreconcile_test.go covers the reconcile listing that breaks strand panes left in the shell window out into windows of their own, against the scripted multiplexer fake.

package reedengine

import (
	"errors"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// TestListReconcilePanesLocked_MovesShellWindowStrands drives the listing the three reconcile call sites share:
// list-panes answers the pre-move listing until a break-pane succeeds and the post-move listing after.
func TestListReconcilePanesLocked_MovesShellWindowStrands(t *testing.T) {
	t.Parallel()
	type breakAnswer struct {
		out string
		err error
	}
	bound := func(guid, paneID string) Strand {
		// The anchor and group an older binary left; a moved strand reads own-window with no group afterwards.
		return Strand{GUID: guid, Name: guid, PaneID: paneID, Display: render.Display{Anchor: render.Anchor("legacy"), Window: "g"}}
	}
	pane := func(id string) LivePane { return LivePane{ID: id} }

	tests := []struct {
		name      string
		selvage   string
		strands   []Strand
		pre       []LivePane
		post      []LivePane
		markers   string
		breakFail map[string]breakAnswer
		optionErr error
		relistErr error
		firstErr  error

		wantBreaks    []string
		wantMoved     []string
		wantPaneLists int
		wantErr       bool
		// wantLive is the pane ids the call returns.
		wantLive []string
	}{
		{
			name:          "SelvagePlusTwoBoundPanesMovesBoth",
			selvage:       "%1",
			strands:       []Strand{bound("a", "%2"), bound("b", "%3")},
			pre:           []LivePane{pane("%1"), pane("%2"), pane("%3")},
			post:          []LivePane{pane("%1")},
			wantBreaks:    []string{"%2", "%3"},
			wantMoved:     []string{"a", "b"},
			wantPaneLists: 2,
			wantLive:      []string{"%1"},
		},
		{
			name:          "UnboundPaneBesideSelvageMovesNothing",
			selvage:       "%1",
			strands:       []Strand{bound("a", "%5")},
			pre:           []LivePane{pane("%1"), pane("%9")},
			wantPaneLists: 1,
			wantLive:      []string{"%1", "%9"},
		},
		{
			name:          "DeadBoundPaneBesideSelvageMoves",
			selvage:       "%1",
			strands:       []Strand{bound("a", "%2")},
			pre:           []LivePane{pane("%1"), {ID: "%2", Dead: true}},
			post:          []LivePane{pane("%1")},
			wantBreaks:    []string{"%2"},
			wantMoved:     []string{"a"},
			wantPaneLists: 2,
			wantLive:      []string{"%1"},
		},
		{
			name:          "SoleBoundPaneWithoutSelvageStays",
			strands:       []Strand{bound("a", "%2")},
			pre:           []LivePane{pane("%2")},
			markers:       "@0 1\n",
			wantPaneLists: 1,
			wantLive:      []string{"%2"},
		},
		{
			name:          "TwoBoundPanesWithoutSelvageMoveTheFirstOnly",
			strands:       []Strand{bound("a", "%2"), bound("b", "%3")},
			pre:           []LivePane{pane("%2"), pane("%3")},
			post:          []LivePane{pane("%3")},
			markers:       "@0 1\n",
			wantBreaks:    []string{"%2"},
			wantMoved:     []string{"a"},
			wantPaneLists: 2,
			wantLive:      []string{"%3"},
		},
		{
			name:          "NoSelvageAndNoMarkedWindowMovesNothing",
			strands:       []Strand{bound("a", "%2"), bound("b", "%3")},
			pre:           []LivePane{pane("%2"), pane("%3")},
			wantPaneLists: 1,
			wantLive:      []string{"%2", "%3"},
		},
		{
			name:    "FailedBreakLeavesItsPaneAndTriesTheRest",
			selvage: "%1",
			strands: []Strand{bound("a", "%2"), bound("b", "%3")},
			pre:     []LivePane{pane("%1"), pane("%2"), pane("%3")},
			post:    []LivePane{pane("%1"), pane("%2")},
			breakFail: map[string]breakAnswer{
				"%2": {err: errors.New("cannot break")},
			},
			wantBreaks:    []string{"%2", "%3"},
			wantMoved:     []string{"b"},
			wantPaneLists: 2,
			wantLive:      []string{"%1", "%2"},
		},
		{
			name:    "EveryBreakFailingNeverRelists",
			selvage: "%1",
			strands: []Strand{bound("a", "%2"), bound("b", "%3")},
			pre:     []LivePane{pane("%1"), pane("%2"), pane("%3")},
			breakFail: map[string]breakAnswer{
				"%2": {err: errors.New("cannot break")},
				"%3": {err: errors.New("cannot break")},
			},
			wantBreaks:    []string{"%2", "%3"},
			wantPaneLists: 1,
			wantLive:      []string{"%1", "%2", "%3"},
		},
		{
			name:    "MalformedBreakAnswerCountsAsFailure",
			selvage: "%1",
			strands: []Strand{bound("a", "%2")},
			pre:     []LivePane{pane("%1"), pane("%2")},
			breakFail: map[string]breakAnswer{
				"%2": {out: "no window here\n"},
			},
			wantBreaks:    []string{"%2"},
			wantPaneLists: 1,
			wantLive:      []string{"%1", "%2"},
		},
		{
			name:          "FailedOwnMarkerStillRewritesTheAnchor",
			selvage:       "%1",
			strands:       []Strand{bound("a", "%2")},
			pre:           []LivePane{pane("%1"), pane("%2")},
			post:          []LivePane{pane("%1")},
			optionErr:     errors.New("cannot set"),
			wantBreaks:    []string{"%2"},
			wantMoved:     []string{"a"},
			wantPaneLists: 2,
			wantLive:      []string{"%1"},
		},
		{
			name:          "FailedFirstListingIsReturned",
			selvage:       "%1",
			strands:       []Strand{bound("a", "%2")},
			firstErr:      errors.New("no server running"),
			wantPaneLists: 1,
			wantErr:       true,
		},
		{
			name:          "FailedRelistAfterAMoveIsReturned",
			selvage:       "%1",
			strands:       []Strand{bound("a", "%2")},
			pre:           []LivePane{pane("%1"), pane("%2")},
			relistErr:     errors.New("no server running"),
			wantBreaks:    []string{"%2"},
			wantMoved:     []string{"a"},
			wantPaneLists: 2,
			wantErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newTestEngine(t)
			fake := installFakeTmux(t, e)
			fake.answer("list-panes", encodeLivePanes(tt.pre), tt.firstErr)
			fake.answer(markerListVerb, tt.markers, nil)
			if tt.optionErr != nil {
				fake.answer("set-option", "", tt.optionErr)
			}
			fake.answerFunc("break-pane", func(args []string) (string, error) {
				paneID := args[slices.Index(args, "-s")+1]
				if failure, failed := tt.breakFail[paneID]; failed {
					return failure.out, failure.err
				}
				fake.answer("list-panes", encodeLivePanes(tt.post), tt.relistErr)
				return "@1" + paneID[1:] + "\n", nil
			})
			st := &ReedState{SelvagePaneID: tt.selvage, Strands: slices.Clone(tt.strands)}

			live, windows, err := e.listReconcilePanesLocked(st)
			if (err != nil) != tt.wantErr {
				t.Fatalf("listReconcilePanesLocked() error = %v, wantErr %v", err, tt.wantErr)
			}

			var breaks []string
			for _, argv := range fake.ArgvFor("break-pane") {
				breaks = append(breaks, argv[slices.Index(argv, "-s")+1])
			}
			if !slices.Equal(breaks, tt.wantBreaks) {
				t.Errorf("break-pane sources = %v, want %v", breaks, tt.wantBreaks)
			}
			for _, argv := range fake.ArgvFor("break-pane") {
				want := []string{"break-pane", "-d", "-b", "-s", argv[4], "-t", fakeStrandWindow, "-n", argv[8], "-P", "-F", "#{window_id}"}
				if !slices.Equal(argv, want) {
					t.Errorf("break-pane argv = %v, want %v", argv, want)
				}
			}
			if lists := fake.Count("list-panes"); lists != tt.wantPaneLists {
				t.Errorf("shell window listings = %d, want %d", lists, tt.wantPaneLists)
			}
			for _, s := range st.Strands {
				wantMoved := slices.Contains(tt.wantMoved, s.GUID)
				moved := s.Display.Anchor == render.AnchorOwnWindow && s.Display.Window == ""
				if moved != wantMoved {
					t.Errorf("strand %s display = %+v, moved = %v, want moved = %v", s.GUID, s.Display, moved, wantMoved)
				}
				if wantMoved && tt.optionErr == nil {
					window := "@1" + s.PaneID[1:]
					ownMark := []string{"set-option", "-w", "-t", window, ownWindowOption, "1"}
					if !slices.ContainsFunc(fake.ArgvFor("set-option"), func(argv []string) bool { return slices.Equal(argv, ownMark) }) {
						t.Errorf("strand %s window %s never got the own marker; set-option calls = %v", s.GUID, window, fake.ArgvFor("set-option"))
					}
				}
			}
			if tt.wantErr {
				return
			}
			var gotLive []string
			for _, p := range live {
				gotLive = append(gotLive, p.ID)
			}
			if !slices.Equal(gotLive, tt.wantLive) {
				t.Errorf("returned panes = %v, want %v", gotLive, tt.wantLive)
			}
			if len(windows) != len(tt.wantLive) {
				t.Errorf("returned window panes = %d, want %d", len(windows), len(tt.wantLive))
			}
		})
	}
}
