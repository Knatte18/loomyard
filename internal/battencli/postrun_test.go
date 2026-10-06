// postrun_test.go covers battenPostRun's contract: its extras reach the success envelope only,
// so batten's error envelope keeps exactly its own keys.

package battencli

import (
	"context"
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

//testtiming:keep pins that the extras reach the success envelope alone and the error path returns none, which the envelope-key test reads only on success
func TestBattenPostRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		runErr  error
		wantNil bool
	}{
		{name: "ErrorPathReturnsNil", runErr: errors.New("boom"), wantNil: true},
		{name: "SuccessPathReturnsAbandonedSession"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := &battenCLI{abandonedSession: "some-abandoned-session"}

			got := c.battenPostRun(context.Background(), shedengine.Result{}, tt.runErr)
			if tt.wantNil {
				if got != nil {
					t.Errorf("battenPostRun on a non-nil runErr = %v; want nil", got)
				}
				return
			}
			if len(got) != 1 || got["abandonedSession"] != "some-abandoned-session" {
				t.Errorf("battenPostRun on a nil runErr = %v; want only abandonedSession", got)
			}
		})
	}
}
