// leftoverpairs_test.go pins the guarded write of a kept cancel intent over a bare location, with no hub.

package battencli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestWriteKeptCancel_WritesNothingWhileAMergeIsActive(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name        string
		mergeActive bool
		wantWritten bool
	}{
		{"NoMergeActiveWritesTheIntent", false, true},
		{"MergeActiveWritesNothing", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			prime := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "main", AnchorRel: "."}
			statusPath := shedrun.StatusFile(prime, "run")
			if err := os.MkdirAll(filepath.Dir(statusPath), 0o755); err != nil {
				t.Fatalf("create the run directory: %v", err)
			}
			before := []byte(`{"state":"cancelled"}`)
			if err := os.WriteFile(statusPath, before, 0o644); err != nil {
				t.Fatalf("write the status: %v", err)
			}
			deps := commitStatusDeps{MergeActive: func() (bool, error) { return tt.mergeActive, nil }}

			written, err := writeKeptCancel(prime, deps, "run", "this-machine")

			if (err != nil) != tt.mergeActive {
				t.Errorf("writeKeptCancel() error = %v; want an error = %v", err, tt.mergeActive)
			}
			if written != tt.wantWritten {
				t.Errorf("writeKeptCancel() written = %v; want %v", written, tt.wantWritten)
			}
			after, readErr := os.ReadFile(statusPath)
			if readErr != nil {
				t.Fatalf("read the status: %v", readErr)
			}
			if unchanged := bytes.Equal(before, after); unchanged == tt.wantWritten {
				t.Errorf("status after writeKeptCancel() = %s; want it unchanged = %v", after, !tt.wantWritten)
			}
		})
	}
}
