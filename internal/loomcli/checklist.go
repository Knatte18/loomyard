// checklist.go implements the hidden `checklist` loom verb, the pane process of the pair's loom-status strand, and renderChecklist, the pure frame it draws, top to bottom:
// one line per recipe row with its box, the time spent in it and its visit count, each visited row's visit lines under it, a review segment drawn as one block grouped by round, the run's state line, and the wait line carrying what the run waits on in full.

package loomcli

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedadapters"
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
	checklistBouncedWord   = "bounced"
	checklistEscalatedWord = "escalated"
)

// checklistInput is what renderChecklist draws a frame from.
// rows is the recipe's rows, and rowsErr is set when the recipe could not be read.
type checklistInput struct {
	rows      []loomrecipe.RecipeRow
	rowsErr   error
	poll      statuspane.StatusPoll
	waitNote  string
	startedAt string
}

// checklistVisit is one visit of a row, read from one status history entry or from the current producer's open visit.
// row is the name of the row visited, index the entry's position in the status history (-1 for the open visit), routedTo the entry's routed_to and isGoto true for a goto entry.
type checklistVisit struct {
	row          string
	index        int
	routedTo     string
	isGoto       bool
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
// It reads files only.
func (c *loomCLI) checklistCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "checklist",
		Short:       "draw this worktree's gate checklist in a pane (run by the loom-status strand)",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `checklist draws the run's frame, redrawing in place until it is signalled:
one line per recipe row in recipe order with its box, the time spent in it and
its visit count, each visited row's visit lines under it, then the state line,
and last the wait line with the full reason the run waits. Every line is drawn
uncut, so a short terminal scrolls the top away and keeps the state and wait
lines in view. The loom-status strand runs it. It reads files only.`,
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
			pane := statuspane.Pane{
				Out:     out,
				Size:    statuspane.TerminalSize(os.Stdout),
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
		rows      []loomrecipe.RecipeRow
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
				rows, rowsErr = loomrecipe.Rows(seed.Recipe)
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
		return renderChecklist(checklistInput{rows: rows, rowsErr: rowsErr, poll: poll, waitNote: waitNote, startedAt: startedAt}, time.Now())
	}
}

// renderChecklist returns the checklist frame, top to bottom:
// one line per row of the recipe in recipe order, each visited row's visit lines under it, then the history-only rows the same way, marked checklistNotInRecipe;
// a review segment, drawn by checklistReviewBlock at the place of its first row, takes the place of its rows' own lines;
// with in.rowsErr set, the single line checklistRecipeBroken and then every visit line in history order, each naming its row;
// the state line, without the wait the wait line carries; and the wait line.
// in.startedAt is the run's seed time, where its first visit starts; an empty or unparsable value starts it at the first entry's own time.
func renderChecklist(in checklistInput, now time.Time) statuspane.Frame {
	stateLine := shedverbs.UnavailableLine(checklistLabel)
	if in.poll.Found {
		quiet := in.poll.Status
		quiet.Activity.Wait = ""
		stateLine = shedverbs.RenderStatusLine(checklistLabel, quiet)
	}
	waitLine := checklistWaitLine(in.poll, in.waitNote)

	var status shedengine.Status
	if in.poll.Found {
		status = in.poll.Status
	}
	visits, visitedOrder, history := checklistVisitsOf(status, in.startedAt, now)

	if in.rowsErr != nil {
		lines := []string{checklistRecipeBroken}
		for _, visit := range history {
			if !visit.isGoto {
				lines = append(lines, "  "+checklistVisitLine(visit, visit.row))
			}
		}
		return statuspane.Frame{Lines: append(lines, stateLine, waitLine)}
	}

	inRecipe := make(map[string]bool, len(in.rows))
	ordered := make([]string, 0, len(in.rows))
	for _, row := range in.rows {
		inRecipe[row.Name] = true
		ordered = append(ordered, row.Name)
	}
	for _, name := range visitedOrder {
		if !inRecipe[name] {
			ordered = append(ordered, name)
		}
	}

	recipeRow := make(map[string]loomrecipe.RecipeRow, len(in.rows))
	reviewSegment := map[string]bool{}
	for _, row := range in.rows {
		recipeRow[row.Name] = row
		if row.Bouncer && row.Segment != "" {
			reviewSegment[row.Segment] = true
		}
	}

	var lines []string
	drawnSegment := map[string]bool{}
	for _, name := range ordered {
		if row, ok := recipeRow[name]; ok && reviewSegment[row.Segment] {
			if !drawnSegment[row.Segment] {
				drawnSegment[row.Segment] = true
				lines = append(lines, checklistReviewBlock(row.Segment, in.rows, history, status)...)
			}
			continue
		}
		line := checklistBox(name, visits[name], status) + " " + name
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
		for _, visit := range visits[name].log {
			lines = append(lines, "  "+checklistVisitLine(visit, ""))
		}
	}
	return statuspane.Frame{Lines: append(lines, stateLine, waitLine)}
}

