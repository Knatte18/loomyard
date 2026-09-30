package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDriverPrompt_NamesRunIDReportPathAndAutonomousMode asserts the prompt names the run-id, the
// report path verbatim, states autonomous mode, and mentions no step cap.
func TestDriverPrompt_NamesRunIDReportPathAndAutonomousMode(t *testing.T) {
	runID := "worktree"
	reportPath := "/hub/worktree/.lyx/shed/worktree/drive-report-20260920-120000-cafe.md"

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

// TestDriverPrompt_NamesSlugRunIDAndExactTeardownCommand asserts the prompt addresses the run by its
// slug and ends with the exact end-of-session command, spelled out literally so a drifted strand
// name or flag fails here.
func TestDriverPrompt_NamesSlugRunIDAndExactTeardownCommand(t *testing.T) {
	got := driverPrompt("operator-surface", "/hub/wt/.lyx/shed/operator-surface/drive-report-x.md")

	if !strings.Contains(got, `"operator-surface"`) {
		t.Errorf("driverPrompt() = %q; want it to name the slug run-id", got)
	}
	const want = "lyx loom commit-records; lyx reed remove --name loom-driver --detach"
	if !strings.Contains(got, want) {
		t.Errorf("driverPrompt() = %q; want it to name the teardown command %q", got, want)
	}
	if n := strings.Count(got, driverTeardownCommand); n != 1 {
		t.Errorf("driverPrompt() = %q; teardown command appears %d times, want exactly once", got, n)
	}
}

// TestDriverPrompt_TiesTeardownToDoneAndBusyAndParksElsewhere asserts the teardown command sits between the done/busy condition and the parking sentence.
// That keeps a done child's driver exiting before batten's teardown, and every other stop parking.
// It also asserts the parking sentence names the park command, with the report path, which the recipe-blind skill runs at a park.
func TestDriverPrompt_TiesTeardownToDoneAndBusyAndParksElsewhere(t *testing.T) {
	got := driverPrompt("run", "/hub/wt/report.md")

	done := strings.Index(got, "the run is done, or a step is refused as busy")
	teardown := strings.Index(got, driverTeardownCommand)
	park := strings.Index(got, "At every other stop, park")
	if done < 0 || teardown < 0 || park < 0 || done >= teardown || teardown >= park {
		t.Fatalf("driverPrompt() = %q; want done/busy condition, then the teardown command, then parking for every other stop", got)
	}
	if !strings.Contains(got, "leave this session open") {
		t.Errorf("driverPrompt() = %q; want it to leave the session open when parking", got)
	}
	want := `lyx loom commit-records --park "/hub/wt/report.md"`
	if !strings.Contains(got[park:], want) {
		t.Errorf("driverPrompt() = %q; want the parking sentence to name the park command %q", got, want)
	}
}

// TestWriteParkMarker_CreatesDirAndHoldsReportPath asserts the marker is written under a not-yet-existing directory and holds the report path.
func TestWriteParkMarker_CreatesDirAndHoldsReportPath(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "shed", "run", "driver-parked")
	if err := writeParkMarker(marker, "/hub/wt/report.md"); err != nil {
		t.Fatalf("writeParkMarker() error = %v; want nil", err)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if string(got) != "/hub/wt/report.md" {
		t.Errorf("marker = %q; want %q", got, "/hub/wt/report.md")
	}
}

// TestDriverTeardownCommand_CommitsRecordsBeforeRemovingStrandJoinedBySemicolon asserts the records commit precedes the strand removal and the two are joined by `;`, so a failed commit still ends the session.
func TestDriverTeardownCommand_CommitsRecordsBeforeRemovingStrandJoinedBySemicolon(t *testing.T) {
	commit := strings.Index(driverTeardownCommand, "lyx loom commit-records")
	remove := strings.Index(driverTeardownCommand, "lyx reed remove")
	if commit < 0 || remove < 0 || commit >= remove {
		t.Fatalf("driverTeardownCommand = %q; want lyx loom commit-records before lyx reed remove", driverTeardownCommand)
	}
	if !strings.Contains(driverTeardownCommand, "lyx loom commit-records; lyx reed remove") {
		t.Errorf("driverTeardownCommand = %q; want the two joined by ';'", driverTeardownCommand)
	}
	if strings.Contains(driverTeardownCommand, "&&") {
		t.Errorf("driverTeardownCommand = %q; want no '&&', so removal runs after a failed commit", driverTeardownCommand)
	}
}

// TestDriverPrompt_StaysWellUnderLaunchPromptCap is a bound check, not an exact-length pin: the
// prompt must stay well under the Claude engine's maxLaunchPromptBytes for a realistic run-id and
// report path, since a prompt that grew into a copy of the skill would fail only at launch, after a
// bootstrap has already seeded and committed.
func TestDriverPrompt_StaysWellUnderLaunchPromptCap(t *testing.T) {
	runID := "some-realistic-worktree-name"
	reportPath := "/hub/some-realistic-worktree-name/.lyx/shed/some-realistic-worktree-name/drive-report-20260920-120000-cafe.md"

	got := driverPrompt(runID, reportPath)

	// 30000 mirrors claudeengine's maxLaunchPromptBytes; this package must not import
	// internal/shuttleengine/claudeengine (provider specifics stay there per the Shuttle
	// Provider-Seam Invariant), so the bound is restated here rather than imported.
	// The report path appears twice (report target and park command), so the bound leaves room for a long one;
	// a copied-in skill runs to several kilobytes and still fails it.
	const wellUnderLaunchPromptCap = 1500
	if len(got) >= wellUnderLaunchPromptCap {
		t.Errorf("driverPrompt() length = %d; want well under %d (the launch prompt cap is 30000)", len(got), wellUnderLaunchPromptCap)
	}
}
