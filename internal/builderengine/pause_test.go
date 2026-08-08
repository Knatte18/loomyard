// pause_test.go covers the pause flag's request/observe/clear cycle (PauseFlagPath, RequestPause,
// PauseRequested, ClearPause) end-to-end against a temp builder scratch dir, plus the idempotent-clear
// case a resumed run's entry-clear relies on, and the scratch/durable split: RequestPause/
// PauseRequested/ClearPause all agree against a scratch dir distinct from the durable dir, and no
// pause file ever lands in the durable dir.

package builderengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
)

func TestPause_RequestObserveClearCycle(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	builderDir := filepath.Join(base, "_lyx", "builder")
	scratchDir := filepath.Join(base, ".lyx", "builder")

	if builderengine.PauseRequested(scratchDir) {
		t.Fatalf("PauseRequested() = true before any RequestPause; want false")
	}

	if err := builderengine.RequestPause(scratchDir); err != nil {
		t.Fatalf("RequestPause() error = %v; want nil", err)
	}
	if !builderengine.PauseRequested(scratchDir) {
		t.Errorf("PauseRequested() = false after RequestPause; want true")
	}

	wantPath := filepath.Join(scratchDir, "pause")
	if got := builderengine.PauseFlagPath(scratchDir); got != wantPath {
		t.Errorf("PauseFlagPath() = %q; want %q", got, wantPath)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Errorf("pause flag file not found at %q: %v", wantPath, err)
	}
	if _, err := os.Stat(filepath.Join(builderDir, "pause")); err == nil {
		t.Errorf("pause flag file found in durable dir %q; want it only in the scratch dir", builderDir)
	}

	if err := builderengine.ClearPause(scratchDir); err != nil {
		t.Fatalf("ClearPause() error = %v; want nil", err)
	}
	if builderengine.PauseRequested(scratchDir) {
		t.Errorf("PauseRequested() = true after ClearPause; want false")
	}
}

func TestPause_RequestIsIdempotent(t *testing.T) {
	t.Parallel()

	scratchDir := t.TempDir()

	if err := builderengine.RequestPause(scratchDir); err != nil {
		t.Fatalf("first RequestPause() error = %v; want nil", err)
	}
	if err := builderengine.RequestPause(scratchDir); err != nil {
		t.Fatalf("second RequestPause() error = %v; want nil", err)
	}
	if !builderengine.PauseRequested(scratchDir) {
		t.Errorf("PauseRequested() = false after two RequestPause calls; want true")
	}
}

func TestPause_ClearIsIdempotent(t *testing.T) {
	t.Parallel()

	scratchDir := t.TempDir()

	// ClearPause against a scratch dir that never saw a RequestPause call at
	// all — the entry-clear rule must be safe to call unconditionally on a
	// fresh run.
	if err := builderengine.ClearPause(scratchDir); err != nil {
		t.Fatalf("ClearPause() on a never-paused dir error = %v; want nil", err)
	}

	if err := builderengine.RequestPause(scratchDir); err != nil {
		t.Fatalf("RequestPause() error = %v; want nil", err)
	}
	if err := builderengine.ClearPause(scratchDir); err != nil {
		t.Fatalf("first ClearPause() error = %v; want nil", err)
	}
	// A second consecutive clear must still succeed — this is exactly the
	// entry-then-terminal double-clear pattern Run performs.
	if err := builderengine.ClearPause(scratchDir); err != nil {
		t.Fatalf("second ClearPause() error = %v; want nil", err)
	}
}
