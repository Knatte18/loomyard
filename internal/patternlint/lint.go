// lint.go reads the PATTERN overview at the base and the head of a comparison and hands both to the headroom check.

package patternlint

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/pattern"
)

// overviewPath is the overview's path at the worktree root.
const overviewPath = "PATTERN.md"

// WayForward is the fix a headroom finding calls for.
const WayForward = "cut the named bytes from PATTERN.md, moving detail into the entries' background files"

// Lint compares the overview of worktree at the base with the head and reports growth past the headroom line.
// An empty base compares HEAD with the working tree; otherwise it compares base with HEAD.
// It answers false for no finding; a git or read error is returned, never a finding.
func Lint(worktree, base string) (pattern.Finding, bool, error) {
	baseRevision, headRevision := "HEAD", ""
	if base != "" {
		baseRevision, headRevision = base, "HEAD"
	}
	baseOverview, err := readOverview(worktree, baseRevision)
	if err != nil {
		return pattern.Finding{}, false, err
	}
	headOverview, err := readOverview(worktree, headRevision)
	if err != nil {
		return pattern.Finding{}, false, err
	}
	finding, found := pattern.Headroom(baseOverview, headOverview)
	return finding, found, nil
}

// readOverview returns the overview's bytes at revision, or from the working tree when revision is empty.
// An overview absent there answers nil, and every CRLF in the result is turned into LF.
func readOverview(worktree, revision string) ([]byte, error) {
	var data []byte
	if revision == "" {
		var err error
		data, err = os.ReadFile(filepath.Join(worktree, overviewPath))
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("patternlint: read %s: %w", overviewPath, err)
		}
	} else {
		listing, err := gitexec.Run([]string{"ls-tree", revision, "--", overviewPath}, worktree)
		if err != nil {
			return nil, fmt.Errorf("patternlint: list %s at %s: %w", overviewPath, revision, err)
		}
		if listing == "" {
			return nil, nil
		}
		shown, err := gitexec.Run([]string{"show", revision + ":" + overviewPath}, worktree)
		if err != nil {
			return nil, fmt.Errorf("patternlint: read %s at %s: %w", overviewPath, revision, err)
		}
		data = []byte(shown)
	}
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")), nil
}
