//go:build tier0 && !windows

// reflect_unix_test.go holds the reflection tests that rest on directory mode bits, which Windows does not enforce.

package frictionengine

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReflect_UnwritableRecord_FailedNoSpawn covers a record write that fails before the spawn.
func TestReflect_UnwritableRecord_FailedNoSpawn(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("directory permissions do not bar writes to root")
	}
	shuttle := doneShuttle()
	deps := newTestDeps(t, shuttle, fixedClock())
	writeNote(t, deps, "note-1", "x")
	if err := os.Chmod(deps.FrictionDir, 0o555); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(deps.FrictionDir, 0o755) })

	report, err := Reflect(deps)
	if err != nil {
		t.Fatalf("Reflect() error = %v; want nil", err)
	}
	if report.Status != StatusFailed {
		t.Errorf("Reflect() status = %q; want %q", report.Status, StatusFailed)
	}
	if shuttle.Called {
		t.Error("Run called; want no spawn")
	}
	requireExists(t, filepath.Join(deps.FrictionDir, "note-1.md"))
}
