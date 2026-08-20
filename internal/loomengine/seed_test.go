// seed_test.go is the Tier-1 suite for CheckSeed, driving it directly over t.TempDir paths.
// It is untagged: no spawn, no git -- CheckSeed itself only stats a file, MkdirAll's a lock
// parent, and decodes JSON.

package loomengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// writeSeed marshals product into shedengine.Status's Product field and writes the resulting shell
// to statusPath/statusLockPath via state.WriteJSON, failing the test on error.
func writeSeed(t *testing.T, statusPath, statusLockPath string, shed shedengine.Status, product Status) {
	t.Helper()

	raw, err := json.Marshal(product)
	if err != nil {
		t.Fatalf("writeSeed: marshal product: %v", err)
	}
	shed.Product = raw

	if err := os.MkdirAll(filepath.Dir(statusLockPath), 0o755); err != nil {
		t.Fatalf("writeSeed: create status lock parent dir: %v", err)
	}
	if err := state.WriteJSON(statusPath, statusLockPath, shed); err != nil {
		t.Fatalf("writeSeed: state.WriteJSON(...) = %v", err)
	}
}

// coherentFreshShed returns a coherent, fresh shedengine.Status shell naming expectedProducer as
// current_producer, running, with one Done history entry naming a tolerated producer.
func coherentFreshShed(expectedProducer, historyProducer string) shedengine.Status {
	return shedengine.Status{
		CurrentProducer: expectedProducer,
		State:           shedengine.StateRunning,
		History: []shedengine.HistoryEntry{
			{Producer: historyProducer, Outcome: shedengine.Done, At: "2026-07-17T10:01:30Z"},
		},
	}
}

// coherentFreshProduct returns a coherent, fresh loom Status product: non-empty slug/parent and a
// null start_sha.
func coherentFreshProduct() Status {
	return Status{
		Slug:   "loom-contracts",
		Parent: "main",
	}
}

func TestCheckSeed_StatusFileMissing(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "ephemeral", "status.json.lock")

	report, err := CheckSeed(statusPath, statusLockPath, "Loom-Preflight", []string{"Preflight", "Loom-Preflight"})
	if err != nil {
		t.Fatalf("CheckSeed(...) error = %v; want nil", err)
	}
	if report.OK {
		t.Errorf("CheckSeed(...).OK = true; want false")
	}
	if !containsCheck(report.Failures, CheckSeedMissing) {
		t.Errorf("CheckSeed(...).Failures = %+v; want to contain %q", report.Failures, CheckSeedMissing)
	}
}

func TestCheckSeed_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "ephemeral", "status.json.lock")

	if err := os.WriteFile(statusPath, []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("write malformed status file: %v", err)
	}

	report, err := CheckSeed(statusPath, statusLockPath, "Loom-Preflight", []string{"Preflight", "Loom-Preflight"})
	if err != nil {
		t.Fatalf("CheckSeed(...) error = %v; want nil", err)
	}
	if !containsCheck(report.Failures, CheckSeedIncoherent) {
		t.Errorf("CheckSeed(...).Failures = %+v; want to contain %q", report.Failures, CheckSeedIncoherent)
	}
}

func TestCheckSeed_UnknownTopLevelField(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "ephemeral", "status.json.lock")

	body := `{"current_producer":"Loom-Preflight","state":"running","error":"","pause_requested":false,"activity":{"now":"","last":"","wait":""},"history":[],"product":{},"bogus_field":true}`
	if err := os.WriteFile(statusPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write status file with unknown field: %v", err)
	}

	report, err := CheckSeed(statusPath, statusLockPath, "Loom-Preflight", []string{"Preflight", "Loom-Preflight"})
	if err != nil {
		t.Fatalf("CheckSeed(...) error = %v; want nil", err)
	}
	if !containsCheck(report.Failures, CheckSeedIncoherent) {
		t.Errorf("CheckSeed(...).Failures = %+v; want to contain %q", report.Failures, CheckSeedIncoherent)
	}
}

