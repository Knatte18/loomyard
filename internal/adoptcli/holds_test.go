// holds_test.go pins the fix-tries file of a subject.

package adoptcli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

//testtiming:keep pins that an absent tries file reads zero and a read creates no directory, which the fix-loop test covering its blocks does not assert
func TestTriesStore(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("AbsentFileReadsZeroAndCreatesNothing", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "subject")
		if got, err := readTries(dir); err != nil || got != 0 {
			t.Errorf("readTries() = %d, %v; want 0", got, err)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("Stat(%s) error = %v; want a read to create no directory", dir, err)
		}
	})

	t.Run("TriesReadWhatWasWritten", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := writeTries(dir, 2); err != nil {
			t.Fatalf("writeTries: %v", err)
		}
		if got, err := readTries(dir); err != nil || got != 2 {
			t.Errorf("readTries() = %d, %v; want 2", got, err)
		}
	})
}
