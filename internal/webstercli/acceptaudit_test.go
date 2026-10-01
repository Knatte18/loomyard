//go:build integration

// acceptaudit_test.go covers the accept-audit verb through its cobra.Command, on the verbs fixture's hub.
package webstercli

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestAcceptAuditCmd_AcceptsPendingThenIsIdempotent proves the verb lists a pending finding, clears it and records its identity as accepted, and that a second call reports nothing.
func TestAcceptAuditCmd_AcceptsPendingThenIsIdempotent(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	st := fx.initState(t, "master-model")
	st.PendingAuditFindings = []websterengine.PendingAuditFinding{{ID: "sess/parent-write-1", Class: "parent-write", Detail: "wrote outside the contract", Paths: []string{"internal/x.go"}}}
	if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	var out strings.Builder
	if code := clihelp.Execute(fx.CLI.acceptAuditCmd(), &out, nil); code != 0 {
		t.Fatalf("accept-audit = %d; want 0, output: %s", code, out.String())
	}
	for _, want := range []string{"parent-write", "wrote outside the contract", "internal/x.go"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q; got %q", want, out.String())
		}
	}
	loaded, err := websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() = %v, %v; want a state, nil", loaded, err)
	}
	if len(loaded.PendingAuditFindings) != 0 {
		t.Errorf("PendingAuditFindings = %v; want none", loaded.PendingAuditFindings)
	}
	if got := loaded.AuditDispositions["sess/parent-write-1"]; got != "accepted" {
		t.Errorf("disposition = %q; want accepted", got)
	}

	out.Reset()
	if code := clihelp.Execute(fx.CLI.acceptAuditCmd(), &out, nil); code != 0 {
		t.Fatalf("second accept-audit = %d; want 0, output: %s", code, out.String())
	}
	if !strings.Contains(out.String(), `"accepted":[]`) {
		t.Errorf("second call output = %q; want an empty accepted list", out.String())
	}
}

// TestAcceptAuditCmd_NoRunRefuses proves the verb names the run verb when there is no state.json.
func TestAcceptAuditCmd_NoRunRefuses(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newVerbsFixture(t)

	var out strings.Builder
	if code := clihelp.Execute(fx.CLI.acceptAuditCmd(), &out, nil); code == 0 {
		t.Fatalf("accept-audit with no run = 0; want non-zero, output: %s", out.String())
	}
	if !strings.Contains(out.String(), "webster run") {
		t.Errorf("output = %q; want it to name the run verb", out.String())
	}
}

// TestAcceptAuditCmd_FabricSyncFailureNamesWayForward proves a fabric sync failure still leaves the acceptance saved and names the sentinel and `lyx fabric commit`.
func TestAcceptAuditCmd_FabricSyncFailureNamesWayForward(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "")
	fx := newVerbsFixture(t)
	st := fx.initState(t, "master-model")
	st.PendingAuditFindings = []websterengine.PendingAuditFinding{{ID: "sess/parent-write-1", Class: "parent-write", Detail: "wrote outside the contract", Paths: []string{"internal/x.go"}}}
	if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	syncErr := errors.New("probe: fabric unreachable")
	fx.CLI.openFabric = func() (*fabricengine.Fabric, error) { return nil, syncErr }

	var out strings.Builder
	if code := clihelp.Execute(fx.CLI.acceptAuditCmd(), &out, nil); code == 0 {
		t.Fatalf("accept-audit = 0; want non-zero, output: %s", out.String())
	}
	for _, want := range []string{"audit findings accepted but the fabric sync failed", syncErr.Error(), "lyx fabric commit"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q; got %q", want, out.String())
		}
	}
	loaded, err := websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() = %v, %v; want a state, nil", loaded, err)
	}
	if len(loaded.PendingAuditFindings) != 0 {
		t.Errorf("PendingAuditFindings = %v; want none", loaded.PendingAuditFindings)
	}
	if got := loaded.AuditDispositions["sess/parent-write-1"]; got != "accepted" {
		t.Errorf("disposition = %q; want accepted", got)
	}
}
