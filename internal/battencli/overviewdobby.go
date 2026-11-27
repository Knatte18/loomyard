// overviewdobby.go renders a worktree's dobby as the lines a beam's window draws above its orchs,
// and `lym beam status` prints the same lines.

package battencli

import (
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/durfmt"
	"github.com/Knatte18/loomyard/internal/logger"
)

const (
	// dobbyIndent opens every line under the dobby's header.
	dobbyIndent = "  "
	// dobbyNoRuns is the line of a dobby that drives no run.
	dobbyNoRuns = "no runs driven"
	// dobbyNotifyOnLand ends the line of a run or task whose board entry asks for a landing notice.
	dobbyNotifyOnLand = "notify_on_land"
	// dobbyUnreadableFormat is the one line of a dobby whose view does not read, filled with its label.
	dobbyUnreadableFormat = "dobby %s: unreadable"
)

// DobbyLines renders the dobby labeled label as lines at now.
// It takes the dobby's state from view and the cells of its runs from runCells.
// The header gives max_runs and land_notice while the dobby is alive, else says it is not running and how to start it.
// Each run it drives takes one line, with its cells and its last pass.
// Each released task it has not started takes one line, naming everything that holds it back.
// A run or task whose board entry asks for a landing notice ends its line with notify_on_land.
// A run whose cells are unreadable draws `status unreadable` in their place and takes no part in their column widths.
// A duration still growing renders in whole minutes, so the lines change at most once a minute.
func DobbyLines(label string, view dobbyengine.View, runCells map[string]RunCells, now time.Time) []string {
	header := fmt.Sprintf("dobby %s: alive, max_runs %d, land_notice %s", label, view.MaxRuns, view.LandNotice)
	if !view.Alive {
		header = fmt.Sprintf("dobby %s: not running; way forward: lym beam start", label)
	}

	slugWidth := 0
	for _, run := range view.Runs {
		slugWidth = max(slugWidth, len([]rune(run.Slug)))
	}
	for _, task := range view.Waiting {
		slugWidth = max(slugWidth, len([]rune(task.Slug)))
	}

	lines := []string{header}
	if len(view.Runs) == 0 {
		lines = append(lines, dobbyIndent+dobbyNoRuns)
	}
	stateWidth, stageWidth, inStageWidth := 0, 0, 0
	for _, run := range view.Runs {
		cells := runCells[run.Slug]
		if cells.Unreadable {
			continue
		}
		stateWidth = max(stateWidth, len([]rune(cells.State)))
		stageWidth = max(stageWidth, len([]rune(cells.Stage)))
		inStageWidth = max(inStageWidth, len([]rune(cells.InStage)))
	}
	for _, run := range view.Runs {
		cells := runCells[run.Slug]
		drawn := []string{padRight(run.Slug, slugWidth)}
		if cells.Unreadable {
			drawn = append(drawn, overviewStatusBroken)
		} else {
			drawn = append(drawn, padRight(cells.State, stateWidth), padRight(cells.Stage, stageWidth), padRight(cells.InStage, inStageWidth))
		}
		drawn = append(drawn, dobbyRunPass(run, now))
		lines = append(lines, dobbyIndent+strings.Join(drawn, overviewColumnGap))
	}
	for _, task := range view.Waiting {
		wait := dobbyTaskWait(task, view.MaxRuns, now)
		if task.NotifyOnLand {
			wait += ", " + dobbyNotifyOnLand
		}
		lines = append(lines, dobbyIndent+padRight(task.Slug, slugWidth)+overviewColumnGap+wait)
	}
	return lines
}

// overviewDobbyLines reads the dobby through reader and renders its lines at now, with each run's cells from cells.
// A dobby whose view does not read renders the one unreadable line instead.
func overviewDobbyLines(reader *DobbyViewReader, cells *DobbyRunCells, now time.Time) []string {
	view, err := reader.Read(now)
	if err != nil {
		logger.Debug("batten: reading the dobby for the overview failed", "dobby", reader.Label(), "err", err)
		return []string{fmt.Sprintf(dobbyUnreadableFormat, reader.Label())}
	}
	return DobbyLines(reader.Label(), view, cells.Of(view.Runs, now), now)
}

// dobbyRunPass renders a run's pass cell: a pass in flight, or the minutes since its last pass exited, or no pass yet.
// The cell then adds what the dobby's record says of the run.
func dobbyRunPass(run dobbyengine.ViewRun, now time.Time) string {
	pass := "no pass yet"
	switch {
	case run.PassInFlight:
		pass = "pass in flight"
	case !run.LastPassExit.IsZero():
		pass = "last pass " + durfmt.FormatMinutes(now.Sub(run.LastPassExit)) + " ago"
	}
	notes := []string{pass}
	if run.FullPassOwed {
		notes = append(notes, "full pass owed")
	}
	if run.OwnedElsewhere {
		notes = append(notes, "owned by another machine")
	}
	if run.FailingPassCount > 0 {
		notes = append(notes, fmt.Sprintf("failing %d× in a row: %s", run.FailingPassCount, firstLine(run.FailingPassError)))
	}
	if run.NotifyOnLand {
		notes = append(notes, dobbyNotifyOnLand)
	}
	return strings.Join(notes, ", ")
}

// dobbyTaskWait renders what a released task waits on, or that nothing holds it back.
// The last refused start is shown when one is recorded.
func dobbyTaskWait(task dobbyengine.WaitingTask, maxRuns int, now time.Time) string {
	if task.Ready() {
		if task.Refusal != "" {
			return "ready to start, last refused start: " + firstLine(task.Refusal)
		}
		return "ready to start"
	}
	var reasons []string
	if task.ReadyHold {
		reasons = append(reasons, "the beam is flagged ready: lym batten cancel "+task.Slug+" --remove")
	}
	if task.FrontFirst != "" {
		reasons = append(reasons, "front task "+task.FrontFirst+" runs first")
	}
	if len(task.Dependencies) > 0 {
		reasons = append(reasons, "dependencies "+strings.Join(task.Dependencies, ", "))
	}
	if task.OwnedElsewhere {
		reasons = append(reasons, "another machine's ownership")
	}
	if task.Paused {
		reasons = append(reasons, "a pause")
	}
	if task.StartRefusal != "" {
		reasons = append(reasons, "a start batten refuses: "+firstLine(task.StartRefusal))
	}
	if !task.NextTry.IsZero() {
		reasons = append(reasons, fmt.Sprintf("a refused start (%s), retry in %s", firstLine(task.Refusal), durfmt.FormatMinutes(task.NextTry.Sub(now))))
	}
	if task.AtMaxRuns {
		reasons = append(reasons, fmt.Sprintf("max_runs %d", maxRuns))
	}
	return "waits on " + strings.Join(reasons, "; ")
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return line
}
