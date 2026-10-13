// checklist.go implements the hidden `checklist` loom verb, the pane process of the pair's loom-status strand, and renderChecklist, the pure frame it draws:
// the run's state line, with what the run waits on, over one line per recipe row with its box, the time spent in it and its visit count.

package loomcli

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/statuspane"
)

const (
	checklistLabel          = "loom"
	checklistRecipeBroken   = "recipe unreadable"
	checklistNotInRecipe    = "(not in recipe)"
	checklistBoxDone        = "[x]"
	checklistBoxRunning     = "[>]"
	checklistBoxFailed      = "[!]"
	checklistBoxHalted      = "[-]"
	checklistBoxNotReached  = "[ ]"
	checklistMoreRowsFormat = "… %d more rows not shown"
)

// checklistVisits is the time and visit count the history gives one row.
type checklistVisits struct {
	spent  time.Duration
	visits int
}

// checklistCmd builds the hidden `checklist` subcommand: the loom-status strand's pane process.
// It reads files only; its one write is the content-rows report, which reed confines to the strand that runs it.
func (c *loomCLI) checklistCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "checklist",
		Short:       "draw this worktree's gate checklist in a pane (run by the loom-status strand)",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `checklist draws the run's state line and one line per recipe row with its
box, the time spent in it and its visit count, redrawing in place until it is
signalled. The loom-status strand runs it, and it reports its line count to
reed so the strand is sized to it. It reads files only.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			cwd, err := lyxcwd.CwdFrom(cmd.Context())
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			location, err := lyxcwd.Resolve(cwd)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			reedCfg, err := reedengine.LoadConfig(fabricengine.ConfigDirs(location), "reed")
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			reedGeom, err := hubgeom.ReedGeometry(location)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			engine := reedengine.New(reedCfg, reedGeom)

			pane := statuspane.Pane{
				Out:     out,
				Size:    statuspane.TerminalSize(os.Stdout),
				Report:  statuspane.ReportFromEnv(os.Getenv, engine.SetContentRows),
				Resized: statuspane.NotifyResize(cmd.Context()),
			}
			pane.Run(cmd.Context(), c.checklistPoll(location))
			return nil
		},
	}
}

// checklistPoll returns the pane's poll for the worktree at location.
// The self run's seed is read until one read succeeds, and its recipe's rows once after that.
// While the run is running, the state line carries the note the worktree's Waiting hook reports.
func (c *loomCLI) checklistPoll(location *lyxcwd.Location) func() statuspane.Frame {
	waiting := reviewWaitingFor(location)
	var (
		seedRead  bool
		rows      []string
		rowsErr   error
		startedAt string
	)
	cache := statuspane.NewStatusCache()
	statusFile := shedrun.StatusFile(location, shedrun.SelfRunID)

	return func() statuspane.Frame {
		if !seedRead {
			seed, found, err := shedrun.ReadSeed(location, shedrun.SelfRunID)
			switch {
			case err != nil:
				rowsErr = err
			case !found:
				rowsErr = errors.New("no seed is recorded")
			default:
				seedRead = true
				startedAt = seed.StartedAt
				rows, rowsErr = loomrecipe.RowNames(seed.Recipe)
			}
		}
		poll := cache.Poll(statusFile)
		var waitNote string
		// A hook error drops the note rather than the frame; a settled run waits on nothing.
		if waiting != nil && poll.Found && poll.Status.State == shedengine.StateRunning {
			if note, err := waiting(poll.Status); err == nil {
				waitNote = note
			}
		}
		return renderChecklist(rows, rowsErr, poll, waitNote, startedAt, time.Now())
	}
}

// renderChecklist returns the checklist frame: the state line, then one line per row of the recipe in recipe order, then one per history-only row.
// A non-empty waitNote is appended to the state line as " | waiting <note>".
// With rowsErr set the only line after the state line is checklistRecipeBroken.
// startedAt is the run's seed time, where its first visit starts; an empty or unparsable value starts it at the first entry's own time.
func renderChecklist(rows []string, rowsErr error, poll statuspane.StatusPoll, waitNote, startedAt string, now time.Time) statuspane.Frame {
	stateLine := shedverbs.UnavailableLine(checklistLabel)
	if poll.Found {
		stateLine = shedverbs.RenderStatusLine(checklistLabel, poll.Status)
		if waitNote != "" {
			stateLine += " | waiting " + waitNote
		}
	}
	if rowsErr != nil {
		return statuspane.Frame{Lines: []string{stateLine, checklistRecipeBroken}}
	}

	var status shedengine.Status
	if poll.Found {
		status = poll.Status
	}
	visits, visitedOrder, lastVisited := checklistVisitsOf(status, startedAt, now)

	inRecipe := make(map[string]bool, len(rows))
	for _, name := range rows {
		inRecipe[name] = true
	}
	ordered := append([]string(nil), rows...)
	for _, name := range visitedOrder {
		if !inRecipe[name] {
			ordered = append(ordered, name)
		}
	}

	rowLines := make([]string, 0, len(ordered))
	runningRow, lastVisitedRow := -1, 0
	for i, name := range ordered {
		box := checklistBox(name, visits[name], status)
		if box == checklistBoxRunning {
			runningRow = i
		}
		if name == lastVisited {
			lastVisitedRow = i
		}
		line := box + " " + name
		if v := visits[name]; v.visits > 0 {
			line += " " + statuspane.FormatMinutes(v.spent)
			if v.visits > 1 {
				line += fmt.Sprintf(" x%d", v.visits)
			}
		}
		if !inRecipe[name] {
			line += " " + checklistNotInRecipe
		}
		rowLines = append(rowLines, line)
	}

	anchor := lastVisitedRow
	if runningRow >= 0 {
		anchor = runningRow
	}
	lines := append([]string{stateLine}, rowLines...)
	return statuspane.Frame{
		Lines: lines,
		Fit: func(fitRows int) []string {
			return fitChecklist(stateLine, rowLines, anchor, fitRows)
		},
	}
}

// checklistVisitsOf sums the status history into per-row time and visit counts, in the order the rows were first visited, and names the row of the last visit.
// A visit lasts from the previous entry's end to its own end; a goto entry is no visit and starts the next one at its own time.
// While the run is not done, the current producer's open visit runs from the last entry's end to now.
func checklistVisitsOf(status shedengine.Status, startedAt string, now time.Time) (visits map[string]checklistVisits, order []string, lastVisited string) {
	visits = map[string]checklistVisits{}
	visit := func(name string, spent time.Duration, count int) {
		v, seen := visits[name]
		if !seen {
			order = append(order, name)
		}
		v.spent += max(spent, 0)
		v.visits += count
		visits[name] = v
		lastVisited = name
	}

	prevEnd, prevKnown := parseChecklistTime(startedAt)
	for _, entry := range status.History {
		at, atKnown := parseChecklistTime(entry.At)
		if entry.Outcome == shedengine.OutcomeGoto {
			if atKnown {
				prevEnd, prevKnown = at, true
			}
			continue
		}
		end, endKnown := at, atKnown
		if entry.LastAt != "" {
			if lastAt, ok := parseChecklistTime(entry.LastAt); ok {
				end, endKnown = lastAt, true
			}
		}
		if !prevKnown && atKnown {
			prevEnd, prevKnown = at, true
		}
		var spent time.Duration
		if prevKnown && endKnown {
			spent = end.Sub(prevEnd)
		}
		visit(entry.Producer, spent, 1+entry.Repeats)
		if endKnown {
			prevEnd, prevKnown = end, true
		}
	}

	if status.CurrentProducer != "" && status.State != shedengine.StateDone {
		var spent time.Duration
		if prevKnown {
			spent = now.Sub(prevEnd)
		}
		visit(status.CurrentProducer, spent, 1)
	}
	return visits, order, lastVisited
}

// parseChecklistTime parses an RFC 3339 time and reports whether value held one.
func parseChecklistTime(value string) (time.Time, bool) {
	parsed, err := time.Parse(time.RFC3339, value)
	return parsed, err == nil
}

// checklistBox is the box of the row name: the current producer's by the run's state, otherwise done when the row has a visit and not reached when it has none.
func checklistBox(name string, v checklistVisits, status shedengine.Status) string {
	if name == status.CurrentProducer {
		switch status.State {
		case shedengine.StateRunning:
			return checklistBoxRunning
		case shedengine.StateFailed:
			return checklistBoxFailed
		case shedengine.StatePaused, shedengine.StateBlocked, shedengine.StateAwaiting:
			return checklistBoxHalted
		}
	}
	if v.visits > 0 {
		return checklistBoxDone
	}
	return checklistBoxNotReached
}

// fitChecklist returns at most rows lines: the state line, the rows nearest the anchor row in their own order, and a closing line counting the rows left out.
// A tie in distance keeps the later row.
func fitChecklist(stateLine string, rowLines []string, anchor, rows int) []string {
	lines := append([]string{stateLine}, rowLines...)
	if rows >= len(lines) {
		return lines
	}
	switch {
	case rows <= 1:
		return lines[:1]
	case rows == 2:
		return []string{stateLine, rowLines[anchor]}
	}

	byDistance := make([]int, len(rowLines))
	for i := range byDistance {
		byDistance[i] = i
	}
	distance := func(i int) int {
		if i < anchor {
			return anchor - i
		}
		return i - anchor
	}
	sort.SliceStable(byDistance, func(a, b int) bool {
		da, db := distance(byDistance[a]), distance(byDistance[b])
		if da != db {
			return da < db
		}
		return byDistance[a] > byDistance[b]
	})

	kept := byDistance[:rows-2]
	sort.Ints(kept)
	fitted := []string{stateLine}
	for _, i := range kept {
		fitted = append(fitted, rowLines[i])
	}
	return append(fitted, fmt.Sprintf(checklistMoreRowsFormat, len(rowLines)-len(kept)))
}
