// driverspec.go composes the shuttleengine.Spec that the loom driver session launches from.
// It is pure, and it lives beside bootstrap.go.

package loomcli

import (
	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// driverRole is the agent-name role that the driver spawn carries.
const driverRole = "driver"

// driverSpec composes the shuttleengine.Spec that the loom driver session launches from.
// It takes the composed prompt, the composed report path, and the resolved driver-role model choice.
//
// OutputFiles holds only the report path, since the driver run has exactly one output file.
// ModelChoice is the resolved engine, model, effort and version, never a raw config alias.
// Only resolved values mean anything to the engine that reads them.
//
// NameOverride is the explicit driver strand name.
// Reed refuses an add whose explicit role is already held rather than renumbering it.
// The bootstrap removes a dead driver strand before the add, so the role is free when the add arrives.
// The next bootstrap's lookup uses the same constant.
// Reed's add has no upsert, so add and lookup must agree on one byte-stable role.
//
// Interactive is false, because the session runs unattended.
// Shuttle then adds the skip-permissions flag and denies operator prompts.
// ForkSubagents is true: the claude engine permits fork subagents and denies every other type.
// At an error or interrupted stop the driver forks one of itself to read the traces and repair.
// The traces stay out of the driver's own context.
// Requires is background-wake: the driver runs its loop and arbiter as background jobs and wakes on them.
// Role is the literal "driver", which the run directory records.
// It tells a driver run apart from a producer round.
// Round and Parent are empty: a driver is not one round of anything, and it is top-level.
// Segment is the coordinator, since the driver drives the whole run.
//
// Display.Anchor is own-window and is never hidden.
// The operator attaches to this session to watch a run that may last hours.
// A hidden pane would leave only the log file to read.
// Display.Focus is false.
// The flag is persisted on the strand and re-evaluated on every later add.
// True would take focus back on each agent pane the run spawns.
//
// Timeout and KeepPane stay zero, a statement about this path rather than a retention choice.
// A zero Timeout defaults to run_timeout_min and, on this path, bounds only the startup step.
// The startup window is startup_timeout_s.
// The session is never waited on to completion, so no other timeout covers it.
// KeepPane is read only when the session is waited on, so zero says nothing about pane retention.
func driverSpec(prompt string, reportPath string, choice shuttleengine.ModelChoice) shuttleengine.Spec {
	return shuttleengine.Spec{
		Prompt:        prompt,
		ConfigRole:    agentconfig.RoleDriver,
		OutputFiles:   []string{reportPath},
		ModelChoice:   choice,
		NameOverride:  driverStrandDisplayName,
		Interactive:   false,
		ForkSubagents: true,
		Requires:      []shuttleengine.Requirement{shuttleengine.RequirementBackgroundWake},
		Role:          driverRole,
		Segment:       segmentcolor.Coordinator,
		Round:         "",
		Parent:        "",
		Display: render.Display{
			Anchor: render.AnchorOwnWindow,
			Focus:  false,
		},
	}
}
