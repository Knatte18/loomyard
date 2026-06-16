// enforcement_test.go is a repo-wide guard: it walks every package and fails
// the build if any file outside internal/paths reaches for raw cwd or top-level
// git geometry, keeping internal/paths the sole geometry owner.

package paths

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestEnforcement walks the repo source tree and verifies that no source file
// outside internal/paths and cmd/lyx contains the raw cwd/root primitives
// os.Getwd or git rev-parse --show-toplevel.
func TestEnforcement(t *testing.T) {
	t.Run("tree-scan", func(t *testing.T) {
		// Resolve repo root relative to this test file.
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatal("could not determine test file location")
		}
		// Two levels up from internal/paths/enforcement_test.go → repo root
		repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(file)))

		// Predicate: returns true if the bytes contain a banned token.
		isBanned := func(data []byte) bool {
			content := string(data)
			return strings.Contains(content, "os.Getwd") ||
				strings.Contains(content, "--show-toplevel")
		}

		var failures []string

		// Walk the entire tree.
		err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			// Skip .git, testdata, and _test.go files.
			if d.IsDir() && (d.Name() == ".git" || strings.Contains(d.Name(), "testdata")) {
				return filepath.SkipDir
			}
			if strings.HasSuffix(d.Name(), "_test.go") {
				return nil
			}

			// Only check .go files.
			if !d.IsDir() && strings.HasSuffix(d.Name(), ".go") {
				// Determine the package-relative directory.
				relPath, err := filepath.Rel(repoRoot, path)
				if err != nil {
					return err
				}
				pkgDir := filepath.Dir(relPath)

				// Normalize path separators to forward slashes for comparison.
				pkgDir = filepath.ToSlash(pkgDir)

				// Check allowlist: internal/paths, cmd/lyx/main.go
				isAllowed := pkgDir == "internal/paths" ||
					(pkgDir == "cmd/lyx" && d.Name() == "main.go")

				// Skip files in the allowlist (they are allowed to contain banned tokens).
				if isAllowed {
					return nil
				}

				// Check the file for banned tokens.
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if isBanned(data) {
					failures = append(failures, relPath)
				}
			}

			return nil
		})

		if err != nil {
			t.Fatalf("failed to walk repo tree: %v", err)
		}

		if len(failures) > 0 {
			t.Errorf("found banned tokens in files: %v", failures)
		}
	})

	// Sub-test: verify the predicate itself on synthetic snippets.
	t.Run("predicate", func(t *testing.T) {
		tests := []struct {
			name    string
			content string
			want    bool
		}{
			{
				name:    "os.Getwd",
				content: "x := os.Getwd()",
				want:    true,
			},
			{
				name:    "--show-toplevel",
				content: `git rev-parse --show-toplevel`,
				want:    true,
			},
			{
				name:    "clean",
				content: "fmt.Println(hello)",
				want:    false,
			},
		}

		isBanned := func(content string) bool {
			return strings.Contains(content, "os.Getwd") ||
				strings.Contains(content, "--show-toplevel")
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := isBanned(tt.content)
				if got != tt.want {
					t.Errorf("isBanned(%q) = %v, want %v", tt.content, got, tt.want)
				}
			})
		}
	})
}
