// prebuild_test.go covers the prebuilt lyx store over plain files: the content-derived name, reuse of a stored binary, and pruning by last use.

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

	tests := []struct {
		name string
		// stored is the store's content before the call, entry name to age.
		stored map[string]time.Duration
		// wantKept and wantPruned name entries of stored that must survive and must be gone.
		wantKept, wantPruned []string
	}{
		{name: "EmptyStore"},
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
			writeAged(t, bin, "built lyx", 0)
			if err := os.MkdirAll(storeDir, 0o755); err != nil {
				t.Fatal(err)
			}
			for name, age := range tt.stored {
				writeAged(t, filepath.Join(storeDir, name), name, age)
			}

			stored, err := storePrebuilt(bin, storeDir, now)
			if err != nil {
				t.Fatalf("storePrebuilt() error = %v", err)
			}
			// The first 16 hex digits of the SHA-256 of "built lyx".
			if want := filepath.Join(storeDir, "lyx-1d3aedeaa33d679f"); stored != want {
				t.Errorf("storePrebuilt() = %q; want %q", stored, want)
			}
			if got, err := os.ReadFile(stored); err != nil || string(got) != "built lyx" {
				t.Errorf("stored binary = (%q, %v); want the build's content", got, err)
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
}
