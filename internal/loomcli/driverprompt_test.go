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

// TestDriverPrompt asserts the rendered prompt carries every marker value and the parent directive for the given parent name, leaves no marker unfilled, and that a missing driver stencil is refused rather than rendered empty.
func TestDriverPrompt(t *testing.T) {
	t.Parallel()
	const runID = "operator-surface"
	const reportPath = "/hub/wt/.lyx/shed/operator-surface/drive-report-20260920-120000-cafe.md"

	// The teardown joins commit and removal by ';', so a failed commit still ends the session.
	if strings.Contains(driverTeardownCommand, "&&") {
		t.Errorf("driverTeardownCommand = %q; want no '&&', so removal runs after a failed commit", driverTeardownCommand)
	}

	cases := []struct {
		name           string
		parent         string
		removeStencil  bool
		wantStaticText map[string]string
	}{
		{
			name:   "carries every marker value",
			parent: "hub:orch",
			wantStaticText: map[string]string{
				"run-id":            "run `" + runID + "`",
				"report path":       reportPath,
				"park command":      driverParkCommand(reportPath),
				"teardown command":  driverTeardownCommand,
				"parent name":       "hub:orch",
				"literal teardown":  "lyx loom commit-records; lyx reed remove --name driver --detach",
				"literal park mark": `lyx loom commit-records --park "` + reportPath + `"`,
			},
		},
		{name: "no parent renders the no-parent variant", parent: ""},
		{name: "unreadable stencil is an error", removeStencil: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := seededStencils(t)
			if tc.removeStencil {
				stencilkit.Remove(t, dir, driverStencilName)
				if _, err := driverPrompt(dir, tc.parent, runID, reportPath); err == nil {
					t.Fatalf("driverPrompt() with %s missing from %s error = nil; want an error", driverStencilName, stencilstore.Path(dir, driverStencilName))
				}
				return
			}

			got, err := driverPrompt(dir, tc.parent, runID, reportPath)
			if err != nil {
				t.Fatalf("driverPrompt() error = %v; want nil", err)
			}
			directive, err := parentdirective.Directive(dir, tc.parent, false)
			if err != nil {
				t.Fatalf("parentdirective.Directive: %v", err)
			}
			if !strings.Contains(got, directive) {
				t.Errorf("driverPrompt() does not carry the parent directive %q", directive)
			}
			for name, want := range tc.wantStaticText {
				if !strings.Contains(got, want) {
					t.Errorf("driverPrompt() does not carry the %s %q", name, want)
				}
			}
			if strings.Contains(got, "{{") {
				t.Errorf("driverPrompt() left a marker unfilled")
			}
		})
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
