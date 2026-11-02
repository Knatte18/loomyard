// runs.go ensures the `runs` strand of the prime or a beam: the session's overview window, named `beam: <slug>`, running `lym batten overview`, which `start` brings up before the orchestrator strand.

package orchcli

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/statuspane"
)

// RunsStrandName is the role the runs strand is added and addressed by in the reed session of the prime or a beam.
// A loom bootstrap in a beam's session reads it to leave the session's one overview window to this strand.
const RunsStrandName = "runs"

// runsSidecarFile is the filename, under the orch directory, of the sidecar recording the runs strand's guid and the lym build that launched it.
const runsSidecarFile = "runs-strand.json"

// runsWindowPrefix opens the name of the runs strand's window, which the beam's slug or runsWindowPrimeLabel completes.
const runsWindowPrefix = "beam: "

// runsWindowPrimeLabel completes the runs strand's window name in the prime.
const runsWindowPrimeLabel = "prime"

// runsWindowTitleOf returns the name of the runs strand's window in the worktree named worktree: `beam: <worktree>` when it is a beam, `beam: prime` otherwise.
func runsWindowTitleOf(worktree string, beam bool) string {
	if beam {
		return runsWindowPrefix + worktree
	}
	return runsWindowPrefix + runsWindowPrimeLabel
}

// runsStrandAddSpec builds the runs strand's reedengine.AddSpec for the given pane command and window title: the session's overview window, with no size, no segment and no color.
func runsStrandAddSpec(cmd, title string) reedengine.AddSpec {
	return reedengine.AddSpec{
		NameOverride: RunsStrandName,
		Cmd:          cmd,
		Display:      render.Display{Anchor: render.AnchorOverview, Title: title},
	}
}

// ensureRunsStrand adds the runs strand when the session has none, keeps a live one that the sidecar records as launched by this build, and replaces any other, then records the strand and the build in the sidecar.
// With no runs strand it replaces a loom status strand instead of adding beside it.
// A pawl run's loom bootstrap may have put that strand in the session's one overview window first, and reed refuses a second overview strand.
// It touches no other strand.
// It returns the warning naming a failure, empty when there is none, and never fails `start`: the overview is the operator's view, not the orchestrator's bookkeeping.
func (c *orchCLI) ensureRunsStrand() string {
	build, err := c.currentBuild()
	if err != nil {
		return runsStrandWarning("could not identify the running lym build, so the runs strand was left as it is", err)
	}
	exe, err := c.executable()
	if err != nil {
		return runsStrandWarning("could not locate the running lym binary, so the runs strand was left as it is", err)
	}
	strands, err := c.strands.Strands()
	if err != nil {
		return runsStrandWarning("could not read the strands to ensure the runs strand", err)
	}

	sidecarPath := filepath.Join(c.paths.Dir, runsSidecarFile)
	spec := runsStrandAddSpec(runsStrandCmd(shell.ForGOOS(), exe), c.runsWindowTitle)

	var runs, loomStatus *reedengine.StrandStatus
	for i := range strands {
		switch {
		case runs == nil && agentname.Matches(strands[i].Name, RunsStrandName):
			runs = &strands[i]
		case loomStatus == nil && agentname.Matches(strands[i].Name, loomengine.LoomStatusStrandName):
			loomStatus = &strands[i]
		}
	}

	var strand reedengine.Strand
	switch {
	case runs != nil && statuspane.StrandCurrent(runs.Live, runs.GUID, statuspane.ReadSidecar(sidecarPath), build):
		return ""
	case runs != nil:
		if strand, err = c.strands.ReplaceStrand(runs.GUID, spec); err != nil {
			return runsStrandWarning("could not replace the runs strand, so the beam overview is missing or stale", err)
		}
	case loomStatus != nil:
		if strand, err = c.strands.ReplaceStrand(loomStatus.GUID, spec); err != nil {
			return runsStrandWarning("could not take the overview window from the loom status strand, so the beam overview is missing", err)
		}
	default:
		if strand, err = c.strands.AddStrand(spec); err != nil {
			return runsStrandWarning("could not add the runs strand, so the beam overview is missing", err)
		}
	}

	if err := statuspane.WriteSidecar(sidecarPath, strand.GUID, build); err != nil {
		return runsStrandWarning("could not record the runs strand's build, so the next start replaces the strand", err)
	}
	return ""
}

// runsStrandWarning logs what failed with its cause and returns the report warning naming both and `lym beam start`.
func runsStrandWarning(what string, cause error) string {
	logger.Warn("orch: "+what, "cause", cause)
	return fmt.Sprintf("%s (%v); way forward: fix the cause it names, then run `lym beam start`", what, cause)
}

// runsStrandCmd composes the runs strand's pane command line through the shell seam: exe invoked with the two-word overview verb.
func runsStrandCmd(sh shell.Shell, exe string) string {
	return sh.Invoke(exe) + " " + sh.Quote("batten") + " " + sh.Quote("overview")
}
