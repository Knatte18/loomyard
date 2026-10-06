// update_test.go covers UpdateJSON's missing-file, existing-file and mutate-error dispositions, plus its core concurrency property: many concurrent callers each appending one element under UpdateJSON never lose or clobber each other's write.
// Its corrupt-file disposition is covered by TestCorruptFile in state_test.go.

package state_test

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/state"
)

// TestUpdateJSON_MutateSeesCurrentValue verifies that mutate sees the zero value with found=false when the file does not exist yet and the decoded value with found=true when it does, and that mutate's returned value is on disk afterwards, created or replacing.
//
//testtiming:keep covering tests TestUpdateJSON_Concurrency and TestUpdateJSON_MutateError, which the report ties it to and which stay
func TestUpdateJSON_MutateSeesCurrentValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		existing  *sample
		wantFound bool
		wantCur   sample
		want      sample
	}{
		{
			name: "MissingFile",
			want: sample{Name: "created", N: 1},
		},
		{
			name:      "ExistingFile",
			existing:  &sample{Name: "original", N: 1},
			wantFound: true,
			wantCur:   sample{Name: "original", N: 1},
			want:      sample{Name: "updated", N: 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			path := filepath.Join(tmpDir, "state.json")
			lockPath := path + ".lock"

			if tt.existing != nil {
				if err := state.WriteJSON(path, lockPath, *tt.existing); err != nil {
					t.Fatalf("setup: WriteJSON() error: %v", err)
				}
			}

			var sawFound bool
			var sawCur sample
			err := state.UpdateJSON(path, lockPath, func(cur sample, found bool) (sample, error) {
				sawCur = cur
				sawFound = found
				return tt.want, nil
			})
			if err != nil {
				t.Fatalf("UpdateJSON() error: %v", err)
			}
			if sawFound != tt.wantFound {
				t.Errorf("UpdateJSON() mutate found = %v; want %v", sawFound, tt.wantFound)
			}
			if sawCur != tt.wantCur {
				t.Errorf("UpdateJSON() mutate cur = %+v; want %+v", sawCur, tt.wantCur)
			}

			got, found, err := state.ReadJSON[sample](path, lockPath)
			if err != nil {
				t.Fatalf("ReadJSON() error: %v", err)
			}
			if !found {
				t.Fatal("ReadJSON() found = false; want true")
			}
			if got != tt.want {
				t.Errorf("ReadJSON() = %+v; want %+v", got, tt.want)
			}
		})
	}
}

// TestUpdateJSON_MutateError verifies that a mutate error aborts the call with no write: an
// existing file is left byte-identical, and a missing file stays absent.
func TestUpdateJSON_MutateError(t *testing.T) {
	t.Parallel()

	mutateErr := errors.New("mutate boom")

	t.Run("ExistingFile", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "state.json")
		lockPath := path + ".lock"

		orig := sample{Name: "original", N: 1}
		if err := state.WriteJSON(path, lockPath, orig); err != nil {
			t.Fatalf("setup: WriteJSON() error: %v", err)
		}
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("setup: ReadFile() error: %v", err)
		}

		err = state.UpdateJSON(path, lockPath, func(cur sample, found bool) (sample, error) {
			return sample{}, mutateErr
		})
		if !errors.Is(err, mutateErr) {
			t.Errorf("UpdateJSON() error = %v; want errors.Is(err, mutateErr)", err)
		}

		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile() error: %v", err)
		}
		if string(after) != string(before) {
			t.Errorf("UpdateJSON() modified the file on a mutate error:\nbefore:\n%s\nafter:\n%s", before, after)
		}
	})

	t.Run("MissingFile", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "state.json")
		lockPath := path + ".lock"

		err := state.UpdateJSON(path, lockPath, func(cur sample, found bool) (sample, error) {
			return sample{}, mutateErr
		})
		if !errors.Is(err, mutateErr) {
			t.Errorf("UpdateJSON() error = %v; want errors.Is(err, mutateErr)", err)
		}

		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("UpdateJSON() created %s on a mutate error; want it to remain absent", path)
		}
	})
}

// TestUpdateJSON_Concurrency drives many goroutines through UpdateJSON at once, each appending one distinct element to a shared []int, and verifies every element lands exactly once.
// The test is driven entirely through UpdateJSON itself, rather than a separate read phase followed by an update phase, because the latter needs an artificial barrier to fail deterministically pre-fix — driving through the primitive avoids that entirely.
//
//testtiming:keep pins that concurrent appends never lose or clobber a write, which no covering test asserts
func TestUpdateJSON_Concurrency(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "state.json")
	lockPath := path + ".lock"

	const n = 50

	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(elem int) {
			defer wg.Done()
			err := state.UpdateJSON(path, lockPath, func(cur []int, found bool) ([]int, error) {
				return append(cur, elem), nil
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("UpdateJSON() error: %v", err)
		}
	}

	got, found, err := state.ReadJSON[[]int](path, lockPath)
	if err != nil {
		t.Fatalf("ReadJSON() error: %v", err)
	}
	if !found {
		t.Fatal("ReadJSON() found = false; want true")
	}
	if len(got) != n {
		t.Fatalf("ReadJSON() len = %d; want %d", len(got), n)
	}

	seen := make(map[int]int, n)
	for _, v := range got {
		seen[v]++
	}
	for i := 0; i < n; i++ {
		if seen[i] != 1 {
			t.Errorf("element %d appeared %d times; want exactly 1", i, seen[i])
		}
	}
	if t.Failed() {
		t.Logf("got: %v", got)
	}
}
