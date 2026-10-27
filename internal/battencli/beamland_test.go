//go:build tier0

// beamland_test.go pins the beam landing refusal's judgement over told facts: each blocker names its way forward, and a beam with nothing under it passes.

package battencli

import (
	"strings"
	"testing"
)

func TestBeamLandRefusal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		blockers BeamLandBlockers
		// wantErrors are substrings the refusal carries; empty means the landing passes.
		wantErrors []string
	}{
		{name: "NothingUnderTheBeamPasses"},
		{
			name:       "MemberWithAPairNamesItsLandAndItsCancel",
			blockers:   BeamLandBlockers{Members: []BeamLandMember{{Slug: "m1", HasPair: true}}},
			wantErrors: []string{"m1", "`lym batten run m1`", "`lym batten cancel m1 --remove`"},
		},
		{
			name:       "MemberWithNoPairNamesMovingItsProject",
			blockers:   BeamLandBlockers{Members: []BeamLandMember{{Slug: "m2"}}},
			wantErrors: []string{"m2", "no pair on this machine", "`project`", "`lym board upsert`"},
		},
		{
			name:       "MemberRunningWithAPairIsNamedLikeAnyMemberWithAPair",
			blockers:   BeamLandBlockers{Members: []BeamLandMember{{Slug: "m3", HasPair: true, Running: true}}},
			wantErrors: []string{"m3", "`lym batten cancel m3 --remove`"},
		},
		{
			name:       "EveryMemberIsNamed",
			blockers:   BeamLandBlockers{Members: []BeamLandMember{{Slug: "m1", HasPair: true}, {Slug: "m2"}}},
			wantErrors: []string{"m1", "m2"},
		},
		{
			name:       "ChildPairNamesItsLandAndItsCancel",
			blockers:   BeamLandBlockers{ChildPairs: []string{"c1"}},
			wantErrors: []string{"pair c1", "`lym batten run c1`", "`lym batten cancel c1 --remove`"},
		},
		{
			name:       "EveryBlockerIsNamedTogether",
			blockers:   BeamLandBlockers{Members: []BeamLandMember{{Slug: "m1", HasPair: true}}, ChildPairs: []string{"c1"}},
			wantErrors: []string{"m1", "c1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := beamLandRefusal("b", tt.blockers)

			if len(tt.wantErrors) == 0 {
				if err != nil {
					t.Fatalf("beamLandRefusal() = %v; want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("beamLandRefusal() = nil; want a refusal carrying %q", tt.wantErrors)
			}
			for _, want := range tt.wantErrors {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("beamLandRefusal() = %q; want it to carry %q", err.Error(), want)
				}
			}
			if strings.Contains(err.Error(), "lym orch") {
				t.Errorf("beamLandRefusal() = %q; want no orch way forward, since the beam's orch runs until Beam-Down", err.Error())
			}
		})
	}
}
