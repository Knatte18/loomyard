// archive_test.go pins ArchiveRunRecord: what it moves, and when it refuses.

package weaverengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

func TestArchiveRunRecord(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	newGeom := func(t *testing.T) weaverengine.Geometry {
		root := t.TempDir()
		return weaverengine.Geometry{RunDir: filepath.Join(root, "run"), ScratchDir: filepath.Join(root, "scratch")}
	}

	t.Run("EntriesMoveIntoDest", func(t *testing.T) {
		t.Parallel()
		geom := newGeom(t)
		if err := os.MkdirAll(filepath.Join(geom.RunDir, "reports"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(geom.RunDir, "state.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(t.TempDir(), "archive")

		if err := weaverengine.ArchiveRunRecord(geom, dest); err != nil {
			t.Fatalf("ArchiveRunRecord() error = %v; want nil", err)
		}

		for _, name := range []string{"state.json", "reports"} {
			if _, err := os.Stat(filepath.Join(dest, name)); err != nil {
				t.Errorf("stat %s in dest = %v; want it moved", name, err)
			}
			if _, err := os.Stat(filepath.Join(geom.RunDir, name)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("stat %s in the run dir = %v; want it gone", name, err)
			}
		}
	})

	t.Run("HeldRunLockRefuses", func(t *testing.T) {
		t.Parallel()
		geom := newGeom(t)
		if err := os.MkdirAll(geom.ScratchDir, 0o755); err != nil {
			t.Fatal(err)
		}
		held, acquired, err := lock.TryAcquireWriteLock(filepath.Join(geom.ScratchDir, "run.lock"))
		if err != nil || !acquired {
			t.Fatalf("acquire the run lock = %v, %v", acquired, err)
		}
		defer func() { _ = held.Release() }()

		if err := weaverengine.ArchiveRunRecord(geom, filepath.Join(t.TempDir(), "archive")); !errors.Is(err, weaverengine.ErrRunBusy) {
			t.Errorf("ArchiveRunRecord() error = %v; want ErrRunBusy", err)
		}
	})

	t.Run("AbsentRunDirectoryMovesNothing", func(t *testing.T) {
		t.Parallel()
		geom := newGeom(t)
		dest := filepath.Join(t.TempDir(), "archive")

		if err := weaverengine.ArchiveRunRecord(geom, dest); err != nil {
			t.Fatalf("ArchiveRunRecord() error = %v; want nil", err)
		}

		if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat dest = %v; want it never created", err)
		}
	})
}
