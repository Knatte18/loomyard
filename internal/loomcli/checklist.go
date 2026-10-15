// checklist.go implements the hidden `checklist` loom verb, the pane process of the pair's loom-status strand, and renderChecklist, the pure frame it draws, top to bottom:
// one line per recipe row with its box, the time spent in it and its visit count, each visited row's visit lines under it, a review segment drawn as one block grouped by round, the run's state line, and the wait line carrying what the run waits on in full.

package loomcli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/pauserecord"
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
// paused is the pause record's coverage of the pair's slug, whose time no visit counts.
type checklistInput struct {
	rows      []loomrecipe.RecipeRow
	rowsErr   error
	poll      statuspane.StatusPoll
	waitNote  string
	startedAt string
	arbiters  []checklistArbiter
	paused    []pauserecord.Interval
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
// The run's arbiter attempts and the pause record's coverage of the pair are read from disk on every frame.
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
	arbiterDir := shedrun.ArbiterDir(location, shedrun.SelfRunID)
	pauseGeometry, pauseErr := hubgeom.PauseGeometry(location)

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
		now := time.Now()
		var paused []pauserecord.Interval
		if pauseErr == nil {
			paused = readChecklistPaused(pauseGeometry.RecordPath, pauseGeometry.Slug, now)
		}
		return renderChecklist(checklistInput{rows: rows, rowsErr: rowsErr, poll: poll, waitNote: waitNote, startedAt: startedAt, arbiters: readChecklistArbiters(arbiterDir), paused: paused}, now)
	}
}

// readChecklistPaused returns the intervals the pause record at recordPath covers for slug, live entries running to now.
// A record that cannot be read or decoded gives none, so the frame counts nothing as paused and never breaks.
func readChecklistPaused(recordPath, slug string, now time.Time) []pauserecord.Interval {
	coverage, err := pauserecord.ReadCoverage(recordPath, slug, now)
	if err != nil {
		return nil
	}
	return coverage.Intervals
}

// renderChecklist returns the checklist frame, top to bottom:
// one line per row of the recipe in recipe order, each visited row's visit lines under it, then the history-only rows the same way, marked checklistNotInRecipe;
// a review segment, drawn by checklistReviewBlock at the place of its first row, takes the place of its rows' own lines;
// each arbiter attempt standing right after the line of the history entry its stop key names, in every list the frame draws, and the others after the last row;
// with in.rowsErr set, the single line checklistRecipeBroken and then every visit line in history order, each naming its row;
// the state line, without the wait the wait line carries; and the wait line.
// in.startedAt is the run's seed time, where its first visit starts; an empty or unparsable value starts it at the first entry's own time.
// in.paused is left out of every visit's time.
// A row's total is drawn to the second unless one of its visits is the open one, and then in whole minutes.
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
	visits, visitedOrder, history := checklistVisitsOf(status, in.startedAt, in.paused, now)
	arbiters := newChecklistArbiterPlacement(in.arbiters, len(status.History), now)

	if in.rowsErr != nil {
		lines := []string{checklistRecipeBroken}
		for _, visit := range history {
			if visit.isGoto {
				continue
			}
			lines = append(lines, "  "+checklistVisitLine(visit, visit.row))
			for _, arbiter := range arbiters.linesAfter(visit.index) {
				lines = append(lines, "  "+arbiter)
			}
		}
		lines = append(lines, arbiters.endLines()...)
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
				lines = append(lines, checklistReviewBlock(row.Segment, in.rows, history, status, arbiters)...)
			}
			continue
		}
		line := checklistBox(name, visits[name], status) + " " + name
		if v := visits[name]; v.visits > 0 {
			line += " " + checklistTime(v.spent, slices.ContainsFunc(v.log, checklistVisitIsOpen))
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
			for _, arbiter := range arbiters.linesAfter(visit.index) {
				lines = append(lines, "  "+arbiter)
			}
		}
	}
	lines = append(lines, arbiters.endLines()...)
	return statuspane.Frame{Lines: append(lines, stateLine, waitLine)}
}

// checklistBlockLine is one visit line of a review block, the status history index of the entry it shows (-1 for the open visit) and the time the line spent.
// open is true for the open visit.
type checklistBlockLine struct {
	text  string
	index int
	spent time.Duration
	open  bool
}

// checklistRound is one opened round of a review block: its number and the lines under it, in the order they ran.
type checklistRound struct {
	number int
	lines  []checklistBlockLine
}

// total returns the round's time as "<time>": the sum of its lines' spent, in whole minutes when the round holds the open visit and to the second otherwise.
func (r checklistRound) total() string {
	var spent time.Duration
	open := false
	for _, line := range r.lines {
		spent += line.spent
		open = open || line.open
	}
	return checklistTime(spent, open)
}

// checklistVisitIsOpen reports whether visit is the current producer's open visit.
func checklistVisitIsOpen(visit checklistVisit) bool {
	return visit.index < 0
}

