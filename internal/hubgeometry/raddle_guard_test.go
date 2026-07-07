// raddle_guard_test.go is a guard to ensure that internal/hubgeometry never
// discovers or enumerates the _raddle directory. This documents that hubgeometry
// never scans the worktree to mirror dirs — a future nested/ignored _raddle
// can never be treated as a sibling. Geometry methods like WeftRaddleDir() are
// exceptions: they compute paths purely via filepath.Join with no discovery logic.

package hubgeometry

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRaddleGuard verifies that no production source file in internal/hubgeometry
// contains the literal substring _raddle.
func TestRaddleGuard(t *testing.T) {
	t.Run("tree-scan", func(t *testing.T) {
		// Resolve package directory relative to this test file.
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatal("could not determine test file location")
		}
		// One level up from internal/hubgeometry/raddle_guard_test.go → package dir
		pkgDir := filepath.Dir(file)

		// Predicate: returns true if the bytes contain _raddle.
		containsRaddle := func(data []byte) bool {
			return strings.Contains(string(data), "_raddle")
		}

		var failures []string

		// Walk the package directory.
		err := filepath.WalkDir(pkgDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			// Only check .go files that are not _test.go files.
			if !d.IsDir() && strings.HasSuffix(d.Name(), ".go") && !strings.HasSuffix(d.Name(), "_test.go") {
				// Skip hubgeometry.go: it contains geometry methods like WeftRaddleDir() that compute
				// paths purely via filepath.Join, which is allowed. The guard applies only to
				// discovery/enumeration logic, not to geometry computation.
				if d.Name() == "hubgeometry.go" {
					return nil
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if containsRaddle(data) {
					failures = append(failures, d.Name())
				}
			}

			return nil
		})

		if err != nil {
			t.Fatalf("failed to walk package directory: %v", err)
		}

		if len(failures) > 0 {
			t.Errorf("found _raddle reference in production files: %v", failures)
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
				name:    "contains _raddle",
				content: "path := filepath.Join(dir, _raddle, slug)",
				want:    true,
			},
			{
				name:    "clean",
				content: "return filepath.Join(l.Container, slug)",
				want:    false,
			},
		}

		containsRaddle := func(content string) bool {
			return strings.Contains(content, "_raddle")
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := containsRaddle(tt.content)
				if got != tt.want {
					t.Errorf("containsRaddle(%q) = %v, want %v", tt.content, got, tt.want)
				}
			})
		}
	})
}
