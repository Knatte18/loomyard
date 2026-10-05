// entryobservation_test.go holds the status-file helpers the entry-observation and handoff-voucher tests share, and the table of entries that write no crash-resume note.

package loomcli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
)

// writeStatusFixture writes st to path/lockPath via the production WriteJSON primitive.
func writeStatusFixture(t *testing.T, path, lockPath string, st shedengine.Status) {
	t.Helper()
	if err := state.WriteJSON(path, lockPath, st); err != nil {
		t.Fatalf("write status file: %v", err)
	}
}

// productJSON marshals p for embedding as a shedengine.Status.Product payload.
func productJSON(t *testing.T, p loomengine.Status) []byte {
	t.Helper()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal product: %v", err)
	}
	return raw
}

func TestNoteCrashResumeAtEntry_WritesNoNote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		state          shedengine.State
		historyEntries int
		holdRunLock    bool
		tierTwoOff     bool
		unwritableDir  bool
	}{
		{name: "RunLockHeld", state: shedengine.StateRunning, historyEntries: 2, holdRunLock: true},
		{name: "StateNotRunning", state: shedengine.StateBlocked, historyEntries: 2},
		{name: "EmptyHistory", state: shedengine.StateRunning, historyEntries: 0},
		{name: "TierTwoOff", state: shedengine.StateRunning, historyEntries: 2, tierTwoOff: true},
		{name: "UnwritableFrictionDirectory", state: shedengine.StateRunning, historyEntries: 2, unwritableDir: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			frictionDir := filepath.Join(root, "friction")
			if tt.unwritableDir {
				blocker := filepath.Join(root, "blocker")
				if err := os.WriteFile(blocker, []byte("a file, not a directory"), 0o644); err != nil {
					t.Fatalf("WriteFile(%q) = %v; want nil", blocker, err)
				}
				frictionDir = filepath.Join(blocker, "friction")
			}

			loc := locationkit.Location(root, "pair", ".")
			if err := os.MkdirAll(filepath.Dir(loomengine.LoomHandoffVoucherLock(loc)), 0o755); err != nil {
				t.Fatalf("MkdirAll(voucher directory) = %v; want nil", err)
			}
			c := &loomCLI{
				location:    loc,
				frictionDir: frictionDir,
				shedPaths: shedbuild.ShedPaths{
					LockPath:       filepath.Join(root, "run.lock"),
					StatusPath:     filepath.Join(root, "status.json"),
					StatusLockPath: filepath.Join(root, "status.json.lock"),
				},
			}
			if tt.tierTwoOff {
				c.frictionDir = ""
			}
			writeStatusFixture(t, c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, shedengine.Status{
				CurrentProducer: "Plan-Write",
				State:           tt.state,
				History:         make([]shedengine.HistoryEntry, tt.historyEntries),
				Product:         productJSON(t, loomengine.Status{Slug: "a-task", Parent: "main"}),
			})
			if tt.holdRunLock {
				held, err := lock.AcquireWriteLock(c.shedPaths.LockPath)
				if err != nil {
					t.Fatalf("AcquireWriteLock(%q) = %v; want nil", c.shedPaths.LockPath, err)
				}
				t.Cleanup(func() { _ = held.Release() })
			}

			c.noteCrashResumeAtEntry("run")

			notePath := filepath.Join(frictionDir, "loom-crash-resume.md")
			if _, err := os.ReadFile(notePath); err == nil {
				t.Errorf("crash-resume note %q exists; want none written", notePath)
			}
		})
	}
}
