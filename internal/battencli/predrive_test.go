// predrive_test.go pins the pre-drive check at its two call sites:
// the seeding of an absent status, and the shed's run lock for a resume.

package battencli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newPreDriveReceiver returns a fake receiver.
// Its pre-drive check reads the real cancel lock and seed under its location.
func newPreDriveReceiver(t *testing.T) *battenCLI {
	t.Helper()
	c := newFakeReceiver(t, nil)
	c.guard = startGuard{Lock: func() (cancelMarkerReading, error) { return readCancelLock(c.location, c.slug) }}
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

// TestPreDriveCheck drives the check through both pre-run hooks and the built shed:
// a seeded run with no cancel lock held passes, a held cancel lock refuses with the cancel's way forward,
// and a missing seed refuses naming the fresh start.
// Each refusal writes no status and clears no run marker.
func TestPreDriveCheck(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const markerWayForward = "way forward: wait for it to end"
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
		status, found, err := state.ReadJSON[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
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
		t.Cleanup(func() { _ = held.release() })
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
		t.Cleanup(func() { _ = held.release() })
		var producerCalled bool
		c.env.CreateWorktree = func(context.Context, battenshed.HoldPrimeLock) error {
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

// TestFreshStartCommitsWorktreeCreateThroughTheSeam pins that seeding an absent status makes the run's one start push.
// Both pre-run hooks seed through the same call: the Worktree-Create commit pushes,
// and the Seed-Child row that follows commits without a push.
func TestFreshStartCommitsWorktreeCreateThroughTheSeam(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	hooks := map[string]func(*battenCLI) error{
		"battenPreRun":  func(c *battenCLI) error { return c.battenPreRun(context.Background()) },
		"battenPreStep": func(c *battenCLI) error { _, err := c.battenPreStep(context.Background()); return err },
	}
	for name, hook := range hooks {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newPreDriveReceiver(t)
			writeBattenSeed(t, c)
			markerPath := filepath.Join(t.TempDir(), "last-commit")
			var commits, pushes int
			c.commitStatus = newCommitStatusSeam(commitStatusDeps{
				MergeActive: func() (bool, error) { return false, nil },
				Commit:      func(string) (bool, error) { commits++; return true, nil },
				Push:        func() error { pushes++; return nil },
			}, markerPath, markerPath+".lock")

			if err := hook(c); err != nil {
				t.Fatalf("%s() = %v; want nil", name, err)
			}
			if commits != 1 || pushes != 1 {
				t.Fatalf("after the fresh start, commits = %d, pushes = %d; want 1 and 1", commits, pushes)
			}
			if err := c.commitStatus(battenrecipe.NameSeedChild, "running"); err != nil {
				t.Fatalf("commitStatus(Seed-Child, running) = %v; want nil", err)
			}

			if commits != 2 || pushes != 1 {
				t.Errorf("after Seed-Child, commits = %d, pushes = %d; want 2 and 1", commits, pushes)
			}
		})
	}
}

// TestFreshStartSeedCommitFailure pins that a failed commit of the seeded status is returned with its cause intact.
// The step hook reports it as a bootstrap failure rather than an unseeded run, since the status is already durable;
// the run hook returns an error that still matches the seam's failure.
func TestFreshStartSeedCommitFailure(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	boom := errors.New("push rejected")
	failing := func(c *battenCLI) {
		c.commitStatus = func(string, string) error { return boom }
	}

	t.Run("StepReportsBootstrap", func(t *testing.T) {
		t.Parallel()
		c := newPreDriveReceiver(t)
		writeBattenSeed(t, c)
		failing(c)

		kind, err := c.battenPreStep(context.Background())

		if kind != shedverbs.KindBootstrap || !errors.Is(err, boom) {
			t.Errorf("battenPreStep() = (%q, %v); want (%q, the seam failure)", kind, err, shedverbs.KindBootstrap)
		}
	})

	t.Run("RunReturnsTheSeamFailure", func(t *testing.T) {
		t.Parallel()
		c := newPreDriveReceiver(t)
		writeBattenSeed(t, c)
		failing(c)

		if err := c.battenPreRun(context.Background()); !errors.Is(err, boom) {
			t.Errorf("battenPreRun() = %v; want an error matching the seam failure", err)
		}
	})
}

func TestBuildShed_TellsTheMachine(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name        string
		machineName func() (string, error)
		wantMachine string
		wantErrText []string
	}{
		{"TellsTheName", func() (string, error) { return "neve", nil }, "neve", nil},
		{"RefusesAnUnknownName", func() (string, error) {
			return "", errors.New("name unreadable; way forward: run `lym machine name <name>`")
		}, "", []string{"name unreadable", "lym machine name", "lym batten run"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := newPreDriveReceiver(t)
			writeBattenSeed(t, c)
			c.machineName = tt.machineName

			shed, err := c.specFor("run").BuildShed()

			if tt.wantErrText != nil {
				if shed != nil {
					t.Errorf("BuildShed() shed = %+v; want nil", shed)
				}
				for _, text := range tt.wantErrText {
					if err == nil || !strings.Contains(err.Error(), text) {
						t.Errorf("BuildShed() error = %v; want it to contain %q", err, text)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("BuildShed() = %v", err)
			}
			if shed.Machine != tt.wantMachine {
				t.Errorf("BuildShed() Machine = %q; want %q", shed.Machine, tt.wantMachine)
			}
		})
	}
}
