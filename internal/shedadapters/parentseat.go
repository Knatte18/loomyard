// parentseat.go implements the parent seat of a review row.
// It decides which round asks the run's parent orch to review.
// It posts the ask's one creel line and settles a round's ask once its review is accepted.
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
// A round of the segment may ask the run's parent orch to review the decision record beside the reviewer.
// The chair keeps every finding the parent reports.
type ParentReviewSeat struct {
	// Files locates the run's ask record and the parent's reports.
	Files burlerengine.ParentReviewFiles
	// Slug is the run's slug and RecordPath the decision record's path in the task worktree; the ask's line names both.
	Slug       string
	RecordPath string
	// AskEnabled reads discussion_review_ask_parent for the round about to start.
	// It decides only whether the round sends a new ask; an ask the round already holds is carried through whatever it reads.
	AskEnabled func() bool
	// Wait returns the time from an ask to its deadline,
	// and a non-empty reason when the chair's timeout leaves the parent no time, so the round sends no ask.
	Wait func() (time.Duration, string)
	// RecordWrittenAt returns the latest time the run's history records a Discussion-Write call, the zero time when none.
	RecordWrittenAt func() (time.Time, error)
	// Resolve answers the orch strand an ask goes to and whether that orch's state records one.
	Resolve func() (strand string, recorded bool, err error)
	// Post sends line to strand as creel mail signed as lym's loom program.
	Post func(strand, line string) error
}

// ParentAskLineTag opens every line that asks the parent orch to review a Discussion-Review round.
const ParentAskLineTag = "[discussion-review request]"

// parentDeadlineLayout renders an ask's deadline in the line as a wall-clock time.
const parentDeadlineLayout = "2006-01-02 15:04:05 MST"

// roundSeed returns the identity of runDir's review generation: the write time of its round-1 focus file.
// The time is in RFC 3339 with nanoseconds.
// A fresh start, a new generation and a clear each write that file anew; a goto, a resume and a restarted row keep it.
func roundSeed(runDir string) (string, error) {
	info, err := os.Stat(focusPath(runDir, 1))
	if err != nil {
		return "", err
	}
	return info.ModTime().UTC().Format(time.RFC3339Nano), nil
}

// readParentAsks returns the run's ask record and the seed of runDir's generation.
// ok is false when the row has no parent seat or either read failed, which it logs.
func (p *BurlerReviewProducer) readParentAsks(round int) (record burlerengine.ParentAskRecord, seed string, ok bool) {
	if p.parent == nil {
		return burlerengine.ParentAskRecord{}, "", false
	}
	seed, err := roundSeed(p.runDir)
	if err != nil {
		logger.Warn("shedadapters: the review generation's seed is unreadable; the round goes on without the parent", "producer", p.name, "engine", burlerReviewEngineLabel, "round", round, "error", err)
		return burlerengine.ParentAskRecord{}, "", false
	}
	record, _, err = burlerengine.ReadParentAsks(p.parent.Files)
	if err != nil {
		logger.Warn("shedadapters: the parent-review ask record is unreadable; the round goes on without the parent", "producer", p.name, "engine", burlerReviewEngineLabel, "round", round, "error", err)
		return burlerengine.ParentAskRecord{}, "", false
	}
	return record, seed, true
}

// roundParentAsk returns the number of round's parent-review ask, sending a new ask when the round asks.
// It returns 0 when the round holds none.
// An ask the round already holds is never sent again, whatever discussion_review_ask_parent now reads.
// One still pending-post is posted again to the strand it records.
// A new ask goes out only when the key is on and parentAskDue says the round asks.
// A failure to read or write the ask record is logged, and the round goes on without the parent.
func (p *BurlerReviewProducer) roundParentAsk(round int) int {
	record, seed, ok := p.readParentAsks(round)
	if !ok {
		return 0
	}
	if ask, held := record.RoundAsk(seed, round); held {
		if ask.State == burlerengine.ParentAskPendingPost {
			p.postParentAsk(ask)
		}
		return ask.Number
	}
	if !p.parent.AskEnabled() || !p.parentAskDue(record, seed, round) {
		return 0
	}
	return p.sendParentAsk(seed, round)
}

// parentAskDue reports whether round of the generation seed asks the parent, read against the run's latest ask.
// It asks when the run has no ask yet or the latest ask has another seed.
// It asks when the run's history records a Discussion-Write call after the latest ask.
// Within the same seed it asks when the round before ended done with findings, so the parent confirms their fixes.
// Within the same seed it asks when an earlier round's ask was not-sent, so a raised chair timeout takes effect.
func (p *BurlerReviewProducer) parentAskDue(record burlerengine.ParentAskRecord, seed string, round int) bool {
	latest, ok := record.Latest()
	if !ok || latest.Seed != seed {
		return true
	}
	rewrittenAt, err := p.parent.RecordWrittenAt()
	switch {
	case err != nil:
		logger.Warn("shedadapters: the run's history is unreadable; judging the round as though the record was not rewritten", "producer", p.name, "engine", burlerReviewEngineLabel, "round", round, "error", err)
	case rewrittenAt.After(latest.AskedAt):
		return true
	}
	switch {
	case latest.Round == round-1 && latest.State == burlerengine.ParentAskDone:
		report, err := burlerengine.ReadParentReport(latest.Report)
		if err != nil {
			logger.Warn("shedadapters: the previous round's parent report is unreadable; asking the parent again", "producer", p.name, "engine", burlerReviewEngineLabel, "round", round, "error", err)
			return true
		}
		return len(report.Findings) > 0
	case latest.Round < round && latest.State == burlerengine.ParentAskNotSent:
		return true
	}
	return false
}

