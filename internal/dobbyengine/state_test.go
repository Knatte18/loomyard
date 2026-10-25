//go:build tier0

// state_test.go pins the state record's tolerance: the keys a newer binary wrote survive a rewrite, and a damaged file never wedges the beam.

package dobbyengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestState_RoundTripKeepsUnknownKeys(t *testing.T) {
	t.Parallel()

	t.Run("a foreign top-level key and a foreign run key survive an update", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "state.json")
		written := `{"pid": 1, "foreign_top": "kept", "runs": {"a": {"full_pass_owed": true, "foreign_run": 7}}}`
		if err := os.WriteFile(path, []byte(written), 0o644); err != nil {
			t.Fatalf("WriteFile() = %v; want nil", err)
		}

		if err := UpdateState(path, func(s *State) error {
			s.PID = 2
			return nil
		}); err != nil {
			t.Fatalf("UpdateState() = %v; want nil", err)
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile() = %v; want nil", err)
		}
		for _, key := range []string{"foreign_top", "foreign_run"} {
			if !strings.Contains(string(raw), key) {
				t.Errorf("the rewritten record lost %q:\n%s", key, raw)
			}
		}
		got, found, err := ReadState(path)
		if err != nil || !found || got.PID != 2 || !got.Runs["a"].FullPassOwed {
			t.Errorf("ReadState() = %+v, %v, %v; want pid 2 and run a owing a full pass", got, found, err)
		}
	})

	t.Run("an undecodable record reads as absent and is rewritten fresh", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "state.json")
		if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
			t.Fatalf("WriteFile() = %v; want nil", err)
		}

		if got, found, err := ReadState(path); err != nil || found {
			t.Fatalf("ReadState() = %+v, %v, %v; want absent and no error", got, found, err)
		}
		if err := UpdateState(path, func(s *State) error {
			s.PID = 3
			return nil
		}); err != nil {
			t.Fatalf("UpdateState() = %v; want nil", err)
		}
		got, found, err := ReadState(path)
		if err != nil || !found || got.PID != 3 {
			t.Errorf("ReadState() = %+v, %v, %v; want the fresh record with pid 3", got, found, err)
		}
	})
}
