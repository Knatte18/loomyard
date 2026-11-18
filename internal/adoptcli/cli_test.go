// cli_test.go pins the adopt envelope's fields: the always-present ones and the ones that appear only when set.

package adoptcli

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

//testtiming:keep pins which envelope keys appear only when set and the keys the retired worker envelope held, which TestAdopt never reads from the map
func TestAdoptFields(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	hold := &boardengine.Hold{Kind: boardengine.HoldKindAdoptFix, Machine: here, FixTask: "fix1"}
	tests := []struct {
		name   string
		result outcome
		// want holds the optional fields the envelope carries, with their values.
		want map[string]any
	}{
		{
			name: "OwnerWritesThatStoodNameTheArchivesCall",
			result: outcome{PrimeRecorded: true, Slugs: []slugOutcome{
				{Slug: "beta", OwnerRecorded: true}, {Slug: "gamma"}, {Slug: "alpha", OwnerRecorded: true},
			}},
			want: map[string]any{"archives": "lym adopt --archives alpha beta root"},
		},
		{
			name:   "AStandingHoldAndItsFixTaskAreFields",
			result: outcome{Hold: hold, FixTask: "fix1", PrimeCheck: checkFailed, CheckLines: []string{"the adopt check of root failed"}},
			want: map[string]any{
				"hold": hold, "fix_task": "fix1", "prime_check": checkFailed, "check_lines": []string{"the adopt check of root failed"},
			},
		},
		{
			name:   "WarningsAreTheWarningsField",
			result: outcome{Warnings: []string{"the hub workspace files were not regenerated"}},
			want:   map[string]any{"warnings": []string{"the hub workspace files were not regenerated"}},
		},
		{
			name:   "ARunDeployIsTheDeployField",
			result: outcome{Deploy: &deployOutcome{Installed: "87553450ec14", Head: "2d319fe5", Log: "deploy.log"}},
			want:   map[string]any{"deploy": &deployOutcome{Installed: "87553450ec14", Head: "2d319fe5", Log: "deploy.log"}},
		},
		{
			name:   "ThePrimesBeamStepsAndWarningAreFields",
			result: outcome{PrimeBeamSteps: &beamStepsOutcome{Dobby: "started"}, PrimeBeamWarning: "the beam steps of the prime failed"},
			want:   map[string]any{"prime_beam_steps": &beamStepsOutcome{Dobby: "started"}, "prime_beam_warning": "the beam steps of the prime failed"},
		},
		{name: "NothingSetCarriesOnlyTheAlwaysPresentFields"},
	}
	optional := []string{"hold", "fix_task", "prime_check", "check_lines", "deploy", "warnings", "prime_beam_steps", "prime_beam_warning", "archives"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fields := adoptFields(tt.result)

			if _, present := fields["slugs"]; !present {
				t.Errorf("adoptFields() lacks the always-present slugs")
			}
			if recorded, present := fields["prime_recorded"]; !present || recorded != tt.result.PrimeRecorded {
				t.Errorf("adoptFields()[prime_recorded] = %v, present %v; want %v", recorded, present, tt.result.PrimeRecorded)
			}
			for _, key := range optional {
				got, present := fields[key]
				want, wanted := tt.want[key]
				if present != wanted || (wanted && !reflect.DeepEqual(got, want)) {
					t.Errorf("adoptFields()[%s] = %v, present %v; want %v, present %v", key, got, present, want, wanted)
				}
			}
			for _, retired := range []string{"reconcile", "mutations", "partial", "runs", "checks", "message", "check_warnings", "prime_lost_to"} {
				if _, present := fields[retired]; present {
					t.Errorf("adoptFields() holds the retired field %q", retired)
				}
			}
			if len(fields) != 2+len(tt.want) {
				keys := make([]string, 0, len(fields))
				for key := range fields {
					keys = append(keys, key)
				}
				slices.Sort(keys)
				t.Errorf("adoptFields() holds %v; want the two always-present fields and %d optional", keys, len(tt.want))
			}
		})
	}
}
