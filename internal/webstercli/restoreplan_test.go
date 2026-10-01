//go:build integration

// restoreplan_test.go covers the restore-plan verb through its cobra.Command, on the verbs fixture's hub.
package webstercli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
)

// TestRestorePlanCmd_RestoresEditedCard proves an edited card is reported and written back, with state.json byte-identical.
func TestRestorePlanCmd_RestoresEditedCard(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	fx.initState(t, "master-model")
	planDir := fx.CLI.geom.PlanDir
	cardPath := filepath.Join(planDir, "01-only.md")
	recorded, err := os.ReadFile(cardPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cardPath, []byte("edited after the run recorded the plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(fx.CLI.geom.WebsterDir, "state.json")
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	if code := clihelp.Execute(fx.CLI.restorePlanCmd(), &out, nil); code != 0 {
		t.Fatalf("restore-plan = %d; want 0, output: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "01-only.md") {
		t.Errorf("output %q does not name the restored card", out.String())
	}
	got, err := os.ReadFile(cardPath)
	if err != nil || string(got) != string(recorded) {
		t.Errorf("card = %q, %v; want the recorded bytes %q", got, err, recorded)
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("state.json changed by restore-plan")
	}
}

// TestRestorePlanCmd_NoRunNamesRun proves a missing state.json refuses toward `lyx webster run`.
func TestRestorePlanCmd_NoRunNamesRun(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newVerbsFixture(t)

	var out strings.Builder
	if code := clihelp.Execute(fx.CLI.restorePlanCmd(), &out, nil); code == 0 {
		t.Fatalf("restore-plan = 0; want non-zero, output: %s", out.String())
	}
	if !strings.Contains(out.String(), "lyx webster run") {
		t.Errorf("output %q does not name lyx webster run", out.String())
	}
}
