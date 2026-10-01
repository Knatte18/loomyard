// goto_test.go covers batten's PreGoto hook: Worktree-Teardown is admitted only from Worktree-Teardown itself or once the child run reads done.

package battencli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

func writeGotoStatus(t *testing.T, dir, row string, st shedengine.State) string {
	t.Helper()
	path := filepath.Join(dir, "status.json")
	lockPath := filepath.Join(dir, "status.lock", "lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(path, lockPath, shedengine.Status{CurrentProducer: row, State: st, History: []shedengine.HistoryEntry{}}); err != nil {
		t.Fatalf("write status: %v", err)
	}
	return path
}

func TestPreGoto(t *testing.T) {
	const teardown = "Worktree-Teardown"
	tests := []struct {
		name        string
		target      string
		ownRow      string
		childState  shedengine.State // empty: no child status file
		wantRefused bool
	}{
		{name: "ChildRunningRefused", target: teardown, ownRow: "Run-Shed", childState: shedengine.StateRunning, wantRefused: true},
		{name: "ChildDoneAdmitted", target: teardown, ownRow: "Run-Shed", childState: shedengine.StateDone},
		{name: "OwnRowTeardownAdmitted", target: teardown, ownRow: teardown, childState: shedengine.StateRunning},
		{name: "MissingChildRefused", target: teardown, ownRow: "Run-Shed", wantRefused: true},
		{name: "BackwardAdmittedUnread", target: "Run-Shed", ownRow: "Run-Shed", childState: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			own := writeGotoStatus(t, filepath.Join(t.TempDir(), "own"), tt.ownRow, shedengine.StateBlocked)
			childPath := filepath.Join(t.TempDir(), "child", "status.json")
			if tt.childState != "" {
				childPath = writeGotoStatus(t, filepath.Dir(childPath), "Plan-Write", tt.childState)
			}
			childStatus := func() (string, error) {
				if tt.name == "BackwardAdmittedUnread" {
					t.Error("child status read for a non-teardown target")
				}
				return childPath, nil
			}
			err := preGoto(tt.target, "some-slug", own, childStatus)
			if !tt.wantRefused {
				if err != nil {
					t.Fatalf("preGoto = %v; want admitted", err)
				}
				return
			}
			if err == nil {
				t.Fatal("preGoto admitted; want refused")
			}
			for _, want := range []string{"lyx batten step some-slug", "lyx batten status some-slug", "way forward:"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refusal %q does not contain %q", err, want)
				}
			}
		})
	}
}

// TestPreGoto_UnresolvedWorktreeRefused asserts a task worktree that does not resolve counts as not done.
func TestPreGoto_UnresolvedWorktreeRefused(t *testing.T) {
	own := writeGotoStatus(t, t.TempDir(), "Run-Shed", shedengine.StateBlocked)
	err := preGoto("Worktree-Teardown", "s", own, func() (string, error) { return "", errors.New("no worktree") })
	if err == nil || !strings.Contains(err.Error(), "lyx batten step s") {
		t.Fatalf("preGoto = %v; want a refusal naming lyx batten step", err)
	}
}
