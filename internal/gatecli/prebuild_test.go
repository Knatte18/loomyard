// prebuild_test.go covers the prebuilt lym store over plain files, and which targets make a run build the binary.
// It tests the content-derived name, reuse of a stored binary, refreshing a missing entry and pruning by last use.
// It also tests a destination another run published first and a store that cannot be created.

package gatecli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestStorePrebuilt(t *testing.T) {
	tiergate.Require(t, 0)
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

	// builtContent is the build's content.
	// contentName is the store entry name its SHA-256 gives: `lym-` and the first 16 hex digits.
	const builtContent, contentName = "built lym", "lym-e6717f894100bad1"

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
			stored:     map[string]time.Duration{"lym-0000000000000000": prebuiltRetention + time.Minute, "lym-1111111111111111": prebuiltRetention - time.Minute, "lym-2222222222222222.partial-1": 3 * prebuiltRetention},
			wantKept:   []string{"lym-1111111111111111"},
			wantPruned: []string{"lym-0000000000000000", "lym-2222222222222222.partial-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			buildDir, storeDir := t.TempDir(), filepath.Join(t.TempDir(), "prebuilt")
			bin := filepath.Join(buildDir, "lym")
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

	// A destination that cannot be replaced stands for one another run published first:
	// rename fails and the destination exists.
	t.Run("CopyIntoPlaceKeepsADestinationPublishedFirst", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		src := filepath.Join(dir, "lym")
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

	// An entry a concurrent prune removed reports as not refreshed with no error, so the caller publishes it again.
	t.Run("RefreshStoredReportsAMissingEntryAsNotRefreshed", func(t *testing.T) {
		t.Parallel()

		refreshed, err := refreshStored(filepath.Join(t.TempDir(), contentName), now)
		if refreshed || err != nil {
			t.Errorf("refreshStored() = (%v, %v); want (false, nil) for a missing entry", refreshed, err)
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

func TestInvocationsUseLymBinary(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	root := t.TempDir()
	// user's tests import the kit, plain's do not, and mod/user is a nested module's package whose tests import it.
	for rel, content := range map[string]string{
		"user/user_test.go":     "package user_test\n\nimport \"github.com/Knatte18/loomyard/internal/testkit/lymbin\"\n",
		"plain/plain_test.go":   "package plain\n\nimport \"testing\"\n",
		"mod/user/user_test.go": "package user\n\nimport \"example.com/mod/internal/testkit/lymbin\"\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name        string
		invocations []invocation
		want        bool
	}{
		{"NoTargetRunsWhole", []invocation{{dir: "."}}, true},
		{"WholePattern", []invocation{{dir: ".", targets: []string{"./..."}}}, true},
		{"SubtreePattern", []invocation{{dir: ".", targets: []string{"./plain/..."}}}, true},
		{"ImportPathTarget", []invocation{{dir: ".", targets: []string{"example.com/plain"}}}, true},
		{"PackageWhoseTestsImportTheKit", []invocation{{dir: ".", targets: []string{"./plain", "./user"}}}, true},
		{"AbsolutePackageWhoseTestsImportTheKit", []invocation{{dir: ".", targets: []string{filepath.Join(root, "user")}}}, true},
		{"NestedModulePackageResolvedFromItsRunDir", []invocation{{dir: ".", targets: []string{"./plain"}}, {dir: "mod", targets: []string{"./user"}}}, true},
		{"PackagesWhoseTestsDoNotImportTheKit", []invocation{{dir: ".", targets: []string{"./plain"}}}, false},
		{"CurrentDirectoryTarget", []invocation{{dir: "plain", targets: []string{"."}}}, false},
		{"ParentDirectoryTarget", []invocation{{dir: "plain/sub", targets: []string{".."}}}, false},
		{"TargetOpeningWithTheParentDirectory", []invocation{{dir: "mod", targets: []string{"../plain"}}}, false},
		{"NoInvocation", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := invocationsUseLymBinary(root, tt.invocations); got != tt.want {
				t.Errorf("invocationsUseLymBinary(%+v) = %v; want %v", tt.invocations, got, tt.want)
			}
		})
	}

	t.Run("CommandWithoutTargetsRunsWhole", func(t *testing.T) {
		t.Parallel()
		run := tierRun{
			tier:        gateslot.Tier{Command: "go test ./...", PrebuiltLym: true},
			invocations: []invocation{{dir: ".", targets: []string{"./plain"}}},
		}
		if !needsPrebuiltLym(run, root) {
			t.Errorf("needsPrebuiltLym(%q over ./plain) = false; want true", run.tier.Command)
		}
	})
}