func TestCheckSeed_ProductDoesNotDecodeAsLoomStatus(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "ephemeral", "status.json.lock")

	shed := coherentFreshShed("Loom-Preflight", "Loom-Preflight")
	shed.Product = []byte(`{"slug": 7}`)
	if err := os.MkdirAll(filepath.Dir(statusLockPath), 0o755); err != nil {
		t.Fatalf("create status lock parent dir: %v", err)
	}
	if err := state.WriteJSON(statusPath, statusLockPath, shed); err != nil {
		t.Fatalf("state.WriteJSON(...) = %v", err)
	}

	report, err := CheckSeed(statusPath, statusLockPath, "Loom-Preflight", []string{"Preflight", "Loom-Preflight"})
	if err != nil {
		t.Fatalf("CheckSeed(...) error = %v; want nil", err)
	}
	found := false
	for _, f := range report.Failures {
		if f.Check == CheckSeedIncoherent && strings.Contains(f.Reason, "product does not decode as loom's status shape") {
			found = true
		}
	}
	if !found {
		t.Errorf("CheckSeed(...).Failures = %+v; want a %q entry whose reason contains %q", report.Failures, CheckSeedIncoherent, "product does not decode as loom's status shape")
	}
}

func TestCheckSeed_CoherentPostRow1Seed(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "ephemeral", "status.json.lock")

	writeSeed(t, statusPath, statusLockPath, coherentFreshShed("Loom-Preflight", "Preflight"), coherentFreshProduct())

	report, err := CheckSeed(statusPath, statusLockPath, "Loom-Preflight", []string{"Preflight", "Loom-Preflight"})
	if err != nil {
		t.Fatalf("CheckSeed(...) error = %v; want nil", err)
	}
	if !report.OK {
		t.Errorf("CheckSeed(...) = %+v; want OK true, no failures", report)
	}
}

func TestCheckSeed_MkdirAllGuardRegression(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	// statusLockPath sits several directory levels deep in a tree that does not exist yet --
	// the guard is what stops a worktree with no ephemeral tree from escalating this to an
	// infra error.
	statusLockPath := filepath.Join(dir, "a", "b", "c", "d", "status.json.lock")

	shed := coherentFreshShed("Loom-Preflight", "Preflight")
	product := coherentFreshProduct()
	raw, err := json.Marshal(product)
	if err != nil {
		t.Fatalf("marshal product: %v", err)
	}
	shed.Product = raw
	// The status file itself is written to an already-existing directory -- only the lock
	// parent is missing, since that is what the guard exists to create.
	if err := state.WriteJSON(statusPath, filepath.Join(dir, "seed.lock"), shed); err != nil {
		t.Fatalf("state.WriteJSON(...) = %v", err)
	}

	report, err := CheckSeed(statusPath, statusLockPath, "Loom-Preflight", []string{"Preflight", "Loom-Preflight"})
	if err != nil {
		t.Fatalf("CheckSeed(...) error = %v; want nil (a determined verdict, not an infra error)", err)
	}
	if !report.OK {
		t.Errorf("CheckSeed(...) = %+v; want OK true, no failures", report)
	}
}

func TestCheckSeed_ToldNamesAreGenuinelyTold(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "ephemeral", "status.json.lock")

	writeSeed(t, statusPath, statusLockPath, coherentFreshShed("Loom-Preflight", "Preflight"), coherentFreshProduct())

	report, err := CheckSeed(statusPath, statusLockPath, "Some-Other-Producer", []string{"Some-Other-Producer"})
	if err != nil {
		t.Fatalf("CheckSeed(...) error = %v; want nil", err)
	}
	if !containsCheck(report.Failures, CheckSeedIncoherent) {
		t.Errorf("CheckSeed(...).Failures = %+v; want to contain %q", report.Failures, CheckSeedIncoherent)
	}
}