// checklistTime renders spent in whole minutes when it includes the open visit and to the second when it is closed, so a frame changes at most once a minute.
func checklistTime(spent time.Duration, includesOpen bool) string {
	if includesOpen {
		return statuspane.FormatMinutes(spent)
	}
	return statuspane.FormatSeconds(spent)
}

// checklistReviewBlock renders a review segment, a recipe segment holding a Bouncer row, as one block from the history-order visits.
// The heading reads "<box> <segment> <time> rounds <n>"; an unvisited segment is the single line "[ ] <segment>".
// Under it come the lines placed before any round opened, then each opened round in the order opened as "  round <n> <time>", the sum of its lines' time, with its lines, and last a Bouncer done that ends the segment.
// A time that includes the open visit is drawn in whole minutes and every other time to the second.
// A review-row entry, the row a Bouncer row's OnStuck names, opens or joins the round its output names through shedadapters.OutputRound.
// A bounced Bouncer line waits and stands first in the next round that opens.
// A fix-row entry, a goto into the segment, a reroute into the segment and a Bouncer entry that routes nowhere stand as the last line of the round in progress, or before round 1.
// The open visit of a segment row opens the round after the last when a bounced Bouncer line waits, and otherwise joins the round in progress.
// Each line is followed by the arbiter lines placed after its history entry.
func checklistReviewBlock(segment string, rows []loomrecipe.RecipeRow, visits []checklistVisit, status shedengine.Status, arbiters *checklistArbiterPlacement) []string {
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
	segmentOpen := false
	visitCount := map[string]int{}
	for _, visit := range visits {
		if !visit.isGoto && inSegment[visit.row] {
			spent += visit.spent
			segmentOpen = segmentOpen || checklistVisitIsOpen(visit)
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
		before  []checklistBlockLine
		rounds  []checklistRound
		waiting []checklistBlockLine
		closing []checklistBlockLine
		current = -1
	)
	roundIndex := map[int]int{}
	openRound := func(number int) {
		rounds = append(rounds, checklistRound{number: number, lines: waiting})
		waiting = nil
		current = len(rounds) - 1
		roundIndex[number] = current
	}
	placeInProgress := func(line checklistBlockLine) {
		if current < 0 {
			before = append(before, line)
			return
		}
		rounds[current].lines = append(rounds[current].lines, line)
	}
	for _, visit := range visits {
		line := checklistBlockLine{text: checklistVisitLine(visit, visit.row), index: visit.index, spent: visit.spent, open: checklistVisitIsOpen(visit)}
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

	lines := []string{fmt.Sprintf("%s %s %s rounds %d", box, segment, checklistTime(spent, segmentOpen), len(rounds))}
	lines = appendChecklistBlockLines(lines, "  ", before, arbiters)
	for _, round := range rounds {
		lines = append(lines, fmt.Sprintf("  round %d %s", round.number, round.total()))
		lines = appendChecklistBlockLines(lines, "    ", round.lines, arbiters)
	}
	return appendChecklistBlockLines(lines, "  ", closing, arbiters)
}

// appendChecklistBlockLines appends each of block to lines at indent, each followed by the arbiter lines placed after its history entry.
func appendChecklistBlockLines(lines []string, indent string, block []checklistBlockLine, arbiters *checklistArbiterPlacement) []string {
	for _, line := range block {
		lines = append(lines, indent+line.text)
		for _, arbiter := range arbiters.linesAfter(line.index) {
			lines = append(lines, indent+arbiter)
		}
	}
	return lines
}

// checklistArbiter is what one arbiter attempt directory gave.
// With unreadable set only dir, the directory's name, is filled.
// Otherwise the attempt's record gave number, spawnedAt, historyLength (its stop key's) and deadline, and the envelope read gave hasEnvelope and envelope, or envelopeErr.
type checklistArbiter struct {
	dir           string
	unreadable    bool
	number        int
	spawnedAt     time.Time
	historyLength int
	deadline      time.Time
	envelope      loomengine.ArbiterEnvelope
	hasEnvelope   bool
	envelopeErr   bool
}

// readChecklistArbiters reads every subdirectory of arbiterDir as one arbiter attempt, read-only and without a lock, so one bad attempt leaves the others drawn.
// An absent directory gives none; an attempt whose record errors is unreadable and names its directory; an envelope read error marks that attempt alone.
// It reads the envelope, never the agent's verdict.yaml, so a hand-on the verb concluded itself shows too.
func readChecklistArbiters(arbiterDir string) []checklistArbiter {
	entries, err := os.ReadDir(arbiterDir)
	if err != nil {
		return nil
	}
	var arbiters []checklistArbiter
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(arbiterDir, entry.Name())
		attempt, err := loomengine.ReadArbiterAttempt(dir)
		if err != nil {
			arbiters = append(arbiters, checklistArbiter{dir: entry.Name(), unreadable: true})
			continue
		}
		arbiter := checklistArbiter{
			dir:           entry.Name(),
			number:        attempt.Number,
			spawnedAt:     attempt.SpawnedAt,
			historyLength: attempt.StopKey.HistoryLength,
			deadline:      attempt.Deadline,
		}
		envelope, found, err := loomengine.ReadArbiterEnvelope(loomengine.ArbiterEnvelopePath(dir))
		arbiter.envelope, arbiter.hasEnvelope, arbiter.envelopeErr = envelope, found, err != nil
		arbiters = append(arbiters, arbiter)
	}
	return arbiters
}

// checklistArbiterLine renders one attempt as "<spawn hh:mm> arbiter attempt <n> <verdict>".
// The verdict is the envelope's, followed by " (<cause>)" when it has a cause; with no envelope it is "running" before the attempt's deadline and "no verdict" from it on, and "verdict unreadable" when the envelope read failed.
// An unreadable attempt reads "arbiter attempt unreadable (<directory name>)".
func checklistArbiterLine(a checklistArbiter, now time.Time) string {
	if a.unreadable {
		return "arbiter attempt unreadable (" + a.dir + ")"
	}
	var verdict string
	switch {
	case a.envelopeErr:
		verdict = "verdict unreadable"
	case a.hasEnvelope:
		verdict = a.envelope.Verdict
		if a.envelope.Cause != "" {
			verdict += " (" + a.envelope.Cause + ")"
		}
	case now.Before(a.deadline):
		verdict = checklistOpenOutcome
	default:
		verdict = "no verdict"
	}
	return fmt.Sprintf("%s arbiter attempt %d %s", a.spawnedAt.UTC().Format("15:04"), a.number, verdict)
}

// checklistArbiterPlacement holds the rendered arbiter lines of a frame by where they stand.
// A readable attempt whose stop key names history entry k, 1 <= k <= the history's length, stands after the line of entry k-1; the rest stand at the end of the frame, unreadable ones last.
type checklistArbiterPlacement struct {
	after      map[int][]string
	outside    []string
	unreadable []string
}

// newChecklistArbiterPlacement sorts arbiters into a placement for a status history of historyLength entries.
func newChecklistArbiterPlacement(arbiters []checklistArbiter, historyLength int, now time.Time) *checklistArbiterPlacement {
	placement := &checklistArbiterPlacement{after: map[int][]string{}}
	for _, a := range arbiters {
		line := checklistArbiterLine(a, now)
		switch {
		case a.unreadable:
			placement.unreadable = append(placement.unreadable, line)
		case a.historyLength >= 1 && a.historyLength <= historyLength:
			placement.after[a.historyLength-1] = append(placement.after[a.historyLength-1], line)
		default:
			placement.outside = append(placement.outside, line)
		}
	}
	return placement
}

// linesAfter takes the lines that stand after the line of history entry index, none for the open visit's -1.
// Each is taken once, so a history entry drawn in two lists carries them in the first.
func (p *checklistArbiterPlacement) linesAfter(index int) []string {
	lines := p.after[index]
	delete(p.after, index)
	return lines
}

// endLines returns the lines that stand at the end of the frame: attempts outside the history, readable ones whose entry no list drew, and last the unreadable ones.
func (p *checklistArbiterPlacement) endLines() []string {
	indexes := make([]int, 0, len(p.after))
	for index := range p.after {
		indexes = append(indexes, index)
	}
	slices.Sort(indexes)
	var lines []string
	for _, index := range indexes {
		lines = append(lines, p.after[index]...)
	}
	lines = append(lines, p.outside...)
	return append(lines, p.unreadable...)
}

// checklistVisitLine renders one visit as "<start> <spent> <outcome>", the time to the second except for the open visit's in whole minutes, then " x<n>" for a folded entry, " <output>" when it has one and " gate attempts <n>" when gated.
// A non-empty row is written before the outcome, for a list that does not sit under the row's own line.
func checklistVisitLine(visit checklistVisit, row string) string {
	start := checklistUnknownStart
	if visit.startKnown {
		start = visit.start.UTC().Format("15:04")
	}
	line := start + " " + checklistTime(visit.spent, checklistVisitIsOpen(visit))
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
// A visit lasts from the previous entry's end to its own end, less the time paused within that span; a goto entry is no visit and starts the next one at its own time.
// A goto entry is in the history-order result as a zero-time line and in no per-row log.
// While the run is not done, the current producer's open visit runs from the last entry's end to now.
// A Stuck entry that records where it was routed reads checklistBouncedWord and one that does not reads as stored; an awaiting entry reads checklistEscalatedWord.
func checklistVisitsOf(status shedengine.Status, startedAt string, paused []pauserecord.Interval, now time.Time) (visits map[string]checklistVisits, order []string, history []checklistVisit) {
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
			spent = end.Sub(prevEnd) - pauserecord.Excluded(paused, prevEnd, end)
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
			spent = now.Sub(prevEnd) - pauserecord.Excluded(paused, prevEnd, now)
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
