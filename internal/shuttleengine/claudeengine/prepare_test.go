// prepare_test.go covers Prepare's effort handling: an unrealizable effort
// is rejected before any artifact is written (mirroring
// TestPrepare_PromptLaunchLimit's before-artifacts guarantee), a valid
// effort ends up in the returned Launch.Cmd, and an empty effort emits no
// --effort flag at all.

package claudeengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// TestPrepare_BadEffortRejectedBeforeArtifacts proves an unrealizable effort
// value fails Prepare before prompt.md/settings.json are written — the same
// before-artifacts guarantee TestPrepare_PromptLaunchLimit pins for the
// prompt-size guard, since a half-prepared run dir would look resumable to a
// later diagnosis pass.
func TestPrepare_BadEffortRejectedBeforeArtifacts(t *testing.T) {
	runDir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "do the thing", Effort: "bogus"}
	cfg := shuttleengine.Config{}

	c := New()
	_, err := c.Prepare(runDir, spec, cfg)
	if err == nil {
		t.Fatal("Prepare() with an unrealizable effort = nil error; want the validateEffort rejection")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("Prepare() error = %q; want it to name the invalid effort value", err)
	}

	if _, statErr := os.Stat(filepath.Join(runDir, "prompt.md")); !os.IsNotExist(statErr) {
		t.Errorf("prompt.md exists after a rejected Prepare (stat err=%v); want no artifacts written", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(runDir, "settings.json")); !os.IsNotExist(statErr) {
		t.Errorf("settings.json exists after a rejected Prepare (stat err=%v); want no artifacts written", statErr)
	}
}

// TestPrepare_ValidEffortLandsInLaunchCmd proves a valid effort survives
// Prepare's validation and is threaded into buildLaunchCmd, appearing in the
// returned Launch.Cmd exactly as buildLaunchCmd would render it.
func TestPrepare_ValidEffortLandsInLaunchCmd(t *testing.T) {
	runDir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "do the thing", Effort: "high"}
	cfg := shuttleengine.Config{}

	c := New()
	launch, err := c.Prepare(runDir, spec, cfg)
	if err != nil {
		t.Fatalf("Prepare() with a valid effort error: %v; want nil", err)
	}
	if !strings.Contains(launch.Cmd, "--effort 'high'") {
		t.Errorf("Launch.Cmd = %q; want it to contain --effort 'high'", launch.Cmd)
	}
}

// TestPrepare_EmptyEffortEmitsNoFlag proves the zero-value Effort (the
// common case — no operator override) succeeds and emits no --effort flag
// at all, deferring entirely to claude's own default.
func TestPrepare_EmptyEffortEmitsNoFlag(t *testing.T) {
	runDir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "do the thing"}
	cfg := shuttleengine.Config{}

	c := New()
	launch, err := c.Prepare(runDir, spec, cfg)
	if err != nil {
		t.Fatalf("Prepare() with an empty effort error: %v; want nil", err)
	}
	if strings.Contains(launch.Cmd, "--effort") {
		t.Errorf("Launch.Cmd = %q; want no --effort flag for an empty Spec.Effort", launch.Cmd)
	}
}
