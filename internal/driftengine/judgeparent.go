// judgeparent.go implements the drift judge's ask to the run's parent orch, a gate entry on the judge's table.
// The entry mails the parent the judge's proposed rulings.
// It holds the judge until the parent agrees to those rulings, or the ask's deadline passes.
// The ask lives in burlerengine's never-tracked ask record, beside the review rounds' asks.

package driftengine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// JudgeParentSeat is what the judge's ask to the run's parent orch needs from the run.
type JudgeParentSeat struct {
	// Files locates the run's ask record and the parent's reports.
	Files burlerengine.ParentReviewFiles
	// Slug is the run's slug, which the ask's line and its answer verb name.
	Slug string
	// Resolve answers the orch strand an ask goes to and whether that orch's state records one.
	Resolve func() (strand string, recorded bool, err error)
	// Post sends line to strand as creel mail signed as lym's loom program.
	Post func(strand, line string) error
}

// JudgeAskLineTag opens every line that asks the parent orch to agree with the drift judge's proposed rulings.
const JudgeAskLineTag = "[drift-judge request]"

// ParentAgreementEntryName names the parent agreement entry in a gate report.
const ParentAgreementEntryName = "parent-agreement"

const (
	// judgeAskSeedPrefix leads the seed of a judge's ask, followed by the spawn id, so each spawn has its own asks.
	judgeAskSeedPrefix = "drift-judge "
	// judgeAskRound is the round of a spawn's first ask; each later ask of the spawn counts one more.
	judgeAskRound = 1
	// parentAgreementAttempts is the re-prompt budget of the parent agreement entry.
	// An ask costs one attempt to hold the judge, and a findings answer one more until the judge rewrites its rulings.
	parentAgreementAttempts = 6
	// judgeDeadlineLayout renders an ask's deadline in the line as a wall-clock time.
	judgeDeadlineLayout = "2006-01-02 15:04:05 MST"
	// judgeParentMargin is the time the judge keeps after its wait for the parent ends, to end its turn before its timeout.
	judgeParentMargin = 5 * time.Minute
	// rulingsChangedReason is the reason of an ask set timeout because the judge's rulings changed under it.
	rulingsChangedReason = "the judge's rulings changed before the parent answered"
)

// judgeAskSeed returns the seed of the asks the judge spawn spawnID makes.
func judgeAskSeed(spawnID string) string {
	return judgeAskSeedPrefix + spawnID
}

// judgeAskLimit returns the latest deadline an ask of the judge run started at startedAt may carry:
// the run's timeout less judgeParentMargin.
// The zero time, which a judge that has not started gives, has no limit.
func judgeAskLimit(startedAt time.Time) time.Time {
	if startedAt.IsZero() {
		return time.Time{}
	}
	return startedAt.Add(judgeTimeout - judgeParentMargin)
}

