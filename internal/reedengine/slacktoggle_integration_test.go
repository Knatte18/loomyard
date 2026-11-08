//go:build !windows

// slacktoggle_integration_test.go boots a real tmux server from this test binary.
// It reads back that no Slack line was pinned.

package reedengine

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestSlackLines_RealTmuxBootByATestBinaryPinsNeither pins that a server booted by this test binary holds no Slack line.
// The engine resolves its executable to this binary.
// A pinned state job would run `<this binary> slack bar`, and with it the whole suite, at every redraw.
func TestSlackLines_RealTmuxBootByATestBinaryPinsNeither(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()

	e := newIntegrationEngine(t, "off")
	if _, err := e.Up(); err != nil {
		t.Fatalf("Up(): %v", err)
	}

	for _, option := range []string{slackStateCommandOption, slackToggleCommandOption} {
		got, err := e.tmux.output("show-options", "-gqv", option)
		if err != nil {
			t.Fatalf("show-options -gqv %s: %v", option, err)
		}
		if got = strings.TrimSpace(got); got != "" {
			t.Errorf("show-options -gqv %s after a boot by the test binary = %q; want it unset", option, got)
		}
	}
	follow, err := e.tmux.output("show-options", "-gqv", followCommandOption)
	if err != nil {
		t.Fatalf("show-options -gqv %s: %v", followCommandOption, err)
	}
	if strings.TrimSpace(follow) == "" {
		t.Errorf("show-options -gqv %s after a boot by the test binary is unset; want the other lines still pinned", followCommandOption)
	}
}
