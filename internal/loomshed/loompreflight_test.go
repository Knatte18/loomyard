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
	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
}

func TestLoomPreflight_Call_IncoherentSeedReportsStuck(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "status.json.lock")
	// currentProducer is left at NamePreflight, which does not match the row's own told expected
	// name (NameLoomPreflight) -- an incoherent seed.
	writeLoomPreflightFixture(t, statusPath, statusLockPath, NamePreflight)

	p := NewLoomPreflight(NameLoomPreflight, statusPath, statusLockPath)
	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
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
		_, pointer, err := NewLoomPreflight(NameLoomPreflight, statusPath, statusLockPath).Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
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

// TestLoomPreflight_Call_HalfFinishedStuckNamesGoto pins the way forward on the half-finished refusal:
// a run moved onto Loom-Preflight over a history that has already passed it is Stuck with a reason naming goto,
// and taking that goto makes Discussion-Write the current producer.
func TestLoomPreflight_Call_HalfFinishedStuckNamesGoto(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "status.json.lock")
	runLockPath := filepath.Join(dir, "run.lock")

	product, err := json.Marshal(loomengine.Status{Slug: "fixture-slug", Parent: "fixture-parent"})
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

	producers := []shedengine.ProducerDef{{Name: NameLoomPreflight}, {Name: NameDiscussionWrite}}
	gotoTo := func(target string) {
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

	gotoTo(NameLoomPreflight)
	outcome, pointer, err := NewLoomPreflight(NameLoomPreflight, statusPath, statusLockPath).Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Fatalf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	want := "lyx loom goto --to " + NameDiscussionWrite
	if !strings.Contains(pointer.Reason, want) {
		t.Errorf("Reason = %q; want it to contain %q", pointer.Reason, want)
	}

	gotoTo(NameDiscussionWrite)
	st, found, err := state.ReadJSONStrict[shedengine.Status](statusPath, statusLockPath)
	if err != nil || !found {
		t.Fatalf("ReadJSONStrict() = found %v, err %v", found, err)
	}
	if st.CurrentProducer != NameDiscussionWrite {
		t.Errorf("CurrentProducer = %q; want %q after taking the way forward", st.CurrentProducer, NameDiscussionWrite)
	}
}
