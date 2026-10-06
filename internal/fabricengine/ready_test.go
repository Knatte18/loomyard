// ready_test.go covers Ready(l)'s three outcomes: sibling absent, sibling present, and a stat
// failure other than not-exist.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
)

// TestReady covers the weft sibling worktree absent, present, and unstatable.
func TestReady(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// location builds the Location under test from a fresh temp dir.
		location  func(t *testing.T, tmp string) string
		wantReady bool
		wantErr   bool
	}{
		{
			name:      "sibling absent",
			location:  func(t *testing.T, tmp string) string { return tmp },
			wantReady: false,
		},
		{
			name: "sibling present",
			location: func(t *testing.T, tmp string) string {
				if err := os.Mkdir(fabricengine.WeftWorktree(locationkit.Location(tmp, "worktree", ".")), 0o755); err != nil {
					t.Fatalf("mkdir weft sibling: %v", err)
				}
				return tmp
			},
			wantReady: true,
		},
		{
			// A hub path under a regular file makes the sibling's stat fail with "not a directory",
			// not "not exist"; no portable chmod-based simulation exists across platforms.
			name: "stat failure other than not-exist",
			location: func(t *testing.T, tmp string) string {
				regularFile := filepath.Join(tmp, "not-a-hub")
				if err := os.WriteFile(regularFile, []byte("x"), 0o644); err != nil {
					t.Fatalf("write regular file: %v", err)
				}
				return filepath.Join(regularFile, "hub")
			},
			wantReady: false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := locationkit.Location(tt.location(t, t.TempDir()), "worktree", ".")

			ready, err := fabricengine.Ready(l)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Ready() error = %v; wantErr %v", err, tt.wantErr)
			}
			if ready != tt.wantReady {
				t.Errorf("Ready() = %v; want %v", ready, tt.wantReady)
			}
		})
	}
}
