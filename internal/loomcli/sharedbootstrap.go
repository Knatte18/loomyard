// sharedbootstrap.go holds the blocks `run` and `step` share.
// Every helper here is lock-agnostic: none acquires or releases loomengine.LoomBootstrapLock,
// because the lock's position differs between the two calling verbs -- `run` acquires it later than
// its own seed/verify/commit block and holds it across the strand work, the driver spawn, and the
// run-lock handshake, while `step` spawns no driver and runs no handshake, so it wraps the strand
// block alone and releases before calling the producer. Each verb wraps its own lock window around
// these calls; no helper here may assume either window.

package loomcli

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
)

// bootstrapStage names the sub-step of seedAndCommitBootstrap a failure occurred at, so a caller can
// classify the failure without re-running the sub-steps: `run` deliberately ignores it and keeps
// writing its envelope exactly as before this extraction, while `step` maps it onto its own refusal-
// kind vocabulary.
type bootstrapStage int

const (
	// bootstrapStageNone is the zero value: no failure occurred, or the caller has not yet inspected
	// the returned stage.
	bootstrapStageNone bootstrapStage = iota
	// bootstrapStageOrigin means the failure occurred while reading or writing the origin record, or
	// while resolving the parent branch from it.
	bootstrapStageOrigin
	// bootstrapStageSeed means the failure occurred while seeding the status file, for a reason other
	// than the file already existing.
	bootstrapStageSeed
	// bootstrapStageOwnership means the failure occurred while verifying the seeded status file
	// belongs to this task's own slug.
	bootstrapStageOwnership
	// bootstrapStageCommit means the failure occurred while committing the seed and the origin
	// record into the fabric.
	bootstrapStageCommit
)

// seedAndCommitBootstrap resolves the parent branch, seeds the status file when absent, verifies
// seed ownership, and commits the seed and the origin record into the fabric -- today's steps 1
// through 3 from run.go's RunE, held here verbatim and in today's order so `step` can call the exact
// same sequence.
//
// On success it returns the resolved parent branch, bootstrapStageNone, and a nil error. On failure
// it returns the empty string, the stage that failed, and the error unwrapped.
func (c *loomCLI) seedAndCommitBootstrap(slug, parentFlag string) (string, bootstrapStage, error) {
	// Step 1: resolve the recorded parent branch, writing the provenance record only for a legacy
	// worktree created before it existed.
	recorded, found, err := fabricengine.ReadOrigin(c.location)
	if err != nil {
		return "", bootstrapStageOrigin, err
	}
	parent, writeOrigin, err := resolveParentBranch(recorded, found, parentFlag)
	if err != nil {
		return "", bootstrapStageOrigin, err
	}
	if writeOrigin {
		// loom's envelope deliberately gains no mutation keys here: the Mutation Record Invariant
		// binds fabric verb outcomes, and loom's own result is not one, so the recorder is thrown
		// away rather than surfaced.
		originRec := fabricengine.NewMutations("")
		if err := fabricengine.WriteOrigin(originRec, c.location, slug, fabricengine.Origin{ParentBranch: parent}); err != nil {
			return "", bootstrapStageOrigin, err
		}
	}

	// Step 2: seed the status file, tolerating exactly the already-seeded sentinel so a re-run works.
	// A stat-then-seed probe here would reintroduce the exact race the seeder's single lock exists to
	// close, so ErrSeedExists is the only accepted outcome.
	if err := loomshed.Seed(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, slug, parent); err != nil && !errors.Is(err, loomshed.ErrSeedExists) {
		return "", bootstrapStageSeed, err
	}
	// An already-present status file must be THIS task's own: `lyx fabric add` run from a task
	// worktree forks the whole pair, `_lyx` task state included, and the driver would otherwise
	// silently resume the inherited task's run under the wrong slug (crucible round fable5-high-r3,
	// F-B7).
	if err := loomengine.VerifySeedOwnership(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, slug); err != nil {
		return "", bootstrapStageOwnership, err
	}

	// Step 3: commit the seed and the provenance record into the fabric, unconditionally on every
	// invocation -- not gated on this invocation's own writeOrigin. The origin record's path is
	// included every time for the same reason the status file's path always is: if a prior
	// invocation wrote the record to disk (step 1) but crashed before this step committed it,
	// resolveParentBranch's next read finds the record present with a matching value and reports
	// write == false, even though the record is still untracked in the fabric. Including the path
	// unconditionally makes that state self-heal on the very next call, exactly as the status file
	// already does -- and costs nothing on the ordinary path, since committing an already-clean,
	// already-tracked path is a no-op (StageAndCommit reports committed == false).
	// This must precede any producer call: the phase machine's very first precondition row scans the
	// fabric including untracked files, and neither file is on the never-tracked exclude list, so an
	// uncommitted seed or record would fail that check immediately.
	commitPaths := []string{loomengine.LoomStatusRel(), fabricengine.OriginRecordRel()}
	commitRec := fabricengine.NewMutations("")
	commitMsg := fmt.Sprintf("loom: seed session bootstrap for %s", slug)
	if _, _, err := fabricengine.CommitAnchoredPaths(commitRec, c.location, commitPaths, commitMsg, fabricengine.EnvSyncOptions()); err != nil {
		return "", bootstrapStageCommit, err
	}

	return parent, bootstrapStageNone, nil
}