// sendParentAsk records round's new ask and posts it, returning its number, or 0 when the record could not be written.
// A wait the chair's timeout leaves no room for records the ask not-sent with that reason,
// and a parent that does not resolve, or records no orch strand, records it no-parent; neither posts anything.
// Otherwise the destination strand is resolved once.
// The ask is recorded pending-post with its deadline and that strand, then posted.
func (p *BurlerReviewProducer) sendParentAsk(seed string, round int) int {
	seat := p.parent
	now := p.now()
	ask := burlerengine.ParentAsk{Round: round, Seed: seed, AskedAt: now}
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
		logger.Warn("shedadapters: the round's parent-review ask was not recorded; the round goes on without the parent", "producer", p.name, "engine", burlerReviewEngineLabel, "round", round, "error", err)
		return 0
	}
	if recorded.State == burlerengine.ParentAskPendingPost {
		p.postParentAsk(recorded)
	} else {
		logger.Warn("shedadapters: the round's parent-review ask did not go out; the round goes on without the parent", "producer", p.name, "engine", burlerReviewEngineLabel, "round", round, "ask", recorded.Number, "state", recorded.State, "reason", recorded.Reason)
	}
	return recorded.Number
}

// postParentAsk posts ask's line to the strand it records and sets the ask open.
// A failed post sets the ask no-parent with the error as its reason, and the round goes on without the parent.
func (p *BurlerReviewProducer) postParentAsk(ask burlerengine.ParentAsk) {
	seat := p.parent
	if err := seat.Post(ask.Strand, ParentAskLine(seat.Slug, seat.RecordPath, ask)); err != nil {
		logger.Warn("shedadapters: the parent-review request was not delivered; the round goes on without the parent", "producer", p.name, "engine", burlerReviewEngineLabel, "round", ask.Round, "ask", ask.Number, "error", err)
		if _, _, endErr := burlerengine.EndParentAsk(seat.Files, ask.Number, burlerengine.ParentAskNoParent, fmt.Sprintf("the request was not delivered: %s", err)); endErr != nil {
			logger.Warn("shedadapters: the undelivered parent-review ask was not set no-parent", "producer", p.name, "engine", burlerReviewEngineLabel, "ask", ask.Number, "error", endErr)
		}
		return
	}
	if err := burlerengine.MarkParentAskPosted(seat.Files, ask.Number); err != nil {
		logger.Warn("shedadapters: the posted parent-review ask was not set open", "producer", p.name, "engine", burlerReviewEngineLabel, "ask", ask.Number, "error", err)
	}
}

// ParentAskLine renders the one creel line that asks the parent to review the decision record at recordPath under ask.
// It names the slug, the round, the ask number, and the record's path.
// It also names the deadline as a wall-clock time and the answer verb with its --ask flag filled in.
func ParentAskLine(slug, recordPath string, ask burlerengine.ParentAsk) string {
	answer := fmt.Sprintf("lym loom parent-review answer %s --ask %d", slug, ask.Number)
	return fmt.Sprintf(`%s %s round %d ask %d: review the decision record %s and answer by %s with "%s --satisfied" or "%s --findings-file <path>"`,
		ParentAskLineTag, slug, ask.Round, ask.Number, recordPath, ask.Deadline.Local().Format(parentDeadlineLayout), answer, answer)
}

// settleRoundParentAsk settles round's ask once the review at reviewPath is accepted:
// an ask still pending-post or open is set timeout, and a done report not yet copied is copied beside the review.
// A round with no ask settles nothing, and a failure is logged, since the parent seat never fails the round.
func (p *BurlerReviewProducer) settleRoundParentAsk(round, number int, reviewPath string) {
	if p.parent == nil || number == 0 {
		return
	}
	copied, err := burlerengine.SettleParentAsk(p.parent.Files, number, reviewPath)
	if err != nil {
		logger.Warn("shedadapters: the round's parent-review ask was not settled", "producer", p.name, "engine", burlerReviewEngineLabel, "round", round, "ask", number, "error", err)
		return
	}
	if copied != "" {
		logger.Info("shedadapters: copied the parent's report beside the accepted review", "producer", p.name, "engine", burlerReviewEngineLabel, "round", round, "ask", number, "copy", copied)
	}
}

// heldParentAsk returns the number of the ask round already holds, 0 when it holds none; it sends nothing.
func (p *BurlerReviewProducer) heldParentAsk(round int) int {
	record, seed, ok := p.readParentAsks(round)
	if !ok {
		return 0
	}
	ask, _ := record.RoundAsk(seed, round)
	return ask.Number
}
