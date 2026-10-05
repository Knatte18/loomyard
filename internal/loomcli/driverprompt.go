// driverprompt.go implements the driver session's launch prompt: the shed driver stencil filled over the
// run-id, the drive report path, the park and teardown commands and the parent directive. It lives beside bootstrap.go per the
// pure-decisions-live-in-bootstrap-go Shared Decision.

package loomcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// driverStencilName is the registered name of the shed driver stencil.
const driverStencilName = "shed-template-driver"

// driverPrompt composes the driver session's launch prompt: the shed driver stencil, read from stencilsDir through stencilstore.Read and filled with the run-id, the report path the session writes at every stop,
// driverParkCommand(reportPath), driverTeardownCommand and parentdirective.Directive(stencilsDir, parentName, false).
// The prompt is the driver's whole procedure, so the session depends on no installed skill.
// At a done run or a busy refusal, as its last act after writing its stop report, the session runs driverTeardownCommand to end its own strand;
// at every other stop it parks, leaving the session open for `lyx loom start` to resume by typing one line.
// The park and the command ride this loom-launched prompt alone, as markers the recipe-blind stencil body never names.
// The teardown at done is what keeps batten's half of the end-to-end criterion:
// batten's Inner-Run row, on a done child, waits for the driver strand to end before it returns Done and teardown runs (callDone),
// and that command is what ends the strand.
// It returns the read or fill error when the stencil or the directive cannot be rendered.
// This file composes prompt text alone and names no Claude flag and no command line: the Shuttle Provider-Seam Invariant keeps provider specifics under the claude engine package.
func driverPrompt(stencilsDir, parentName, runID, reportPath string) (string, error) {
	directive, err := parentdirective.Directive(stencilsDir, parentName, false)
	if err != nil {
		return "", err
	}
	template, err := stencilstore.Read(stencilsDir, driverStencilName)
	if err != nil {
		return "", fmt.Errorf("loom: driver prompt: %w", err)
	}
	filled, err := stencil.Fill(template, map[string]string{
		"run_id":                   runID,
		"report_path":              reportPath,
		"park_command":             driverParkCommand(reportPath),
		"teardown_command":         driverTeardownCommand,
		parentdirective.MarkerName: directive,
	})
	if err != nil {
		return "", fmt.Errorf("loom: driver prompt: fill stencil %q: %w", driverStencilName, err)
	}
	return string(filled), nil
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
