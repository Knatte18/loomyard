//go:build !windows

// slacktoggle_integration_test.go boots a real tmux server from this test binary.
// It reads back that neither Slack line nor the follow line was pinned.

package reedengine

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestPinnedLines_RealTmuxBootByATestBinaryPinsNone pins that a server booted by this test binary holds no lym line.
// The engine resolves its executable to this binary.
// A pinned state job would run `<this binary> slack bar`, and with it the whole suite, at every redraw;
// a pinned follow or toggle line would do the same at a click.
func TestPinnedLines_RealTmuxBootByATestBinaryPinsNone(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()

	e := newIntegrationEngine(t, "off")
	if _, err := e.Up(); err != nil {
		t.Fatalf("Up(): %v", err)
	}

	for _, option := range []string{slackStateCommandOption, slackToggleCommandOption, followCommandOption} {
		got, err := e.tmux.output("show-options", "-gqv", option)
		if err != nil {
			t.Fatalf("show-options -gqv %s: %v", option, err)
		}
		if got = strings.TrimSpace(got); got != "" {
			t.Errorf("show-options -gqv %s after a boot by the test binary = %q; want it unset", option, got)
		}
	}
}