// checklistRound is one opened round of a review block: its number and the lines under it, in the order they ran.
type checklistRound struct {
	number int
	lines  []string
}

// checklistReviewBlock renders a review segment, a recipe segment holding a Bouncer row, as one block from the history-order visits.
// The heading reads "<box> <segment> <time> rounds <n>"; an unvisited segment is the single line "[ ] <segment>".
// Under it come the lines placed before any round opened, then each opened round in the order opened as "  round <n>" with its lines, and last a Bouncer done that ends the segment.
// A review-row entry, the row a Bouncer row's OnStuck names, opens or joins the round its output names through shedadapters.OutputRound.
// A bounced Bouncer line waits and stands first in the next round that opens.
// A fix-row entry, a goto into the segment, a reroute into the segment and a Bouncer entry that routes nowhere stand as the last line of the round in progress, or before round 1.
// The open visit of a segment row opens the round after the last when a bounced Bouncer line waits, and otherwise joins the round in progress.
func checklistReviewBlock(segment string, rows []loomrecipe.RecipeRow, visits []checklistVisit, status shedengine.Status) []string {
	inSegment := map[string]bool{}
	bouncers := map[string]bool{}
	reviewRows := map[string]bool{}
	for _, row := range rows {
		if row.Segment != segment {
			continue
		}
		inSegment[row.Name] = true
		if row.Bouncer {
			bouncers[row.Name] = true
			reviewRows[row.OnStuck] = true
		}
	}

	var spent time.Duration
	visitCount := map[string]int{}
	for _, visit := range visits {
		if !visit.isGoto && inSegment[visit.row] {
			spent += visit.spent
			visitCount[visit.row]++
		}
	}
	if len(visitCount) == 0 {
		return []string{checklistBoxNotReached + " " + segment}
	}
	box := checklistBoxDone
	for _, row := range rows {
		if row.Segment != segment {
			continue
		}
		if current := checklistBox(row.Name, checklistVisits{visits: visitCount[row.Name]}, status); current != checklistBoxDone && current != checklistBoxNotReached {
			box = current
			break
		}
	}

	var (
		before  []string
		rounds  []checklistRound
		waiting []string
		closing []string
		current = -1
	)
	roundIndex := map[int]int{}
	openRound := func(number int) {
		rounds = append(rounds, checklistRound{number: number, lines: waiting})
		waiting = nil
		current = len(rounds) - 1
		roundIndex[number] = current
	}
	placeInProgress := func(line string) {
		if current < 0 {
			before = append(before, line)
			return
		}
		rounds[current].lines = append(rounds[current].lines, line)
	}
	for _, visit := range visits {
		line := checklistVisitLine(visit, visit.row)
		switch {
		case visit.isGoto:
			if inSegment[visit.row] {
				placeInProgress(line)
			}
		case visit.outcome == string(shedengine.OutcomeReroute) && inSegment[visit.routedTo]:
			placeInProgress(line)
		case !inSegment[visit.row]:
		case visit.index < 0:
			if len(waiting) == 0 {
				placeInProgress(line)
				continue
			}
			next := 1
			if len(rounds) > 0 {
				next = rounds[len(rounds)-1].number + 1
			}
			openRound(next)
			placeInProgress(line)
		case bouncers[visit.row]:
			switch visit.outcome {
			case checklistBouncedWord:
				waiting = append(waiting, line)
			case string(shedengine.Done):
				closing = append(closing, line)
			default:
				placeInProgress(line)
			}
		case reviewRows[visit.row]:
			number, named := shedadapters.OutputRound(visit.output)
			if !named {
				placeInProgress(line)
				continue
			}
			if at, opened := roundIndex[number]; opened {
				rounds[at].lines = append(rounds[at].lines, line)
				continue
			}
			openRound(number)
			placeInProgress(line)
		default:
			placeInProgress(line)
		}
	}
	for _, line := range waiting {
		placeInProgress(line)
	}

	lines := []string{fmt.Sprintf("%s %s %s rounds %d", box, segment, statuspane.FormatMinutes(spent), len(rounds))}
	for _, line := range before {
		lines = append(lines, "  "+line)
	}
	for _, round := range rounds {
		lines = append(lines, fmt.Sprintf("  round %d", round.number))
		for _, line := range round.lines {
			lines = append(lines, "    "+line)
		}
	}
	for _, line := range closing {
		lines = append(lines, "  "+line)
	}
	return lines
}

