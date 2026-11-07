// nosession_test.go pins Status's answer for a worktree whose session was never started.
// That answer is an error satisfying errors.Is(err, ErrNoSession),
// so a caller outside reed can read Status as a liveness probe.
// tmux's exit-1 has-session answer is scripted through the fake seam, so no server runs.

package reedengine

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestStatus_NoSessionWrapsErrNoSession(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	e := newTestEngine(t)
	fake := installFakeTmux(t, e)
	fake.answer("has-session", "", exitCodeErr{code: 1})

	_, err := e.Status()
	if err == nil {
		t.Fatal("Status on a never-started session = nil error, want a no-session error")
	}
	if !errors.Is(err, ErrNoSession) {
		t.Errorf("Status error = %v, want errors.Is(err, ErrNoSession)", err)
	}
	if want := noSessionMessage(0, true); !strings.Contains(err.Error(), want) {
		t.Errorf("Status error text = %q, want it to carry %q", err.Error(), want)
	}
}
