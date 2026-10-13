// checklist.go implements the hidden `checklist` loom verb, the pane process of the pair's loom-status strand, and renderChecklist, the pure frame it draws, top to bottom:
// one detail block per visited row listing its visits, one line per recipe row with its box, the time spent in it and its visit count, the run's state line, and the wait line carrying what the run waits on in full.

package loomcli

import (
	"errors"
	"fmt"
	"os"
	"strings"
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
	checklistLabel         = "loom"
	checklistRecipeBroken  = "recipe unreadable"
	checklistNotInRecipe   = "(not in recipe)"
	checklistBoxDone       = "[x]"
	checklistBoxRunning    = "[>]"
	checklistBoxFailed     = "[!]"
	checklistBoxHalted     = "[-]"
	checklistBoxNotReached = "[ ]"
	checklistWaitNothing   = "waiting on nothing"
	checklistOpenOutcome   = "running"
	checklistUnknownStart  = "--:--"
)

// checklistVisit is one visit of a row, read from one status history entry or from the current producer's open visit.
type checklistVisit struct {
	start        time.Time
	startKnown   bool
	spent        time.Duration
	outcome      string
	output       string
	gateAttempts *int
	folds        int
}

// checklistVisits is the time, visit count and visit log the history gives one row.
type checklistVisits struct {
	spent  time.Duration
	visits int
	log    []checklistVisit
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

// renderChecklist returns the checklist frame, top to bottom:
// one checklistDetailLines block per visited row, recipe rows first in recipe order and history-only rows after;
// one line per row of the recipe in recipe order, then one per history-only row, or the single line checklistRecipeBroken with rowsErr set;
// the state line, without the wait the wait line carries; and the wait line.
// startedAt is the run's seed time, where its first visit starts; an empty or unparsable value starts it at the first entry's own time.
func renderChecklist(rows []string, rowsErr error, poll statuspane.StatusPoll, waitNote, startedAt string, now time.Time) statuspane.Frame {
	stateLine := shedverbs.UnavailableLine(checklistLabel)
	if poll.Found {
		quiet := poll.Status
		quiet.Activity.Wait = ""
		stateLine = shedverbs.RenderStatusLine(checklistLabel, quiet)
	}
	waitLine := checklistWaitLine(poll, waitNote)

	var status shedengine.Status
	if poll.Found {
		status = poll.Status
	}
	visits, visitedOrder := checklistVisitsOf(status, startedAt, now)

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

	var lines []string
	for _, name := range ordered {
		lines = append(lines, checklistDetailLines(name, visits[name])...)
	}

	if rowsErr != nil {
		lines = append(lines, checklistRecipeBroken, stateLine, waitLine)
		return statuspane.Frame{Lines: lines}
	}

	for _, name := range ordered {
		box := checklistBox(name, visits[name], status)
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
		lines = append(lines, line)
	}
	return statuspane.Frame{Lines: append(lines, stateLine, waitLine)}
}

// checklistDetailLines renders one row's detail block: the row name, then one indented line per visit of v.
// A visit line reads "<start> <spent> <outcome>", then " x<n>" for a folded entry, " <output>" when it has one and " gate attempts <n>" when gated.
// A row with no visit has no block.
func checklistDetailLines(name string, v checklistVisits) []string {
	if len(v.log) == 0 {
		return nil
	}
	lines := []string{name}
	for _, visit := range v.log {
		start := checklistUnknownStart
		if visit.startKnown {
			start = visit.start.UTC().Format("15:04")
		}
		line := fmt.Sprintf("  %s %s %s", start, statuspane.FormatMinutes(visit.spent), visit.outcome)
		if visit.folds > 1 {
			line += fmt.Sprintf(" x%d", visit.folds)
		}
		if visit.output != "" {
			line += " " + visit.output
		}
		if visit.gateAttempts != nil {
			line += fmt.Sprintf(" gate attempts %d", *visit.gateAttempts)
		}
		lines = append(lines, line)
	}
	return lines
}

// checklistWaitLine is the frame's last line: the full reason the run waits, as `lyx loom status` gives it.
// A running run with a waitNote waits on it; an awaiting run has handed off to its parent; a blocked or failed run waits on its activity's wait.
// Every other case, an unreadable status included, waits on nothing.
func checklistWaitLine(poll statuspane.StatusPoll, waitNote string) string {
	if !poll.Found {
		return checklistWaitNothing
	}
	status := poll.Status
	switch status.State {
	case shedengine.StateRunning:
		if waitNote != "" {
			return "waiting " + waitNote
		}
	case shedengine.StateAwaiting:
		var line strings.Builder
		line.WriteString("waiting on the parent: escalation from " + status.CurrentProducer)
		if status.ParentNotice != "" {
			line.WriteString(": " + status.ParentNotice)
		}
		if status.Activity.Wait != "" {
			line.WriteString(" | " + status.Activity.Wait)
		}
		return line.String()
	case shedengine.StateBlocked, shedengine.StateFailed:
		if status.Activity.Wait != "" {
			return "wait " + status.Activity.Wait
		}
	}
	return checklistWaitNothing
}

// checklistVisitsOf sums the status history into per-row time, visit counts and visit logs, and lists the rows in the order they were first visited.
// A visit lasts from the previous entry's end to its own end; a goto entry is no visit and starts the next one at its own time.
// While the run is not done, the current producer's open visit runs from the last entry's end to now.
func checklistVisitsOf(status shedengine.Status, startedAt string, now time.Time) (visits map[string]checklistVisits, order []string) {
	visits = map[string]checklistVisits{}
	visit := func(name string, count int, entry checklistVisit) {
		v, seen := visits[name]
		if !seen {
			order = append(order, name)
		}
		entry.spent = max(entry.spent, 0)
		v.spent += entry.spent
		v.visits += count
		v.log = append(v.log, entry)
		visits[name] = v
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
		visit(entry.Producer, 1+entry.Repeats, checklistVisit{
			start:        prevEnd,
			startKnown:   prevKnown,
			spent:        spent,
			outcome:      string(entry.Outcome),
			output:       entry.Output,
			gateAttempts: entry.GateAttempts,
			folds:        1 + entry.Repeats,
		})
		if endKnown {
			prevEnd, prevKnown = end, true
		}
	}

	if status.CurrentProducer != "" && status.State != shedengine.StateDone {
		var spent time.Duration
		if prevKnown {
			spent = now.Sub(prevEnd)
		}
		visit(status.CurrentProducer, 1, checklistVisit{
			start:      prevEnd,
			startKnown: prevKnown,
			spent:      spent,
			outcome:    checklistOpenOutcome,
			folds:      1,
		})
	}
	return visits, order
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
