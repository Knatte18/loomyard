//go:build tier0

// land_test.go covers the Land row's pure wiring helpers: how a pull's error splits into a records-side stop and a failure.

package battencli

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// TestPullStopOutcome asserts a lone records-side stop comes back as its message with a nil error, and a stop joined with a code-side error or any other error comes back as that error with no stop.
func TestPullStopOutcome(t *testing.T) {
	t.Parallel()

	stop := fabricengine.NewRecordsStopError("main", errors.New("replay conflict"), "settle it")
	codeSideErr := errors.New("code side fetch failed")
	joined := errors.Join(stop, codeSideErr)
	unrelated := errors.New("open fabric failed")

	tests := []struct {
		name        string
		err         error
		wantStop    string
		wantPullErr error
	}{
		{"LoneRecordsStop", stop, stop.Error(), nil},
		{"RecordsStopJoinedWithCodeSideError", joined, "", joined},
		{"UnrelatedError", unrelated, "", unrelated},
		{"NoError", nil, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotStop, gotErr := pullStopOutcome(tt.err)
			if gotStop != tt.wantStop || gotErr != tt.wantPullErr {
				t.Errorf("pullStopOutcome(%v) = %q, %v; want %q, %v", tt.err, gotStop, gotErr, tt.wantStop, tt.wantPullErr)
			}
		})
	}
}
