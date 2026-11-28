// session_test.go covers how a replayed session is built on a host unlike the recording's.

package tmuxrec_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxrec"
)

// TestIdleSession_BindsTheBareNameOfAShellTheHostLacks pins that such a host still builds a session.
// It runs without t.Parallel: the shell override of reed's config template is process-global.
func TestIdleSession_BindsTheBareNameOfAShellTheHostLacks(t *testing.T) {
	tiergate.Require(t, 0)
	const shell = "tmuxrec-no-such-shell"
	t.Setenv("LYM_REED_SHELL", shell)

	session := tmuxrec.IdleSession(t, "session-start-stop")

	if got := session.Bindings["{shell}"]; got != shell {
		t.Errorf("Bindings[{shell}] = %q; want the configured name %q", got, shell)
	}
}
