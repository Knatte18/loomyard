// acceptaudit.go implements the `accept-audit` webster verb: the operator's acknowledgement of the run-exit correctness findings that stay pending and block run entry.
// It runs websterengine.AcceptPendingAudit under the state-mutation lease (load, accept, save, release), then fabric-syncs the updated state.json.
package webstercli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// absentContractNext is the envelope's next step when accept-audit accepted on an absent contract file:
// the run is still in flight, and only a Master that writes both files again ends it.
const absentContractNext = "re-run `lyx webster run` (in a shed-driven run, re-step the Webster row); a fresh Master resumes from state.json and writes outcome.yaml and summary.md"

// acceptAuditCmd builds the `accept-audit` subcommand.
func (c *websterCLI) acceptAuditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "accept-audit",
		Short: "clear the pending run-exit audit findings once their paths are restored",
		Long: `accept-audit clears the run-exit audit findings the last run left pending.
It checks every suspect path against the last batch head and refuses, changing
nothing, while any differs or cannot be checked.
It also refuses while HEAD carries a commit past the last batch head other
than a clean parent merge; run git reset --keep <that head> first, which
refuses rather than discards uncommitted changes.
Restore the named paths with git first, then run it.
A contract file (outcome.yaml or summary.md) clears when it is absent or when
Master wrote it after the fork did; a file a fork wrote last refuses, naming
rm as the way forward. When it accepts on an absent contract file the envelope
also carries next, the step that has Master write both files again.
It only edits state.json and never changes the task worktree.
With nothing pending it succeeds and reports an empty accepted list, so a
repeated call is harmless.
On success the envelope carries accepted, one entry per accepted finding with
its class, detail and paths.

Example:
  lyx webster accept-audit`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			mutateLock, err := websterengine.AcquireStateMutation(c.geom.ScratchDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			mutateHeld := true
			defer func() {
				if mutateHeld {
					_ = mutateLock.Release()
				}
			}()

			st, err := websterengine.LoadState(c.geom.WebsterDir, c.geom.ScratchDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if st == nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, `webster: no run in progress; run "lyx webster run" first`))
				return nil
			}

			pending, onAbsentContract, err := websterengine.AcceptPendingAudit(c.engine, st, c.geom, c.parentBranch)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if err := websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, st); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			_ = mutateLock.Release()
			mutateHeld = false

			if _, syncErr := fabricSync(c.openFabric, c.anchorRel, "accept-audit"); syncErr != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: audit findings accepted but the fabric sync failed: %v; %s", syncErr, fabricSyncWayForward)))
				return nil
			}

			accepted := make([]map[string]any, 0, len(pending))
			for _, f := range pending {
				accepted = append(accepted, map[string]any{
					"class":  f.Class,
					"detail": f.Detail,
					"paths":  f.Paths,
				})
			}
			result := map[string]any{"accepted": accepted}
			if onAbsentContract {
				result["next"] = absentContractNext
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, result))
			return nil
		},
	}
}
