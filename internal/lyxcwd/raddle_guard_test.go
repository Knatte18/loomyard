// raddle_guard_test.go is a guard to ensure that internal/lyxcwd never discovers or enumerates the
// _raddle directory.
// This documents that lyxcwd never scans the worktree to mirror dirs — a future nested/ignored
// _raddle can never be treated as a sibling.
// The guard scans every non-test .go file in the package;
// no file is exempted.

package lyxcwd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// TestRaddleGuard verifies that no production source file in internal/lyxcwd contains the literal
// substring _raddle.
func TestRaddleGuard(t *testing.T) {
	t.Run("tree-scan", func(t *testing.T) {
		// Predicate: returns true if the bytes contain _raddle.
		containsRaddle := func(data []byte) bool {
			return strings.Contains(string(data), "_raddle")
		}

		var failures []string

		scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/lyxcwd"}, Shallow: true}, func(f *scankit.File) {
			if containsRaddle(f.Data) {
				failures = append(failures, filepath.Base(f.Rel))
			}
		})
		scankit.RequireFloor(t, scanned, 1, "raddle guard")

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
