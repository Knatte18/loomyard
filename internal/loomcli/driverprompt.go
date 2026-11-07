// driverprompt.go implements the driver session's launch prompt.
// The shed driver stencil is filled with the run-id and the drive report path.
// The park and teardown commands and the parent directive fill its remaining markers.
// It lives beside bootstrap.go, where the package's pure decisions live.

package loomcli

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// driverStencilName is the registered name of the shed driver stencil.
const driverStencilName = "shed-template-driver"

// driverGuideStencilName is the registered name of the repair guide stencil the driver's one-shot fork reads.
const driverGuideStencilName = "shed-template-driver-guide"

// driverGuideMarker is the driver stencil's marker for the deployed path of the repair guide.
const driverGuideMarker = "guide_path"

// driverNotifyStencilName and driverNotifyWatchedStencilName are the registered names of the parent-notification rules.
// driverPrompt fills the driver stencil's parent_notify marker with the watched rule when a batten watches the run.
// Otherwise it fills the plain rule.
const (
	driverNotifyStencilName        = "shed-template-driver-notify"
	driverNotifyWatchedStencilName = "shed-template-driver-notify-watched"
)

// driverNotifyMarker is the driver stencil's marker for the parent-notification rule.
const driverNotifyMarker = "parent_notify"

// driverPrompt composes the driver session's launch prompt from the shed driver stencil, read from stencilsDir.
// The stencil takes the run-id and the report path the session writes at every stop.
// It also takes the park, arbiter and teardown commands and the parent and edit directives.
// The parent-notification marker takes the watched rule when watched is true, and the plain rule otherwise.
// The guide marker takes the deployed path of the repair guide stencil.
// The fork a driver spawns at a failure stop reads that copy.
// The prompt is the driver's whole procedure, so the session depends on no installed skill.
// At a done run or a busy refusal, the session's last act is the teardown command, after its stop report.
// That command ends the session's own strand.
// At every other stop it parks, leaving the session open for `lym loom start` to resume by typing one line.
// The park and teardown commands reach the driver through this prompt alone.
// They are markers that the recipe-blind stencil body never names.
// On a done child, the batten waits for the driver strand to end before it returns Done.
// The teardown command is what ends that strand.
// It returns the read or fill error when the stencil or the directive cannot be rendered.
// This file composes prompt text only and names no provider flag.
// The Shuttle Provider-Seam Invariant keeps provider specifics in the claudeengine package.
func driverPrompt(stencilsDir, parentName, runID, reportPath string, watched bool) (string, error) {
	directive, err := parentdirective.Directive(stencilsDir, parentName, false)
	if err != nil {
		return "", err
	}
	editDirective, err := editdirective.Directive(stencilsDir)
	if err != nil {
		return "", err
	}
	notifyStencilName := driverNotifyStencilName
	if watched {
		notifyStencilName = driverNotifyWatchedStencilName
	}
	notifyTemplate, err := stencilstore.Read(stencilsDir, notifyStencilName)
	if err != nil {
		return "", fmt.Errorf("loom: driver prompt: %w", err)
	}
	notify := strings.TrimRight(stencil.StripLeadingComment(string(notifyTemplate)), "\r\n")
	template, err := stencilstore.Read(stencilsDir, driverStencilName)
	if err != nil {
		return "", fmt.Errorf("loom: driver prompt: %w", err)
	}
	filled, err := stencil.Fill(template, map[string]string{
		"run_id":                   runID,
		"report_path":              reportPath,
		"park_command":             driverParkCommand(reportPath),
		"arbiter_command":          driverArbiterCommand(runID),
		"teardown_command":         driverTeardownCommand,
		parentdirective.MarkerName: directive,
		editdirective.MarkerName:   editDirective,
		driverNotifyMarker:         notify,
		driverGuideMarker:          stencilstore.Path(stencilsDir, driverGuideStencilName),
	})
	if err != nil {
		return "", fmt.Errorf("loom: driver prompt: fill stencil %q: %w", driverStencilName, err)
	}
	return string(filled), nil
}

// driverWatched reports whether a live batten watches the run.
// markerPath names a file whose first line holds a pid in decimal, and isAlive confirms that pid.
// An absent or unreadable marker, one that does not parse as a pid and one whose pid is dead are all unwatched,
// so the marker a batten left behind when it died never silences a fresh driver.
func driverWatched(markerPath string, isAlive func(pid int) bool) bool {
	_, watched := liveBattenPID(markerPath, isAlive)
	return watched
}

// liveBattenPID returns the pid that the marker at markerPath names, and true when isAlive confirms it.
// It returns false when the marker is absent, unreadable, unparseable, or its pid is dead.
func liveBattenPID(markerPath string, isAlive func(pid int) bool) (int, bool) {
	watch, alive := readBattenWatch(markerPath, isAlive)
	if !alive {
		return 0, false
	}
	return watch.PID, true
}

// battenWatch is a batten-watched marker's reading: the batten's pid and, when it recorded them, the prime's batten scratch directory and the Run-Shed row's name.
type battenWatch struct {
	PID        int
	ScratchDir string
	Producer   string
}

// readBattenWatch parses the marker at markerPath and returns what it holds whenever it parses.
// The bool is true only when isAlive confirms the pid.
// An unwatched driver still learns the prime a dead batten named.
// An absent, unreadable or unparseable marker returns the zero battenWatch.
func readBattenWatch(markerPath string, isAlive func(pid int) bool) (battenWatch, bool) {
	data, err := os.ReadFile(markerPath)
	if err != nil {
		return battenWatch{}, false
	}
	pid, scratchDir, producer, err := shedrun.ParseBattenWatched(data)
	if err != nil {
		return battenWatch{}, false
	}
	return battenWatch{PID: pid, ScratchDir: scratchDir, Producer: producer}, isAlive(pid)
}

// driverRecordsCommand is the command that commits the run records; the park and teardown commands both start with it.
const driverRecordsCommand = "lym loom commit-records"

// driverParkCommand is the command a parking driver runs after its stop report: it commits the run records and writes the park marker holding reportPath.
// The marker is written by Go rather than by the agent, since a driver that skipped writing it left `lym loom start` refusing to resume.
// The prompt names it so the driver stencil can stay recipe-blind.
func driverParkCommand(reportPath string) string {
	return fmt.Sprintf("%s --park %q", driverRecordsCommand, reportPath)
}

// driverArbiterCommand is the command a driver runs at a stop it cannot settle: it spawns the run's arbiter, or returns the verdict of one that already judged the stop.
// The envelope and report paths are angle-bracket placeholders that the driver fills from the stop it holds.
func driverArbiterCommand(runID string) string {
	return fmt.Sprintf("lym loom arbiter %s --envelope <envelope_path> --report <report_path>", runID)
}

// driverTeardownCommand is the end-of-session command a loom-launched driver runs after its stop report.
// It commits the run records first, then removes the driver's own strand.
// The two are joined with `;` rather than `&&` so the strand removal runs even when the commit fails:
// a driver that stayed open on a failed commit would leave its strand alive for a run that has finished.
// It is built from `driverStrandDisplayName`, so the name cannot drift from the strand's own.
const driverTeardownCommand = driverRecordsCommand + "; lym reed remove --name " + driverStrandDisplayName + " --detach"
