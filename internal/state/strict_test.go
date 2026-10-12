// strict_test.go tests ReadJSONNoMkdir's unknown-field tolerance and no-MkdirAll contract using only temp
// files — no git, no spawning, untagged (Tier 1).

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

func TestReadJSONNoMkdir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// content is written to value.json under a real t.TempDir() parent, so a file miss is a clean os.IsNotExist and not a lock-acquire failure caused by an absent parent directory (flock.RLock fails immediately if the lock file's directory does not exist -- see TestReadJSONNoMkdir_MissingFileCreatesNoDirectory below).
		// A nil content leaves the file absent.
		content       []byte
		wantOK        bool
		wantName      string
		wantErrDecode bool
	}{
		{name: "ValidDecode", content: []byte(`{"name":"widget"}`), wantOK: true, wantName: "widget"},
		{name: "MissingFileExistingParent", content: nil},
		{name: "UnknownField", content: []byte(`{"name":"widget","extra":true}`), wantOK: true, wantName: "widget"},
		{name: "MalformedJSON", content: []byte(`{"name":`), wantErrDecode: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := filepath.Join(dir, "value.json")
			lockPath := filepath.Join(dir, "value.json.lock")
			if tt.content != nil {
				if err := os.WriteFile(path, tt.content, 0o644); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
			}

			got, ok, err := ReadJSONNoMkdir[strictTestValue](path, lockPath)
			if tt.wantErrDecode {
				if !errors.Is(err, ErrDecode) {
					t.Errorf("ReadJSONNoMkdir() error = %v; want errors.Is(err, ErrDecode)", err)
				}
			} else if err != nil {
				t.Fatalf("ReadJSONNoMkdir() error = %v; want nil", err)
			}
			if ok != tt.wantOK {
				t.Errorf("ReadJSONNoMkdir() ok = %v; want %v", ok, tt.wantOK)
			}
			if got.Name != tt.wantName {
				t.Errorf("ReadJSONNoMkdir() Name = %q; want %q", got.Name, tt.wantName)
			}
		})
	}
}

func TestReadJSONNoMkdir_MissingFileCreatesNoDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// A subdirectory that is never created ahead of time.
	// Because ReadJSONNoMkdir deliberately skips os.MkdirAll (unlike ReadJSON), the lock acquisition on a lockPath inside a nonexistent directory fails outright rather than silently creating the directory and returning a clean (zero, false, nil) miss.
	subdir := filepath.Join(dir, "nested", "sub")
	path := filepath.Join(subdir, "value.json")
	lockPath := filepath.Join(subdir, "value.json.lock")

	_, ok, err := ReadJSONNoMkdir[strictTestValue](path, lockPath)
	if err == nil {
		t.Fatalf("ReadJSONNoMkdir() error = nil; want a lock-acquire error since the parent directory was never created")
	}
	if ok {
		t.Errorf("ReadJSONNoMkdir() ok = true; want false")
	}
	if _, statErr := os.Stat(subdir); !os.IsNotExist(statErr) {
		t.Errorf("ReadJSONNoMkdir() created parent directory %q; want it to remain absent (no MkdirAll)", subdir)
	}
}
