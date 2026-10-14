// guardstep_test.go covers scanGuardTests over fixture trees written under t.TempDir.
// Tier-1 (pure logic, no git, no spawn).

package websterengine

import (
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// writeGuardFixture writes each slash-separated path of files under root with its content, creating directories.
func writeGuardFixture(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for relative, content := range files {
		full := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
}

const (
	guardSource       = "package a\n\n//lyx:guard\nfunc TestG(t *testing.T) {}\n"
	guardSourceTwo    = "package a\n\n//lyx:guard\nfunc TestH(t *testing.T) {}\n\n//lyx:guard\nfunc TestB(t *testing.T) {}\n"
	misplacedGuardSrc = "package a\n\n//lyx:guard\nfunc helper() {}\n"
)

func TestScanGuardTests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files map[string]string
		// unreadable lists slash-separated paths chmodded 0o000 after the fixture is written.
		unreadable   []string
		wantNames    map[string][]string
		wantGoFiles  map[string][]string
		wantScanFile string
		// wantErrPath is the slash-separated path the returned error names, empty for no error.
		wantErrPath string
	}{
		{
			name:  "no root go.mod yields the zero value",
			files: map[string]string{"internal/a/a_test.go": guardSource},
		},
		{
			name:  "a root go.mod and no marked test yields no directory",
			files: map[string]string{"go.mod": "module m\n", "internal/a/a_test.go": "package a\n"},
		},
		{
			name: "marked tests in a package and at the root are listed with every go file",
			files: map[string]string{
				"go.mod":                "module m\n",
				"root_test.go":          guardSource,
				"internal/a/a.go":       "package a\n",
				"internal/a/a_test.go":  guardSourceTwo,
				"internal/a/b_test.go":  guardSource,
				"internal/a/notes.txt":  "x",
				"internal/b/b_test.go":  "package b\n",
				"internal/a/sub/s.go":   "package sub\n",
				"internal/a/sub/README": "x",
			},
			wantNames: map[string][]string{
				".":          {"TestG"},
				"internal/a": {"TestB", "TestG", "TestH"},
			},
			wantGoFiles: map[string][]string{
				".":          {"root_test.go"},
				"internal/a": {"internal/a/a.go", "internal/a/a_test.go", "internal/a/b_test.go"},
			},
		},
		{
			name: "directories go list skips are not scanned",
			files: map[string]string{
				"go.mod":                      "module m\n",
				"testdata/x/x_test.go":        misplacedGuardSrc,
				"vendor/x/x_test.go":          misplacedGuardSrc,
				"_skip/x/x_test.go":           misplacedGuardSrc,
				".hidden/x/x_test.go":         misplacedGuardSrc,
				"nested/go.mod":               "module n\n",
				"nested/x/x_test.go":          misplacedGuardSrc,
				"internal/a/testdata/t.go":    "package t\n",
				"internal/a/a_test.go":        guardSource,
				"internal/a/vendor/v_test.go": misplacedGuardSrc,
			},
			wantNames:   map[string][]string{"internal/a": {"TestG"}},
			wantGoFiles: map[string][]string{"internal/a": {"internal/a/a_test.go"}},
		},
		{
			name: "a misplaced marker is kept as a scan error with its file",
			files: map[string]string{
				"go.mod":               "module m\n",
				"internal/a/a_test.go": misplacedGuardSrc,
			},
			wantScanFile: "internal/a/a_test.go",
		},
		{
			name: "an unreadable directory is an error naming it",
			files: map[string]string{
				"go.mod":               "module m\n",
				"internal/a/a_test.go": guardSource,
				"internal/locked/l.go": "package l\n",
			},
			unreadable:  []string{"internal/locked"},
			wantErrPath: "internal/locked",
		},
		{
			name: "an unreadable test file is an error naming it",
			files: map[string]string{
				"go.mod":               "module m\n",
				"internal/a/a_test.go": guardSource,
			},
			unreadable:  []string{"internal/a/a_test.go"},
			wantErrPath: "internal/a/a_test.go",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if len(tt.unreadable) > 0 && (runtime.GOOS == "windows" || os.Geteuid() == 0) {
				t.Skip("file permissions do not bind here")
			}

			root := t.TempDir()
			writeGuardFixture(t, root, tt.files)
			for _, relative := range tt.unreadable {
				full := filepath.Join(root, filepath.FromSlash(relative))
				info, err := os.Stat(full)
				if err != nil {
					t.Fatalf("stat %s: %v", full, err)
				}
				mode := info.Mode().Perm()
				if err := os.Chmod(full, 0o000); err != nil {
					t.Fatalf("chmod %s: %v", full, err)
				}
				t.Cleanup(func() { _ = os.Chmod(full, mode) })
			}

			got, err := scanGuardTests(root)
			if tt.wantErrPath != "" {
				want := filepath.Join(root, filepath.FromSlash(tt.wantErrPath))
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("scanGuardTests() error = %v; want it to name %q", err, want)
				}
				if got.scanErr != nil {
					t.Errorf("scanGuardTests() scanErr = %v; want nil beside a read error", got.scanErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("scanGuardTests() error = %v; want nil", err)
			}
			if !maps.EqualFunc(got.names, tt.wantNames, slices.Equal) {
				t.Errorf("names = %v; want %v", got.names, tt.wantNames)
			}
			if !maps.EqualFunc(got.goFiles, tt.wantGoFiles, slices.Equal) {
				t.Errorf("goFiles = %v; want %v", got.goFiles, tt.wantGoFiles)
			}
			switch {
			case tt.wantScanFile == "" && got.scanErr != nil:
				t.Errorf("scanErr = %v; want nil", got.scanErr)
			case tt.wantScanFile != "" && (got.scanErr == nil || got.scanErr.File != tt.wantScanFile):
				t.Errorf("scanErr = %+v; want File %q", got.scanErr, tt.wantScanFile)
			}
		})
	}
}
