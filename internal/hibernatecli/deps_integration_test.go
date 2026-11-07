//go:build linux

// deps_integration_test.go pins what hubDeps's Notify seam does over a real hub whose prime records no orch strand.
// The test calls no t.Parallel: it sets `agentname.StrandNameEnv` and captures the process-global logger.

package hibernatecli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestHubDepsNotify_WarnsWhenNoOrchStrandIsRecorded(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv(agentname.StrandNameEnv, "")
	var captured bytes.Buffer
	previous := logger.DefaultOutput()
	logger.SetOutput(&captured)
	t.Cleanup(func() { logger.SetOutput(previous) })
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	prime, err := lymcwd.ResolveWorktree(h.PrimeWorktree())
	if err != nil {
		t.Fatal(err)
	}
	deps, err := hubDeps(prime)
	if err != nil {
		t.Fatal(err)
	}

	err = deps.Notify("[lym hibernate] probe")

	if err != nil {
		t.Errorf("Notify() error = %v; want nil for a prime with no orch strand", err)
	}
	if log := captured.String(); !strings.Contains(log, "no orch strand is recorded") || !strings.Contains(log, "[lym hibernate] probe") {
		t.Errorf("log = %q; want a warning naming the dropped notice", log)
	}
}
