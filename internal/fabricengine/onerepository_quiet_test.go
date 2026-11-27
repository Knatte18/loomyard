// onerepository_quiet_test.go covers the layout read of a path that holds no worktree, which spawns no git.
// It is an internal-package test because oneRepositoryAt is unexported.

package fabricengine

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestOneRepositoryAt_NoWorktreeOnDiskReadsAsTwoRepositoriesQuietly pins the layout read of a path with no worktree.
// Such a path, a project or a task not started, answers false and logs nothing.
// It swaps the process-global log output, so it does not run in parallel.
func TestOneRepositoryAt_NoWorktreeOnDiskReadsAsTwoRepositoriesQuietly(t *testing.T) {
	tiergate.Require(t, 0)

	tests := []struct {
		name string
		// path returns the path to read inside dir.
		path func(t *testing.T, dir string) string
	}{
		{"MissingPath", func(t *testing.T, dir string) string { return filepath.Join(dir, "missing") }},
		{"DirectoryWithoutGit", func(t *testing.T, dir string) string { return dir }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logged bytes.Buffer
			logger.SetOutput(&logged)
			t.Cleanup(func() { logger.SetOutput(logger.DefaultOutput()) })
			path := tt.path(t, t.TempDir())

			if oneRepositoryAt(path) {
				t.Errorf("oneRepositoryAt(%s) = true; want false", path)
			}
			if got := logged.String(); got != "" {
				t.Errorf("oneRepositoryAt(%s) logged %q; want nothing", path, got)
			}
		})
	}
}
