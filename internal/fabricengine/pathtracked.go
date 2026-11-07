// pathtracked.go answers one read-only question: does the repo's index track a given path.
// It lives in fabricengine with the other git calls the package makes.

package fabricengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// PathTracked reports whether relPath is in the index of the repository checked out at worktreeDir.
// relPath is slash-separated and relative to worktreeDir.
// The answer is index-only: an untracked file present on disk and an absent path both report false.
func PathTracked(worktreeDir, relPath string) (bool, error) {
	stdout, err := gitexec.Run([]string{"ls-files", "--", relPath}, worktreeDir)
	if err != nil {
		return false, fmt.Errorf("fabricengine: git ls-files %q in %q: %w", relPath, worktreeDir, err)
	}
	return strings.TrimSpace(stdout) != "", nil
}
