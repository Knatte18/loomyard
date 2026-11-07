// samplerdone.go implements the `sampler-done` loom verb: the operator's flag that ends a sampler run's build.
// The verb only writes the done flag into the Sampler-Session row's record, at the row's current entry stamp.
// The row reads the flag, checks the worktree and the decision record, and ends itself Done.

package loomcli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/samplerengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/spf13/cobra"
)

// samplerDoneDeps carries the injected seams through which the sampler-done verb reads and writes.
type samplerDoneDeps struct {
	// row answers the name of the recipe's SamplerSession row, and false when the recipe has none.
	row func() (string, bool, error)
	// currentRow answers the status file's current producer, empty when the run has no status file.
	currentRow func() (string, error)
	// recordPath and recordLockPath are the Sampler-Session row's record and its lock.
	recordPath     string
	recordLockPath string
	// stamp answers the row's current entry stamp.
	stamp func() (int, error)
	// dirty answers the task worktree's uncommitted paths.
	dirty func() ([]string, error)
}

// samplerDoneReadFailure is the error envelope of a seam that could not be read, naming a re-run.
func samplerDoneReadFailure(out io.Writer, what string, err error) int {
	return output.Err(out, "loom: sampler-done: "+what+": "+err.Error()+"; way forward: run lym loom sampler-done again, and tell the operator when it keeps failing")
}

// samplerDoneVerb sets the done flag of the Sampler-Session row's record to the row's current entry stamp.
// It reports a JSON envelope naming that stamp, and it returns the exit code.
// It refuses, each with its way forward, a recipe with no SamplerSession row or a run standing at another row.
// It also refuses a record that is absent or names no strand, and a task worktree with uncommitted paths.
// A failed read of any seam is an error envelope carrying the error.
// A refusal or a failed read leaves the record unchanged.
// A repeat call writes the same stamp again.
// The verb only writes the flag.
// The row ends only on a flag at its current stamp, with a clean checkout and an intact decision record.
func samplerDoneVerb(out io.Writer, deps samplerDoneDeps) int {
	row, hasRow, err := deps.row()
	if err != nil {
		return samplerDoneReadFailure(out, "read the run's recipe rows", err)
	}
	if !hasRow {
		return output.Err(out, "loom: sampler-done: this run's recipe has no Sampler-Session row, so there is no session to end; way forward: none is needed, since the run's build ends on its own rows; `lym loom status` shows where it stands")
	}

	current, err := deps.currentRow()
	if err != nil {
		return samplerDoneReadFailure(out, "read the run's current row", err)
	}
	if current != row {
		if current == "" {
			current = "none"
		}
		return output.Err(out, fmt.Sprintf("loom: sampler-done: the run's current row is %s, not %s; way forward: run lym loom sampler-done while the run stands at %s; lym loom status shows the current row", current, row, row))
	}

	record, found, err := samplerengine.ReadRecord(deps.recordPath, deps.recordLockPath)
	if err != nil {
		return samplerDoneReadFailure(out, "read the sampler record", err)
	}
	if !found || record.StrandGUID == "" {
		return output.Err(out, "loom: sampler-done: the "+row+" row has recorded no session, so there is no build to end; way forward: wait until the "+row+" row has started its session, then run lym loom sampler-done again; when the row halted because its session could not start, fix what its halt reason names and run lym loom resume, or end the run with lym batten cancel <slug>")
	}

	dirty, err := deps.dirty()
	if err != nil {
		return samplerDoneReadFailure(out, "read the task worktree's uncommitted paths", err)
	}
	if len(dirty) > 0 {
		return output.Err(out, "loom: sampler-done: the task worktree has uncommitted paths: "+strings.Join(dirty, ", ")+"; way forward: commit the work, or discard what the operator does not keep, then run lym loom sampler-done again")
	}

	stamp, err := deps.stamp()
	if err != nil {
		return samplerDoneReadFailure(out, "read the row's entry stamp", err)
	}
	if err := samplerengine.UpdateRecord(deps.recordPath, deps.recordLockPath, func(stored *samplerengine.Record) { stored.DoneStamp = stamp }); err != nil {
		return samplerDoneReadFailure(out, "write the sampler record", err)
	}
	return output.Ok(out, map[string]any{
		"done":  true,
		"stamp": stamp,
		"note":  "the run ends " + row + " on a clean worktree and goes on to the Code-Review perch or Describe",
	})
}

// samplerDoneCmd builds the `sampler-done` subcommand.
func (c *loomCLI) samplerDoneCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "sampler-done",
		Short:       "end a sampler run's build, so the run goes on to its review and description",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `sampler-done sets the run's done flag in its Sampler-Session record. The
operator, or the sampler agent when the operator asks it to, runs it in the task
worktree once the work is committed. The Sampler-Session row then ends the
session and the run goes on through the Code-Review perch, when it is on, and
Describe. It takes no arguments, and is refused until the row has started its
session and while the run stands at another row or the worktree has uncommitted
paths.

Example:
  lym loom sampler-done`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			sessionRow := func() (string, bool, error) { return loomrecipe.EngineRow(c.recipe, samplerSessionEngine) }
			code := samplerDoneVerb(cmd.OutOrStdout(), samplerDoneDeps{
				row: sessionRow,
				currentRow: func() (string, error) {
					status, found, err := state.ReadJSON[shedengine.Status](shedrun.StatusFile(c.location, shedrun.SelfRunID), shedrun.StatusLock(c.location, shedrun.SelfRunID))
					// A run directory not yet created fails the lock open with not-exist; that is an absent status file too.
					if errors.Is(err, fs.ErrNotExist) || (err == nil && !found) {
						return "", nil
					}
					return status.CurrentProducer, err
				},
				recordPath:     shedrun.SamplerRecord(c.location, shedrun.SelfRunID),
				recordLockPath: shedrun.SamplerRecordLock(c.location, shedrun.SelfRunID),
				stamp: func() (int, error) {
					row, _, err := sessionRow()
					if err != nil {
						return 0, err
					}
					return samplerStamp(c.location, shedrun.SelfRunID, c.recipe, row)()
				},
				dirty: func() ([]string, error) { return verifytree.DirtyPaths(c.location.WorktreePath()) },
			})
			clihelp.SetExit(cmd.Context(), code)
			return nil
		},
	}
}
