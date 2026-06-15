// codeguide_guard_test.go is a guard to ensure that internal/paths never
// references or enumerates the _codeguide directory. This documents that paths
// never scans the worktree to mirror dirs — a future nested/ignored _codeguide
// can never be treated as a sibling.

package paths

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestCodeguideGuard verifies that no production source file in internal/paths
// contains the literal substring _codeguide.
func TestCodeguideGuard(t *testing.T) {
	t.Run("tree-scan", func(t *testing.T) {
		// Resolve package directory relative to this test file.
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatal("could not determine test file location")
		}
		// One level up from internal/paths/codeguide_guard_test.go → package dir
		pkgDir := filepath.Dir(file)

		// Predicate: returns true if the bytes contain _codeguide.
		containsCodeguide := func(data []byte) bool {
			return strings.Contains(string(data), "_codeguide")
		}

		var failures []string

		// Walk the package directory.
		err := filepath.WalkDir(pkgDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			// Only check .go files that are not _test.go files.
			if !d.IsDir() && strings.HasSuffix(d.Name(), ".go") && !strings.HasSuffix(d.Name(), "_test.go") {
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if containsCodeguide(data) {
					failures = append(failures, d.Name())
				}
			}

			return nil
		})

		if err != nil {
			t.Fatalf("failed to walk package directory: %v", err)
		}

		if len(failures) > 0 {
			t.Errorf("found _codeguide reference in production files: %v", failures)
		}
	})

	// Sub-test: verify the predicate itself on synthetic strings.
	t.Run("predicate", func(t *testing.T) {
		tests := []struct {
			name    string
			content string
			want    bool
		}{
			{
				name:    "contains _codeguide",
				content: "path := filepath.Join(dir, _codeguide, slug)",
				want:    true,
			},
			{
				name:    "clean",
				content: "return filepath.Join(l.Container, slug)",
				want:    false,
			},
		}

		containsCodeguide := func(content string) bool {
			return strings.Contains(content, "_codeguide")
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := containsCodeguide(tt.content)
				if got != tt.want {
					t.Errorf("containsCodeguide(%q) = %v, want %v", tt.content, got, tt.want)
				}
			})
		}
	})
}
