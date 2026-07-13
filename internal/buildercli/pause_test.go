// pause_test.go covers the pause verb's envelope shape and confirms it
// writes the same flag file builderengine.PauseRequested/spawn-batch's own
// gate reads. The seam a spawn-batch test observes against a real git
// fixture is covered in pause_spawnbatch_test.go (integration-tagged,
// since it needs newSpawnBatchFixture); the two tests below only touch
// t.TempDir(), so they stay in Tier 1.

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
