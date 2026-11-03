// followup_test.go covers the wording of the step that settles a batten run's record after a removal of its pair.

package pairteardown_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestBattenFollowUp(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	at := func(state shedengine.State, producer string) shedengine.Status {
		return shedengine.Status{State: state, CurrentProducer: producer}
	}
	tests := []struct {
		name      string
		record    shedengine.Status
		found     bool
		ownerHeld bool
		// want lists substrings the follow-up must carry; none means the follow-up must be empty.
		want []string
		// notWant lists substrings it must never carry.
		notWant []string
	}{
		{name: "NoRecord"},
		{name: "DoneRecord", record: at(shedengine.StateDone, battenrecipe.NameLand), found: true},
		{name: "RecordAtWorktreeTeardown", record: at(shedengine.StateBlocked, battenrecipe.NameWorktreeTeardown), found: true},
		{name: "OwnerPauseCoveringTheSlug", record: at(shedengine.StateRunning, battenrecipe.NameRunShed), found: true, ownerHeld: true},
		{
			name:   "RecordAtRunShedNamesTheCancel",
			record: at(shedengine.StateRunning, battenrecipe.NameRunShed), found: true,
			want: []string{"lym batten cancel slug --remove"},
		},
		{
			name:   "RecordAtWorktreeCreateNamesTheCancel",
			record: at(shedengine.StateBlocked, battenrecipe.NameWorktreeCreate), found: true,
			want: []string{"lym batten cancel slug --remove"},
		},
		{
			name:   "RecordAtLandNamesTheRunAndNeverTheCancel",
			record: at(shedengine.StateBlocked, battenrecipe.NameLand), found: true,
			want:    []string{"lym batten run slug"},
			notWant: []string{"lym batten cancel"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := pairteardown.BattenFollowUp("slug", tt.record, tt.found, tt.ownerHeld)

			if len(tt.want) == 0 && got != "" {
				t.Errorf("BattenFollowUp() = %q; want empty", got)
			}
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("BattenFollowUp() = %q; want it to carry %q", got, want)
				}
			}
			for _, unwanted := range tt.notWant {
				if strings.Contains(got, unwanted) {
					t.Errorf("BattenFollowUp() = %q; want it free of %q", got, unwanted)
				}
			}
		})
	}
}
