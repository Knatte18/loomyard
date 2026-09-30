//go:build integration

// nosession_integration_test.go pins that Status on a worktree whose session was never started returns an error satisfying errors.Is(err, ErrNoSession), so a caller outside reed can read Status as a liveness probe.
// It needs a real multiplexer server to answer has-session, so it sits with the integration tier alongside ensuresession_integration_test.go.

package reedengine

import (
	"errors"
	"strings"
	"testing"
)

func TestStatus_NoSessionWrapsErrNoSession(t *testing.T) {
	e := newColdScratchEngine(t)

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