// rulingsDigest returns the digest of judgement's rulings, which names what an ask put to the parent.
// It reads the parsed rulings, so a rewrite of the file that changes no ruling or reason keeps the digest.
func rulingsDigest(judgement Judgement) (string, error) {
	encoded, err := json.Marshal(judgement)
	if err != nil {
		return "", fmt.Errorf("drift: digest the judgement's rulings: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// heldJudgeAsk reports whether the judge spawn spawnID already holds an ask in the seat's record.
func (s *JudgeParentSeat) heldJudgeAsk(spawnID string) (bool, error) {
	record, _, err := burlerengine.ReadParentAsks(s.Files)
	if err != nil {
		return false, err
	}
	_, held := record.LatestOfSeed(judgeAskSeed(spawnID))
	return held, nil
}

// ParentAgreementGateEntry returns the gate entry that holds the judge until the run's parent orch answers its rulings.
// It follows the judgement entry, so it reads a judgement.yaml at judgementPath that parses against want.
// An ask records the digest of the rulings it presented.
// A spawn's first evaluation records an ask whose deadline is wait from now, or limit when that is earlier.
// It mails the parent the rulings and fails, telling the judge to await the answer.
// A limit at or before now leaves the parent no time: the ask is recorded not-sent and the entry passes.
// The zero limit sets none.
// An ask still open before its deadline fails the same way, and one at or past its deadline is set timeout and passes.
// A done ask whose report holds findings fails while judgement.yaml still holds the rulings it answered.
// A judgement that differs from the one the latest ask presented has no answer to pass on:
// an open ask is set timeout, and a new ask naming the new rulings goes out in its place.
// A satisfied answer to the current rulings passes, and so does an ask that never reached the parent:
// the judge goes on without it.
// A held ask that was cleared answers nothing, so a new ask of the next round goes out in its place.
func ParentAgreementGateEntry(seat *JudgeParentSeat, want JudgementWant, judgementPath string, wait time.Duration, limit time.Time, now func() time.Time) shuttleengine.GateEntry {
	return shuttleengine.GateEntry{Name: ParentAgreementEntryName, Gate: seat.agreementGate(want, judgementPath, wait, limit, now), Attempts: parentAgreementAttempts}
}

// agreementGate returns the closure of the parent agreement entry; see ParentAgreementGateEntry.
// An unreadable judgement, ask record or report is a gate error.
// The entry runs only after the judgement parsed, and the record and report are lym's own files.
func (s *JudgeParentSeat) agreementGate(want JudgementWant, judgementPath string, wait time.Duration, limit time.Time, now func() time.Time) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		data, err := os.ReadFile(judgementPath)
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("drift: read the judgement for the parent's ask: %w", err)
		}
		judgement, err := ParseJudgement(data, want)
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("drift: parse the judgement for the parent's ask: %w", err)
		}
		digest, err := rulingsDigest(judgement)
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		line := func(ask burlerengine.ParentAsk) string {
			return JudgeAskLine(s.Slug, judgementPath, judgementSummary(judgement), ask)
		}
		seed := judgeAskSeed(want.SpawnID)
		record, _, err := burlerengine.ReadParentAsks(s.Files)
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		ask, held := record.LatestOfSeed(seed)
		round := judgeAskRound
		if held && ask.Cleared {
			held, round = false, ask.Round+1
		}
		if held && ask.Digest != digest {
			switch {
			case !ask.State.Ended():
				if _, _, err := burlerengine.EndParentAsk(s.Files, ask.Number, burlerengine.ParentAskTimeout, rulingsChangedReason); err != nil {
					return shuttleengine.GateResult{}, err
				}
				held, round = false, ask.Round+1
			case ask.State == burlerengine.ParentAskDone:
				held, round = false, ask.Round+1
			}
		}
		switch {
		case !held:
			if ask, err = s.sendJudgeAsk(seed, round, digest, wait, limit, now(), line); err != nil {
				return shuttleengine.GateResult{}, err
			}
		case ask.State == burlerengine.ParentAskPendingPost:
			ask = s.postJudgeAsk(ask, line(ask))
		}
		return s.agreement(ask, judgementPath, now())
	}
}

