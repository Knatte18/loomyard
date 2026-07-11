// prepare_test.go covers Prepare's effort and model/version handling: an
// unrealizable effort is rejected before any artifact is written (mirroring
// TestPrepare_PromptLaunchLimit's before-artifacts guarantee), a valid
// effort ends up in the returned Launch.Cmd, an empty effort emits no
// --effort flag at all, a bare-word model plus version composes into the
// pinned model id in Launch.Cmd, and a dashed model plus version is rejected
// before any artifact is written.

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

// TestPrepare_ModelAndVersionComposePinnedID proves Prepare threads
// spec.Model and spec.Version through resolveModelID, so a Spec naming a
// bare-word model plus a dotted version produces a launch Cmd containing
// the pinned model id ("sonnet" + "4.5" -> "claude-sonnet-4-5"), not the
// bare-word model.
func TestPrepare_ModelAndVersionComposePinnedID(t *testing.T) {
	runDir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "do the thing", Model: "sonnet", Version: "4.5"}
	cfg := shuttleengine.Config{}

	c := New()
	launch, err := c.Prepare(runDir, spec, cfg)
	if err != nil {
		t.Fatalf("Prepare() with model+version error: %v; want nil", err)
	}
	if !strings.Contains(launch.Cmd, "--model 'claude-sonnet-4-5'") {
		t.Errorf("Launch.Cmd = %q; want it to contain --model 'claude-sonnet-4-5'", launch.Cmd)
	}
}

// TestPrepare_DashedModelWithVersionRejectedBeforeArtifacts proves a full
// model id (already containing a dash) combined with a non-empty Version
// fails Prepare — the id already pins its own version, so a second pin is a
// contradiction — and that the rejection happens before any run artifact is
// written, mirroring TestPrepare_BadEffortRejectedBeforeArtifacts's
// before-artifacts guarantee.
func TestPrepare_DashedModelWithVersionRejectedBeforeArtifacts(t *testing.T) {
	runDir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "do the thing", Model: "claude-sonnet-4-5", Version: "4.5"}
	cfg := shuttleengine.Config{}

	c := New()
	_, err := c.Prepare(runDir, spec, cfg)
	if err == nil {
		t.Fatal("Prepare() with a dashed model + version = nil error; want the resolveModelID rejection")
	}

	if _, statErr := os.Stat(filepath.Join(runDir, "prompt.md")); !os.IsNotExist(statErr) {
		t.Errorf("prompt.md exists after a rejected Prepare (stat err=%v); want no artifacts written", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(runDir, "settings.json")); !os.IsNotExist(statErr) {
		t.Errorf("settings.json exists after a rejected Prepare (stat err=%v); want no artifacts written", statErr)
	}
}
