// stop.go implements the orch's bring-down, `BeamDown`, and the `stop` orch verb over its first step: remove the orchestrator strand and let the watcher notice and exit on its own.

package orchcli

import (
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// stopStartLockWait is how long `stop` waits for start.lock, which a bring-up holds across its origin reconcile and session launch.
const stopStartLockWait = time.Minute

// BeamDownReport is what one BeamDown stopped.
type BeamDownReport struct {
	// Worktree is the name of the worktree brought down.
	Worktree string `json:"worktree"`
	// OrchRemoved is whether a tracked orch strand was removed, and Strand is the strand the orch state records.
	OrchRemoved bool   `json:"orch_removed"`
	Strand      string `json:"strand,omitempty"`
	// DobbyEnded is whether a live dobby was ended.
	DobbyEnded bool `json:"dobby_ended"`
}

// BeamDown takes the worktree at location down: it removes the orch strand, after which the watcher exits on its own, and ends the worktree's dobby when it is live.
// Each part stops only when live, so a second call stops nothing.
// It refuses a worktree that is neither the prime nor a beam.
func BeamDown(location *lyxcwd.Location) (BeamDownReport, error) {
	c := &orchCLI{}
	if err := wireCLI(c, location); err != nil {
		return BeamDownReport{}, err
	}
	return c.beamDown()
}

// beamDown is BeamDown over a wired receiver.
// The dobby is ended through the receiver's role-scoped end, so no other process of the beam is signalled.
func (c *orchCLI) beamDown() (BeamDownReport, error) {
	report := BeamDownReport{Worktree: c.location.WorktreeName}
	removed, strand, err := c.stopOrch()
	if err != nil {
		return report, err
	}
	report.OrchRemoved, report.Strand = removed, strand
	if c.dobbyLive == nil {
		return report, nil
	}
	live, err := c.dobbyLive()
	if err != nil {
		return report, fmt.Errorf("orch: read whether the dobby of %s is live: %w", c.location.WorktreeName, err)
	}
	if !live {
		return report, nil
	}
	if err := c.endDobby(); err != nil {
		return report, fmt.Errorf("orch: end the dobby of %s: %w", c.location.WorktreeName, err)
	}
	report.DobbyEnded = true
	return report, nil
}

// stopOrch removes the recorded orchestrator strand when reed tracks it, under start.lock so a status restart never binds a watcher to a strand it is removing.
// An untracked or unrecorded strand reports removed false; strand is the one the orch state records.
// It waits for a running bring-up up to startLockWait, then refuses naming the lock.
func (c *orchCLI) stopOrch() (removed bool, strand string, err error) {
	if err := os.MkdirAll(c.paths.Dir, 0o755); err != nil {
		return false, "", fmt.Errorf("orch: create %s: %w", c.paths.Dir, err)
	}
	startLock, acquired, err := lock.AcquireWriteLockWithin(c.paths.StartLockPath, c.startLockWait)
	if err != nil {
		return false, "", err
	}
	if !acquired {
		return false, "", fmt.Errorf("orch: start.lock (%s) is still held by a running `lyx beam start`; way forward: rerun `lyx orch stop` once it ends", c.paths.StartLockPath)
	}
	defer func() { _ = startLock.Release() }()

	st, err := orchengine.LoadState(c.paths)
	if err != nil {
		return false, "", err
	}
	strands, err := c.strands.Strands()
	if err != nil {
		return false, "", err
	}
	_, tracked := trackedStrand(strands, st.Strand)
	if tracked {
		if err := c.strands.RemoveStrand(st.Strand); err != nil {
			return false, "", err
		}
	}
	return tracked, st.Strand, nil
}

// stopCmd builds the `stop` subcommand.
func (c *orchCLI) stopCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "stop",
		Short:       "remove the orchestrator strand",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `stop removes the recorded orchestrator strand when reed tracks it. An untracked
or unrecorded strand reports removed: false rather than an error. The watcher
exits on its own once the strand is gone, and the next start resumes the
recorded session. stop holds start.lock around the removal, so a status
restart (watcher_restarted) never binds a watcher to a strand stop is removing;
it waits for a running start up to a minute, then refuses and names the lock.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			removed, strand, err := c.stopOrch()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"removed": removed,
				"strand":  strand,
			}))
			return nil
		},
	}
}