// sendJudgeAsk records the spawn's ask for round under seed, presenting the rulings with digest, and posts its line.
// It returns the ask as it then stands.
// The deadline is wait from now, or limit when that is earlier.
// A limit at or before now records the ask not-sent.
// A parent that does not resolve, or records no orch strand, records it no-parent.
// Neither posts anything.
func (s *JudgeParentSeat) sendJudgeAsk(seed string, round int, digest string, wait time.Duration, limit, now time.Time, line func(burlerengine.ParentAsk) string) (burlerengine.ParentAsk, error) {
	ask := burlerengine.ParentAsk{Round: round, Seed: seed, Tag: JudgeAskLineTag, Digest: digest, AskedAt: now}
	deadline := now.Add(wait)
	capped := !limit.IsZero() && deadline.After(limit)
	if capped && !limit.After(now) {
		ask.State, ask.Reason = burlerengine.ParentAskNotSent, fmt.Sprintf("%s leaves the parent no time: the judge's own timeout ends its session by %s", keyName(judgeParentTimeoutKey), limit.UTC().Format(time.RFC3339))
	} else {
		strand, recorded, err := s.Resolve()
		switch {
		case err != nil:
			ask.State, ask.Reason = burlerengine.ParentAskNoParent, fmt.Sprintf("the run's parent orch could not be resolved: %s", err)
		case !recorded:
			ask.State, ask.Reason = burlerengine.ParentAskNoParent, "no orch strand is recorded at the run's parent"
		default:
			if capped {
				logger.Warn("drift: "+keyName(judgeParentTimeoutKey)+" is capped to leave the judge room to end its turn before its own timeout", "key", keyName(judgeParentTimeoutKey), "value", wait, "capped", limit.Sub(now))
				deadline = limit
			}
			ask.State, ask.Strand, ask.Deadline = burlerengine.ParentAskPendingPost, strand, deadline
		}
	}
	recordedAsk, err := burlerengine.RecordParentAsk(s.Files, ask)
	if err != nil {
		return burlerengine.ParentAsk{}, err
	}
	if recordedAsk.State != burlerengine.ParentAskPendingPost {
		logger.Warn("drift: the judge's ask to the parent did not go out; the judge goes on without the parent", "ask", recordedAsk.Number, "state", recordedAsk.State, "reason", recordedAsk.Reason)
		return recordedAsk, nil
	}
	return s.postJudgeAsk(recordedAsk, line(recordedAsk)), nil
}

// postJudgeAsk posts line to the strand ask records and returns ask set open.
// A failed post sets the ask undelivered with the error as its reason, and the judge goes on without the parent.
func (s *JudgeParentSeat) postJudgeAsk(ask burlerengine.ParentAsk, line string) burlerengine.ParentAsk {
	if err := s.Post(ask.Strand, line); err != nil {
		reason := fmt.Sprintf("the request was not delivered: %s", err)
		logger.Warn("drift: the judge's ask to the parent was not delivered; the judge goes on without the parent", "ask", ask.Number, "error", err)
		if _, _, endErr := burlerengine.EndParentAsk(s.Files, ask.Number, burlerengine.ParentAskUndelivered, reason); endErr != nil {
			logger.Warn("drift: the undelivered ask to the parent was not set undelivered", "ask", ask.Number, "error", endErr)
		}
		ask.State, ask.Reason = burlerengine.ParentAskUndelivered, reason
		return ask
	}
	if err := burlerengine.MarkParentAskPosted(s.Files, ask.Number); err != nil {
		logger.Warn("drift: the posted ask to the parent was not set open", "ask", ask.Number, "error", err)
	}
	ask.State = burlerengine.ParentAskOpen
	return ask
}

// agreement judges the entry against ask, which presented the judgement's current rulings, as it stands at now;
// see ParentAgreementGateEntry.
func (s *JudgeParentSeat) agreement(ask burlerengine.ParentAsk, judgementPath string, now time.Time) (shuttleengine.GateResult, error) {
	switch ask.State {
	case burlerengine.ParentAskPendingPost, burlerengine.ParentAskOpen:
		if now.Before(ask.Deadline) {
			return shuttleengine.GateResult{Findings: awaitParentFindings(s.Slug, judgementPath, ask)}, nil
		}
		ended, _, err := burlerengine.EndParentAsk(s.Files, ask.Number, burlerengine.ParentAskTimeout, "")
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if ended.State == burlerengine.ParentAskDone {
			return s.agreement(ended, judgementPath, now)
		}
	case burlerengine.ParentAskDone:
		report, err := burlerengine.ReadParentReport(ask.Report)
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if len(report.Findings) > 0 {
			return shuttleengine.GateResult{Findings: fmt.Sprintf(
				"The parent's report %s changes your rulings, and judgement.yaml (%s) still holds the rulings it answered.\n\n"+
					"Rewrite judgement.yaml so each ruling a finding of the report names is as the parent says, with your reason, then end your turn.",
				ask.Report, judgementPath)}, nil
		}
	}
	return shuttleengine.GateResult{Passed: true}, nil
}

