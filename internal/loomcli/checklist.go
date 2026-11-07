// checklist.go implements the hidden `checklist` loom verb, the pane process of the pair's loom-status strand.
// renderChecklist is the pure function that builds the frame the pane draws:
// the recipe rows with their visits, a review segment, the state line and the wait line.

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
	"github.com/Knatte18/loomyard/internal/durfmt"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/lymcwd"
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
	checklistBoxHalted     = "[=]"
	checklistBoxSkipped    = "[-]"
	checklistBoxNotReached = "[ ]"
	checklistWaitNothing   = "waiting on nothing"
	checklistOpenOutcome   = "running"
	checklistUnknownStart  = "--:--"
	checklistBouncedWord   = "bounced"
	checklistEscalatedWord = "escalated"
	checklistEnteredWord   = "entered"
)

// checklistInput is the input one checklist frame is drawn from.
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
// row is the name of the row visited.
// block is the entry's block path, empty for the open visit and for a row outside every block.
// index is the entry's position in the status history, -1 for the open visit.
// routedTo is the entry's routed_to, and isGoto is true for a goto entry.
type checklistVisit struct {
	row          string
	block        string
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
	// hasGateSpans is true when the entry carried a gate span list, however short; gateLines are the visit's gate lines.
	hasGateSpans bool
	gateLines    []string
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
its visit count, each visited row's visit lines under it with no output path
and each gate line opening with its UTC start, then the state line, and last
the wait line with the full reason the run waits. Every line is drawn
uncut, so a short terminal scrolls the top away and keeps the state and wait
lines in view. The loom-status strand runs it. It reads files only.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			cwd, err := lymcwd.CwdFrom(cmd.Context())
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			location, err := lymcwd.Resolve(cwd)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			pane := statuspane.Pane{
				Out:     out,
				Size:    statuspane.TerminalSize(os.Stdout),
				Resized: statuspane.NotifyResize(cmd.Context()),
			}
			pane.Run(cmd.Context(), c.checklistPoll(location, reviewWaitingFor(location)))
			return nil
		},
	}
}

