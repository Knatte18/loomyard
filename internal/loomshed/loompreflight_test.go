// loompreflight_test.go is the Tier-1 suite for newLoomPreflight's outcome mapping, driving it
// directly over status files under t.TempDir. It is untagged: no spawn, no git -- newLoomPreflight
// only delegates to loomengine.CheckSeed, which itself only stats a file, MkdirAll's a lock parent,
// and decodes JSON.
//
// This file does not re-test CheckSeed's own check set -- that is internal/loomengine's job (see
// its own seed_test.go), and duplicating it here would couple this package's tests to another
// package's checks, the same reasoning the moved Tier-2 wrapper test already records. It pins only
// the three-way outcome mapping: Done, Stuck, and a returned error.

package loomshed

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// writeLoomPreflightFixture writes a shedengine.Status shell naming currentProducer as
// current_producer, running, carrying one Done Preflight history entry and a coherent loom Status
// product, to statusPath/statusLockPath via state.WriteJSON.
//
// This is deliberately NOT the shape production Seed writes: Seed writes
// current_producer: "Preflight", which row 2 rejects because it tells NameLoomPreflight as the
// expected value, so a Seed-built fixture would yield Stuck and the "coherent seed -> Done" case
// below would silently assert the opposite of what it names. This helper instead writes the exact
// shape Shed itself persists after row 1 finishes: current_producer already advanced to row 2's own
// name, with row 1's Done entry appended to history.
func writeLoomPreflightFixture(t *testing.T, statusPath, statusLockPath, currentProducer string) {
	t.Helper()

	product, err := json.Marshal(loomengine.Status{Slug: "fixture-slug", Parent: "fixture-parent"})
	if err != nil {
		t.Fatalf("marshal product: %v", err)
	}

	shed := shedengine.Status{
		CurrentProducer: currentProducer,
		State:           shedengine.StateRunning,
		History: []shedengine.HistoryEntry{
			{Producer: NamePreflight, Outcome: shedengine.Done, At: "2026-07-17T10:01:30Z"},
		},
		PauseRequested: false,
		Product:        product,
	}

	if err := state.WriteJSON(statusPath, statusLockPath, shed); err != nil {
		t.Fatalf("state.WriteJSON(...) = %v", err)
	}
}

func TestLoomPreflight_Call_CoherentSeedReportsDone(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "status.json.lock")
	writeLoomPreflightFixture(t, statusPath, statusLockPath, NameLoomPreflight)

	p := NewLoomPreflight(NameLoomPreflight, statusPath, statusLockPath)
	shedfake.RequireOutcome(t, p, shedengine.Done)
}

func TestLoomPreflight_Call_IncoherentSeedReportsStuck(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "status.json.lock")
	// currentProducer is left at NamePreflight, which does not match the row's own told expected
	// name (NameLoomPreflight) -- an incoherent seed.
	writeLoomPreflightFixture(t, statusPath, statusLockPath, NamePreflight)

	p := NewLoomPreflight(NameLoomPreflight, statusPath, statusLockPath)
	shedfake.RequireOutcome(t, p, shedengine.Stuck)
}

func TestLoomPreflight_Call_LockParentUncreatableReturnsError(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	// The status file is written under a lock path distinct from the one Call is given below, so
	// the status file itself exists and is coherent -- the failure this test drives is provably
	// the lock-parent creation, not the stat.
	writeLoomPreflightFixture(t, statusPath, filepath.Join(dir, "seed.lock"), NameLoomPreflight)

	// blocker is a regular file, not a directory. Pointing statusLockPath at a child path beneath
	// it makes CheckSeed's os.MkdirAll(filepath.Dir(statusLockPath), ...) guard fail.
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	statusLockPath := filepath.Join(blocker, "status.json.lock")

	p := NewLoomPreflight(NameLoomPreflight, statusPath, statusLockPath)
	outcome, _, err := p.Call(context.Background())
	if err == nil {
		t.Fatalf("Call() error = nil; want non-nil (lock parent dir cannot be created)")
	}
	if outcome == shedengine.Done || outcome == shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want no verdict alongside an infra error", outcome)
	}
}

// TestLoomPreflight_Call_StuckReasonNamesTheFailures pins that the incoherent-seed Reason is the
// fixed prefix followed by formatSeedFailures of the same report, and that a seed failing a
// different check yields a different Reason.
func TestLoomPreflight_Call_StuckReasonNamesTheFailures(t *testing.T) {
	reasonFor := func(currentProducer string) (string, string) {
		dir := t.TempDir()
		statusPath := filepath.Join(dir, "status.json")
		statusLockPath := filepath.Join(dir, "status.json.lock")
		writeLoomPreflightFixture(t, statusPath, statusLockPath, currentProducer)

		report, err := loomengine.CheckSeed(statusPath, statusLockPath, NameLoomPreflight, []string{NamePreflight, NameLoomPreflight})
		if err != nil {
			t.Fatalf("CheckSeed() error = %v", err)
		}
		_, pointer := shedfake.CallOK(t, NewLoomPreflight(NameLoomPreflight, statusPath, statusLockPath))
		return pointer.Reason, "seed is not a coherent fresh start: " + formatSeedFailures(report)
	}

	got1, want1 := reasonFor(NamePreflight)
	if got1 != want1 {
		t.Errorf("Reason = %q; want %q", got1, want1)
	}
	got2, want2 := reasonFor("Some-Other-Producer")
	if got2 != want2 {
		t.Errorf("Reason = %q; want %q", got2, want2)
	}
	if got1 == got2 {
		t.Errorf("two different seed failures gave the same Reason %q", got1)
	}
}

