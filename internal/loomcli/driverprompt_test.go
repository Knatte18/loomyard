package loomcli

import (
	"strings"
	"testing"
)

// TestDriverPrompt_NamesRunIDReportPathAndAutonomousMode asserts the prompt names the run-id, the
// report path verbatim, states autonomous mode, and mentions no step cap.
func TestDriverPrompt_NamesRunIDReportPathAndAutonomousMode(t *testing.T) {
	runID := "self"
	reportPath := "/hub/worktree/.lyx/shed/self/drive-report-20260920-120000-cafe.md"

	got := driverPrompt(runID, reportPath)

	if !strings.Contains(got, runID) {
		t.Errorf("driverPrompt() = %q; want it to name the run-id %q", got, runID)
	}
	if !strings.Contains(got, reportPath) {
		t.Errorf("driverPrompt() = %q; want it to name the report path %q verbatim", got, reportPath)
	}
	if !strings.Contains(strings.ToLower(got), "autonomous") {
		t.Errorf("driverPrompt() = %q; want it to state autonomous mode explicitly", got)
	}
	if !strings.Contains(got, "ly plugin") {
		t.Errorf("driverPrompt() = %q; want it to name the ly plugin the skill ships in", got)
	}
	if !strings.Contains(got, "do not search the filesystem") {
		t.Errorf("driverPrompt() = %q; want it to forbid searching for a copy of a missing skill", got)
	}
	if strings.Contains(got, "step cap") {
		t.Errorf("driverPrompt() = %q; want no mention of a step cap", got)
	}
}

// TestDriverPrompt_StaysWellUnderLaunchPromptCap is a bound check, not an exact-length pin: the
// prompt must stay well under the Claude engine's maxLaunchPromptBytes for a realistic run-id and
// report path, since a prompt that grew into a copy of the skill would fail only at launch, after a
// bootstrap has already seeded and committed.
func TestDriverPrompt_StaysWellUnderLaunchPromptCap(t *testing.T) {
	runID := "self"
	reportPath := "/hub/some-realistic-worktree-name/.lyx/shed/self/drive-report-20260920-120000-cafe.md"

	got := driverPrompt(runID, reportPath)

	// 30000 mirrors claudeengine's maxLaunchPromptBytes; this package must not import
	// internal/shuttleengine/claudeengine (provider specifics stay there per the Shuttle
	// Provider-Seam Invariant), so the bound is restated here rather than imported.
	const wellUnderLaunchPromptCap = 1000
	if len(got) >= wellUnderLaunchPromptCap {
		t.Errorf("driverPrompt() length = %d; want well under %d (the launch prompt cap is 30000)", len(got), wellUnderLaunchPromptCap)
	}
}
