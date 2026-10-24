//go:build tier0

// fixerattempts_test.go — tier0 unit tests for the fixer attempt record.

package landingshed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixerAttempts(t *testing.T) {
	t.Parallel()

	t.Run("RoundTripsUnderItsOwnEntry", func(t *testing.T) {
		t.Parallel()
		path := FixerAttemptsPath(filepath.Join(t.TempDir(), "scratch"), "merge-in")
		want := FixerAttempts{Entry: 3, Spent: 2, Baseline: "abc123", AlignFrom: "def456", Halt: "over-cap"}

		if err := WriteFixerAttempts(path, want); err != nil {
			t.Fatalf("WriteFixerAttempts() = %v; want nil", err)
		}
		got, err := ReadFixerAttempts(path, 3)
		if err != nil || got != want {
			t.Errorf("ReadFixerAttempts(entry 3) = %+v, %v; want %+v, nil", got, err, want)
		}
	})

	t.Run("OtherEntryAndAbsentFileReadZero", func(t *testing.T) {
		t.Parallel()
		path := FixerAttemptsPath(t.TempDir(), "finalize")
		if got, err := ReadFixerAttempts(path, 5); err != nil || got != (FixerAttempts{Entry: 5}) {
			t.Errorf("ReadFixerAttempts(absent) = %+v, %v; want a zero record for entry 5, nil", got, err)
		}

		if err := WriteFixerAttempts(path, FixerAttempts{Entry: 3, Spent: 2}); err != nil {
			t.Fatalf("WriteFixerAttempts() = %v; want nil", err)
		}
		if got, err := ReadFixerAttempts(path, 4); err != nil || got != (FixerAttempts{Entry: 4}) {
			t.Errorf("ReadFixerAttempts(other entry) = %+v, %v; want a zero record for entry 4, nil", got, err)
		}
	})

	t.Run("ToleratesAnUnknownKey", func(t *testing.T) {
		t.Parallel()
		path := FixerAttemptsPath(t.TempDir(), "merge-in")
		if err := os.WriteFile(path, []byte(`{"entry":1,"spent":2,"future_key":"x"}`), 0o644); err != nil {
			t.Fatalf("WriteFile() = %v; want nil", err)
		}
		if got, err := ReadFixerAttempts(path, 1); err != nil || got.Spent != 2 {
			t.Errorf("ReadFixerAttempts(unknown key) = %+v, %v; want Spent 2, nil", got, err)
		}
	})

	t.Run("RefusesAnUndecodableFileNamingIt", func(t *testing.T) {
		t.Parallel()
		path := FixerAttemptsPath(t.TempDir(), "merge-in")
		if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
			t.Fatalf("WriteFile() = %v; want nil", err)
		}
		if _, err := ReadFixerAttempts(path, 1); err == nil || !strings.Contains(err.Error(), path) {
			t.Errorf("ReadFixerAttempts(undecodable) error = %v; want one naming %s", err, path)
		}
	})

	t.Run("RemovesTheFileTwice", func(t *testing.T) {
		t.Parallel()
		path := FixerAttemptsPath(t.TempDir(), "merge-in")
		if err := WriteFixerAttempts(path, FixerAttempts{Entry: 1}); err != nil {
			t.Fatalf("WriteFixerAttempts() = %v; want nil", err)
		}
		for i := 0; i < 2; i++ {
			if err := RemoveFixerAttempts(path); err != nil {
				t.Errorf("RemoveFixerAttempts() call %d = %v; want nil", i+1, err)
			}
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("os.Stat(%s) = %v; want not exist", path, err)
		}
	})
}
