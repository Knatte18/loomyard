// postrun_test.go covers battenPostRun's contract: its extras reach the success envelope only,
// so batten's error envelope keeps exactly its own keys.

package battencli

import (
	"context"
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

func TestBattenPostRun_ErrorPathReturnsNil(t *testing.T) {
	c := &battenCLI{abandonedSession: "some-abandoned-session"}

	if got := c.battenPostRun(context.Background(), shedengine.Result{}, errors.New("boom")); got != nil {
		t.Errorf("battenPostRun on a non-nil runErr = %v; want nil", got)
	}
}

func TestBattenPostRun_SuccessPathReturnsAbandonedSession(t *testing.T) {
	c := &battenCLI{abandonedSession: "some-abandoned-session"}

	got := c.battenPostRun(context.Background(), shedengine.Result{}, nil)
	if len(got) != 1 || got["abandonedSession"] != "some-abandoned-session" {
		t.Errorf("battenPostRun on a nil runErr = %v; want only abandonedSession", got)
	}
}
