// runs.go ensures the prime's `runs` strand: the session's overview window, running `lyx batten overview`, which `start` brings up before the orchestrator strand.

package orchcli

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/statuspane"
)

// runsStrandName is the role the runs strand is added and addressed by in the prime's reed session.
const runsStrandName = "runs"

// runsSidecarFile is the filename, under the orch directory, of the sidecar recording the runs strand's guid and the lyx build that launched it.
const runsSidecarFile = "runs-strand.json"

// runsStrandAddSpec builds the runs strand's reedengine.AddSpec for the given pane command: the session's overview window, with no size, no segment and no color.
func runsStrandAddSpec(cmd string) reedengine.AddSpec {
	return reedengine.AddSpec{
		NameOverride: runsStrandName,
		Cmd:          cmd,
		Display:      render.Display{Anchor: render.AnchorOverview},
	}
}

// ensureRunsStrand adds the runs strand when the session has none, keeps a live one that the sidecar records as launched by this build, and replaces any other, then records the strand and the build in the sidecar.
// It touches no strand but the one named runs.
// Every failure is a Warn and never fails `start`: the overview is the operator's view, not the orchestrator's bookkeeping.
func (c *orchCLI) ensureRunsStrand() {
	build, err := c.currentBuild()
	if err != nil {
		logger.Warn("orch: could not identify the running lyx build; the runs strand is left as it is", "cause", err)
		return
	}
	exe, err := c.executable()
	if err != nil {
		logger.Warn("orch: could not locate the running lyx binary; the runs strand is left as it is", "cause", err)
		return
	}
	strands, err := c.strands.Strands()
	if err != nil {
		logger.Warn("orch: could not read the strands to ensure the runs strand", "cause", err)
		return
	}

	sidecarPath := filepath.Join(c.paths.Dir, runsSidecarFile)
	spec := runsStrandAddSpec(runsStrandCmd(shell.ForGOOS(), exe))

	var (
		strand reedengine.Strand
		found  bool
	)
	for _, existing := range strands {
		if !agentname.Matches(existing.Name, runsStrandName) {
			continue
		}
		if statuspane.StrandCurrent(existing.Live, existing.GUID, statuspane.ReadSidecar(sidecarPath), build) {
			return
		}
		strand, err = c.strands.ReplaceStrand(existing.GUID, spec)
		if err != nil {
			logger.Warn("orch: could not replace the runs strand; the overview will be missing or stale", "guid", existing.GUID, "cause", err)
			return
		}
		found = true
		break
	}
	if !found {
		strand, err = c.strands.AddStrand(spec)
		if err != nil {
			logger.Warn("orch: could not add the runs strand; the overview will be missing", "cause", err)
			return
		}
	}

	if err := statuspane.WriteSidecar(sidecarPath, strand.GUID, build); err != nil {
		logger.Warn("orch: could not record the runs strand's build; the next start will replace it", "cause", err)
	}
}

// runsStrandCmd composes the runs strand's pane command line through the shell seam: exe invoked with the two-word overview verb.
func runsStrandCmd(sh shell.Shell, exe string) string {
	return sh.Invoke(exe) + " " + sh.Quote("batten") + " " + sh.Quote("overview")
}
