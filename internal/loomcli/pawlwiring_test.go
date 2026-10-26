//go:build tier0

// pawlwiring_test.go covers the words of the pawl run's failed landing re-check.

package loomcli

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battencli"
)

func TestPawlRecheckLines(t *testing.T) {
	t.Parallel()

	member := battencli.BeamLandBlockers{Members: []battencli.BeamLandMember{{Slug: "m", HasPair: true}}}
	refusal := errors.New("battencli: the beam b cannot land while work remains under it; way forward: end m with `lyx batten cancel m --remove`; end the pair p with `lyx batten cancel p --remove`")

	tests := []struct {
		name          string
		blockers      battencli.BeamLandBlockers
		refusal       error
		beamOwner     string
		parentOwner   string
		parentSubject string
		want          []string
	}{
		{name: "NothingBlocksAndBothOwnersAreHere", beamOwner: "here", parentOwner: "here", parentSubject: "root"},
		{name: "NoOwnerRecordedReadsAsOwnedHere", parentSubject: "root"},
		{
			name:          "ABlockerGivesEachWayForward",
			blockers:      member,
			refusal:       refusal,
			beamOwner:     "here",
			parentOwner:   "here",
			parentSubject: "root",
			want:          []string{"end m with `lyx batten cancel m --remove`", "end the pair p with `lyx batten cancel p --remove`"},
		},
		{
			name:          "ABeamAnotherMachineOwnsNamesAdoptOfTheBeam",
			beamOwner:     "other",
			parentOwner:   "here",
			parentSubject: "root",
			want:          []string{"the beam b is owned by other, not this machine (here): run `lyx adopt b` to take it"},
		},
		{
			name:          "AParentAnotherMachineOwnsNamesAdoptOfItsSubject",
			beamOwner:     "here",
			parentOwner:   "other",
			parentSubject: "outer-beam",
			want:          []string{"the beam's parent is owned by other, not this machine (here): run `lyx adopt outer-beam` to take it"},
		},
		{
			name:          "ThePrimeAnotherMachineOwnsNamesAdoptRoot",
			beamOwner:     "here",
			parentOwner:   "other",
			parentSubject: "root",
			want:          []string{"the beam's parent is owned by other, not this machine (here): run `lyx adopt root` to take it"},
		},
		{
			name:          "EveryFailureGivesItsOwnLineInOrder",
			blockers:      member,
			refusal:       errors.New("battencli: the beam b cannot land while work remains under it; way forward: end m"),
			beamOwner:     "other",
			parentOwner:   "third",
			parentSubject: "root",
			want: []string{
				"end m",
				"the beam b is owned by other, not this machine (here): run `lyx adopt b` to take it",
				"the beam's parent is owned by third, not this machine (here): run `lyx adopt root` to take it",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := pawlRecheckLines("b", tt.blockers, tt.refusal, "here", tt.beamOwner, tt.parentOwner, tt.parentSubject)
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("pawlRecheckLines() = %q; want %q", got, tt.want)
			}
		})
	}
}
