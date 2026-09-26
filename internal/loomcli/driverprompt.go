// driverprompt.go implements the ly-drive session's launch prompt: a short pointer composed over the
// run-id and the drive report path. It lives beside bootstrap.go per the
// pure-decisions-live-in-bootstrap-go Shared Decision.

package loomcli

import "fmt"

// driverPrompt composes the ly-drive session's launch prompt: a short pointer, never a copy of the
// skill. It names the ly-drive skill invocation, the run-id, the report path the session must write
// at every stop condition, and an explicit statement that this session runs in autonomous mode with
// no operator to ask.
//
// It is kept short on purpose: the Claude engine caps a prompt at maxLaunchPromptBytes (declared in
// internal/shuttleengine/claudeengine/command.go, enforced in claudeengine.go), because the whole
// prompt expands into one command-line argument. A prompt that grew into a copy of the skill would
// fail only at launch, after a bootstrap has already seeded and committed. This file composes prompt
// text alone and names no Claude flag and no command line: the Shuttle Provider-Seam Invariant keeps
// provider specifics under the claude engine package.
func driverPrompt(runID string, reportPath string) string {
	return fmt.Sprintf(
		"Run the ly-drive skill (from loomyard's ly plugin) for run-id %q. You are running autonomously, with no operator to ask -- decide and proceed on your own judgment. Write your report to %q at every stop condition (task done, or a failure you cannot repair).",
		runID, reportPath,
	)
}