// halfFinishedRun writes a blocked status file whose history reaches Discussion-Write,
// and returns the paths plus a goto helper that moves the run with shedengine.Goto as the verb does.
func halfFinishedRun(t *testing.T, slug string) (statusPath, statusLockPath string, gotoTo func(string)) {
	t.Helper()
	dir := t.TempDir()
	statusPath = filepath.Join(dir, "status.json")
	statusLockPath = filepath.Join(dir, "status.json.lock")
	runLockPath := filepath.Join(dir, "run.lock")

	product, err := json.Marshal(loomengine.Status{Slug: slug, Parent: "fixture-parent"})
	if err != nil {
		t.Fatalf("marshal product: %v", err)
	}
	if err := state.WriteJSON(statusPath, statusLockPath, shedengine.Status{
		CurrentProducer: NameDiscussionWrite,
		State:           shedengine.StateBlocked,
		History: []shedengine.HistoryEntry{
			{Producer: NamePreflight, Outcome: shedengine.Done, At: "2026-07-17T10:01:30Z"},
			{Producer: NameLoomPreflight, Outcome: shedengine.Done, At: "2026-07-17T10:01:31Z"},
			{Producer: NameDiscussionWrite, Outcome: shedengine.Done, At: "2026-07-17T10:01:32Z"},
		},
		Product: product,
	}); err != nil {
		t.Fatalf("state.WriteJSON(...) = %v", err)
	}

	producers := []shedengine.ProducerDef{{Name: NamePreflight}, {Name: NameLoomPreflight}, {Name: NameDiscussionWrite}}
	gotoTo = func(target string) {
		t.Helper()
		if _, err := shedengine.Goto(shedengine.GotoRequest{
			StatusPath:     statusPath,
			LockPath:       runLockPath,
			StatusLockPath: statusLockPath,
			Producers:      producers,
			Target:         target,
		}); err != nil {
			t.Fatalf("shedengine.Goto(%q) = %v", target, err)
		}
	}
	return statusPath, statusLockPath, gotoTo
}

// TestLoomPreflight_Call_GotoReentryPassesAndLeavesStatusUnchanged pins that a run deliberately moved back onto Loom-Preflight is a policy case: the half-finished failure is waived.
func TestLoomPreflight_Call_GotoReentryPassesAndLeavesStatusUnchanged(t *testing.T) {
	statusPath, statusLockPath, gotoTo := halfFinishedRun(t, "fixture-slug")
	gotoTo(NameLoomPreflight)
	before, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status: %v", err)
	}

	outcome, pointer := shedfake.CallOK(t, NewLoomPreflight(NameLoomPreflight, statusPath, statusLockPath))
	if outcome != shedengine.Done || pointer.Reason != "" {
		t.Errorf("Call() = (%q, reason %q); want (%q, no reason)", outcome, pointer.Reason, shedengine.Done)
	}
	after, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("status file changed by Call():\nbefore %s\nafter  %s", before, after)
	}
}

func TestLoomPreflight_Call_HalfFinishedWithoutGotoStuckNamesReentry(t *testing.T) {
	statusPath, statusLockPath, _ := halfFinishedRun(t, "fixture-slug")
	// Loom-Preflight is the current producer, but no goto entry marks a deliberate re-entry.
	if err := state.UpdateJSON(statusPath, statusLockPath, func(cur shedengine.Status, _ bool) (shedengine.Status, error) {
		cur.CurrentProducer = NameLoomPreflight
		return cur, nil
	}); err != nil {
		t.Fatalf("UpdateJSON: %v", err)
	}

	pointer := shedfake.RequireOutcome(t, NewLoomPreflight(NameLoomPreflight, statusPath, statusLockPath), shedengine.Stuck)
	for _, want := range []string{"seed a new run", "lyx loom goto --to " + NameLoomPreflight} {
		if !strings.Contains(pointer.Reason, want) {
			t.Errorf("Reason = %q; want it to contain %q", pointer.Reason, want)
		}
	}
	if strings.Contains(pointer.Reason, "--to "+NameDiscussionWrite) {
		t.Errorf("Reason = %q; want no forward goto to %s", pointer.Reason, NameDiscussionWrite)
	}
}

func TestLoomPreflight_Call_GotoThenLaterRowIsNotReentry(t *testing.T) {
	statusPath, statusLockPath, gotoTo := halfFinishedRun(t, "fixture-slug")
	gotoTo(NameLoomPreflight)
	if err := state.UpdateJSON(statusPath, statusLockPath, func(cur shedengine.Status, _ bool) (shedengine.Status, error) {
		cur.History = append(cur.History, shedengine.HistoryEntry{Producer: NameDiscussionWrite, Outcome: shedengine.Done, At: "2026-07-17T10:02:00Z"})
		return cur, nil
	}); err != nil {
		t.Fatalf("UpdateJSON: %v", err)
	}

	shedfake.RequireOutcome(t, NewLoomPreflight(NameLoomPreflight, statusPath, statusLockPath), shedengine.Stuck)
}

func TestLoomPreflight_Call_ReentryWithOtherFailureStaysStuck(t *testing.T) {
	// An empty slug fails CheckSeedIncoherent alongside the half-finished history.
	statusPath, statusLockPath, gotoTo := halfFinishedRun(t, "")
	gotoTo(NameLoomPreflight)

	pointer := shedfake.RequireOutcome(t, NewLoomPreflight(NameLoomPreflight, statusPath, statusLockPath), shedengine.Stuck)
	if !strings.Contains(pointer.Reason, string(loomengine.CheckSeedIncoherent)) {
		t.Errorf("Reason = %q; want it to name %q", pointer.Reason, loomengine.CheckSeedIncoherent)
	}
}
