// strict_test.go tests ReadJSONStrict's decode-strictness and no-MkdirAll
// contract using only temp files — no git, no spawning, untagged (Tier 1).

package state

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type strictTestValue struct {
	Name string `json:"name"`
}

func TestReadJSONStrict_ValidDecode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "value.json")
	lockPath := filepath.Join(dir, "value.json.lock")

	if err := os.WriteFile(path, []byte(`{"name":"widget"}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, ok, err := ReadJSONStrict[strictTestValue](path, lockPath)
	if err != nil {
		t.Fatalf("ReadJSONStrict() error = %v; want nil", err)
	}
	if !ok {
		t.Fatalf("ReadJSONStrict() ok = false; want true")
	}
	if got.Name != "widget" {
		t.Errorf("ReadJSONStrict() Name = %q; want %q", got.Name, "widget")
	}
}

func TestReadJSONStrict_UnknownField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "value.json")
	lockPath := filepath.Join(dir, "value.json.lock")

	if err := os.WriteFile(path, []byte(`{"name":"widget","extra":true}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, ok, err := ReadJSONStrict[strictTestValue](path, lockPath)
	if !errors.Is(err, ErrDecode) {
		t.Errorf("ReadJSONStrict() error = %v; want errors.Is(err, ErrDecode)", err)
	}
	if ok {
		t.Errorf("ReadJSONStrict() ok = true; want false")
	}
}

func TestReadJSONStrict_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "value.json")
	lockPath := filepath.Join(dir, "value.json.lock")

	if err := os.WriteFile(path, []byte(`{"name":`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, ok, err := ReadJSONStrict[strictTestValue](path, lockPath)
	if !errors.Is(err, ErrDecode) {
		t.Errorf("ReadJSONStrict() error = %v; want errors.Is(err, ErrDecode)", err)
	}
	if ok {
		t.Errorf("ReadJSONStrict() ok = true; want false")
	}
}

func TestReadJSONStrict_MissingFile_ExistingParent(t *testing.T) {
	// Use a real t.TempDir() as the parent so the file miss is a clean
	// os.IsNotExist, not a lock-acquire failure caused by an absent parent
	// directory (flock.RLock fails immediately if the lock file's directory
	// does not exist — see TestReadJSONStrict_MissingFile_NoMkdirAll below).
	dir := t.TempDir()
	path := filepath.Join(dir, "value.json")
	lockPath := filepath.Join(dir, "value.json.lock")

	got, ok, err := ReadJSONStrict[strictTestValue](path, lockPath)
	if err != nil {
		t.Fatalf("ReadJSONStrict() error = %v; want nil", err)
	}
	if ok {
		t.Errorf("ReadJSONStrict() ok = true; want false")
	}
	var zero strictTestValue
	if got != zero {
		t.Errorf("ReadJSONStrict() value = %+v; want zero value", got)
	}
}

func TestReadJSONStrict_MissingFile_NoMkdirAll(t *testing.T) {
	dir := t.TempDir()
	// A subdirectory that is never created ahead of time. Because
	// ReadJSONStrict deliberately skips os.MkdirAll (unlike ReadJSON), the
	// lock acquisition on a lockPath inside a nonexistent directory fails
	// outright rather than silently creating the directory and returning a
	// clean (zero, false, nil) miss.
	subdir := filepath.Join(dir, "nested", "sub")
	path := filepath.Join(subdir, "value.json")
	lockPath := filepath.Join(subdir, "value.json.lock")

	_, ok, err := ReadJSONStrict[strictTestValue](path, lockPath)
	if err == nil {
		t.Fatalf("ReadJSONStrict() error = nil; want a lock-acquire error since the parent directory was never created")
	}
	if ok {
		t.Errorf("ReadJSONStrict() ok = true; want false")
	}
	if _, statErr := os.Stat(subdir); !os.IsNotExist(statErr) {
		t.Errorf("ReadJSONStrict() created parent directory %q; want it to remain absent (no MkdirAll)", subdir)
	}
}
