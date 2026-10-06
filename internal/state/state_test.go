package state_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/state"
)

// sample is a small struct type used to instantiate the generics in tests.
type sample struct {
	Name string
	N    int
}

// TestWriteJSON_RoundTripOverwriteAndLayout writes a value, reads it back, overwrites it with a
// different one, and verifies the second read returns the new value, the file is JSON indented by two
// spaces, and the directory holds only the data file and its lock file at exactly path + ".lock",
// with no .tmp- entries left behind.
//
//testtiming:keep pins the two-space indentation and the data-plus-lock-only directory layout, which no covering test asserts
func TestWriteJSON_RoundTripOverwriteAndLayout(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "state.json")
	lockPath := path + ".lock"

	orig := sample{Name: "test", N: 42}
	if err := state.WriteJSON(path, lockPath, orig); err != nil {
		t.Fatalf("WriteJSON() error: %v", err)
	}

	got, found, err := state.ReadJSON[sample](path, lockPath)
	if err != nil {
		t.Fatalf("ReadJSON() error: %v", err)
	}
	if !found {
		t.Fatal("ReadJSON() found = false; want true")
	}
	if got != orig {
		t.Errorf("ReadJSON() = %+v; want %+v", got, orig)
	}

	replacement := sample{Name: "second", N: 2}
	if err := state.WriteJSON(path, lockPath, replacement); err != nil {
		t.Fatalf("second WriteJSON() error: %v", err)
	}
	got, found, err = state.ReadJSON[sample](path, lockPath)
	if err != nil {
		t.Fatalf("ReadJSON() after overwrite error: %v", err)
	}
	if !found {
		t.Fatal("ReadJSON() after overwrite found = false; want true")
	}
	if got != replacement {
		t.Errorf("ReadJSON() after overwrite = %+v; want %+v", got, replacement)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	wantData, _ := json.MarshalIndent(replacement, "", "  ")
	if string(data) != string(wantData) {
		t.Errorf("JSON formatting mismatch:\ngot:\n%s\nwant:\n%s", string(data), string(wantData))
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir() error: %v", err)
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	if want := []string{"state.json", "state.json.lock"}; !slices.Equal(names, want) {
		t.Errorf("directory entries = %v; want exactly %v", names, want)
	}
}

// TestMissingFile reads a never-written path and verifies found=false, err=nil, and that the parent
// dir and lock file now exist.
//
//testtiming:keep pins the parent directory and lock file ReadJSON creates for a missing path, which the UpdateJSON tests never assert
func TestMissingFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "subdir", "missing.json")
	lockPath := path + ".lock"

	got, found, err := state.ReadJSON[sample](path, lockPath)
	if err != nil {
		t.Fatalf("ReadJSON() on missing file error: %v", err)
	}
	if found {
		t.Fatal("ReadJSON() found = true; want false")
	}
	if got != (sample{}) {
		t.Errorf("ReadJSON() = %+v; want zero value", got)
	}

	// Verify parent dir and lock file exist.
	parentDir := filepath.Dir(path)
	if _, err := os.Stat(parentDir); err != nil {
		t.Errorf("parent directory does not exist: %v", err)
	}

	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("lock file does not exist at %s: %v", lockPath, err)
	}
}

// TestCorruptFile writes invalid JSON and verifies ReadJSON returns a non-nil error, and that
// UpdateJSON aborts the same way without running mutate or touching the file.
//
//testtiming:keep named by internal/loomcli's smoke test as the lenient read's decode-failure pin
func TestCorruptFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "corrupt.json")
	lockPath := path + ".lock"

	// Write corrupt JSON.
	corrupt := []byte("{not json")
	if err := os.WriteFile(path, corrupt, 0o644); err != nil {
		t.Fatalf("setup: WriteFile() error: %v", err)
	}

	_, _, err := state.ReadJSON[sample](path, lockPath)
	if err == nil {
		t.Fatal("ReadJSON() on corrupt file error = nil; want non-nil")
	}
	// The lenient read path now wraps ErrDecode too, matching ReadJSONStrict, so a caller can tell a
	// decode failure apart from a read or lock failure with errors.Is (loomshed.Seed relies on this
	// to refuse rather than escalate a present-but-undecodable status file — crucible round
	// fable5-high-r5, F3).
	if !errors.Is(err, state.ErrDecode) {
		t.Errorf("ReadJSON() on corrupt file error = %v; want errors.Is(err, ErrDecode)", err)
	}

	// UpdateJSON shares the same lenient read, so it aborts before its mutate with the same wrapped
	// error and never overwrites the corrupt file.
	mutateCalled := false
	uerr := state.UpdateJSON(path, lockPath, func(cur sample, found bool) (sample, error) {
		mutateCalled = true
		return cur, nil
	})
	if !errors.Is(uerr, state.ErrDecode) {
		t.Errorf("UpdateJSON() on corrupt file error = %v; want errors.Is(err, ErrDecode)", uerr)
	}
	if mutateCalled {
		t.Error("UpdateJSON() called mutate on a corrupt file; want it to abort before mutate so the file is left untouched")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(after) != string(corrupt) {
		t.Errorf("UpdateJSON() modified a corrupt file:\nbefore:\n%s\nafter:\n%s", corrupt, after)
	}
}
