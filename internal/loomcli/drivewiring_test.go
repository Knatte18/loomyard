// drivewiring_test.go — untagged Tier-1 tests for the Park seam loom hands the shed driver, over a fake commit in a temp dir.

package loomcli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/state"
)

// TestLoomPark covers loom's Park seam: a clean commit writes the park marker and returns nil, a failing commit still writes the marker and returns nil, and a marker path whose directory cannot be created returns an error naming it.
func TestLoomPark(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		commitErr error
		// blockMarkerDir puts a regular file where the marker's directory would be.
		blockMarkerDir bool
		wantErr        bool
	}{
		{name: "a clean commit writes the marker and returns nil"},
		{name: "a failing commit still writes the marker and returns nil", commitErr: errors.New("disk full")},
		{name: "a marker directory that cannot be created returns an error naming the marker", blockMarkerDir: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			markerPath := filepath.Join(root, "scratch", "driver-parked")
			if tt.blockMarkerDir {
				blocker := filepath.Join(root, "blocker")
				if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
					t.Fatalf("seed the blocking file: %v", err)
				}
				markerPath = filepath.Join(blocker, "driver-parked")
			}
			commit := commitStatusDeps{
				MergeActive: func() (bool, error) { return false, nil },
				Commit:      func(string) error { return tt.commitErr },
				Push:        func() error { return nil },
			}
			const reportPath = "/reports/report-1.md"

			err := loomPark(commit, markerPath)(reportPath)

			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), markerPath) {
					t.Fatalf("park error = %v; want one naming %s", err, markerPath)
				}
				return
			}
			if err != nil {
				t.Fatalf("park error = %v; want nil", err)
			}
			marker, readErr := os.ReadFile(markerPath)
			if readErr != nil || string(marker) != reportPath {
				t.Errorf("park marker = %q, error %v; want the report path %q", marker, readErr, reportPath)
			}
		})
	}
}

// TestGoDriverSeams_LabelShed covers the Go driver's goto and step seams: each records the shed label, the goto on the entry it appends and the step as the pending label before the loop is spawned.
func TestGoDriverSeams_LabelShed(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	statusPath := filepath.Join(root, "status.json")
	statusLockPath := filepath.Join(root, "status.lock")
	if err := state.WriteJSON(statusPath, statusLockPath, shedengine.Status{CurrentProducer: "B", State: shedengine.StateBlocked, History: []shedengine.HistoryEntry{}}); err != nil {
		t.Fatalf("seed status: %v", err)
	}
	spawned := 0
	c := &loomCLI{spec: &shedverbs.Spec{
		StatusPath:     statusPath,
		LockPath:       filepath.Join(root, "run.lock"),
		StatusLockPath: statusLockPath,
		Routing:        shedengine.Routing{Producers: []shedengine.ProducerDef{{Name: "A"}, {Name: "B"}}},
		Loop: shedverbs.LoopSpec{
			LockPath:     filepath.Join(root, "loop.lock"),
			PIDPath:      filepath.Join(root, "loop.pid"),
			EnvelopePath: filepath.Join(root, "loop-envelope.json"),
			Spawn: func([]string) (<-chan struct{}, error) {
				spawned++
				return nil, errors.New("no loop in a test")
			},
		},
	}}
	read := func() shedengine.Status {
		t.Helper()
		st, _, err := state.ReadJSONNoMkdir[shedengine.Status](statusPath, statusLockPath)
		if err != nil {
			t.Fatalf("read status: %v", err)
		}
		return st
	}

	if _, err := c.driverStep(t.Context(), ""); err != nil {
		t.Fatalf("driverStep = _, %v; want the envelope the refused spawn printed", err)
	}
	if st := read(); spawned != 1 || st.PendingBy != shedengine.CallerShed {
		t.Errorf("spawns, PendingBy = %d, %q; want 1, %q", spawned, st.PendingBy, shedengine.CallerShed)
	}

	if err := c.driverGoto(t.Context(), "A"); err != nil {
		t.Fatalf("driverGoto = %v; want nil", err)
	}
	if st := read(); len(st.History) != 1 || st.History[0].By != shedengine.CallerShed || st.PendingBy != "" {
		t.Errorf("History, PendingBy = %+v, %q; want one goto entry by %q and nothing pending", st.History, st.PendingBy, shedengine.CallerShed)
	}
}
