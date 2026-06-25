//go:build integration

// status_test.go — tests for weft content-sync status reporting.
//
// Junction integrity assertions have been removed from this file; they are now
// owned by internal/warp/status_test.go which exercises checkJunctionHealth directly.

package weft

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

func TestStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		modify     bool
		wantDirty  bool
		wantBranch string
	}{
		{"BranchReporting_Clean", false, false, "main"},
		{"DirtyFlag_Clean", false, false, "main"},
		{"DirtyFlag_Modified", true, true, "main"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fixture := lyxtest.CopyWeft(t)
			weftRepo := fixture.WeftPath

			if tt.modify {
				lyxFile := filepath.Join(weftRepo, "_lyx", "config.yaml")
				if err := os.WriteFile(lyxFile, []byte("modified"), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			}

			// Call the trimmed signature: Status no longer accepts hostLink or weftLyxDir;
			// junction topology is now reported by warp status.
			status, err := Status(weftRepo, []string{"_lyx"})
			if err != nil {
				t.Fatalf("Status: %v", err)
			}

			branch, ok := status["branch"].(string)
			if !ok || branch == "" {
				t.Errorf("status[branch] should be a non-empty string; got %v", status["branch"])
			}
			if branch != tt.wantBranch {
				t.Errorf("branch = %q; want %q", branch, tt.wantBranch)
			}

			dirty, ok := status["dirty"].(bool)
			if !ok {
				t.Errorf("status[dirty] should be a bool; got %v", status["dirty"])
			}
			if dirty != tt.wantDirty {
				t.Errorf("dirty = %v; want %v", dirty, tt.wantDirty)
			}
		})
	}
}
