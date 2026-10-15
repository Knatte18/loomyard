// drivewiring_test.go — untagged Tier-1 tests for the Park seam loom hands the shed driver, over a fake commit in a temp dir.

package loomcli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
