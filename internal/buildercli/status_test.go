// status_test.go covers the status verb's four envelope shapes through
// RunCLI: uninitialized (no state.json), initialized with a mix of
// terminal and in-flight batches, an on-disk report promoting a
// not-yet-Terminal batch to its real status, and the paused flag surfacing
// correctly.

package buildercli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

func TestRunCLI_Status_Uninitialized(t *testing.T) {
	seedBuilderFixture(t)

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"status"})
	if exitCode != 0 {
		t.Fatalf("RunCLI([status]) = %d; want 0, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"initialized":false`) {
		t.Errorf("output missing initialized:false; got %q", out.String())
	}
}

func TestRunCLI_Status_Initialized(t *testing.T) {
	fixture := seedBuilderFixture(t)

	st := &builderengine.State{
		RunGUID:         "guid-1",
		PlanFingerprint: "fp-1",
		CurrentBatch:    2,
		Batches: map[int]*builderengine.BatchState{
			1: {Slug: "json-flag", StartSHA: "sha1", Role: "implementer", Terminal: true, Status: "done"},
			2: {Slug: "list-tests", StartSHA: "sha2", Role: "implementer", Terminal: false, Status: ""},
		},
	}
	builderDir := hubgeometry.BuilderDir(fixture.Hub)
	if err := builderengine.SaveState(builderDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"status"})
	if exitCode != 0 {
		t.Fatalf("RunCLI([status]) = %d; want 0, output: %s", exitCode, out.String())
	}
	got := out.String()
	for _, want := range []string{
		`"run_guid":"guid-1"`, `"current_batch":2`, `"plan_fingerprint":"fp-1"`,
		`"json-flag"`, `"done"`, `"paused":false`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q; got %q", want, got)
		}
	}
}

func TestRunCLI_Status_ReportOverridesStaleState(t *testing.T) {
	fixture := seedBuilderFixture(t)

	st := &builderengine.State{
		Batches: map[int]*builderengine.BatchState{
			1: {Slug: "json-flag", StartSHA: "sha1", Role: "implementer", Terminal: false},
		},
	}
	builderDir := hubgeometry.BuilderDir(fixture.Hub)
	if err := builderengine.SaveState(builderDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	reportsDir := hubgeometry.BuilderReportsDir(fixture.Hub)
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatalf("mkdir reports dir: %v", err)
	}
	reportPath := filepath.Join(reportsDir, "01-json-flag.yaml")
	if err := os.WriteFile(reportPath, []byte("batch: 01-json-flag\nstatus: done\ntests: green\nstuck_reason: null\n"), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"status"})
	if exitCode != 0 {
		t.Fatalf("RunCLI([status]) = %d; want 0, output: %s", exitCode, out.String())
	}
	got := out.String()
	if !strings.Contains(got, `"terminal":true`) || !strings.Contains(got, `"status":"done"`) {
		t.Errorf("output does not reflect the on-disk report overriding stale state; got %q", got)
	}
}

func TestRunCLI_Status_PausedTrue(t *testing.T) {
	fixture := seedBuilderFixture(t)

	builderDir := hubgeometry.BuilderDir(fixture.Hub)
	st := &builderengine.State{RunGUID: "guid-2"}
	if err := builderengine.SaveState(builderDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	if err := builderengine.RequestPause(builderDir); err != nil {
		t.Fatalf("RequestPause() error = %v", err)
	}

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"status"})
	if exitCode != 0 {
		t.Fatalf("RunCLI([status]) = %d; want 0, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"paused":true`) {
		t.Errorf("output missing paused:true; got %q", out.String())
	}
}
