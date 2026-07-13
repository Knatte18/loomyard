// pause_test.go covers the pause verb's envelope shape and confirms it
// writes the same flag file builderengine.PauseRequested/spawn-batch's own
// gate reads -- the seam a subsequent spawn-batch test can observe.

package buildercli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

func TestPauseCmd_WritesFlagAndOkEnvelope(t *testing.T) {
	hub := t.TempDir()
	c := &builderCLI{
		layout:     &hubgeometry.Layout{WorktreeRoot: hub, Cwd: hub, RelPath: "."},
		builderDir: hubgeometry.BuilderDir(hub),
	}

	var out bytes.Buffer
	exitCode := clihelp.Execute(c.pauseCmd(), &out, nil)

	if exitCode != 0 {
		t.Fatalf("pause() = %d; want 0, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"pause_requested":true`) {
		t.Errorf("output missing pause_requested:true; got %q", out.String())
	}
	if !builderengine.PauseRequested(c.builderDir) {
		t.Error("PauseRequested() = false after pause; want true")
	}
}

func TestPauseCmd_IdempotentRePause(t *testing.T) {
	hub := t.TempDir()
	c := &builderCLI{
		layout:     &hubgeometry.Layout{WorktreeRoot: hub, Cwd: hub, RelPath: "."},
		builderDir: hubgeometry.BuilderDir(hub),
	}

	var out1 bytes.Buffer
	if exitCode := clihelp.Execute(c.pauseCmd(), &out1, nil); exitCode != 0 {
		t.Fatalf("first pause() = %d; want 0, output: %s", exitCode, out1.String())
	}

	var out2 bytes.Buffer
	exitCode := clihelp.Execute(c.pauseCmd(), &out2, nil)
	if exitCode != 0 {
		t.Fatalf("second pause() = %d; want 0 (idempotent), output: %s", exitCode, out2.String())
	}
	if !strings.Contains(out2.String(), `"ok":true`) {
		t.Errorf("second pause() output missing ok:true; got %q", out2.String())
	}
}

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
