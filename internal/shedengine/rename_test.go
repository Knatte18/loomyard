package shedengine

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
)

// TestRenameProducers pins the rename of every retired row name the status file carries, and the cases that leave the file untouched.
func TestRenameProducers(t *testing.T) {
	t.Parallel()
	renames := map[string]string{"Old": "New"}
	request := func(shed *Shed) RenameRequest {
		return RenameRequest{StatusPath: shed.StatusPath, LockPath: shed.LockPath, StatusLockPath: shed.StatusLockPath, Renames: renames}
	}

	t.Run("every field naming a retired row is rewritten", func(t *testing.T) {
		t.Parallel()
		shed, _, _, _ := newTestShed(t)
		seed := commonSeed("Old")
		seed.State = StateBlocked
		seed.PauseBefore = "Old"
		seed.PauseAfter = "Other"
		seed.History = []HistoryEntry{{Producer: "Other", Outcome: Done, At: "2026-10-11T00:00:00Z"}, {Producer: "Old", Outcome: Stuck, At: "2026-10-11T00:00:01Z"}}
		seedStatus(t, shed.StatusPath, shed.StatusLockPath, seed)

		wrote, err := RenameProducers(request(shed))
		if err != nil || !wrote {
			t.Fatalf("RenameProducers() = %v, %v; want true, nil", wrote, err)
		}
		got := readStatus(t, shed.StatusPath, shed.StatusLockPath)
		if got.CurrentProducer != "New" || got.PauseBefore != "New" || got.PauseAfter != "Other" {
			t.Errorf("CurrentProducer, PauseBefore, PauseAfter = %q, %q, %q; want New, New, Other", got.CurrentProducer, got.PauseBefore, got.PauseAfter)
		}
		if got.History[0].Producer != "Other" || got.History[1].Producer != "New" || got.History[1].Outcome != Stuck {
			t.Errorf("History = %+v; want Other then New, outcomes kept", got.History)
		}
		if got.State != StateBlocked {
			t.Errorf("State = %q; want blocked, untouched", got.State)
		}
	})

	t.Run("an absent status file is not an error", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		req := RenameRequest{StatusPath: filepath.Join(dir, "run", "status.json"), LockPath: filepath.Join(dir, "scratch", "run.lock"), StatusLockPath: filepath.Join(dir, "scratch", "status.json.lock"), Renames: renames}
		if wrote, err := RenameProducers(req); err != nil || wrote {
			t.Errorf("RenameProducers() = %v, %v; want false, nil", wrote, err)
		}
	})

	t.Run("a file naming no retired row is left byte for byte and takes no lock", func(t *testing.T) {
		t.Parallel()
		shed, _, _, _ := newTestShed(t)
		seedStatus(t, shed.StatusPath, shed.StatusLockPath, commonSeed("Other"))
		before, err := os.ReadFile(shed.StatusPath)
		if err != nil {
			t.Fatal(err)
		}
		holdRunLock(t, shed.LockPath)

		if wrote, err := RenameProducers(request(shed)); err != nil || wrote {
			t.Fatalf("RenameProducers() = %v, %v; want false, nil", wrote, err)
		}
		after, err := os.ReadFile(shed.StatusPath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Errorf("status file changed:\n%s\nwant:\n%s", after, before)
		}
	})

	t.Run("a held run lock refuses busy and leaves the file", func(t *testing.T) {
		t.Parallel()
		shed, _, _, _ := newTestShed(t)
		seedStatus(t, shed.StatusPath, shed.StatusLockPath, commonSeed("Old"))
		holdRunLock(t, shed.LockPath)

		wrote, err := RenameProducers(request(shed))
		if !errors.Is(err, ErrShedBusy) || wrote {
			t.Fatalf("RenameProducers() = %v, %v; want false, ErrShedBusy", wrote, err)
		}
		if got := readStatus(t, shed.StatusPath, shed.StatusLockPath); got.CurrentProducer != "Old" {
			t.Errorf("CurrentProducer = %q; want Old, untouched", got.CurrentProducer)
		}
	})
}

// holdRunLock holds the run lock at lockPath until the test ends.
func holdRunLock(t *testing.T, lockPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	held, locked, err := lock.TryAcquireWriteLock(lockPath)
	if err != nil || !locked {
		t.Fatalf("TryAcquireWriteLock(%q) = _, %v, %v; want held", lockPath, locked, err)
	}
	t.Cleanup(func() { _ = held.Release() })
}
