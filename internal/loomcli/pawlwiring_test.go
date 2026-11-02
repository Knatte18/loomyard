// pawlwiring_test.go covers the words of the pawl run's failed landing re-check and of Beam-Up's warning notice.

package loomcli

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestBeamUpWarningLine(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name     string
		warnings []string
		want     string
	}{
		{"NoWarningsPostsNothing", nil, ""},
		{"EveryWarningJoinsOneNotice", []string{"the dobby was not started (refused)", "could not add the runs strand (reed refused)"}, "Beam-Up brought the beam b up with warnings: the dobby was not started (refused); could not add the runs strand (reed refused)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := beamUpWarningLine("b", tt.warnings); got != tt.want {
				t.Errorf("beamUpWarningLine(b, %q) = %q; want %q", tt.warnings, got, tt.want)
			}
		})
	}
}

func TestSurfaceBeamUpWarnings(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name      string
		warnings  []string
		notifyErr error
		wantPosts []string
	}{
		{name: "NoWarningsPostsNothing"},
		{
			name:      "WarningsPostOneNotice",
			warnings:  []string{"the dobby was not started (refused)", "could not add the runs strand (reed refused)"},
			wantPosts: []string{"Beam-Up brought the beam b up with warnings: the dobby was not started (refused); could not add the runs strand (reed refused)"},
		},
		{
			name:      "AFailedPostReturnsWithoutHaltingTheRow",
			warnings:  []string{"the dobby was not started (refused)"},
			notifyErr: errors.New("creel refused"),
			wantPosts: []string{"Beam-Up brought the beam b up with warnings: the dobby was not started (refused)"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var posts []string
			notify := func(line string) error {
				posts = append(posts, line)
				return tt.notifyErr
			}
			surfaceBeamUpWarnings(notify, "b", tt.warnings)
			if !slices.Equal(posts, tt.wantPosts) {
				t.Errorf("surfaceBeamUpWarnings(b, %q) posted %q; want %q", tt.warnings, posts, tt.wantPosts)
			}
		})
	}
}

func TestRunBeamUpStep(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name      string
		report    orchcli.BeamUpReport
		beamUpErr error
		notifyErr error
		wantErr   bool
		wantPosts []string
	}{
		{name: "NoWarningsPostsNothing"},
		{
			name:      "WarningsAreSurfacedAndTheRowContinues",
			report:    orchcli.BeamUpReport{Warnings: []string{"the dobby was not started (refused)"}},
			wantPosts: []string{"Beam-Up brought the beam b up with warnings: the dobby was not started (refused)"},
		},
		{
			name:      "AFailedPostNeverHaltsTheRow",
			report:    orchcli.BeamUpReport{Warnings: []string{"the dobby was not started (refused)"}},
			notifyErr: errors.New("creel refused"),
			wantPosts: []string{"Beam-Up brought the beam b up with warnings: the dobby was not started (refused)"},
		},
		{name: "ABringUpErrorHaltsTheRowWithoutANotice", beamUpErr: errors.New("refused"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var posts []string
			notify := func(line string) error {
				posts = append(posts, line)
				return tt.notifyErr
			}
			err := runBeamUpStep(func() (orchcli.BeamUpReport, error) { return tt.report, tt.beamUpErr }, notify, "b")
			if (err != nil) != tt.wantErr {
				t.Errorf("runBeamUpStep() error = %v; want error %v", err, tt.wantErr)
			}
			if !slices.Equal(posts, tt.wantPosts) {
				t.Errorf("runBeamUpStep() posted %q; want %q", posts, tt.wantPosts)
			}
		})
	}
}

func TestPawlRecheckLines(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	member := battencli.BeamLandBlockers{Members: []battencli.BeamLandMember{{Slug: "m", HasPair: true}}}
	refusal := errors.New("battencli: the beam b cannot land while work remains under it; way forward: end m with `lym batten cancel m --remove`; end the pair p with `lym batten cancel p --remove`")

	tests := []struct {
		name          string
		blockers      battencli.BeamLandBlockers
		refusal       error
		beamOwner     string
		parentOwner   string
		parentSubject string
		// unflagged says the beam's ready record does not flag it.
		unflagged bool
		want      []string
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
			want:          []string{"end m with `lym batten cancel m --remove`", "end the pair p with `lym batten cancel p --remove`"},
		},
		{
			name:          "ABeamAnotherMachineOwnsNamesAdoptOfTheBeam",
			beamOwner:     "other",
			parentOwner:   "here",
			parentSubject: "root",
			want:          []string{"the beam b is owned by other, not this machine (here): run `lym adopt b` to take it"},
		},
		{
			name:          "AParentAnotherMachineOwnsNamesAdoptOfItsSubject",
			beamOwner:     "here",
			parentOwner:   "other",
			parentSubject: "outer-beam",
			want:          []string{"the beam's parent is owned by other, not this machine (here): run `lym adopt outer-beam` to take it"},
		},
		{
			name:          "ThePrimeAnotherMachineOwnsNamesAdoptRoot",
			beamOwner:     "here",
			parentOwner:   "other",
			parentSubject: "root",
			want:          []string{"the beam's parent is owned by other, not this machine (here): run `lym adopt root` to take it"},
		},
		{
			name:          "AnUnflaggedBeamNamesReady",
			beamOwner:     "here",
			parentOwner:   "here",
			parentSubject: "root",
			unflagged:     true,
			want:          []string{"the beam b is not flagged ready to merge: run `lym beam ready b` to flag it"},
		},
		{
			name:          "EveryFailureGivesItsOwnLineInOrder",
			blockers:      member,
			refusal:       errors.New("battencli: the beam b cannot land while work remains under it; way forward: end m"),
			beamOwner:     "other",
			parentOwner:   "third",
			parentSubject: "root",
			unflagged:     true,
			want: []string{
				"end m",
				"the beam b is owned by other, not this machine (here): run `lym adopt b` to take it",
				"the beam's parent is owned by third, not this machine (here): run `lym adopt root` to take it",
				"the beam b is not flagged ready to merge: run `lym beam ready b` to flag it",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := pawlRecheckLines("b", !tt.unflagged, tt.blockers, tt.refusal, "here", tt.beamOwner, tt.parentOwner, tt.parentSubject)
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("pawlRecheckLines() = %q; want %q", got, tt.want)
			}
		})
	}
}
