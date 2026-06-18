package state_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/state"
)

// sample is a small struct type used to instantiate the generics in tests.
type sample struct {
	Name string
	N    int
}

// TestRoundTrip writes a value, reads it back, and verifies equality.
func TestRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "state.json")

	orig := sample{Name: "test", N: 42}
	if err := state.WriteJSON(path, orig); err != nil {
		t.Fatalf("WriteJSON() error: %v", err)
	}

	got, found, err := state.ReadJSON[sample](path)
	if err != nil {
		t.Fatalf("ReadJSON() error: %v", err)
	}
	if !found {
		t.Fatal("ReadJSON() found = false; want true")
	}
	if got != orig {
		t.Errorf("ReadJSON() = %+v; want %+v", got, orig)
	}
}

// TestMissingFile reads a never-written path and verifies found=false, err=nil,
// and that the parent dir and lock file now exist.
func TestMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "subdir", "missing.json")

	got, found, err := state.ReadJSON[sample](path)
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

	lockPath := path + ".lock"
	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("lock file does not exist at %s: %v", lockPath, err)
	}
}

// TestCorruptFile writes invalid JSON and verifies ReadJSON returns a non-nil error.
func TestCorruptFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "corrupt.json")

	// Write corrupt JSON.
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("setup: WriteFile() error: %v", err)
	}

	_, _, err := state.ReadJSON[sample](path)
	if err == nil {
		t.Fatal("ReadJSON() on corrupt file error = nil; want non-nil")
	}
}

// TestNoTempLeak verifies that after WriteJSON, the directory contains only
// the data file and <path>.lock, with no .tmp- entries.
func TestNoTempLeak(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "state.json")

	v := sample{Name: "test", N: 42}
	if err := state.WriteJSON(path, v); err != nil {
		t.Fatalf("WriteJSON() error: %v", err)
	}

	// List all files in tmpDir.
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir() error: %v", err)
	}

	// Check that we have exactly two files: state.json and state.json.lock.
	if len(entries) != 2 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("expected 2 files; found %d: %v", len(entries), names)
	}

	found := map[string]bool{}
	for _, e := range entries {
		found[e.Name()] = true
	}

	if !found["state.json"] {
		t.Error("state.json not found in directory")
	}
	if !found["state.json.lock"] {
		t.Error("state.json.lock not found in directory")
	}
}

// TestOverwrite writes a value, then writes a different value, and verifies
// ReadJSON returns the new value.
func TestOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "state.json")

	v1 := sample{Name: "first", N: 1}
	if err := state.WriteJSON(path, v1); err != nil {
		t.Fatalf("first WriteJSON() error: %v", err)
	}

	v2 := sample{Name: "second", N: 2}
	if err := state.WriteJSON(path, v2); err != nil {
		t.Fatalf("second WriteJSON() error: %v", err)
	}

	got, found, err := state.ReadJSON[sample](path)
	if err != nil {
		t.Fatalf("ReadJSON() error: %v", err)
	}
	if !found {
		t.Fatal("ReadJSON() found = false; want true")
	}
	if got != v2 {
		t.Errorf("ReadJSON() = %+v; want %+v", got, v2)
	}
}

// TestLockFileLocation verifies that the lock file is placed at exactly path + ".lock".
func TestLockFileLocation(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "data.json")
	expectedLockPath := path + ".lock"

	// Write and read to ensure lock files are created.
	v := sample{Name: "test", N: 42}
	if err := state.WriteJSON(path, v); err != nil {
		t.Fatalf("WriteJSON() error: %v", err)
	}

	if _, _, err := state.ReadJSON[sample](path); err != nil {
		t.Fatalf("ReadJSON() error: %v", err)
	}

	// Verify lock file exists at the expected path.
	if _, err := os.Stat(expectedLockPath); err != nil {
		t.Errorf("lock file not found at %s: %v", expectedLockPath, err)
	}

	// Verify the data file and lock are the only files in tmpDir.
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir() error: %v", err)
	}

	expected := map[string]bool{"data.json": true, "data.json.lock": true}
	found := map[string]bool{}
	for _, e := range entries {
		found[e.Name()] = true
	}

	if len(found) != len(expected) {
		t.Errorf("expected %d files; found %d", len(expected), len(found))
	}
	for name := range expected {
		if !found[name] {
			t.Errorf("expected file %q not found", name)
		}
	}
}

// TestJSONFormatting verifies that JSON is written with proper indentation (2 spaces).
func TestJSONFormatting(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "state.json")

	v := sample{Name: "test", N: 42}
	if err := state.WriteJSON(path, v); err != nil {
		t.Fatalf("WriteJSON() error: %v", err)
	}

	// Read raw file and check formatting.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}

	// Verify it's valid JSON and matches the indented format.
	var parsed sample
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("JSON unmarshal error: %v", err)
	}

	// Check that the formatted version matches what we'd expect.
	expectedData, _ := json.MarshalIndent(v, "", "  ")
	if string(data) != string(expectedData) {
		t.Errorf("JSON formatting mismatch:\ngot:\n%s\nwant:\n%s", string(data), string(expectedData))
	}
}
