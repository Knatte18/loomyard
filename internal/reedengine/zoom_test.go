// zoom_test.go pins keepZoomLocked's bracket against the fake tmux: when it unzooms, which pane it zooms again, and when it leaves the window alone.

package reedengine

import (
	"errors"
	"slices"
	"testing"
)

// TestKeepZoomLocked pins the bracket's tmux calls around a stand-in step ("select-layout") per scenario:
// a window that is not zoomed, or whose zoom cannot be read, runs the step plain;
// a zoomed window is unzoomed, stepped and zoomed again on the same pane, also when the step errored;
// a zoomed pane gone after the step is not zoomed again; and a failed unzoom runs the step with no re-zoom.
// The step's own error is always returned unchanged.
func TestKeepZoomLocked(t *testing.T) {
	stepErr := errors.New("step failed")
	unzoom := []string{"resize-pane", "-Z", "-t", "%2"}
	tests := []struct {
		name        string
		zoomAnswer  string
		zoomErr     error
		listing     string
		resizeErr   error
		stepErr     error
		wantToggles int
	}{
		{name: "NotZoomed", zoomAnswer: "0 %2", listing: "%1 0 0 10 3 1\n%2 0 3 10 3 2\n"},
		{name: "NoSession", zoomErr: errors.New("no server running")},
		{name: "UnparseableAnswer", zoomAnswer: "garbage"},
		{name: "ZoomedIsUnzoomedStepAndZoomedAgain", zoomAnswer: "1 %2", listing: "%1 0 0 10 3 1\n%2 0 3 10 3 2\n", wantToggles: 2},
		{name: "ZoomedStepErrorStillZoomsAgain", zoomAnswer: "1 %2", listing: "%1 0 0 10 3 1\n%2 0 3 10 3 2\n", stepErr: stepErr, wantToggles: 2},
		{name: "ZoomedPaneGoneAfterStepStaysInTheOverview", zoomAnswer: "1 %2", listing: "%1 0 0 10 6 1\n", wantToggles: 1},
		{name: "FailedUnzoomRunsTheStepWithoutAReZoom", zoomAnswer: "1 %2", listing: "%1 0 0 10 3 1\n%2 0 3 10 3 2\n", resizeErr: errors.New("boom"), wantToggles: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			fake := installFakeTmux(t, e)
			fake.answerFormat(zoomStateFormat, tt.zoomAnswer, tt.zoomErr)
			fake.answer("list-panes", tt.listing, nil)
			fake.answer("resize-pane", "", tt.resizeErr)

			ran := 0
			err := e.keepZoomLocked(func() error {
				ran++
				return errors.Join(e.tmux.run("select-layout"), tt.stepErr)
			})

			if ran != 1 {
				t.Errorf("step ran %d times, want 1", ran)
			}
			if tt.stepErr != nil && !errors.Is(err, tt.stepErr) {
				t.Errorf("keepZoomLocked() = %v, want the step's error", err)
			}
			if tt.stepErr == nil && err != nil {
				t.Errorf("keepZoomLocked() = %v, want nil", err)
			}
			toggles := fake.ArgvFor("resize-pane")
			if len(toggles) != tt.wantToggles {
				t.Fatalf("resize-pane calls = %v, want %d", toggles, tt.wantToggles)
			}
			for _, argv := range toggles {
				if !slices.Equal(argv, unzoom) {
					t.Errorf("resize-pane argv = %v, want %v", argv, unzoom)
				}
			}
			if tt.wantToggles > 0 {
				// The unzoom precedes the step; a re-zoom, when there is one, follows it.
				seq := fake.Sequence("resize-pane", "select-layout")
				want := []string{"resize-pane", "select-layout", "resize-pane"}[:1+tt.wantToggles]
				if !slices.Equal(seq, want) {
					t.Errorf("call order = %v, want %v", seq, want)
				}
			}
		})
	}
}