// checklistPoll returns the pane's poll for the worktree at location.
// The self run's seed is read until one read succeeds, and its recipe's rows once after that.
// The run's arbiter attempts and the pause record's coverage of the pair are read from disk on every frame.
// While the run is running, the wait line carries the full text of the note waiting reports, elapsed time included.
// A nil waiting reports none.
func (c *loomCLI) checklistPoll(location *lymcwd.Location, waiting func(st shedengine.Status) (shedverbs.WaitNote, error)) func() statuspane.Frame {
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
				waitNote = note.Text
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

// renderChecklist returns the checklist frame, drawn top to bottom.
// Each recipe row is drawn in recipe order, with its visit lines under it.
// Each visit line is followed by its gate lines.
// History-only rows follow the same way, marked "(not in recipe)".
// A review segment is drawn as one block at its first row's place, and it replaces the lines of its rows.
// An arbiter attempt is drawn right after the visit line and gate lines of the history entry its stop key names.
// The placement holds in every list the frame draws; the other attempts come after the last row.
// With in.rowsErr set, the frame starts with the line "recipe unreadable", then lists every visit in history order;
// each visit line names its row.
// The frame ends with the state line, which omits the wait the wait line carries, and then the wait line.
// A block row's Done visit reads "entered".
// A row of a block is named "<row> in <block path>".
// in.startedAt is the run's seed time, where its first visit starts;
// an empty or unparsable value starts it at the first entry's own time.
// in.paused is left out of every visit's time.
// A row's total is drawn to the second, or in whole minutes when one of its visits is still open.
// A gate row's refusal marks the visit of the row it bounced to as rework for that gate row;
// the gate row's line adds the sum of those visits, and the recipe-unreadable lines attribute none.
// Each recipe entry carries its entry number after its box;
// history-only rows and the recipe-unreadable lines carry none.
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
	blockRows := map[string]bool{}
	for _, row := range in.rows {
		if row.IsBlock {
			blockRows[row.Name] = true
		}
	}
	visits, visitedOrder, history := checklistVisitsOf(status, blockRows, in.startedAt, in.paused, now)
	arbiters := newChecklistArbiterPlacement(in.arbiters, len(status.History), now)

	if in.rowsErr != nil {
		lines := []string{checklistRecipeBroken}
		for _, visit := range history {
			if visit.isGoto {
				continue
			}
			lines = append(lines, "  "+checklistVisitLine(visit, visit.row, ""))
			for _, gate := range visit.gateLines {
				lines = append(lines, "    "+gate)
			}
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

	numbers := loomrecipe.EntryNumbers(in.rows)
	recipeRow := make(map[string]loomrecipe.RecipeRow, len(in.rows))
	reviewSegment := map[string]bool{}
	for _, row := range in.rows {
		recipeRow[row.Name] = row
		if row.Bouncer && row.Segment != "" {
			reviewSegment[row.Segment] = true
		}
	}
	perched := map[string]bool{}
	for _, row := range in.rows {
		perched[row.Name] = reviewSegment[row.Segment]
	}
	rework := checklistRework(history, perched)
	skipped := checklistSkippedRows(status)

	var lines []string
	drawnSegment := map[string]bool{}
	for _, name := range ordered {
		if row, ok := recipeRow[name]; ok && reviewSegment[row.Segment] {
			if !drawnSegment[row.Segment] {
				drawnSegment[row.Segment] = true
				lines = append(lines, checklistReviewBlock(row.Segment, numbers[name], in.rows, history, skipped, status, arbiters)...)
			}
			continue
		}
		line := checklistBox(name, visits[name], skipped[name], status) + " " + checklistEntryName(numbers[name], checklistBlockName(name, recipeRow[name].BlockPath))
		if v := visits[name]; v.visits > 0 {
			line += " " + checklistTime(v.spent, slices.ContainsFunc(v.log, checklistVisitIsOpen))
			if v.visits > 1 {
				line += fmt.Sprintf(" x%d", v.visits)
			}
		}
		if total, refused := rework.total[name]; refused {
			line += " rework " + checklistTime(total, rework.includesOpen[name])
		}
		if !inRecipe[name] {
			line += " " + checklistNotInRecipe
		}
		lines = append(lines, line)
		for _, visit := range visits[name].log {
			lines = append(lines, "  "+checklistVisitLine(visit, "", rework.forVisit[visit.index]))
			for _, gate := range visit.gateLines {
				lines = append(lines, "    "+gate)
			}
			for _, arbiter := range arbiters.linesAfter(visit.index) {
				lines = append(lines, "  "+arbiter)
			}
		}
	}
	lines = append(lines, arbiters.endLines()...)
	return statuspane.Frame{Lines: append(lines, stateLine, waitLine)}
}

// checklistBlockLine is one visit line of a review block.
// index is the status history index of the entry the line shows, -1 for the open visit.
// gateLines are the gate lines drawn right under the line.
type checklistBlockLine struct {
	text      string
	index     int
	spent     time.Duration
	open      bool
	gateLines []string
}

// checklistRound is one opened round of a review block: its number and the lines under it, in the order they ran.
type checklistRound struct {
	number int
	lines  []checklistBlockLine
}

// total returns the round's time as "<time>", the sum of its lines' spent.
// The time is in whole minutes when the round holds the open visit, and to the second otherwise.
func (r checklistRound) total() string {
	var spent time.Duration
	open := false
	for _, line := range r.lines {
		spent += line.spent
		open = open || line.open
	}
	return checklistTime(spent, open)
}

// checklistEntryName renders name as "<number>. <name>", or bare when number is 0.
func checklistEntryName(number int, name string) string {
	if number == 0 {
		return name
	}
	return fmt.Sprintf("%d. %s", number, name)
}

// checklistBlockName renders name beside its block path as "<name> in <block>", or bare when block is empty.
func checklistBlockName(name, block string) string {
	if block == "" {
		return name
	}
	return name + " in " + block
}

// checklistReworkSet is the rework that gate rows' refusals caused.
// forVisit maps the history index of a rework visit, -1 for the open visit, to the gate row it was rework for.
// total sums each gate row's rework time, and includesOpen reports whether that sum includes the open visit.
type checklistReworkSet struct {
	forVisit     map[int]string
	total        map[string]time.Duration
	includesOpen map[string]bool
}

// checklistRework attributes rework to gate rows by walking the history-order visits once.
// A gate refusal is a bounced visit of a row outside the review segments; their rework is already in the round totals.
// Its rework visit is the next visit, when that visit is not a goto and is of the routed-to row.
// The open visit is the last visit.
func checklistRework(history []checklistVisit, perched map[string]bool) checklistReworkSet {
	set := checklistReworkSet{forVisit: map[int]string{}, total: map[string]time.Duration{}, includesOpen: map[string]bool{}}
	for i := 0; i+1 < len(history); i++ {
		refusal, next := history[i], history[i+1]
		if refusal.isGoto || refusal.outcome != checklistBouncedWord || perched[refusal.row] {
			continue
		}
		if next.isGoto || next.row != refusal.routedTo {
			continue
		}
		set.forVisit[next.index] = refusal.row
		set.total[refusal.row] += next.spent
		set.includesOpen[refusal.row] = set.includesOpen[refusal.row] || checklistVisitIsOpen(next)
	}
	return set
}

// checklistVisitIsOpen reports whether visit is the current producer's open visit.
func checklistVisitIsOpen(visit checklistVisit) bool {
	return visit.index < 0
}

// checklistTime renders spent in whole minutes when it includes the open visit and to the second when it is closed,
// so a frame changes at most once a minute.
func checklistTime(spent time.Duration, includesOpen bool) string {
	if includesOpen {
		return durfmt.FormatMinutes(spent)
	}
	return durfmt.FormatSeconds(spent)
}

// checklistReviewBlock renders a review segment, one holding a Bouncer row, as one block from the history-order visits.
// The heading reads "<box> <number>. <segment> <time> rounds <n>".
// An unvisited segment is the single line "<box> <number>. <segment>".
// It is boxed as skipped when one of its rows is skipped, and as not reached otherwise.
// number is the segment's entry number, which its rounds and their lines do not carry.
// The segment is named beside the block path of its rows,
// and each line names its row beside the block its entry recorded.
// Under the heading come the lines placed before any round opened, then each opened round in the order it opened.
// A round is drawn as "  round <n> <time>", the sum of its lines' time, followed by its lines.
// The segment ends with a Bouncer done line.
// A review-row entry, the row a Bouncer row's OnStuck names, opens or joins the round its output names.
// A bounced Bouncer line waits and stands first in the next round that opens.
// A fix-row entry, a goto or reroute into the segment, and a Bouncer entry that routes nowhere:
// each stands as the last line of the round in progress, or before any round has opened.
// The open visit of a segment row opens the round after the last one when a bounced Bouncer line waits.
// Otherwise it joins the round in progress.
// Each line is followed by its gate lines and then the arbiter lines placed after its history entry.
func checklistReviewBlock(segment string, number int, rows []loomrecipe.RecipeRow, visits []checklistVisit, skipped map[string]bool, status shedengine.Status, arbiters *checklistArbiterPlacement) []string {
	inSegment := map[string]bool{}
	bouncers := map[string]bool{}
	reviewRows := map[string]bool{}
	segmentName := segment
	for _, row := range rows {
		if row.Segment != segment {
			continue
		}
		if row.BlockPath != "" && segmentName == segment {
			segmentName = checklistBlockName(segment, row.BlockPath)
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
		box := checklistBoxNotReached
		for _, row := range rows {
			if row.Segment == segment && skipped[row.Name] {
				box = checklistBoxSkipped
				break
			}
		}
		return []string{box + " " + checklistEntryName(number, segmentName)}
	}
	box := checklistBoxDone
	for _, row := range rows {
		if row.Segment != segment {
			continue
		}
		// A visited segment takes only a current producer's state box, so a skipped row of it never boxes the heading.
		if current := checklistBox(row.Name, checklistVisits{visits: visitCount[row.Name]}, false, status); current != checklistBoxDone && current != checklistBoxNotReached {
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
		line := checklistBlockLine{text: checklistVisitLine(visit, visit.row, ""), index: visit.index, spent: visit.spent, open: checklistVisitIsOpen(visit), gateLines: visit.gateLines}
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

	lines := []string{fmt.Sprintf("%s %s %s rounds %d", box, checklistEntryName(number, segmentName), checklistTime(spent, segmentOpen), len(rounds))}
	lines = appendChecklistBlockLines(lines, "  ", before, arbiters)
	for _, round := range rounds {
		lines = append(lines, fmt.Sprintf("  round %d %s", round.number, round.total()))
		lines = appendChecklistBlockLines(lines, "    ", round.lines, arbiters)
	}
	return appendChecklistBlockLines(lines, "  ", closing, arbiters)
}

// appendChecklistBlockLines appends each line of block to lines at indent.
// Each line is followed by its gate lines, two spaces deeper.
// The arbiter lines that stand after its history entry follow, at indent.
func appendChecklistBlockLines(lines []string, indent string, block []checklistBlockLine, arbiters *checklistArbiterPlacement) []string {
	for _, line := range block {
		lines = append(lines, indent+line.text)
		for _, gate := range line.gateLines {
			lines = append(lines, indent+"  "+gate)
		}
		for _, arbiter := range arbiters.linesAfter(line.index) {
			lines = append(lines, indent+arbiter)
		}
	}
	return lines
}

// checklistArbiter holds what one arbiter attempt directory yields.
// When unreadable is set, only dir, the directory's name, is filled.
// Otherwise the attempt's record fills number, spawnedAt, historyLength (from its stop key) and deadline,
// and the envelope read fills hasEnvelope and envelope, or sets envelopeErr when the read fails.
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

// readChecklistArbiters reads every subdirectory of arbiterDir as one arbiter attempt, read-only and without a lock.
// One bad attempt leaves the others drawn.
// A directory that cannot be read gives none.
// An attempt whose record errors is unreadable and names its directory;
// an envelope read error marks that attempt alone.
// It reads the envelope, never the agent's verdict.yaml,
// so a hand-on verdict that the verb concluded itself shows too.
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
// The verdict is the envelope's, followed by " (<cause>)" when the envelope has a cause.
// Without an envelope it is "running" before the attempt's deadline and "no verdict" from the deadline on.
// A failed envelope read gives "verdict unreadable".
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
// A readable attempt stands after the line of history entry index k-1, where k is its stop key's history length.
// Only 1 <= k <= the history's length places it there;
// the rest stand at the end of the frame, unreadable ones last.
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

// linesAfter takes the arbiter lines that stand after the line of the history entry at index.
// The open visit, at index -1, has none.
// The lines are removed on the first call, so a later call for the same index returns none.
func (p *checklistArbiterPlacement) linesAfter(index int) []string {
	lines := p.after[index]
	delete(p.after, index)
	return lines
}

// endLines returns the lines that stand at the end of the frame.
// First are the readable attempts whose history entry never took them, in entry order.
// Then come the attempts outside the history, and last the unreadable attempts.
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

// checklistVisitLine renders one visit as "<start> <spent> <outcome>".
// The spent time is to the second, or in whole minutes for the open visit.
// A folded entry adds " x<n>", and a gated entry with no gate span list adds " gate attempts <n>".
// The visit's output is never drawn, since its path would wrap the line.
// A non-empty row is written before the outcome, as "<row> in <block>" when the visit has a block.
// This applies to a list that does not sit under the row's own line.
// A non-empty reworkFor, the gate row whose refusal the visit reworks, ends the line as " rework for <reworkFor>".
func checklistVisitLine(visit checklistVisit, row string, reworkFor string) string {
	start := checklistUnknownStart
	if visit.startKnown {
		start = visit.start.UTC().Format("15:04")
	}
	line := start + " " + checklistTime(visit.spent, checklistVisitIsOpen(visit))
	if row != "" {
		line += " " + checklistBlockName(row, visit.block)
	}
	line += " " + visit.outcome
	if visit.folds > 1 {
		line += fmt.Sprintf(" x%d", visit.folds)
	}
	if visit.gateAttempts != nil && !visit.hasGateSpans {
		line += fmt.Sprintf(" gate attempts %d", *visit.gateAttempts)
	}
	if reworkFor != "" {
		line += " rework for " + reworkFor
	}
	return line
}

// checklistGateLines renders the gate spans of one history entry as one line per gate name.
// The lines come in the order each name first appears, as "<start> gate <name> <gate time>".
// A name with rework spans adds " refused <n> rework <rework time>" to its line.
// The start is the UTC hh:mm of the name's first span with a parsable start, or "--:--" when none has one.
// The gate time is the sum of the name's gate and held spans;
// the rework time is the sum of its rework spans, and n counts those spans.
// Each span counts its end less its start, less the time paused within it, floored at zero;
// a span whose start or end does not parse counts zero.
// Every span here is closed, so the gate and rework times are drawn to the second.
func checklistGateLines(spans []shedengine.GateSpan, paused []pauserecord.Interval) []string {
	type gateTotals struct {
		gate, rework time.Duration
		refusals     int
		start        time.Time
		startKnown   bool
	}
	var names []string
	totals := map[string]*gateTotals{}
	for _, span := range spans {
		t, seen := totals[span.Gate]
		if !seen {
			t = &gateTotals{}
			totals[span.Gate] = t
			names = append(names, span.Gate)
		}
		var counted time.Duration
		start, startKnown := parseChecklistTime(span.Start)
		end, endKnown := parseChecklistTime(span.End)
		if startKnown && !t.startKnown {
			t.start, t.startKnown = start, true
		}
		if startKnown && endKnown && end.After(start) {
			counted = end.Sub(start) - pauserecord.Excluded(paused, start, end)
		}
		counted = max(counted, 0)
		if span.Kind == shedengine.GateSpanKindRework {
			t.rework += counted
			t.refusals++
			continue
		}
		t.gate += counted
	}
	var lines []string
	for _, name := range names {
		t := totals[name]
		start := checklistUnknownStart
		if t.startKnown {
			start = t.start.UTC().Format("15:04")
		}
		line := start + " gate " + name + " " + durfmt.FormatSeconds(t.gate)
		if t.refusals > 0 {
			line += fmt.Sprintf(" refused %d rework %s", t.refusals, durfmt.FormatSeconds(t.rework))
		}
		lines = append(lines, line)
	}
	return lines
}

// checklistWaitLine is the frame's last line: the full reason the run waits, as `lym loom status` gives it.
// A running run with a waitNote waits on it.
// An awaiting run has handed off to its parent.
// A blocked or failed run waits on its activity's wait.
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

// checklistVisitsOf sums the status history into per-row time, visit counts and visit logs.
// It lists the rows in the order they were first visited,
// and it returns every visit in history order.
// A visit lasts from the previous entry's end to its own end, less the time paused within that span;
// a goto entry is no visit and starts the next one at its own time.
// A goto entry is in the returned history as a line that spends no time, and in no per-row log.
// A skipped entry is a row passed through: it is no visit, in no result, and starts the next visit at its own time.
// While the run is not done, the current producer's open visit runs from the last entry's end to now.
// A Stuck entry reads bounced when it records where it was routed, and stuck otherwise.
// An Awaiting entry reads escalated.
// A Done entry of a block row reads entered.
// The result line of each history entry carries the entry's block path.
func checklistVisitsOf(status shedengine.Status, blockRows map[string]bool, startedAt string, paused []pauserecord.Interval, now time.Time) (visits map[string]checklistVisits, order []string, history []checklistVisit) {
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
				block:      entry.Block,
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
		if entry.Outcome == shedengine.OutcomeSkipped {
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
		case entry.Outcome == shedengine.Done && blockRows[entry.Producer]:
			outcome = checklistEnteredWord
		}
		visit(entry.Producer, 1+entry.Repeats, checklistVisit{
			block:        entry.Block,
			index:        index,
			routedTo:     entry.RoutedTo,
			start:        prevEnd,
			startKnown:   prevKnown,
			spent:        spent,
			outcome:      outcome,
			output:       entry.Output,
			gateAttempts: entry.GateAttempts,
			folds:        1 + entry.Repeats,
			hasGateSpans: entry.GateSpans != nil,
			gateLines:    checklistGateLines(entry.GateSpans, paused),
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

// checklistBox returns the box shown for the row name.
// The current producer's box follows the run's state: running, failed, or halted when paused, blocked or awaiting.
// Otherwise the row is done when it has a visit, skipped when it has none and skipped is true, and not reached.
func checklistBox(name string, v checklistVisits, skipped bool, status shedengine.Status) string {
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
	if skipped {
		return checklistBoxSkipped
	}
	return checklistBoxNotReached
}

// checklistSkippedRows returns the set of row names the status history records as skipped by their entry gate.
func checklistSkippedRows(status shedengine.Status) map[string]bool {
	skipped := map[string]bool{}
	for _, entry := range status.History {
		if entry.Outcome == shedengine.OutcomeSkipped {
			skipped[entry.Producer] = true
		}
	}
	return skipped
}
