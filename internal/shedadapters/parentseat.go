// parentseat.go implements the parent seat of a review segment.
// It decides which round asks the run's parent orch to review and posts the ask's one creel line.
// It settles a round's ask once its review is accepted or its judge has ruled.
// The ask itself lives in burlerengine's never-tracked ask record; this file only decides, posts and settles.

package shedadapters

import (
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/logger"
)

// ParentReviewSeat is a review segment's parent seat.
// A round of the segment may ask the run's parent orch to review the segment's subject beside the reviewer or the judge.
// The chair, or the judge, keeps every finding the parent reports.
type ParentReviewSeat struct {
	// Files locates the run's ask record and the parent's reports.
	Files burlerengine.ParentReviewFiles
	// Slug is the run's slug, which the ask's line and its answer verb name.
	Slug string
	// Tag opens the ask's line and names the kind of review asked for.
	Tag string
	// Subject names what the parent reviews, such as the decision record by its path; the ask's line carries it.
	Subject string
	// AskBeforeJudge has the segment's Bouncer ask the parent for every round.
	// The ask goes out once the round's review and fixer report exist and before the Bouncer judges the round.
	// The ask names that round's own review and fixer report, and the judge waits for the answer.
	// The segment's review producer then sends and settles no ask.
	AskBeforeJudge bool
	// AskEnabled reads the segment's ask setting for the round about to start.
	// It decides only whether the round sends a new ask; an ask the round already holds is carried through whatever it reads.
	AskEnabled func() bool
	// Wait returns the time from an ask to its deadline,
	// and a non-empty reason when the chair's timeout leaves the parent no time, so the round sends no ask.
	Wait func() (time.Duration, string)
	// RecordWrittenAt returns the latest time the run's history records a rewrite of the subject, the zero time when none.
	// A seat that asks before the judge never reads it.
	RecordWrittenAt func() (time.Time, error)
	// Resolve answers the orch strand an ask goes to and whether that orch's state records one.
	Resolve func() (strand string, recorded bool, err error)
	// Post sends line to strand as creel mail signed as lym's loom program.
	Post func(strand, line string) error
}

// The tags that open an ask's line, one per review segment that asks the parent.
const (
	// DiscussionParentAskLineTag opens every line that asks the parent orch to review a Discussion-Review round.
	DiscussionParentAskLineTag = "[discussion-review request]"
	// CodeParentAskLineTag opens every line that asks the parent orch to review a Code-Review round.
	CodeParentAskLineTag = "[code-review request]"
)

// parentDeadlineLayout renders an ask's deadline in the line as a wall-clock time.
const parentDeadlineLayout = "2006-01-02 15:04:05 MST"

// roundSeed returns the identity of runDir's review generation: the write time of its round-1 focus file.
// The time is in RFC 3339 with nanoseconds.
// A fresh start, a new generation and a clear each write that file anew; a goto, a resume and a restarted row keep it.
func roundSeed(runDir string) (string, error) {
	info, err := os.Stat(FocusPath(runDir, 1))
	if err != nil {
		return "", err
	}
	return info.ModTime().UTC().Format(time.RFC3339Nano), nil
}

// parentAsker runs a ParentReviewSeat for the review producer or the Bouncer that owns the round's ask.
// name and engine identify that owner in logs, and runDir is the segment's run directory.
// A nil seat asks nothing.
type parentAsker struct {
	seat   *ParentReviewSeat
	name   string
	engine string
	runDir string
	now    func() time.Time
}

// newParentAsker returns the asker of seat for the owner called name under the engine label engine.
func newParentAsker(seat *ParentReviewSeat, name, engine, runDir string, now func() time.Time) *parentAsker {
	return &parentAsker{seat: seat, name: name, engine: engine, runDir: runDir, now: now}
}

// roundStartAsk returns the number of the ask round holds at its start, sending a new ask when the round asks.
// A seat that asks before the judge asks nothing here.
func (a *parentAsker) roundStartAsk(round int) int {
	if a.seat == nil || a.seat.AskBeforeJudge {
		return 0
	}
	return a.ask(round)
}

// beforeJudgeAsk returns the number of the ask round holds for its judge, sending a new ask when the round asks.
// A seat that asks at round start asks nothing here.
func (a *parentAsker) beforeJudgeAsk(round int) int {
	if a.seat == nil || !a.seat.AskBeforeJudge {
		return 0
	}
	return a.ask(round)
}

