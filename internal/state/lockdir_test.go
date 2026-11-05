// lockdir_test.go covers the creation of a lock file's directory by the writers, which take a lock path that may sit in another tree than the record.

package state_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestWriters_CreateAbsentLockDirectory(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	writers := []struct {
		name  string
		write func(path, lockPath string) error
	}{
		{"WriteJSON", func(path, lockPath string) error {
			return state.WriteJSON(path, lockPath, sample{Name: "a", N: 1})
		}},
		{"WriteFile", func(path, lockPath string) error {
			return state.WriteFile(path, lockPath, []byte("data"))
		}},
		{"UpdateJSON", func(path, lockPath string) error {
			return state.UpdateJSON(path, lockPath, func(cur sample, found bool) (sample, error) {
				return sample{Name: "a", N: 1}, nil
			})
		}},
	}
	for _, tt := range writers {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			path := filepath.Join(root, "records", "state.json")
			lockPath := filepath.Join(root, "locks", "nested", "state.json.lock")

			if err := tt.write(path, lockPath); err != nil {
				t.Fatalf("%s() error = %v; want nil with the lock directory absent", tt.name, err)
			}
			for _, created := range []string{path, lockPath} {
				if _, err := os.Stat(created); err != nil {
					t.Errorf("Stat(%q) error = %v; want it created", created, err)
				}
			}
		})
	}
}
