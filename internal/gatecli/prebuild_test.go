// prebuild_test.go covers the prebuilt lyx store over plain files: the content-derived name, reuse of a stored binary, pruning by last use, a destination another run published first and a store that cannot be created.

package gatecli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStorePrebuilt(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	// writeAged writes content to path with its modification time age before now.
	writeAged := func(t *testing.T, path, content string, age time.Duration) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}

	// builtContent is the build's content, and contentName the store entry name its SHA-256 gives: `lyx-` and the first 16 hex digits.
	const builtContent, contentName = "built lyx", "lyx-1d3aedeaa33d679f"

	tests := []struct {
		name string
		// stored is the store's content before the call, entry name to age.
		stored map[string]time.Duration
		// storeIsFile makes the store path a regular file, so the store cannot be created and the call must fail.
		storeIsFile bool
		// wantKept and wantPruned name entries of stored that must survive and must be gone.
		wantKept, wantPruned []string
	}{
		{name: "EmptyStore"},
		{
			name:     "EntryPublishedBeforeTheCallIsKept",
			stored:   map[string]time.Duration{contentName: 10 * time.Minute},
			wantKept: []string{contentName},
		},
		{name: "StoreDirIsARegularFile", storeIsFile: true},
		{
			name:       "OtherEntriesPrunedPastRetention",
			stored:     map[string]time.Duration{"lyx-0000000000000000": prebuiltRetention + time.Minute, "lyx-1111111111111111": prebuiltRetention - time.Minute, "lyx-2222222222222222.partial-1": 3 * prebuiltRetention},
			wantKept:   []string{"lyx-1111111111111111"},
			wantPruned: []string{"lyx-0000000000000000", "lyx-2222222222222222.partial-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			buildDir, storeDir := t.TempDir(), filepath.Join(t.TempDir(), "prebuilt")
			bin := filepath.Join(buildDir, "lyx")
			writeAged(t, bin, builtContent, 0)
			if tt.storeIsFile {
				writeAged(t, storeDir, "not a directory", 0)
				if stored, err := storePrebuilt(bin, storeDir, now); err == nil {
					t.Errorf("storePrebuilt() = %q, nil; want an error for a store path that is a regular file", stored)
				}
				return
			}
			if err := os.MkdirAll(storeDir, 0o755); err != nil {
				t.Fatal(err)
			}
			for name, age := range tt.stored {
				content := name
				if name == contentName {
					// An entry another run published under the content name holds the same build.
					content = builtContent
				}
				writeAged(t, filepath.Join(storeDir, name), content, age)
			}

			stored, err := storePrebuilt(bin, storeDir, now)
			if err != nil {
				t.Fatalf("storePrebuilt() error = %v", err)
			}
			if want := filepath.Join(storeDir, contentName); stored != want {
				t.Errorf("storePrebuilt() = %q; want %q", stored, want)
			}
			if got, err := os.ReadFile(stored); err != nil || string(got) != builtContent {
				t.Errorf("stored binary = (%q, %v); want the build's content", got, err)
			}
			if staging, err := filepath.Glob(filepath.Join(storeDir, "*.partial-*")); err != nil || len(staging) != 0 {
				t.Errorf("staging files after the call = (%q, %v); want none", staging, err)
			}
			for _, name := range tt.wantKept {
				if _, err := os.Stat(filepath.Join(storeDir, name)); err != nil {
					t.Errorf("entry %s: stat error %v; want it kept", name, err)
				}
			}
			for _, name := range tt.wantPruned {
				if _, err := os.Stat(filepath.Join(storeDir, name)); !os.IsNotExist(err) {
					t.Errorf("entry %s: stat error %v; want it pruned", name, err)
				}
			}

			// A later run of the same build an hour on reuses the entry and refreshes its last use.
			later := now.Add(time.Hour)
			again, err := storePrebuilt(bin, storeDir, later)
			if err != nil || again != stored {
				t.Fatalf("storePrebuilt() again = (%q, %v); want (%q, nil)", again, err, stored)
			}
			info, err := os.Stat(stored)
			if err != nil {
				t.Fatal(err)
			}
			if !info.ModTime().Equal(later) {
				t.Errorf("stored binary mod time = %v; want the reuse time %v", info.ModTime(), later)
			}
		})
	}

	// A destination that cannot be replaced stands for one another run published first: rename fails and the destination exists.
	t.Run("CopyIntoPlaceKeepsADestinationPublishedFirst", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		src := filepath.Join(dir, "lyx")
		writeAged(t, src, builtContent, 0)
		dst := filepath.Join(dir, contentName)
		if err := os.Mkdir(dst, 0o755); err != nil {
			t.Fatal(err)
		}
		writeAged(t, filepath.Join(dst, "occupant"), "published first", 0)

		if err := copyIntoPlace(src, dst, now); err != nil {
			t.Fatalf("copyIntoPlace() error = %v; want nil for a destination that exists", err)
		}
		if got, err := os.ReadFile(filepath.Join(dst, "occupant")); err != nil || string(got) != "published first" {
			t.Errorf("destination occupant = (%q, %v); want it untouched", got, err)
		}
		if staging, err := filepath.Glob(filepath.Join(dir, "*.partial-*")); err != nil || len(staging) != 0 {
			t.Errorf("staging files after the call = (%q, %v); want none", staging, err)
		}
	})

	t.Run("CopyIntoPlaceFailsOnAMissingSource", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		dst := filepath.Join(dir, contentName)
		if err := copyIntoPlace(filepath.Join(dir, "absent"), dst, now); err == nil {
			t.Error("copyIntoPlace() error = nil; want one for a source that does not exist")
		}
		if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
			t.Errorf("directory after the failed copy = (%v, %v); want it empty", entries, err)
		}
	})
}
