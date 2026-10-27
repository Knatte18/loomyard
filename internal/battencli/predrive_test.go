//go:build tier0

// predrive_test.go pins the pre-drive check at its two call sites: the seeding of an absent status, and the shed's run lock for a resume.

package battencli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/state"
)

// newPreDriveReceiver returns a fake receiver whose pre-drive check reads the real cancel marker and seed under its location.
func newPreDriveReceiver(t *testing.T) *battenCLI {
	t.Helper()
	c := newFakeReceiver(t, nil)
	c.guard = startGuard{Marker: func() (cancelMarkerReading, error) { return readCancelMarker(c.location, c.slug) }}
	c.preDrive = c.preDriveCheck
	return c
}

// writeBattenSeed writes the seed of a batten run of the receiver's slug.
func writeBattenSeed(t *testing.T, c *battenCLI) {
	t.Helper()
	if err := shedrun.WriteSeed(c.location, c.slug, shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo}); err != nil {
		t.Fatalf("write the seed: %v", err)
	}
}

// requireNoStatus fails the test when the receiver's status file exists.
func requireNoStatus(t *testing.T, c *battenCLI) {
	t.Helper()
	if _, err := os.Stat(c.shedPaths.StatusPath); !os.IsNotExist(err) {
		t.Errorf("stat of the status file = %v; want it absent after the refusal", err)
	}
}

// TestPreDriveCheck drives the check through both pre-run hooks and the built shed: a seeded run with no marker passes, a marker refuses with the cancel's way forward, and a missing seed refuses naming the fresh start.
// Each refusal writes no status and clears no run marker.
func TestPreDriveCheck(t *testing.T) {
	t.Parallel()

	const markerWayForward = "way forward: wait for the cancel"
	const freshStartWayForward = `way forward: run "lym board set-status '{"slug":"some-slug","status":null}'" and "lym batten run some-slug" for a fresh start`

	t.Run("SeededRunWithNoMarkerSeedsTheStatusAndClearsTheRunMarkers", func(t *testing.T) {
		t.Parallel()
		c := newPreDriveReceiver(t)
		writeBattenSeed(t, c)
		rowNotice := filepath.Join(c.env.ScratchDir, battenrecipe.NameWorktreeTeardown+"-row-notice")
		if err := os.WriteFile(rowNotice, []byte("failure 3\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		kind, err := c.battenPreStep(context.Background())

		if err != nil || kind != "" {
			t.Fatalf("battenPreStep() = (%q, %v); want (\"\", nil)", kind, err)
		}
		status, found, err := state.ReadJSONNoMkdir[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
		if err != nil || !found || status.CurrentProducer != battenrecipe.NameWorktreeCreate || status.State != shedengine.StateRunning {
			t.Errorf("status after battenPreStep = (%+v, found %v, err %v); want a running Worktree-Create", status, found, err)
		}
		if _, err := os.Stat(rowNotice); !os.IsNotExist(err) {
			t.Errorf("stat of the row-notice marker = %v; want it cleared by the seeding", err)
		}
	})

	t.Run("CancelMarkerRefusesBothHooksWritingAndClearingNothing", func(t *testing.T) {
		t.Parallel()
		c := newPreDriveReceiver(t)
		writeBattenSeed(t, c)
		held, err := takeCancelMarker(c.location, c.slug, time.Second)
		if err != nil {
			t.Fatalf("takeCancelMarker: %v", err)
		}
		t.Cleanup(func() { _ = held.release(true) })
		rowNotice := filepath.Join(c.env.ScratchDir, battenrecipe.NameWorktreeTeardown+"-row-notice")
		if err := os.WriteFile(rowNotice, []byte("failure 3\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		kind, stepErr := c.battenPreStep(context.Background())
		runErr := c.battenPreRun(context.Background())

		if kind != shedverbs.KindBusy {
			t.Errorf("battenPreStep() kind = %q; want %q", kind, shedverbs.KindBusy)
		}
		for name, err := range map[string]error{"battenPreStep": stepErr, "battenPreRun": runErr} {
			if err == nil || !strings.Contains(err.Error(), markerWayForward) || !strings.HasSuffix(err.Error(), "then run the verb again") {
				t.Errorf("%s() = %v; want the cancel refusal ending in its way forward", name, err)
			}
		}
		requireNoStatus(t, c)
		if _, err := os.Stat(rowNotice); err != nil {
			t.Errorf("stat of the planted run marker = %v; want it left in place", err)
		}
	})

	t.Run("MissingSeedRefusesBothHooksNamingTheFreshStart", func(t *testing.T) {
		t.Parallel()
		c := newPreDriveReceiver(t)

		kind, stepErr := c.battenPreStep(context.Background())
		runErr := c.battenPreRun(context.Background())

		if kind != shedverbs.KindBusy {
			t.Errorf("battenPreStep() kind = %q; want %q", kind, shedverbs.KindBusy)
		}
		for name, err := range map[string]error{"battenPreStep": stepErr, "battenPreRun": runErr} {
			if err == nil || !strings.HasSuffix(err.Error(), freshStartWayForward) {
				t.Errorf("%s() = %v; want a refusal ending in %q", name, err, freshStartWayForward)
			}
		}
		requireNoStatus(t, c)
	})

	t.Run("BuiltShedRefusesUnderTheRunLockCallingNoProducerAndLeavingTheStatus", func(t *testing.T) {
		t.Parallel()
		c := newPreDriveReceiver(t)
		writeBattenSeed(t, c)
		running := shedengine.Status{CurrentProducer: battenrecipe.NameWorktreeCreate, State: shedengine.StateRunning, History: []shedengine.HistoryEntry{}}
		if err := state.WriteJSON(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, running); err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(c.shedPaths.StatusPath)
		if err != nil {
			t.Fatal(err)
		}
		held, err := takeCancelMarker(c.location, c.slug, time.Second)
		if err != nil {
			t.Fatalf("takeCancelMarker: %v", err)
		}
		t.Cleanup(func() { _ = held.release(true) })
		var producerCalled bool
		c.env.CreateWorktree = func(context.Context) error {
			producerCalled = true
			return nil
		}

		shed, err := c.specFor("step").BuildShed()
		if err != nil {
			t.Fatalf("BuildShed: %v", err)
		}
		_, err = shed.Step(context.Background())

		if err == nil || !strings.Contains(err.Error(), markerWayForward) {
			t.Fatalf("Step() = %v; want the cancel refusal", err)
		}
		if producerCalled {
			t.Error("Step() called a producer; want none after the refusal")
		}
		after, err := os.ReadFile(c.shedPaths.StatusPath)
		if err != nil || string(after) != string(before) {
			t.Errorf("status after the refusal = %q (err %v); want it unchanged from %q", after, err, before)
		}
	})
}
