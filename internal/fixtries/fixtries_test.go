// fixtries_test.go pins the fix-tries file of a subject.

package fixtries_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fixtries"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestReadWrite(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("AbsentFileReadsZeroAndCreatesNothing", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "subject")
		if got, err := fixtries.Read(dir); err != nil || got != 0 {
			t.Errorf("Read() = %d, %v; want 0", got, err)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("Stat(%s) error = %v; want a read to create no directory", dir, err)
		}
	})

	t.Run("TriesReadWhatTheFileHolds", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, fixtries.FileName), []byte(`{"count": 2}`), 0o644); err != nil {
			t.Fatalf("seed the tries file: %v", err)
		}
		if got, err := fixtries.Read(dir); err != nil || got != 2 {
			t.Errorf("Read() = %d, %v; want 2", got, err)
		}
	})
}