// readAsks returns the run's ask record and the seed of runDir's generation.
// ok is false when the row has no parent seat or either read failed, which it logs.
func (a *parentAsker) readAsks(round int) (record burlerengine.ParentAskRecord, seed string, ok bool) {
	if a.seat == nil {
		return burlerengine.ParentAskRecord{}, "", false
	}
	seed, err := roundSeed(a.runDir)
	if err != nil {
		logger.Warn("shedadapters: the review generation's seed is unreadable; the round goes on without the parent", "producer", a.name, "engine", a.engine, "round", round, "error", err)
		return burlerengine.ParentAskRecord{}, "", false
	}
	record, _, err = burlerengine.ReadParentAsks(a.seat.Files)
	if err != nil {
		logger.Warn("shedadapters: the parent-review ask record is unreadable; the round goes on without the parent", "producer", a.name, "engine", a.engine, "round", round, "error", err)
		return burlerengine.ParentAskRecord{}, "", false
	}
	return record, seed, true
}

// ask returns the number of round's parent-review ask, sending a new ask when the round asks.
// It returns 0 when the round holds none.
// An ask the round already holds is never sent again, whatever the segment's ask setting now reads.
// One still pending-post is posted again to the strand it records.
// A new ask goes out only when the setting is on and due says the round asks.
// A failure to read or write the ask record is logged, and the round goes on without the parent.
func (a *parentAsker) ask(round int) int {
	record, seed, ok := a.readAsks(round)
	if !ok {
		return 0
	}
	if ask, held := record.RoundAsk(seed, round); held {
		if ask.State == burlerengine.ParentAskPendingPost {
			a.post(ask)
		}
		return ask.Number
	}
	if !a.seat.AskEnabled() || !a.due(record, seed, round) {
		return 0
	}
	return a.send(seed, round)
}

// due reports whether round of the generation seed asks the parent, read against the seat's own latest ask.
// Asks of other seats in the run's one record are not read.
// It asks when the seat has no ask yet or its latest ask has another seed.
// On a seat that asks before the judge it always asks.
// It asks when the run's history records a rewrite of the subject after the latest ask.
// Within the same seed it asks when the round before ended done with findings, so the parent confirms their fixes.
// Within the same seed it asks when an earlier round's ask was not-sent, so a raised chair timeout takes effect.
func (a *parentAsker) due(record burlerengine.ParentAskRecord, seed string, round int) bool {
	latest, ok := record.LatestTagged(a.seat.Tag)
	if !ok && a.seat.Tag == DiscussionParentAskLineTag {
		// Asks recorded before asks carried a tag are the discussion seat's, the only seat that then asked.
		latest, ok = record.LatestTagged("")
	}
	if !ok || latest.Seed != seed || a.seat.AskBeforeJudge {
		return true
	}
	rewrittenAt, err := a.seat.RecordWrittenAt()
	switch {
	case err != nil:
		logger.Warn("shedadapters: the run's history is unreadable; judging the round as though the record was not rewritten", "producer", a.name, "engine", a.engine, "round", round, "error", err)
	case rewrittenAt.After(latest.AskedAt):
		return true
	}
	switch {
	case latest.Round == round-1 && latest.State == burlerengine.ParentAskDone:
		report, err := burlerengine.ReadParentReport(latest.Report)
		if err != nil {
			logger.Warn("shedadapters: the previous round's parent report is unreadable; asking the parent again", "producer", a.name, "engine", a.engine, "round", round, "error", err)
			return true
		}
		return len(report.Findings) > 0
	case latest.Round < round && latest.State == burlerengine.ParentAskNotSent:
		return true
	}
	return false
}

// send records round's new ask and posts it, returning its number, or 0 when the record could not be written.
// A wait the chair's timeout leaves no room for records the ask not-sent with that reason,
// and a parent that does not resolve, or records no orch strand, records it no-parent; neither posts anything.
// Otherwise the destination strand is resolved once.
// The ask is recorded pending-post with its deadline and that strand, then posted.
func (a *parentAsker) send(seed string, round int) int {
	seat := a.seat
	now := a.now()
	ask := burlerengine.ParentAsk{Round: round, Seed: seed, Tag: seat.Tag, AskedAt: now}
	wait, skipReason := seat.Wait()
	if skipReason != "" {
		ask.State, ask.Reason = burlerengine.ParentAskNotSent, skipReason
	} else {
		strand, recorded, err := seat.Resolve()
		switch {
		case err != nil:
			ask.State, ask.Reason = burlerengine.ParentAskNoParent, fmt.Sprintf("the run's parent orch could not be resolved: %s", err)
		case !recorded:
			ask.State, ask.Reason = burlerengine.ParentAskNoParent, "no orch strand is recorded at the run's parent"
		default:
			ask.State, ask.Strand, ask.Deadline = burlerengine.ParentAskPendingPost, strand, now.Add(wait)
		}
	}
	recorded, err := burlerengine.RecordParentAsk(seat.Files, ask)
	if err != nil {
		logger.Warn("shedadapters: the round's parent-review ask was not recorded; the round goes on without the parent", "producer", a.name, "engine", a.engine, "round", round, "error", err)
		return 0
	}
	if recorded.State == burlerengine.ParentAskPendingPost {
		a.post(recorded)
	} else {
		logger.Warn("shedadapters: the round's parent-review ask did not go out; the round goes on without the parent", "producer", a.name, "engine", a.engine, "round", round, "ask", recorded.Number, "state", recorded.State, "reason", recorded.Reason)
	}
	return recorded.Number
}

