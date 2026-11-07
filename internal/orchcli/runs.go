// runs.go ensures the `runs` strand, the session's first overview window in the prime or a beam.
// It runs `lym batten overview` and is named `beam: prime` or `<handle>: beam: <slug>`.
// `start` brings it up before the orchestrator strand.

package orchcli

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/statuspane"
)

// RunsStrandName is the role the runs strand is added and addressed by in the reed session of the prime or a beam.
const RunsStrandName = "runs"

// RunsStrandOrder is the runs strand's overview order key, zero, so the beam overview is the session's first window.
const RunsStrandOrder = 0

// runsSidecarFile is the filename of the sidecar, under the orch directory.
// The sidecar records the runs strand's guid and the lym build that launched it.
const runsSidecarFile = "runs-strand.json"

// runsWindowPrefix opens the runs strand's window name, which the beam's slug or the prime's label completes.
const runsWindowPrefix = "beam: "

// runsWindowPrimeLabel completes the runs strand's window name in the prime.
const runsWindowPrimeLabel = "prime"

// runsWindowTitleOf returns the name of the runs strand's window in the worktree named worktree:
// `beam: <worktree>` when it is a beam, `beam: prime` otherwise.
func runsWindowTitleOf(worktree string, beam bool) string {
	if beam {
		return runsWindowPrefix + worktree
	}
	return runsWindowPrefix + runsWindowPrimeLabel
}

// runsWindowTitleIn returns the name of the runs strand's window in the worktree at location.
// In the prime it is `beam: prime`.
// In a beam it is `beam: <slug>`, opened with the beam project's board handle read at the call.
// The handle is written as in `P1: beam: <slug>`, and the name is bare when the board names no handle or cannot be read.
func runsWindowTitleIn(location *lymcwd.Location, beam bool) string {
	title := runsWindowTitleOf(location.WorktreeName, beam)
	if !beam {
		return title
	}
	return boardengine.TitleWithHandle(fabricengine.BoardDir(location.HubPath), location.WorktreeName, title)
}

// runsStrandAddSpec builds the runs strand's reedengine.AddSpec for the given pane command and window title:
// the session's first overview window, with no size, no segment and no color.
func runsStrandAddSpec(cmd, title string) reedengine.AddSpec {
	return reedengine.AddSpec{
		NameOverride: RunsStrandName,
		Cmd:          cmd,
		Display:      render.Display{Anchor: render.AnchorOverview, Order: RunsStrandOrder, Title: title},
	}
}

// ensureRunsStrand adds the runs strand when the session has none.
// It keeps a live one that the sidecar records as launched by this build, and replaces any other.
// It then records the strand and the build in the sidecar.
// It touches no other strand.
// It returns the warning naming a failure, empty when there is none, and never fails `start`:
// the overview is the operator's view, not the orchestrator's bookkeeping.
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
	spec := runsStrandAddSpec(runsStrandCmd(shell.ForGOOS(), exe), c.runsWindowTitle())

	var runs *reedengine.StrandStatus
	for i := range strands {
		if agentname.Matches(strands[i].Name, RunsStrandName) {
			runs = &strands[i]
			break
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

// runsStrandCmd composes the runs strand's pane command line through the shell seam:
// exe invoked with the two-word overview verb.
func runsStrandCmd(sh shell.Shell, exe string) string {
	return sh.Invoke(exe) + " " + sh.Quote("batten") + " " + sh.Quote("overview")
}
