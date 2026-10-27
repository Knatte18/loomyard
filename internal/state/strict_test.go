//go:build tier0

// strict_test.go tests ReadJSON's unknown-field tolerance, its error sentinels and its contract that a read creates no directory, using only temp files — no git, no spawning.

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

func TestReadJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// dataDir and lockDir are the directories, relative to a t.TempDir() root, that hold value.json and its lock.
		// Setup creates no directory below the root, so each one named here must still be absent after the read.
		dataDir string
		lockDir string
		// content is written to value.json; a nil content leaves the file absent.
		content []byte
		// pathIsDir makes value.json a directory, which no read can open as a file.
		pathIsDir bool
		// lockIsDir makes value.json.lock a directory, so the lock cannot be opened although its parent exists.
		lockIsDir     bool
		wantOK        bool
		wantName      string
		wantErrRead   bool
		wantErrLock   bool
		wantErrDecode bool
	}{
		{name: "ValidDecode", dataDir: ".", lockDir: ".", content: []byte(`{"name":"widget"}`), wantOK: true, wantName: "widget"},
		{name: "MissingFileExistingParent", dataDir: ".", lockDir: "."},
		{name: "MissingDirectory", dataDir: "nested/sub", lockDir: "nested/sub"},
		{name: "LockDirectoryAbsent", dataDir: ".", lockDir: "locks/absent", content: []byte(`{"name":"widget"}`), wantOK: true, wantName: "widget"},
		{name: "LockUnopenableForAnotherReason", dataDir: ".", lockDir: ".", content: []byte(`{"name":"widget"}`), lockIsDir: true, wantErrLock: true},
		{name: "UnknownField", dataDir: ".", lockDir: ".", content: []byte(`{"name":"widget","extra":true}`), wantOK: true, wantName: "widget"},
		{name: "MalformedJSON", dataDir: ".", lockDir: ".", content: []byte(`{"name":`), wantErrDecode: true},
		{name: "UnreadableFile", dataDir: ".", lockDir: ".", pathIsDir: true, wantErrRead: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			path := filepath.Join(root, tt.dataDir, "value.json")
			lockPath := filepath.Join(root, tt.lockDir, "value.json.lock")
			if tt.lockIsDir {
				if err := os.Mkdir(lockPath, 0o755); err != nil {
					t.Fatalf("Mkdir() error = %v", err)
				}
			}
			switch {
			case tt.pathIsDir:
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatalf("Mkdir() error = %v", err)
				}
			case tt.content != nil:
				if err := os.WriteFile(path, tt.content, 0o644); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
			}

			got, ok, err := ReadJSON[strictTestValue](path, lockPath)
			switch {
			case tt.wantErrDecode:
				if !errors.Is(err, ErrDecode) {
					t.Errorf("ReadJSON() error = %v; want errors.Is(err, ErrDecode)", err)
				}
			case tt.wantErrLock:
				if err == nil {
					t.Error("ReadJSON() error = nil; want the lock failure returned, not the unlocked record")
				}
			case tt.wantErrRead:
				if !errors.Is(err, ErrRead) {
					t.Errorf("ReadJSON() error = %v; want errors.Is(err, ErrRead)", err)
				}
			case err != nil:
				t.Fatalf("ReadJSON() error = %v; want nil", err)
			}
			if ok != tt.wantOK {
				t.Errorf("ReadJSON() ok = %v; want %v", ok, tt.wantOK)
			}
			if got.Name != tt.wantName {
				t.Errorf("ReadJSON() Name = %q; want %q", got.Name, tt.wantName)
			}
			for _, dir := range []string{tt.dataDir, tt.lockDir} {
				if dir == "." {
					continue
				}
				if _, statErr := os.Stat(filepath.Join(root, dir)); !os.IsNotExist(statErr) {
					t.Errorf("ReadJSON() left directory %q present; want a read to create none", dir)
				}
			}
		})
	}
}
