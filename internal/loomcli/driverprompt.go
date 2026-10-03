// driverprompt.go implements the ly-drive session's launch prompt: a short pointer composed over the
// run-id and the drive report path. It lives beside bootstrap.go per the
// pure-decisions-live-in-bootstrap-go Shared Decision.

package loomcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/agentname"
)

// driverPrompt composes the ly-drive session's launch prompt: a short pointer, never a copy of the
// skill. It names the ly-drive skill invocation and the plugin it ships in, the run-id, the report
// path the session must write at every stop condition, and an explicit statement that this session
// runs in autonomous mode with no operator to ask.
// At a done run or a busy refusal, as its last act after writing its stop report, the session runs driverTeardownCommand to end its own strand;
// at every other stop it parks as the skill's parking section describes, leaving the session open for `lyx loom start` to resume by typing one line.
// The park and the command ride this loom-launched prompt alone,
// so a session launched without it (an orchestrator fork, an operator-launched ly-drive) never parks and stays open.
// The teardown at done is what keeps batten's half of the end-to-end criterion:
// batten's Inner-Run row, on a done child, waits for the driver strand to end before it returns Done and teardown runs (callDone),
// and that command is what ends the strand.
// A session without the skill is told to stop rather than search for a copy: a filesystem search
// finds whichever checkout happens to exist, so the run would follow an arbitrary version of the
// skill's contract.
//
// It is kept short on purpose: a prompt that grew into a copy of the skill would go stale against the skill it copies.
// This file composes prompt text alone and names no Claude flag and no command line: the Shuttle Provider-Seam Invariant keeps provider specifics under the claude engine package.
func driverPrompt(runID string, reportPath string) string {
	return fmt.Sprintf(
		"Run the ly-drive skill (from loomyard's ly plugin) for run-id %q. If that skill is not available to you, do not search the filesystem for a copy, which may be a stale version: write to the report that the ly plugin is not installed and stop. You are running autonomously, with no operator to ask -- decide and proceed on your own judgment. Write your report to %q at every stop condition. At every stop other than done, after writing the report, if the environment variable %s is set, send one short SendMessage to that name naming the run-id and the report path, and treat a failed send as no send. When the run is done, or a step is refused as busy, then after writing it, as your very last act, commit the run records and end your own session by running: %s . At every other stop, park as the skill describes (park command: %s ) and leave this session open for lyx loom start to resume.",
		runID, reportPath, agentname.ParentEnv, driverTeardownCommand, driverParkCommand(reportPath),
	)
}

// driverRecordsCommand commits the run records; driverTeardownCommand runs it first, and driverParkCommand extends it.
const driverRecordsCommand = "lyx loom commit-records"

// driverParkCommand is the command a parking driver runs after its stop report: it commits the run records and writes the park marker holding reportPath.
// The marker is written by Go rather than by the agent, since a driver that skipped writing it left `lyx loom start` refusing to resume.
// The prompt names it so the ly-drive skill can stay recipe-blind.
func driverParkCommand(reportPath string) string {
	return fmt.Sprintf("%s --park %q", driverRecordsCommand, reportPath)
}

// driverTeardownCommand is the end-of-session command a loom-launched driver runs after its stop report.
// It commits the run records first, then removes the driver's own strand.
// The two are joined with `;` rather than `&&` so the strand removal runs even when the commit fails:
// a driver that stayed open on a failed commit would leave its strand alive for a run that has finished.
// It is built from driverStrandDisplayName so the name cannot drift from the strand's own.
const driverTeardownCommand = driverRecordsCommand + "; lyx reed remove --name " + driverStrandDisplayName + " --detach"