// post posts ask's line to the strand it records and sets the ask open.
// A failed post sets the ask undelivered with the error as its reason, and the round goes on without the parent.
func (a *parentAsker) post(ask burlerengine.ParentAsk) {
	seat := a.seat
	if err := seat.Post(ask.Strand, ParentAskLine(seat.Tag, seat.Slug, a.subject(ask.Round), ask)); err != nil {
		logger.Warn("shedadapters: the parent-review request was not delivered; the round goes on without the parent", "producer", a.name, "engine", a.engine, "round", ask.Round, "ask", ask.Number, "error", err)
		if _, _, endErr := burlerengine.EndParentAsk(seat.Files, ask.Number, burlerengine.ParentAskUndelivered, fmt.Sprintf("the request was not delivered: %s", err)); endErr != nil {
			logger.Warn("shedadapters: the undelivered parent-review ask was not set undelivered", "producer", a.name, "engine", a.engine, "ask", ask.Number, "error", endErr)
		}
		return
	}
	if err := burlerengine.MarkParentAskPosted(seat.Files, ask.Number); err != nil {
		logger.Warn("shedadapters: the posted parent-review ask was not set open", "producer", a.name, "engine", a.engine, "ask", ask.Number, "error", err)
	}
}

// subject returns what round's ask names for the parent to review: the seat's subject.
// On a seat that asks before the judge it also names that round's own review and fixer report,
// so the parent sees the findings the round ruled on and what its fixer did with each.
func (a *parentAsker) subject(round int) string {
	if !a.seat.AskBeforeJudge {
		return a.seat.Subject
	}
	return fmt.Sprintf("%s, with round %d's review %s and fixer report %s",
		a.seat.Subject, round, roundReviewPath(a.runDir, round), roundFixerReportPath(a.runDir, round))
}

// ParentAskLine renders the one creel line, opened by tag, that asks the parent to review subject under ask.
// It names the slug, the round, the ask number and the subject.
// It also names the deadline as a wall-clock time and the answer verb with its --ask flag filled in.
func ParentAskLine(tag, slug, subject string, ask burlerengine.ParentAsk) string {
	answer := fmt.Sprintf("lym loom parent-review answer %s --ask %d", slug, ask.Number)
	return fmt.Sprintf(`%s %s round %d ask %d: review %s and answer by %s with "%s --satisfied" or "%s --findings-file <path>"`,
		tag, slug, ask.Round, ask.Number, subject, ask.Deadline.Local().Format(parentDeadlineLayout), answer, answer)
}

// settle settles round's ask once the review at reviewPath is accepted or judged:
// an ask still pending-post or open is set timeout, and a done report not yet copied is copied beside the review.
// A round with no ask settles nothing, and a failure is logged, since the parent seat never fails the round.
func (a *parentAsker) settle(round, number int, reviewPath string) {
	if a.seat == nil || number == 0 {
		return
	}
	copied, err := burlerengine.SettleParentAsk(a.seat.Files, number, reviewPath)
	if err != nil {
		logger.Warn("shedadapters: the round's parent-review ask was not settled", "producer", a.name, "engine", a.engine, "round", round, "ask", number, "error", err)
		return
	}
	if copied != "" {
		logger.Info("shedadapters: copied the parent's report beside the accepted review", "producer", a.name, "engine", a.engine, "round", round, "ask", number, "copy", copied)
	}
}

// parentAsker returns the asker of the row's parent seat, which is nil for a row with none.
func (p *BurlerReviewProducer) parentAsker() *parentAsker {
	return newParentAsker(p.parent, p.name, burlerReviewEngineLabel, p.runDir, p.now)
}

// heldAsk returns the number of the ask the review producer holds for round, 0 when it holds none; it sends nothing.
// A seat that asks before the judge leaves its asks to the Bouncer, so the producer holds none.
func (a *parentAsker) heldAsk(round int) int {
	if a.seat == nil || a.seat.AskBeforeJudge {
		return 0
	}
	record, seed, ok := a.readAsks(round)
	if !ok {
		return 0
	}
	ask, _ := record.RoundAsk(seed, round)
	return ask.Number
}

// heldBeforeJudgeAsk returns the number of the ask the Bouncer holds for round's judge, 0 when it holds none.
// It sends nothing.
// A seat that asks at round start leaves its asks to the review producer, so the Bouncer holds none.
func (a *parentAsker) heldBeforeJudgeAsk(round int) int {
	if a.seat == nil || !a.seat.AskBeforeJudge {
		return 0
	}
	record, seed, ok := a.readAsks(round)
	if !ok {
		return 0
	}
	ask, _ := record.RoundAsk(seed, round)
	return ask.Number
}
