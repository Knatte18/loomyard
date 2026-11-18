// adoptsection_test.go pins the `adopt` section of `beam status` for each kind of adopt hold.

package beamcli

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestAdoptSectionOf(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	at := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	adopt := boardengine.Hold{
		Kind: boardengine.HoldKindAdopt, Machine: "osl46", At: at,
		Subjects: []boardengine.HoldSubject{{Subject: "b"}, {Subject: boardengine.RootSubject}},
	}
	fix := boardengine.Hold{
		Kind: boardengine.HoldKindAdoptFix, Machine: "osl46", At: at, FixTask: "fix-1",
		Subjects: []boardengine.HoldSubject{{Subject: "b", Tries: 2}, {Subject: boardengine.RootSubject}},
	}

	tests := []struct {
		name string
		hold boardengine.Hold
		own  bool
		want adoptSection
	}{
		{
			name: "AnAdoptHoldOfAnotherMachineShowsKindMachineTimeAndSubjectsAndNoRerun",
			hold: adopt,
			want: adoptSection{
				Kind: "adopt", Machine: "osl46", At: at, Subjects: []string{"b", "root"},
				Lines: []string{"adopt hold (adopt) by osl46 since 2026-11-17T09:00:00Z over b, root"},
			},
		},
		{
			name: "AnAdoptFixHoldShowsItsFixTaskAndTries",
			hold: fix,
			want: adoptSection{
				Kind: "adopt-fix", Machine: "osl46", At: at, Subjects: []string{"b", "root"}, FixTask: "fix-1", Tries: map[string]int{"b": 2, "root": 0},
				Lines: []string{
					"adopt hold (adopt-fix) by osl46 since 2026-11-17T09:00:00Z over b, root",
					"fix task fix-1, tries b 2, root 0",
				},
			},
		},
		{
			name: "ThisMachinesHoldNamesTheRerun",
			hold: adopt,
			own:  true,
			want: adoptSection{
				Kind: "adopt", Machine: "osl46", At: at, Subjects: []string{"b", "root"}, Rerun: "lym adopt b root",
				Lines: []string{
					"adopt hold (adopt) by osl46 since 2026-11-17T09:00:00Z over b, root",
					"way forward: lym adopt b root",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if diff := cmp.Diff(tt.want, *adoptSectionOf(tt.hold, tt.own)); diff != "" {
				t.Errorf("adoptSectionOf() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
