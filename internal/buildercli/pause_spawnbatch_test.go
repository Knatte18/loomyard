//go:build integration

// pause_spawnbatch_test.go covers the one pause-verb behavior that needs a
// real git fixture: spawn-batch observing a flag pause wrote, via
// newSpawnBatchFixture (spawnbatch_test.go). Split out of pause_test.go so
// that file's two git-free tests stay in Tier 1 (Test Tier Purity
// Invariant); the rest of the pause verb's envelope-shape coverage lives
// there.

package buildercli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
)

// TestSpawnBatchCmd_ObservesPauseFlagWrittenByPauseCmd proves pause and
// spawn-batch share the same flag file: a pause written by pauseCmd is
// observed by spawnBatchCmd's own gate, the discussion's shared-seam
// requirement.
func TestSpawnBatchCmd_ObservesPauseFlagWrittenByPauseCmd(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newSpawnBatchFixture(t)
	fx.initState(t)

	var pauseOut bytes.Buffer
	if exitCode := clihelp.Execute(fx.CLI.pauseCmd(), &pauseOut, nil); exitCode != 0 {
		t.Fatalf("pause() = %d; want 0, output: %s", exitCode, pauseOut.String())
	}

	var out bytes.Buffer
	exitCode := clihelp.Execute(fx.CLI.spawnBatchCmd(), &out, []string{"1"})
	if exitCode != 1 {
		t.Fatalf("spawn-batch after pause() = %d; want 1, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"paused":true`) {
		t.Errorf("output missing paused:true; got %q", out.String())
	}
}
