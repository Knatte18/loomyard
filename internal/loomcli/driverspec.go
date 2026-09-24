// driverspec.go implements driverSpec, the pure composer building the shuttleengine.Spec the ly-drive
// session launches from. It lives beside bootstrap.go per the pure-decisions-live-in-bootstrap-go
// Shared Decision.

package loomcli

import (
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// driverSpec composes the shuttleengine.Spec the ly-drive session launches from, from an already-
// composed prompt, an already-composed report path, and the resolved driver-role settings.
//
// Every non-default field is pinned here with its own reason, in the shape operatorStrandAddSpec's
// own doc comment already carries -- the standard this package holds.
//
// Prompt is the argument verbatim. OutputFiles is a single-entry slice holding reportPath: the file
// contract Spec enforces treats a run's output file as its return value, and this run has exactly
// one. Model, Effort, and Version come from settings, which carries the RESOLVED triple -- a provider
// model id plus its effort and version, never a raw config alias -- since only the resolved values
// mean anything to the engine that reads them. NameOverride is driverStrandDisplayName: the constant
// the next bootstrap's lookup uses, and reed's add has no upsert semantics, so add and lookup must
// agree on this exact byte-stable literal.
//
// Interactive is false, which is shuttle's autonomous posture: it is what adds the
// --dangerously-skip-permissions flag and the operator-prompt deny an unattended session needs.
// ForkSubagents is false, because the driving loop reads envelopes and invokes a CLI and has no
// research fan-out to delegate. Role is the literal "driver", which is what the run directory
// records and what makes a driver run distinguishable from a producer round. Round is empty, because
// a driver is not one round of anything. Parent is empty, because the driver is top-level and the
// panes loom's producers spawn while it runs are its siblings.
//
// Display.Anchor is below-parent and must never be hidden: the driver is the session an operator
// attaches to watch, and a hidden pane would make a run that may last hours legible only through its
// log file. Display.Focus is false for the reason operatorStrandAddSpec pins it false -- the flag is
// persisted on the strand and re-evaluated on every subsequent add, so a true value would re-capture
// focus on every agent pane the run spawns afterwards. Display.ShrinkWhenWaitingOnChild is false: the
// driver is never itself waiting on a child in the sense that flag models.
//
// Timeout, KeepPane, and AwaitOperator are left at their zero values deliberately. A zero Timeout
// defaults to run_timeout_min in Spec.validate, and on this path it bounds only shuttle's startup step
// inside Start -- the startup window itself is startup_timeout_s -- because Wait is never entered: the
// driver session runs until it stops itself, and this path never waits on it to completion. KeepPane
// and AwaitOperator are read only by Wait, so leaving them at their zero values is a statement that
// nothing on this path reads them, not a choice about pane retention or interactive behavior.
func driverSpec(prompt string, reportPath string, settings loomengine.DriverSettings) shuttleengine.Spec {
	return shuttleengine.Spec{
		Prompt:        prompt,
		OutputFiles:   []string{reportPath},
		Model:         settings.Model,
		Effort:        settings.Effort,
		Version:       settings.Version,
		NameOverride:  driverStrandDisplayName,
		Interactive:   false,
		ForkSubagents: false,
		Role:          "driver",
		Round:         "",
		Parent:        "",
		Display: render.Display{
			Anchor:                   render.AnchorBelowParent,
			Focus:                    false,
			ShrinkWhenWaitingOnChild: false,
		},
	}
}
