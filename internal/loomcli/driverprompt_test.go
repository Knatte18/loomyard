package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// seededStencils returns a stencils directory holding every shipped stencil.
func seededStencils(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "stencils")
	stencilkit.SeedInto(t, dir)
	return dir
}

// TestDriverPrompt_CarriesEveryMarkerValue asserts the rendered prompt carries the run-id, the report path, the park command, the teardown command and the parent directive, and leaves no marker unfilled.
func TestDriverPrompt_CarriesEveryMarkerValue(t *testing.T) {
	t.Parallel()
	dir := seededStencils(t)
	runID := "operator-surface"
	reportPath := "/hub/wt/.lyx/shed/operator-surface/drive-report-20260920-120000-cafe.md"

	got, err := driverPrompt(dir, "hub:orch", runID, reportPath)
	if err != nil {
		t.Fatalf("driverPrompt() error = %v; want nil", err)
	}

	directive, err := parentdirective.Directive(dir, "hub:orch", false)
	if err != nil {
		t.Fatalf("parentdirective.Directive: %v", err)
	}
	for name, want := range map[string]string{
		"run-id":            "run `" + runID + "`",
		"report path":       reportPath,
		"park command":      driverParkCommand(reportPath),
		"teardown command":  driverTeardownCommand,
		"parent directive":  directive,
		"parent name":       "hub:orch",
		"literal teardown":  "lyx loom commit-records; lyx reed remove --name driver --detach",
		"literal park mark": `lyx loom commit-records --park "` + reportPath + `"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("driverPrompt() does not carry the %s %q", name, want)
		}
	}
	if strings.Contains(got, "{{") {
		t.Errorf("driverPrompt() left a marker unfilled")
	}
}

// TestDriverPrompt_NoParentRendersNoParentVariant asserts an empty parent name renders the directive's no-parent variant.
func TestDriverPrompt_NoParentRendersNoParentVariant(t *testing.T) {
	t.Parallel()
	dir := seededStencils(t)

	got, err := driverPrompt(dir, "", "run", "/hub/wt/report.md")
	if err != nil {
		t.Fatalf("driverPrompt() error = %v; want nil", err)
	}

	none, err := parentdirective.Directive(dir, "", false)
	if err != nil {
		t.Fatalf("parentdirective.Directive: %v", err)
	}
	if !strings.Contains(got, none) {
		t.Errorf("driverPrompt() does not carry the no-parent directive %q", none)
	}
}

// TestDriverPrompt_UnreadableStencilIsAnError asserts a missing driver stencil is refused rather than rendered empty.
func TestDriverPrompt_UnreadableStencilIsAnError(t *testing.T) {
	t.Parallel()
	dir := seededStencils(t)
	stencilkit.Remove(t, dir, driverStencilName)

	if _, err := driverPrompt(dir, "", "run", "/hub/wt/report.md"); err == nil {
		t.Fatalf("driverPrompt() with %s missing from %s error = nil; want an error", driverStencilName, stencilstore.Path(dir, driverStencilName))
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
