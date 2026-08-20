// seedinput.go resolves the two pure inputs the session bootstrap needs before it can seed the
// status file: the parent branch to record the task against, drawn from the pair's recorded
// provenance rather than inferred, and the seed slug, drawn from the worktree's own resolved name.

package loomcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// resolveParentBranch decides the parent branch the session bootstrap seeds against, and whether
// the recorded provenance record needs writing, from the recorded origin record (recorded, found)
// and the --parent flag's value (parentFlag, empty when unset).
//
// The table below is exhaustive; each row states its own reason, because the plan's Shared
// Decisions deliberately give this task no override flag once a value is recorded:
//
//   - record present, non-empty recorded value, empty flag: returns the recorded value, no write --
//     the ordinary re-run case, nothing to do.
//   - record present, recorded value equals a non-empty flag: returns the recorded value, no write --
//     re-passing the flag that already matches is a no-op, not an error.
//   - record present, recorded value differs from a non-empty flag: returns an error naming both
//     values -- changing a recorded parent would silently re-target the eventual merge-back, and
//     there is deliberately no override flag for that.
//   - no record, or a record whose recorded value is empty, plus a non-empty flag: returns the flag,
//     write requested -- this is the legacy-worktree repair path, the one case that writes.
//   - no record, or a record whose recorded value is empty, and an empty flag: returns an error
//     naming the missing record and telling the operator to pass the flag once.
//
// A present-but-empty recorded value is treated exactly as an absent record throughout, because the
// status contract makes the parent mandatory and counts an empty string as absent (see
// contracts/specs/loom-status-spec.md's validation checklist).
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

// seedSlug returns the seed's slug from worktreeName unchanged.
//
// This is the single place the slug's source is stated -- the worktree's own resolved name -- so the
// choice is named here rather than inlined at the call site.
func seedSlug(worktreeName string) string {
	return worktreeName
}
