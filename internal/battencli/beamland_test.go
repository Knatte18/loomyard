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
		name        string
		members     []beamMember
		childPairs  []string
		orchLive    bool
		watcherLive bool
		// wantErrors are substrings the refusal carries; empty means the landing passes.
		wantErrors []string
	}{
		{name: "NothingUnderTheBeamPasses"},
		{
			name:       "MemberWithAPairNamesItsLandAndItsCancel",
			members:    []beamMember{{Slug: "m1", HasPair: true}},
			wantErrors: []string{"m1", "`lyx batten run m1`", "`lyx batten cancel m1 --remove`"},
		},
		{
			name:       "MemberWithNoPairNamesMovingItsProject",
			members:    []beamMember{{Slug: "m2"}},
			wantErrors: []string{"m2", "no pair on this machine", "`project`", "`lyx board upsert`"},
		},
		{
			name:       "EveryMemberIsNamed",
			members:    []beamMember{{Slug: "m1", HasPair: true}, {Slug: "m2"}},
			wantErrors: []string{"m1", "m2"},
		},
		{
			name:       "ChildPairNamesItsLandAndItsCancel",
			childPairs: []string{"c1"},
			wantErrors: []string{"pair c1", "`lyx batten run c1`", "`lyx batten cancel c1 --remove`"},
		},
		{
			name:       "LiveOrchNamesTheStopAndTheStatusRecheck",
			orchLive:   true,
			wantErrors: []string{"orch of the beam b is live", "`lyx orch stop`", "`lyx orch status`"},
		},
		{
			name:        "LiveWatcherNamesTheStop",
			watcherLive: true,
			wantErrors:  []string{"watcher of the beam b is live", "`lyx orch stop`", "`lyx orch status`"},
		},
		{
			name:        "EveryBlockerIsNamedTogether",
			members:     []beamMember{{Slug: "m1", HasPair: true}},
			childPairs:  []string{"c1"},
			orchLive:    true,
			watcherLive: true,
			wantErrors:  []string{"m1", "c1", "orch and its watcher"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := beamLandRefusal("b", tt.members, tt.childPairs, tt.orchLive, tt.watcherLive)

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
		})
	}
}
