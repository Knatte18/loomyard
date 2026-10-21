// taskpatch.go — the read-only git reads a code segment's identity needs on a Fabric:
// the task's patch against its merge base, the merge-in-only test of its history, and the weft HEAD.

package fabricengine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// TaskPatch is the warp working tree's change against a merge base.
// Head is the commit HEAD stands at, Full the full diff bytes in a stable order, and FullHash the hex SHA-256 of Full.
// PatchID is git's stable patch-id of the zero-context form of the same diff, so it survives a rebase that moves the context around the change; it is empty when the diff is.
type TaskPatch struct {
	Head     string
	Full     []byte
	FullHash string
	PatchID  string
}

// ReadTaskPatch reads the warp working tree's change against base, the told merge base.
// The change is the full diff of tracked files, uncommitted changes included, plus every untracked file that is not ignored, each rendered as a new-file diff.
// An untracked path equal to a skip entry, or under a skip entry ending in "/", is left out;
// a skip entry exempts a path only while it is untracked, so a tracked file of that name still counts.
func (f *Fabric) ReadTaskPatch(base string, skip []string) (TaskPatch, error) {
	head, err := f.code.CurrentSHA()
	if err != nil {
		return TaskPatch{}, fmt.Errorf("fabricengine: read task patch: %w", err)
	}

	untracked, err := f.code.UntrackedFiles()
	if err != nil {
		return TaskPatch{}, fmt.Errorf("fabricengine: read task patch: list untracked files: %w", err)
	}
	var counted []string
	for _, path := range untracked {
		if !skippedPath(path, skip) {
			counted = append(counted, path)
		}
	}

	full, err := f.renderTaskDiff(base, counted, "-U3")
	if err != nil {
		return TaskPatch{}, err
	}
	zeroContext, err := f.renderTaskDiff(base, counted, "-U0")
	if err != nil {
		return TaskPatch{}, err
	}

	patchID, err := stablePatchID(f.warpPath, zeroContext)
	if err != nil {
		return TaskPatch{}, err
	}
	sum := sha256.Sum256(full)
	return TaskPatch{Head: head, Full: full, FullHash: hex.EncodeToString(sum[:]), PatchID: patchID}, nil
}

// skippedPath reports whether the untracked path is exempted by a skip entry.
func skippedPath(path string, skip []string) bool {
	for _, entry := range skip {
		if strings.HasSuffix(entry, "/") {
			if strings.HasPrefix(path, entry) {
				return true
			}
		} else if path == entry {
			return true
		}
	}
	return false
}

// renderTaskDiff returns the tracked diff against base followed by one new-file diff per untracked path, with context as git's context-lines flag.
func (f *Fabric) renderTaskDiff(base string, untracked []string, context string) ([]byte, error) {
	tracked, err := gitexec.Run([]string{"diff", "--no-color", "--no-ext-diff", "--no-textconv", "--no-renames", "--binary", context, base, "--"}, f.warpPath)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: read task patch: diff the working tree against %s: %w", base, err)
	}

	var rendered strings.Builder
	rendered.WriteString(tracked)
	for _, path := range untracked {
		diff, err := gitexec.Run([]string{"diff", "--no-index", "--no-color", "--no-ext-diff", "--no-textconv", "--binary", context, "--", "/dev/null", path}, f.warpPath)
		// A new file differs from /dev/null, which git reports as exit 1 with the diff on stdout.
		var gitErr *gitexec.GitError
		if err != nil && !(errors.As(err, &gitErr) && gitErr.ExitCode == 1) {
			return nil, fmt.Errorf("fabricengine: read task patch: render untracked file %s: %w", path, err)
		}
		rendered.WriteString(diff)
	}
	return []byte(rendered.String()), nil
}

// stablePatchID returns git's `patch-id --stable` of diff, or "" for a diff with no change.
func stablePatchID(dir string, diff []byte) (string, error) {
	out, err := gitexec.RunStdin([]string{"patch-id", "--stable"}, dir, string(diff))
	if err != nil {
		return "", fmt.Errorf("fabricengine: read task patch: patch-id: %w", err)
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return "", nil
	}
	return fields[0], nil
}

// MergeInOnly reports whether every first-parent commit from HEAD back to since, since excluded, came from a Merge-In.
// A commit counts when it is a merge commit whose second parent is an ancestor of parentTip, or when it is itself an ancestor of parentTip, which is how a fast-forward Merge-In leaves it.
// It reports false when since is not on HEAD's first-parent history, since a history that skips it cannot be shown to hold only Merge-Ins.
func (f *Fabric) MergeInOnly(since, parentTip string) (bool, error) {
	head, err := f.code.CurrentSHA()
	if err != nil {
		return false, fmt.Errorf("fabricengine: merge-in-only history: %w", err)
	}
	commits, err := f.code.Log(head, gitrepo.LogQuery{FirstParent: true})
	if err != nil {
		return false, fmt.Errorf("fabricengine: merge-in-only history: %w", err)
	}

	for _, commit := range commits {
		if commit.SHA == since {
			return true, nil
		}
		tip := commit.SHA
		if len(commit.Parents) > 1 {
			tip = commit.Parents[1]
		}
		fromParent, err := f.code.IsAncestor(tip, parentTip)
		if err != nil {
			return false, fmt.Errorf("fabricengine: merge-in-only history: %w", err)
		}
		if !fromParent {
			return false, nil
		}
	}
	return false, nil
}

// RecordsHeadSHA returns the commit the weft checkout's HEAD stands at.
// It is a thin, read-only delegation to gitrepo.Repo.CurrentSHA on f.records, so a caller reads the weft side without naming it.
func (f *Fabric) RecordsHeadSHA() (string, error) {
	return f.records.CurrentSHA()
}
