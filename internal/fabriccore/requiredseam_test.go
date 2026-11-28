// requiredseam_test.go tests the refusal of a run that lacks a required seam.

package fabriccore_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/testkit/fabrickit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRefuseUnsupplied(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const wayForward = "way forward: build the hub through internal/hubforge, which installs the seams once per test binary, " +
		"or call hubforge.InstallRequiredSeams before the verb"
	needed := []fabriccore.RequiredSeam{fabriccore.SeamRunRecordPaths, fabriccore.SeamRunLocks, fabriccore.SeamStatusCodec}
	supplied := func(paths, locks, codec bool) fabriccore.Snapshot {
		return fabrickit.NewSnapshot().
			Seam(fabriccore.SeamRunRecordPaths, paths).
			Seam(fabriccore.SeamRunLocks, locks).
			Seam(fabriccore.SeamStatusCodec, codec).
			Build()
	}

	tests := []struct {
		name     string
		snapshot fabriccore.Snapshot
		needed   []fabriccore.RequiredSeam
		// wantReason is empty when nothing is refused.
		wantReason string
	}{
		{"EveryNeededSeamSupplied", supplied(true, true, true), needed, ""},
		{"OneUnsupplied", supplied(true, false, true), needed, "fabric's required seams are not supplied: run_locks"},
		{
			"TwoUnsuppliedInNeededOrder",
			supplied(false, true, false),
			[]fabriccore.RequiredSeam{fabriccore.SeamStatusCodec, fabriccore.SeamRunLocks, fabriccore.SeamRunRecordPaths},
			"fabric's required seams are not supplied: status_codec, run_record_paths",
		},
		{
			"NilMap",
			fabrickit.NewSnapshot().Build(),
			needed,
			"fabric's required seams are not supplied: run_record_paths, run_locks, status_codec",
		},
		{"NoNeededSeam", fabrickit.NewSnapshot().Build(), nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := fabriccore.RefuseUnsupplied(tt.snapshot, tt.needed...)
			if tt.wantReason == "" {
				if got != nil {
					t.Errorf("RefuseUnsupplied() = %+v; want nil", got)
				}
				return
			}
			want := fabriccore.Refusal{Reason: tt.wantReason, WayForward: wayForward}
			if got == nil || *got != want {
				t.Errorf("RefuseUnsupplied() = %+v; want %+v", got, want)
			}
		})
	}
}
