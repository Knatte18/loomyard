// seedownership_test.go covers VerifySeedOwnership, F-B7's (round fable5-high-r3) guard: a status
// file recording a different task's slug — the state a worktree inherits when forked from a task
// worktree — must refuse loudly instead of letting the driver resume the inherited run.

package loomengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// writeSeedFixture writes a shedengine.Status carrying a loom Status product with slug, returning
// the status and lock paths.
func writeSeedFixture(t *testing.T, slug string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	lockPath := filepath.Join(dir, "status.json.lock")

	product, err := json.Marshal(Status{Slug: slug, Parent: "main"})
	if err != nil {
		t.Fatalf("marshal product: %v", err)
	}
	shed := shedengine.Status{
		CurrentProducer: "Preflight",
		State:           shedengine.StateRunning,
		History:         []shedengine.HistoryEntry{},
		Product:         product,
	}
	data, err := json.Marshal(shed)
	if err != nil {
		t.Fatalf("marshal shed status: %v", err)
	}
	if err := os.WriteFile(statusPath, data, 0o644); err != nil {
		t.Fatalf("write status fixture: %v", err)
	}
	return statusPath, lockPath
}

func TestVerifySeedOwnership(t *testing.T) {
	t.Run("MatchingSlugPasses", func(t *testing.T) {
		statusPath, lockPath := writeSeedFixture(t, "own-task")
		if err := VerifySeedOwnership(statusPath, lockPath, "own-task"); err != nil {
			t.Errorf("VerifySeedOwnership(matching) = %v; want nil", err)
		}
	})

	t.Run("InheritedSlugRefuses", func(t *testing.T) {
		statusPath, lockPath := writeSeedFixture(t, "old-task")
		err := VerifySeedOwnership(statusPath, lockPath, "new-task")
		if err == nil {
			t.Fatal("VerifySeedOwnership(inherited) = nil; want a refusal naming both slugs")
		}
		if !strings.Contains(err.Error(), "old-task") || !strings.Contains(err.Error(), "new-task") {
			t.Errorf("VerifySeedOwnership(inherited) = %v; want it to name both the recorded and the expected slug", err)
		}
	})

	t.Run("MissingFilePasses", func(t *testing.T) {
		dir := t.TempDir()
		if err := VerifySeedOwnership(filepath.Join(dir, "absent.json"), filepath.Join(dir, "absent.json.lock"), "any"); err != nil {
			t.Errorf("VerifySeedOwnership(missing file) = %v; want nil — missing-file handling is the caller's own", err)
		}
	})

	// DecodeFailurePasses pins that a malformed status file (an unknown top-level field, exactly
	// what a corrupted or hand-edited status.json produces) is ownership's business to pass on, not
	// to refuse. A decode failure is diagnosed by the spawned driver's Shed.Run step-1 read gate,
	// which does the same strict read at the top of its loop and errors on it before any producer
	// (the Loom-Preflight row that calls CheckSeed included) is ever looked up — so CheckSeed itself
	// never runs on a decode failure, and this ownership check must not stand in for the step-1 gate.
	// Reproduced live: before this test existed, a poisoned status file made both "lyx loom run" and
	// "lyx loom drive" refuse on the envelope before ever spawning a driver, contradicting this
	// function's own doc comment ("each is some other check's business").
	t.Run("DecodeFailurePasses", func(t *testing.T) {
		dir := t.TempDir()
		statusPath := filepath.Join(dir, "status.json")
		lockPath := filepath.Join(dir, "status.json.lock")
		if err := os.WriteFile(statusPath, []byte(`{"current_producer":"Preflight","state":"running","history":[],"__unknown_field__":true}`), 0o644); err != nil {
			t.Fatalf("write malformed status fixture: %v", err)
		}
		if err := VerifySeedOwnership(statusPath, lockPath, "any"); err != nil {
			t.Errorf("VerifySeedOwnership(decode failure) = %v; want nil — a decode failure is CheckSeed's business, not ownership's", err)
		}
	})
}
