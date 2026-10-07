package loomcli

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// TestDriverSpec pins driverSpec's whole output shape, each field as its own named assertion.
//
//testtiming:keep pins every field of the driver launch spec: prompt, the scribe:prose skill, the report as its single output file, the resolved model, effort and version, the name override, role, display anchor and the zero-valued fields; it names Spec.Skills, so the skills-in-one-turn task owns it, and its covering test runs the spec without asserting the fields
func TestDriverSpec(t *testing.T) {
	prompt := "drive the run"
	reportPath := "/hub/wt/.lyx/shed/wt/drive-report-20260920-120000-cafe.md"
	settings := loomengine.DriverSettings{
		Model:   "claude-resolved-model-id",
		Effort:  "high",
		Version: "v9",
	}

	got := driverSpec(prompt, reportPath, settings)

	t.Run("Prompt", func(t *testing.T) {
		if got.Prompt != prompt {
			t.Errorf("driverSpec().Prompt = %q; want %q", got.Prompt, prompt)
		}
	})
	t.Run("Skills", func(t *testing.T) {
		if want := []string{"scribe:prose"}; !slices.Equal(got.Skills, want) {
			t.Errorf("driverSpec().Skills = %v; want %v", got.Skills, want)
		}
	})
	t.Run("OutputFiles", func(t *testing.T) {
		if len(got.OutputFiles) != 1 || got.OutputFiles[0] != reportPath {
			t.Errorf("driverSpec().OutputFiles = %v; want single-entry slice [%q]", got.OutputFiles, reportPath)
		}
	})
	t.Run("Model_ResolvedProviderModelID", func(t *testing.T) {
		if got.Model != settings.Model {
			t.Errorf("driverSpec().Model = %q; want the resolved provider model id %q, not a raw config alias", got.Model, settings.Model)
		}
	})
	t.Run("Effort_ResolvedFromSettings", func(t *testing.T) {
		if got.Effort != settings.Effort {
			t.Errorf("driverSpec().Effort = %q; want %q", got.Effort, settings.Effort)
		}
	})
	t.Run("Version_ResolvedFromSettings", func(t *testing.T) {
		if got.Version != settings.Version {
			t.Errorf("driverSpec().Version = %q; want %q", got.Version, settings.Version)
		}
	})
	t.Run("NameOverride", func(t *testing.T) {
		if got.NameOverride != driverStrandDisplayName {
			t.Errorf("driverSpec().NameOverride = %q; want %q -- the next bootstrap's lookup uses this exact constant", got.NameOverride, driverStrandDisplayName)
		}
	})
	t.Run("Interactive", func(t *testing.T) {
		if got.Interactive {
			t.Error("driverSpec().Interactive = true; want false -- an unattended session needs the autonomous posture")
		}
	})
	t.Run("ForkSubagents", func(t *testing.T) {
		if got.ForkSubagents {
			t.Error("driverSpec().ForkSubagents = true; want false -- the driving loop has no research fan-out to delegate")
		}
	})
	t.Run("Role", func(t *testing.T) {
		if got.Role != "driver" {
			t.Errorf(`driverSpec().Role = %q; want "driver"`, got.Role)
		}
	})
	t.Run("Round", func(t *testing.T) {
		if got.Round != "" {
			t.Errorf("driverSpec().Round = %q; want empty -- a driver is not one round of anything", got.Round)
		}
	})
	t.Run("Parent", func(t *testing.T) {
		if got.Parent != "" {
			t.Errorf("driverSpec().Parent = %q; want empty -- the driver is top-level", got.Parent)
		}
	})
	t.Run("Display_Anchor", func(t *testing.T) {
		if got.Display.Anchor != render.AnchorBelowParent {
			t.Errorf("driverSpec().Display.Anchor = %q; want %q -- the driver's pane must never be hidden", got.Display.Anchor, render.AnchorBelowParent)
		}
	})
	t.Run("Display_Focus", func(t *testing.T) {
		if got.Display.Focus {
			t.Error("driverSpec().Display.Focus = true; want false -- Focus is persisted and re-evaluated on every later AddStrand")
		}
	})
	// Timeout and KeepPane: nothing reads these on the driver path -- the wait loop
	// is never entered -- so this pins the zero value as a statement that nothing consumes it, not a
	// pane-retention or deadline decision.
	t.Run("Timeout_ZeroValue_NothingReadsIt", func(t *testing.T) {
		if got.Timeout != 0 {
			t.Errorf("driverSpec().Timeout = %v; want zero -- nothing on this path reads it, so an assertion of a resolved deadline would pin a dead field", got.Timeout)
		}
	})
	t.Run("KeepPane_ZeroValue_NothingReadsIt", func(t *testing.T) {
		if got.KeepPane {
			t.Error("driverSpec().KeepPane = true; want false (its zero value) -- nothing reads this, the wait loop is never entered")
		}
	})
}
