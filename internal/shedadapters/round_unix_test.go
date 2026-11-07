//go:build !windows

// round_unix_test.go holds the round-path tests that rest on directory mode bits, which Windows does not enforce.

package shedadapters

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestResolveRound_NonNotExistStatErrorIsReturned arranges a stat failure that is not fs.ErrNotExist.
// The failure is a permission error from an unsearchable run dir, and it must surface as an error.
// Read as absence, it would truncate the scan or re-seed an already-judged segment.
// Skipped when running as root, which bypasses directory permission checks.
func TestResolveRound_NonNotExistStatErrorIsReturned(t *testing.T) {
	tiergate.Require(t, 0)
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses directory permission checks")
	}

	parent := t.TempDir()
	dir := filepath.Join(parent, "run")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("Mkdir(%q) = %v; want nil", dir, err)
	}
	writeReport(t, dir, 1)

	// Remove the directory's search (execute) bit, so a stat inside it fails with a permission error.
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatalf("Chmod(%q, 0o000) = %v; want nil", dir, err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o755)
	})

	got, err := ResolveRound(dir, reportName)
	if err == nil {
		t.Fatalf("ResolveRound(%q, ...) = %d, nil; want a non-nil error from the permission-denied stat", dir, got)
	}
}