// checklistVisitLine renders one visit as "<start> <spent> <outcome>", then " x<n>" for a folded entry, " <output>" when it has one and " gate attempts <n>" when gated.
// A non-empty row is written before the outcome, for a list that does not sit under the row's own line.
func checklistVisitLine(visit checklistVisit, row string) string {
	start := checklistUnknownStart
	if visit.startKnown {
		start = visit.start.UTC().Format("15:04")
	}
	line := start + " " + statuspane.FormatMinutes(visit.spent)
	if row != "" {
		line += " " + row
	}
	line += " " + visit.outcome
	if visit.folds > 1 {
		line += fmt.Sprintf(" x%d", visit.folds)
	}
	if visit.output != "" {
		line += " " + visit.output
	}
	if visit.gateAttempts != nil {
		line += fmt.Sprintf(" gate attempts %d", *visit.gateAttempts)
	}
	return line
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

// checklistVisitsOf sums the status history into per-row time, visit counts and visit logs, lists the rows in the order they were first visited, and returns every visit in history order.
// A visit lasts from the previous entry's end to its own end; a goto entry is no visit and starts the next one at its own time.
// A goto entry is in the history-order result as a zero-time line and in no per-row log.
// While the run is not done, the current producer's open visit runs from the last entry's end to now.
// A Stuck entry that records where it was routed reads checklistBouncedWord and one that does not reads as stored; an awaiting entry reads checklistEscalatedWord.
func checklistVisitsOf(status shedengine.Status, startedAt string, now time.Time) (visits map[string]checklistVisits, order []string, history []checklistVisit) {
	visits = map[string]checklistVisits{}
	visit := func(name string, count int, entry checklistVisit) {
		v, seen := visits[name]
		if !seen {
			order = append(order, name)
		}
		entry.row = name
		entry.spent = max(entry.spent, 0)
		v.spent += entry.spent
		v.visits += count
		v.log = append(v.log, entry)
		visits[name] = v
		history = append(history, entry)
	}

	prevEnd, prevKnown := parseChecklistTime(startedAt)
	for index, entry := range status.History {
		at, atKnown := parseChecklistTime(entry.At)
		if entry.Outcome == shedengine.OutcomeGoto {
			history = append(history, checklistVisit{
				row:        entry.Producer,
				index:      index,
				isGoto:     true,
				start:      at,
				startKnown: atKnown,
				outcome:    string(entry.Outcome),
				folds:      1,
			})
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
		outcome := string(entry.Outcome)
		switch {
		case entry.Outcome == shedengine.Stuck && entry.RoutedTo != "":
			outcome = checklistBouncedWord
		case entry.Outcome == shedengine.Awaiting:
			outcome = checklistEscalatedWord
		}
		visit(entry.Producer, 1+entry.Repeats, checklistVisit{
			index:        index,
			routedTo:     entry.RoutedTo,
			start:        prevEnd,
			startKnown:   prevKnown,
			spent:        spent,
			outcome:      outcome,
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
			index:      -1,
			start:      prevEnd,
			startKnown: prevKnown,
			spent:      spent,
			outcome:    checklistOpenOutcome,
			folds:      1,
		})
	}
	return visits, order, history
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
