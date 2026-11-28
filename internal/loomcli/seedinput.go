// seedinput.go resolves the two pure inputs the session bootstrap needs before it seeds the status file.
// The parent branch comes from the pair's recorded provenance, never inferred;
// the seed slug comes from the worktree's own resolved name.

package loomcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// resolveParentBranch decides the parent branch the session bootstrap seeds against, and whether
// the recorded provenance needs writing.
// It reads the origin record (recorded, found) and the --parent flag (parentFlag, empty when unset).
//
// The cases are exhaustive:
//
//   - The record holds a value and the flag is empty: it returns the recorded value and writes nothing,
//     the ordinary re-run case.
//   - The record holds the non-empty flag's value: it returns that value and writes nothing.
//     Re-passing the flag that already matches is a no-op, not an error.
//   - The record holds a different non-empty value than the flag: it returns an error naming both values.
//     Changing a recorded parent would silently re-target the eventual merge-back,
//     so there is deliberately no override flag for it.
//   - No parent is recorded and the flag is non-empty: it returns the flag and requests a write.
//     This is the legacy-worktree repair path, the only case that writes.
//   - No parent is recorded and the flag is empty: it returns an error naming the missing record
//     and telling the operator to pass the flag once.
//
// A present-but-empty recorded value is treated exactly as an absent record throughout.
// The status contract makes the parent mandatory and counts an empty string as absent;
// see contracts/specs/loom-status-spec.md's validation checklist.
func resolveParentBranch(recorded fabricengine.Origin, found bool, parentFlag string) (parent string, write bool, err error) {
	haveRecord := found && recorded.ParentBranch != ""

	if haveRecord {
		if parentFlag == "" || parentFlag == recorded.ParentBranch {
			return recorded.ParentBranch, false, nil
		}
		return "", false, fmt.Errorf(
			"loom: recorded parent branch %q disagrees with --parent %q; there is no override flag once a parent is recorded",
			recorded.ParentBranch, parentFlag,
		)
	}

	if parentFlag != "" {
		return parentFlag, true, nil
	}
	return "", false, fmt.Errorf(
		"loom: no recorded parent branch for this worktree pair; pass --parent once to record it",
	)
}

// resolveLandingParent returns the parent branch a landing run is seeded with.
// Its inputs are the recorded origin record and the task's own branch.
//
// It refuses an unrecorded or empty parent with the parent resolution's own error.
// It also refuses a parent equal to taskBranch, because a task may not be its own parent.
func resolveLandingParent(recorded fabricengine.Origin, found bool, taskBranch string) (parentBranch string, err error) {
	parent, _, err := resolveParentBranch(recorded, found, "")
	if err != nil {
		return "", err
	}
	if parent == taskBranch {
		return "", fmt.Errorf(
			"loom: recorded parent branch %q equals the task's own branch %q; a task may not be its own parent",
			parent, taskBranch,
		)
	}
	return parent, nil
}

// seedSlug returns worktreeName unchanged as the seed's slug.
//
// The slug is the worktree's own resolved name, stated here once instead of at each call site.
func seedSlug(worktreeName string) string {
	return worktreeName
}
