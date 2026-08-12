// update_test.go covers UpdateJSON's four error/creation dispositions — missing file, existing
// file, a mutate error, and a corrupt file — plus its core concurrency property: many concurrent
// callers each appending one element under UpdateJSON never lose or clobber each other's write.

package state_test

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/state"
)

// TestUpdateJSON_MissingFile verifies that mutate sees the zero value with found=false when the
// file does not exist yet, and that mutate's returned value is created on disk.
func TestUpdateJSON_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "state.json")
	lockPath := path + ".lock"

	var sawFound bool
	var sawCur sample
	want := sample{Name: "created", N: 1}

	err := state.UpdateJSON(path, lockPath, func(cur sample, found bool) (sample, error) {
		sawCur = cur
		sawFound = found
		return want, nil
	})
	if err != nil {
		t.Fatalf("UpdateJSON() error: %v", err)
	}
	if sawFound {
		t.Error("UpdateJSON() mutate found = true; want false for a missing file")
	}
	if sawCur != (sample{}) {
		t.Errorf("UpdateJSON() mutate cur = %+v; want zero value", sawCur)
	}

	got, found, err := state.ReadJSON[sample](path, lockPath)
	if err != nil {
		t.Fatalf("ReadJSON() error: %v", err)
	}
	if !found {
		t.Fatal("ReadJSON() found = false; want true")
	}
	if got != want {
		t.Errorf("ReadJSON() = %+v; want %+v", got, want)
	}
}

// TestUpdateJSON_ExistingFile verifies that mutate sees the decoded value with found=true, and
// that mutate's returned value replaces it on disk.
func TestUpdateJSON_ExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "state.json")
	lockPath := path + ".lock"

	orig := sample{Name: "original", N: 1}
	if err := state.WriteJSON(path, lockPath, orig); err != nil {
		t.Fatalf("setup: WriteJSON() error: %v", err)
	}

	var sawFound bool
	var sawCur sample
	want := sample{Name: "updated", N: 2}

	err := state.UpdateJSON(path, lockPath, func(cur sample, found bool) (sample, error) {
		sawCur = cur
		sawFound = found
		return want, nil
	})
	if err != nil {
		t.Fatalf("UpdateJSON() error: %v", err)
	}
	if !sawFound {
		t.Error("UpdateJSON() mutate found = false; want true for an existing file")
	}
	if sawCur != orig {
		t.Errorf("UpdateJSON() mutate cur = %+v; want %+v", sawCur, orig)
	}

	got, found, err := state.ReadJSON[sample](path, lockPath)
	if err != nil {
		t.Fatalf("ReadJSON() error: %v", err)
	}
	if !found {
		t.Fatal("ReadJSON() found = false; want true")
	}
	if got != want {
		t.Errorf("ReadJSON() = %+v; want %+v", got, want)
	}
}

// TestUpdateJSON_MutateError verifies that a mutate error aborts the call with no write: an
// existing file is left byte-identical, and a missing file stays absent.
func TestUpdateJSON_MutateError(t *testing.T) {
	mutateErr := errors.New("mutate boom")

	t.Run("ExistingFile", func(t *testing.T) {
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

// TestUpdateJSON_CorruptFile verifies that an existing file holding invalid JSON aborts with a
// non-nil error, that mutate never runs, and that the file's bytes are unchanged.
func TestUpdateJSON_CorruptFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "corrupt.json")
	lockPath := path + ".lock"

	before := []byte("{not json")
	if err := os.WriteFile(path, before, 0o644); err != nil {
		t.Fatalf("setup: WriteFile() error: %v", err)
	}

	var mutateRan bool
	err := state.UpdateJSON(path, lockPath, func(cur sample, found bool) (sample, error) {
		mutateRan = true
		return cur, nil
	})
	if err == nil {
		t.Fatal("UpdateJSON() error = nil; want non-nil for a corrupt file")
	}
	if mutateRan {
		t.Error("UpdateJSON() ran mutate on a corrupt file; want it never called")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(after) != string(before) {
		t.Errorf("UpdateJSON() modified a corrupt file:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestUpdateJSON_Concurrency drives many goroutines through UpdateJSON at once, each appending one
// distinct element to a shared []int, and verifies every element lands exactly once. The test is
// driven entirely through UpdateJSON itself, rather than a separate read phase followed by an
// update phase, because the latter needs an artificial barrier to fail deterministically pre-fix —
// driving through the primitive avoids that entirely.
func TestUpdateJSON_Concurrency(t *testing.T) {
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
