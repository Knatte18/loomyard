// adoptsection_test.go pins the `adopt` section of `beam status` for each state of an adopt record.

package beamcli

import (
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/adoptrecord"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestAdoptSectionOf(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	record := adoptrecord.Record{
		Request:  adoptrecord.Request{Slugs: []string{"alpha"}, KeepLocal: true},
		Subjects: []string{"alpha", "root"},
		Steps:    []adoptrecord.StepRecord{{Name: adoptrecord.StepReconcile}, {Name: adoptrecord.StepPairs}},
		InFlight: adoptrecord.StepPause,
	}
	stopped := record
	stopped.InFlight = adoptrecord.StepDeploy
	stopped.WayForward = "fix the deploy, then re-run `lym adopt alpha root`"

	tests := []struct {
		name       string
		record     adoptrecord.Record
		state      string
		wantNil    bool
		wantStep   string
		wantInWay  []string
		wantNotWay []string
	}{
		{
			name: "RunningNamesItsStepInFlightAndTheRerunThatFollowsIt", record: record, state: adoptrecord.StateRunning,
			wantStep: adoptrecord.StepPause, wantInWay: []string{"`lym adopt alpha --keep-local` follows the worker"}, wantNotWay: []string{"--discard"},
		},
		{
			name: "InterruptedNamesTheRerunAndDiscard", record: record, state: adoptrecord.StateInterrupted,
			wantStep: adoptrecord.StepPause, wantInWay: []string{"`lym adopt alpha --keep-local` resumes it", "`lym adopt --discard`"},
		},
		{
			name: "StoppedNamesItsStopsWayForwardAndDiscard", record: stopped, state: adoptrecord.StateStopped,
			wantStep: adoptrecord.StepDeploy, wantInWay: []string{"fix the deploy, then re-run `lym adopt alpha root`", "`lym adopt --discard`"},
		},
		{name: "DoneGivesNoSection", record: record, state: adoptrecord.StateDone, wantNil: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := adoptSectionOf(tt.record, tt.state)
			if tt.wantNil {
				if got != nil {
					t.Errorf("adoptSectionOf(done) = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("adoptSectionOf(%s) = nil, want a section", tt.state)
			}
			if got.State != tt.state || got.Step != tt.wantStep || !slices.Equal(got.Subjects, []string{"alpha", "root"}) ||
				!slices.Equal(got.StepsDone, []string{adoptrecord.StepReconcile, adoptrecord.StepPairs}) {
				t.Errorf("section = %+v, want state %q at step %q for alpha and root with reconcile and pairs done", got, tt.state, tt.wantStep)
			}
			for _, text := range tt.wantInWay {
				if !strings.Contains(got.WayForward, text) {
					t.Errorf("way forward = %q, want it to contain %q", got.WayForward, text)
				}
			}
			for _, text := range tt.wantNotWay {
				if strings.Contains(got.WayForward, text) {
					t.Errorf("way forward = %q, want it free of %q", got.WayForward, text)
				}
			}
			if len(got.Lines) == 0 || !strings.HasPrefix(got.Lines[0], "adopt of alpha, root:") {
				t.Errorf("lines = %q, want the first to open with the adopt of alpha, root", got.Lines)
			}
		})
	}
}
