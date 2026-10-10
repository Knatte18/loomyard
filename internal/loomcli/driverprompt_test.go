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
				"edit directive":    "Edit or Write",
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
				if _, err := driverPrompt(dir, tc.parent, runID, reportPath, false); err == nil {
					t.Fatalf("driverPrompt() with %s missing from %s error = nil; want an error", driverStencilName, stencilstore.Path(dir, driverStencilName))
				}
				return
			}

			got, err := driverPrompt(dir, tc.parent, runID, reportPath, false)
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
			if guide := stencilstore.Path(dir, driverGuideStencilName); !strings.Contains(got, guide) {
				t.Errorf("driverPrompt() does not carry the deployed guide path %q", guide)
			}
			if strings.Contains(got, "{{") {
				t.Errorf("driverPrompt() left a marker unfilled")
			}
		})
	}
}

// TestDriverPrompt_ParentNotifyRuleFollowsWatched asserts a watched prompt tells the driver to message the parent only for a relay, a question or a failed friction and never that the run stopped, and an unwatched prompt keeps the generic escalation line.
func TestDriverPrompt_ParentNotifyRuleFollowsWatched(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		watched bool
		want    []string
		wantNot []string
	}{
		{
			name:    "watched",
			watched: true,
			want:    []string{"`parent_notice`", "a question you cannot settle yourself", "`friction: failed`", "Never message the parent that the run stopped, halted or finished"},
			wantNot: []string{"At every escalation"},
		},
		{
			name:    "unwatched",
			watched: false,
			want:    []string{"`parent_notice`", "`friction: failed`", "At every escalation"},
			wantNot: []string{"Never message the parent that the run stopped"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := driverPrompt(seededStencils(t), "hub:orch", "operator-surface", "/hub/wt/report.md", tc.watched)
			if err != nil {
				t.Fatalf("driverPrompt() error = %v; want nil", err)
			}
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("driverPrompt() does not carry %q", want)
				}
			}
			for _, unwanted := range tc.wantNot {
				if strings.Contains(got, unwanted) {
					t.Errorf("driverPrompt() carries %q", unwanted)
				}
			}
			if strings.Contains(got, "{{") {
				t.Errorf("driverPrompt() left a marker unfilled")
			}
		})
	}
}

// TestDriverWatched asserts a marker holding a live pid reads as watched, and an absent marker, a garbage one and a dead pid read as unwatched.
func TestDriverWatched(t *testing.T) {
	t.Parallel()

	const livePID = 4242
	isAlive := func(pid int) bool { return pid == livePID }
	cases := []struct {
		name    string
		content *string
		want    bool
	}{
		{"live pid", ptr("4242\n"), true},
		{"dead pid", ptr("4243\n"), false},
		{"garbage", ptr("not a pid\n"), false},
		{"absent marker", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			marker := filepath.Join(t.TempDir(), "batten-watched")
			if tc.content != nil {
				if err := os.WriteFile(marker, []byte(*tc.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := driverWatched(marker, isAlive); got != tc.want {
				t.Errorf("driverWatched() = %v; want %v", got, tc.want)
			}
		})
	}
}

func ptr(s string) *string { return &s }

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
