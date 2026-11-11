// ownership.go guards the bracket verbs against state that no live `lym webster run` owns.
// The bracket verbs are begin, await, record and recover-batch, fix-verify and fix-plan.
// A bracket verb is Master's own call and legitimately runs only under a live run holding run.lock.
// A Master session that keeps driving verbs after its run ended is a zombie:
// its commits and records drift from the loom row, which still shows the run's last outcome.
// So a bracket verb called from a Master strand refuses while run.lock is free.
// Any other caller, such as the sandbox suite's manual driving, only gets a warning on the envelope.
package webstercli

import (
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// ownerlessRunWarnings returns warnings with a notice appended when no live run holds scratchDir's run.lock.
// A probe error is folded into the warnings rather than failing the verb.
// The probe is advisory here, and a filesystem hiccup reading a lock file must never block a bracket verb.
func ownerlessRunWarnings(scratchDir string, warnings []string) []string {
	active, err := websterengine.RunActive(scratchDir)
	if err != nil {
		return append(warnings, "could not determine whether a live `lym webster run` owns this state: "+err.Error())
	}
	if !active {
		return append(warnings, "no live `lym webster run` owns this state (run.lock is free) -- this verb's mutations have no run-level owner or exit backstop; resume with `lym webster run`")
	}
	return warnings
}

// orphanedMasterRefusal returns the refusal of verb, called by the strand named caller.
// It refuses when caller is a Master strand and no live run holds scratchDir's run.lock, or when the probe fails.
// runEnded is true only for the free-lock refusal.
// A probe failure says nothing about the run, so its way forward is to re-run.
// It returns empty for any other caller, an unnamed shell's included, and for a Master strand whose run is live.
func orphanedMasterRefusal(caller, scratchDir, verb string) (refusal string, runEnded bool) {
	name, err := agentname.Parse(caller)
	if err != nil || !agentname.MatchesRole(name.Role, websterengine.MerriamStrandRole) {
		return "", false
	}
	active, err := websterengine.RunActive(scratchDir)
	if err != nil {
		return fmt.Sprintf("webster: %s could not tell whether a live `lym webster run` owns this state: %v; way forward: re-run the verb", verb, err), false
	}
	if active {
		return "", false
	}
	return fmt.Sprintf("webster: %s refuses: no live `lym webster run` owns this state (run.lock is free), so this session's run has ended and its work would drift from the loom row; way forward: end your turn with no further tool call, and the run's parent re-enters the run", verb), true
}

// refuseOrphanedMaster wraps cmd.RunE so a call from a Master strand whose run has ended refuses before the verb runs.
// The refusal envelope carries `run_ended: true` only when the run has ended, and the verb's own RunE never runs.
func (c *websterCLI) refuseOrphanedMaster(cmd *cobra.Command) *cobra.Command {
	inner := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if refusal, runEnded := orphanedMasterRefusal(os.Getenv(agentname.StrandNameEnv), c.geom.ScratchDir, cmd.Name()); refusal != "" {
			var fields map[string]any
			if runEnded {
				fields = map[string]any{"run_ended": true}
			}
			clihelp.SetExit(cmd.Context(), output.ErrFields(cmd.OutOrStdout(), refusal, fields))
			return nil
		}
		return inner(cmd, args)
	}
	return cmd
}
