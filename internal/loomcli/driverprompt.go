// driverprompt.go implements the ly-drive session's launch prompt: a short pointer composed over the
// run-id and the drive report path, and the exported autonomous step cap the prompt carries. It lives
// beside bootstrap.go per the pure-decisions-live-in-bootstrap-go Shared Decision.

package loomcli

import "fmt"

// AutonomousDriveStepCap is the number of steps an autonomous ly-drive session runs under.
//
// It is exported deliberately: batch 6 pins the same number in the ly-drive skill's own autonomous
// section, no package under the plugins tree compiles Go, and a test outside this package cannot see
// an unexported identifier -- so this constant is the single source both the prompt below
// interpolates and that skill's own test reads.
//
// Its value is derived, not a bare magic number: it is loom's own worst case as the skill already
// computes it -- the longest legitimate sequence of "lyx shed step" calls one run can need -- plus a
// margin that keeps an unlucky-but-legitimate run inside the budget rather than cut off mid-task.
const AutonomousDriveStepCap = 120

// driverPrompt composes the ly-drive session's launch prompt: a short pointer, never a copy of the
// skill. It names the ly-drive skill invocation, the run-id, the report path the session must write
// at every stop condition, and an explicit statement that this session runs in autonomous mode with
// no operator to ask, carrying AutonomousDriveStepCap as the number of steps it runs under.
//
// It is kept short on purpose: the Claude engine caps a prompt at maxLaunchPromptBytes (declared in
// internal/shuttleengine/claudeengine/command.go, enforced in claudeengine.go), because the whole
// prompt expands into one command-line argument. A prompt that grew into a copy of the skill would
// fail only at launch, after a bootstrap has already seeded and committed. This file composes prompt
// text alone and names no Claude flag and no command line: the Shuttle Provider-Seam Invariant keeps
// provider specifics under the claude engine package.
func driverPrompt(runID string, reportPath string) string {
	return fmt.Sprintf(
		"Run the ly-drive skill for run-id %q. You are running autonomously, with no operator to ask -- decide and proceed on your own judgment. You run under a step cap of %d steps. Write your report to %q at every stop condition (task done, step cap reached, or a blocking failure).",
		runID, AutonomousDriveStepCap, reportPath,
	)
}
