// wake_test.go covers the wake record: the stamp a row writes after a completed wake and what a read finds of it.

package shedrun

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestWakeStamp(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	at := time.Date(2026, 11, 5, 9, 30, 0, 0, time.UTC)
	newPath := func(t *testing.T) string { return filepath.Join(t.TempDir(), "scratch", "wake.json") }

	t.Run("RoundTripReadsTheRowsTime", func(t *testing.T) {
		t.Parallel()
		path := newPath(t)
		if err := WriteWakeStamp(path, "Beam-Await", at); err != nil {
			t.Fatalf("WriteWakeStamp() = %v; want nil", err)
		}

		got, found, err := ReadWakeStamp(path, "Beam-Await")

		if err != nil || !found || !got.Equal(at) {
			t.Errorf("ReadWakeStamp() = %v, %v, %v; want %v, true, nil", got, found, err, at)
		}
	})

	t.Run("AnotherRowReadsAsNotFound", func(t *testing.T) {
		t.Parallel()
		path := newPath(t)
		if err := WriteWakeStamp(path, "Beam-Await", at); err != nil {
			t.Fatalf("WriteWakeStamp() = %v; want nil", err)
		}

		got, found, err := ReadWakeStamp(path, "Beam-Up")

		if err != nil || found || !got.IsZero() {
			t.Errorf("ReadWakeStamp(Beam-Up) = %v, %v, %v; want the zero time, false, nil", got, found, err)
		}
	})

	t.Run("AnAbsentFileReadsAsNotFound", func(t *testing.T) {
		t.Parallel()

		got, found, err := ReadWakeStamp(newPath(t), "Beam-Await")

		if err != nil || found || !got.IsZero() {
			t.Errorf("ReadWakeStamp(absent) = %v, %v, %v; want the zero time, false, nil", got, found, err)
		}
	})
}