// awaitParentFindings returns the findings that tell the judge to wait for the parent's answer to ask.
func awaitParentFindings(slug, judgementPath string, ask burlerengine.ParentAsk) string {
	return fmt.Sprintf(
		"Your proposed rulings went to the run's parent orch as ask %d, which it answers by %s.\n\n"+
			"Wait for the answer: run `lym loom parent-review await %s` in the foreground with a shell command timeout above its cap; "+
			"while it answers `ready: false`, run it again in a turn that does nothing else.\n"+
			"When it answers `state: done`, read the parent's report at the path its `report` names. "+
			"When the report holds findings, rewrite judgement.yaml (%s) so each ruling a finding names is as the parent says, with your reason. "+
			"Then end your turn, and the parent is asked again about the rewritten rulings; with any other state, end it with your rulings as they stand.",
		ask.Number, ask.Deadline.Local().Format(judgeDeadlineLayout), slug, judgementPath)
}

// JudgeAskLine renders the one creel line that asks the parent to agree with the rulings in judgementPath under ask.
// It names the slug, the ask number, the judgement file, summary and the deadline as a wall-clock time.
// It also names the answer verb with its --ask flag filled in, in both forms.
func JudgeAskLine(slug, judgementPath, summary string, ask burlerengine.ParentAsk) string {
	answer := fmt.Sprintf("lym loom parent-review answer %s --ask %d", slug, ask.Number)
	return fmt.Sprintf(`%s %s ask %d: the drift judge proposes these rulings, each with its reason in %s: %s. Answer by %s with "%s --satisfied" to agree, or "%s --findings-file <path>" whose findings each name a cell, case or file id and the ruling it should have`,
		JudgeAskLineTag, slug, ask.Number, judgementPath, summary, ask.Deadline.Local().Format(judgeDeadlineLayout), answer, answer)
}

// judgementSummary renders the rulings of judgement as one clause per kind, cells, cases and files, joined by semicolons.
// Cells and files are counted by ruling, naming each cell that does not match and each file holding an uncaused check;
// every case is named with its ruling.
func judgementSummary(judgement Judgement) string {
	var parts []string
	if len(judgement.Cells) > 0 {
		var noMatch []string
		for _, cell := range judgement.Cells {
			if cell.Ruling == RulingNoMatch {
				noMatch = append(noMatch, cell.ID)
			}
		}
		part := fmt.Sprintf("%d cells, %d %s and %d %s", len(judgement.Cells), len(judgement.Cells)-len(noMatch), RulingMatches, len(noMatch), RulingNoMatch)
		if len(noMatch) > 0 {
			part += fmt.Sprintf(" (%s)", strings.Join(noMatch, ", "))
		}
		parts = append(parts, part)
	}
	if len(judgement.Cases) > 0 {
		cases := make([]string, len(judgement.Cases))
		for i, ruling := range judgement.Cases {
			cases[i] = fmt.Sprintf("case %s %s", ruling.Case, ruling.Ruling)
		}
		parts = append(parts, strings.Join(cases, ", "))
	}
	if len(judgement.Files) > 0 {
		loosened := 0
		var uncaused []string
		for _, file := range judgement.Files {
			if file.Ruling == RulingLoosened {
				loosened++
			}
			for _, check := range file.Checks {
				if check.Ruling == RulingUncaused {
					uncaused = append(uncaused, file.ID)
					break
				}
			}
		}
		part := fmt.Sprintf("%d files, %d %s and %d %s", len(judgement.Files), len(judgement.Files)-loosened, RulingNone, loosened, RulingLoosened)
		if len(uncaused) > 0 {
			part += fmt.Sprintf(" (an uncaused check in %s)", strings.Join(uncaused, ", "))
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return "no rulings"
	}
	return strings.Join(parts, "; ")
}
