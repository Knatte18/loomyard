// verify.go implements the Drift-Verify producer, which checks the captured code changes against the sealed expectation.
// One judge rules on the expected changes that no stored ruling covers.
// It also rules on each loosened check of the review range's files that no stored ruling covers.

package driftengine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// The recipe rows that edit code; a Stuck's way forward routes the parent back to one of them.
const (
	darnWriteRow      = "Darn-Write"
	samplerSessionRow = "Sampler-Session"
	websterRow        = "Webster"
)

// Reasons of Stuck passes that name no changed value; each states the parent's way forward.
const (
	recordsMismatchReason = "the drift records on disk differ from their newest committed copy; way forward: the parent runs lym drift amend <slug> --restore, then lym loom resume"
	neverCountedReason    = "the baseline capture never counted, so there is no baseline to check against; way forward: the parent runs lym drift defer, then lym loom resume"
	snapshotGoneReason    = "the snapshot of both checkouts the judge was spawned under went while its session was live, so what that session changed cannot be told; way forward: the parent or the operator checks both checkouts for the judge's edits, then runs lym loom resume, which takes a new snapshot and resumes the judge"
)

// DriftVerify is the Drift-Verify ShedProducer, which checks the captured code changes against the sealed expectation.
// The Drift-Recheck row runs the same engine.
type DriftVerify struct {
	name      string
	row       Row
	judgement shuttleengine.GateEntry
	deps      Deps
	// judgeStartedAt is when the judge run of the pass in progress starts; the zero time before any judge run.
	// A fresh spawn and an attach to a live chair each start a fresh run deadline.
	judgeStartedAt time.Time
}

var _ shedengine.ShedProducer = (*DriftVerify)(nil)

// NewDriftVerify returns the Drift-Verify producer identified as name.
// judgement is the row's resolved `judgement` gate entry, which the judge's table runs at each turn end.
func NewDriftVerify(name string, row Row, judgement shuttleengine.GateEntry, deps Deps) *DriftVerify {
	return &DriftVerify{name: name, row: row, judgement: judgement, deps: deps}
}

// Call runs one Drift-Verify pass.
// With drift switched off it is Done at once.
// Otherwise it first ends the live chairs of the removed drift-baseline and drift-verify agents.
// It then runs these steps in order, each ending the pass when it answers:
// no scope record, scope off, a deferred capture, a waiver and a head already checked with no amendment since are Done;
// a records mismatch, a capture that never counted and a baseline that is missing or edited are a Stuck for the parent;
// an open judge session whose checkouts changed since, or whose snapshot is gone under a live chair, is a Stuck;
// one whose snapshot is gone with no live chair takes a new snapshot and is resumed, and so is one that is intact;
// a failed current capture, a seal fault and any change the expectation does not cover are a Stuck;
// a change whose exact old and new value has a stored ruling reuses it;
// a stored ruling that does not match is a Stuck.
// With no change left to judge the check passes; otherwise one judge rules on the rest, and its rulings are stored.
// Under a recorded diff of command the declared compare replaces the capture and the diff.
// An exit 0 is Done, and a failing compare is ruled on case by case by one judge.
// Where the review base is told, each file of the review range that exists at the base is also listed,
// and the judge rules on the loosened checks of every file that no stored ruling still covers.
// A loosened check with no measured cause is a Stuck for the parent.
// A stored ruling of the same code head stands until an amendment follows it.
// Every Stuck for the parent opens with shedadapters.DecisionHaltPrefix.
// A Stuck that names `lym drift defer` carries DeferHaltPrefix right after it.
// An exhausted `judgement` entry is the shed's Stuck, and its resume respawns the judge under the same spawn id.
func (p *DriftVerify) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	d := p.deps
	err := requireSeams(p.name,
		seam{"Config", d.Config != nil},
		seam{"Status", d.Status != nil},
		seam{"Head", d.Head != nil},
		seam{"Seats", d.Seats != nil},
		seam{"CommitWithTrailers", d.CommitWithTrailers != nil},
		seam{"TrailerCommit", d.TrailerCommit != nil},
		seam{"FileAt", d.FileAt != nil},
		seam{"CaptureCurrent", d.CaptureCurrent != nil},
		seam{"JudgeTable", d.JudgeTable != nil},
		seam{"Checkouts", d.Checkouts != nil},
		seam{"RecordsChangedSince", d.RecordsChangedSince != nil},
		seam{"EndRoles", d.EndRoles != nil},
		seam{"ChangedFiles", d.ChangedFiles != nil},
		seam{"CommitMessages", d.CommitMessages != nil},
		seam{"RangeDiff", d.RangeDiff != nil},
	)
	if err == nil && p.judgement.Gate == nil {
		err = fmt.Errorf("drift: %s is wired without a judgement gate entry", p.name)
	}
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if outcome, pointer, stop := passWhenSwitchedOff(d.Config); stop {
		return outcome, pointer, nil
	}
	if err := d.EndRoles([]string{baselineRole, verifyRole}); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("drift: %s: end the chairs of the removed drift agents: %w", p.name, err)
	}
	return p.pass(ctx)
}

// doneBecause returns a Done carrying why no agent was needed.
func doneBecause(why string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return shedengine.Done, shedengine.OutputPointer{Reason: why}, nil
}

// scopeOffReason words the Done of a scope that is off, naming the config key that set it when the record holds one.
func scopeOffReason(scope ScopeRecord) string {
	if scope.ScopeSource != "" {
		return "the scope is off, set by " + scope.ScopeSource
	}
	return "the scope is off"
}

// pass reads the records, decides the pass from them, and then checks the captured changes.
func (p *DriftVerify) pass(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	d := p.deps
	rec, err := ReadRecords(d)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	switch {
	case !rec.ScopeFound:
		return doneBecause("no scope is recorded, so there is nothing to check")
	case rec.Mismatch:
		return haltForParent(recordsMismatchReason)
	case rec.Scope.Scope == ScopeOff:
		return doneBecause(scopeOffReason(rec.Scope))
	case rec.Scope.Deferred:
		return doneBecause("the baseline capture was deferred, so there is no baseline to check against")
	case rec.Scope.WaivedAt != "":
		return doneBecause("the check is waived: " + rec.Scope.WaivedReason)
	}

	if rec.Scope.Diff == "" {
		if rec, err = p.recordDiffMode(); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
	}
	if rec.Scope.AwaitedAt == "" {
		return haltForParent(DeferHaltPrefix + neverCountedReason)
	}
	baseline, err := CheckBaseline(d)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if baseline.Fault != "" {
		return haltForParent(baseline.Fault)
	}

	head, err := d.Head()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("drift: %s: read the code HEAD: %w", p.name, err)
	}
	if rec.Scope.LastPassHead != "" && head == rec.Scope.LastPassHead && rec.Scope.LastPassAmendments == len(rec.Scope.Amendments) {
		return doneBecause("no change since the check that passed at " + head)
	}

	if rec.Scope.JudgeSession.Open && !rec.Scope.SnapshotLost {
		reason, err := p.settleOpenSession(rec.Scope.JudgeSession)
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		if reason != "" {
			return haltForParent(reason)
		}
		if rec, err = ReadRecords(d); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
	}
	if err := p.recordUnsealed(rec); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if rec.Scope.Diff == DiffCommand {
		report, err := Check(ctx, p.deps)
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		if report.Skip != "" {
			return "", shedengine.OutputPointer{}, fmt.Errorf("drift: %s: the check was skipped, which a run at this row never is: %s", p.name, report.Skip)
		}
		if reason := p.faultReason(report); reason != "" {
			return haltForParent(reason)
		}
		return p.passCommand(ctx, rec, report)
	}
	return p.checkAndJudge(ctx, rec, head)
}

// recordDiffMode writes the config's diff mode, key patterns and volatile lists into a scope record with no mode,
// so the run keeps them.
// It returns the records as written.
func (p *DriftVerify) recordDiffMode() (Records, error) {
	d := p.deps
	cfg, err := d.Config()
	if err != nil {
		return Records{}, fmt.Errorf("drift: %s: load the drift config: %w", p.name, err)
	}
	err = WriteRecords(d, func(scope *ScopeRecord, _ *JudgedRecord) error {
		scope.Diff, scope.KeyPatterns, scope.Volatile = cfg.Diff, cfg.KeyPatterns, cfg.Volatile
		return nil
	})
	if err != nil {
		return Records{}, err
	}
	return ReadRecords(d)
}

// recordUnsealed marks the scope `Unsealed` once, for a run with neither a seal commit nor an expected.yaml.
// The mark makes containment read the empty expectation.
// A run with an expected.yaml and no seal commit stays unmarked, so the seal check faults on it.
// A run whose row is not past Drift-Expect also stays unmarked, and the check refuses such a run.
func (p *DriftVerify) recordUnsealed(rec Records) error {
	d := p.deps
	if rec.Scope.Unsealed {
		return nil
	}
	_, sealed, err := d.TrailerCommit(SealTrailerKey, false)
	if err != nil {
		return fmt.Errorf("drift: %s: find the oldest %s commit: %w", p.name, SealTrailerKey, err)
	}
	if sealed {
		return nil
	}
	if _, present, err := readRecordFile(d.Paths.Expected); err != nil || present {
		return err
	}
	status, err := d.Status()
	if err != nil {
		return fmt.Errorf("drift: %s: read the run status: %w", p.name, err)
	}
	if !PastRow(d.Rows, status.CurrentProducer, expectRow) {
		return nil
	}
	return WriteRecords(d, func(scope *ScopeRecord, _ *JudgedRecord) error {
		scope.Unsealed = true
		return nil
	})
}

// checkAndJudge runs the drift check and maps its report.
// A fault or an unexpected change is a Stuck.
// Stored rulings are reused, and the changes and files left over go to the judge.
// A stored ruling that a loosened check has no measured cause is a Stuck with no judge.
// That holds while the range's diff and trailer lines are unchanged.
func (p *DriftVerify) checkAndJudge(ctx context.Context, rec Records, head string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	report, err := Check(ctx, p.deps)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if report.Skip != "" {
		return "", shedengine.OutputPointer{}, fmt.Errorf("drift: %s: the check was skipped, which a run at this row never is: %s", p.name, report.Skip)
	}
	if reason := p.faultReason(report); reason != "" {
		return haltForParent(reason)
	}
	if unexpected := report.Unexpected(); len(unexpected) > 0 {
		described := make([]string, len(unexpected))
		for i, cell := range unexpected {
			described[i] = describeCell(cell)
		}
		if err := p.recordCheck(p.checkResult(report, head, false, 0, 0), nil); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		return haltForParent(fmt.Sprintf("the code changed what the expectation does not cover: %s; way forward: the parent runs lym drift amend <slug> --entries <file> to expect an intended change, then lym loom resume; or fix the code with %s", strings.Join(described, "; "), p.fixRoute()))
	}

	var toJudge []ClassifiedCell
	var ruledNoMatch []ruledCell
	reused := 0
	for _, cell := range report.Cells {
		tuple, stored := storedRuling(rec.Judged, cell.Cell)
		switch {
		case !stored:
			toJudge = append(toJudge, cell)
		case tuple.Ruling == RulingMatches:
			reused++
		default:
			ruledNoMatch = append(ruledNoMatch, ruledCell{cell: cell, reason: tuple.Reason})
		}
	}
	if len(ruledNoMatch) > 0 {
		if err := p.recordCheck(p.checkResult(report, head, false, 0, reused), nil); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		reason, err := p.cellsNoMatchReason("a change the judge already ruled does not match its expectation", ruledNoMatch)
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		return haltForParent(reason)
	}
	listed, err := p.listFiles(rec)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if len(toJudge) == 0 && len(listed.unruled) == 0 {
		return p.passWithoutJudge(p.checkResult(report, head, true, 0, reused), listed, "the check passed: every change is expected and ruled to match")
	}
	return p.judge(ctx, rec, report, toJudge, listed, reused, head)
}

// passWithoutJudge records result with the loosened checks the stored file rulings hold, when no judge is needed.
// A stored check with no measured cause turns the pass into a Stuck; otherwise the pass is Done for why.
func (p *DriftVerify) passWithoutJudge(result CheckResult, listed listedFiles, why string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	checks := listed.allChecks(nil)
	uncaused := uncausedChecks(checks)
	result.Loosened = checks
	result.Passed = result.Passed && len(uncaused) == 0
	if err := p.recordCheck(result, closeJudgeSession); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if len(uncaused) > 0 {
		return haltForParent(uncausedReason(uncaused))
	}
	return doneBecause(why)
}

// faultReason returns the Stuck reason for a fault in the report, or empty when the report holds none.
// A failed current capture names its reason, its log and the ways forward.
func (p *DriftVerify) faultReason(report CheckReport) string {
	switch {
	case report.BaselineFault != "":
		return report.BaselineFault
	case report.RecordsFault != "":
		return report.RecordsFault
	case report.SealFault != "":
		return report.SealFault
	case report.CaptureFault != "":
		return DeferHaltPrefix + fmt.Sprintf("the current capture did not pass: %s; log %s; way forward: fix the cause the reason names, then lym loom resume; or fix the code with %s; or the parent runs lym drift defer, then lym loom resume", report.Capture.Reason, report.Capture.Log, p.fixRoute())
	}
	return ""
}

// noMatchReason words a Stuck over findings of changes ruled not to match, and writes every finding to the findings file.
// every holds one finding per ruled cell or case, and shown the findings as the reason carries them.
// The reason opens with lead, the count of findings and the way forward:
// the parent's amendment and the route that fixes code.
// It then carries shown, capped at haltFindingsLimit characters.
// It names the findings file by its worktree-relative path.
func (p *DriftVerify) noMatchReason(lead string, every, shown []string) (string, error) {
	rel, err := writeFindingsFile(p.deps, lead, every)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s, %d findings; way forward: the parent runs lym drift amend <slug> --entries <file> with entries that explain the change, then lym loom resume; or fix the code with %s; findings: %s; every finding is in %s",
		lead, len(every), p.fixRoute(), cappedFindings(shown), rel), nil
}

// cellsNoMatchReason words a Stuck over cells ruled not to match.
// The reason carries the cells grouped by case, key, kind and reason, and the findings file holds each cell.
func (p *DriftVerify) cellsNoMatchReason(lead string, ruled []ruledCell) (string, error) {
	every := make([]string, len(ruled))
	for i, item := range ruled {
		every[i] = describeRuledCell(item)
	}
	return p.noMatchReason(lead, every, groupRuledCells(ruled))
}

// fixRoute returns the parent's route to the row that edits code.
// That row is Darn-Write or Sampler-Session when the run's rows hold it.
// Otherwise it is Webster, whose Master meets the cells in its run-level drift entry.
func (p *DriftVerify) fixRoute() string {
	row := websterRow
	for _, writer := range []string{darnWriteRow, samplerSessionRow} {
		if slices.Contains(p.deps.Rows, writer) {
			row = writer
		}
	}
	return "lym loom goto --to " + row + " then lym loom resume"
}

// storedRuling returns the newest stored ruling whose case, identity, kind and exact old and new value are the cell's.
func storedRuling(judged JudgedRecord, cell Cell) (JudgedTuple, bool) {
	for _, tuple := range slices.Backward(judged.Tuples) {
		if tuple.Case == cell.Case && tuple.Identity == cell.Identity && tuple.Kind == cell.Kind && tuple.Old == cell.Old && tuple.New == cell.New {
			return tuple, true
		}
	}
	return JudgedTuple{}, false
}

// checkResult builds the record of a check over report at head.
// It counts the cells the judge ruled matching and the cells a stored ruling decided.
func (p *DriftVerify) checkResult(report CheckReport, head string, passed bool, matched, reused int) CheckResult {
	result := CheckResult{
		Head:             head,
		Passed:           passed,
		At:               stamp(p.now()),
		Mode:             report.Mode,
		Changed:          len(report.Cells),
		Matched:          matched,
		Reused:           reused,
		UncoveredEntries: report.Uncovered,
	}
	for _, cell := range report.Cells {
		if cell.Expected {
			result.Expected++
			continue
		}
		result.Unexpected++
		result.UnexpectedCells = append(result.UnexpectedCells, describeCell(cell))
	}
	return result
}

// recordCheck writes result as the last check through WriteRecords, applying change first when it is not nil.
// A passing result also records its head and the amendment count as the last pass.
func (p *DriftVerify) recordCheck(result CheckResult, change func(scope *ScopeRecord, judged *JudgedRecord)) error {
	return WriteRecords(p.deps, func(scope *ScopeRecord, judged *JudgedRecord) error {
		if change != nil {
			change(scope, judged)
		}
		scope.LastCheck = result
		if result.Passed {
			scope.LastPassHead, scope.LastPassAmendments = result.Head, len(scope.Amendments)
		}
		return nil
	})
}

// closeJudgeSession closes the judge-session mark and clears the snapshot-lost mark.
// A closed session needs no snapshot.
func closeJudgeSession(scope *ScopeRecord, _ *JudgedRecord) {
	scope.JudgeSession = SessionMark{}
	scope.SnapshotLost = false
}

// now returns the told clock, or the system clock when none was told.
func (p *DriftVerify) now() time.Time {
	if p.deps.Now == nil {
		return time.Now()
	}
	return p.deps.Now()
}

// judge spawns the judge on cells that no stored ruling covers and on the files listed leaves unruled.
// It stores the judge's rulings.
// A fresh session removes a stale judgement.yaml and writes the request under a new spawn id.
// It then takes the snapshot of both checkouts and opens the session mark.
// A session an earlier entry left open keeps its spawn id and snapshot.
// It takes a new snapshot only when the records say the old one was lost.
// Under the run's autonomy, a judge ask that ended unanswered halts it before the rulings are stored,
// and a session left open over such an ask asks the parent again instead of spawning a judge.
func (p *DriftVerify) judge(ctx context.Context, rec Records, report CheckReport, cells []ClassifiedCell, listed listedFiles, reused int, head string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	mark := rec.Scope.JudgeSession
	spawnID := mark.SpawnID
	if !mark.Open {
		var err error
		if spawnID, err = newSpawnID(); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
	}
	request, err := p.judgeRequest(spawnID, rec, report, cells, listed)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	outcome, pointer, session, err := p.runJudgeSession(ctx, rec, request, report.Capture.Dir)
	if session == nil {
		return outcome, pointer, err
	}
	return p.storeRulings(report, cells, request, session, listed, reused, head)
}

// newSpawnID returns a fresh id for one judge spawn.
func newSpawnID() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("drift: draw a judge spawn id: %w", err)
	}
	return "judge-" + hex.EncodeToString(raw[:]), nil
}

// judgeCellID is the id the judge rules on the cell at index in the request.
func judgeCellID(index int) string {
	return fmt.Sprintf("c%d", index+1)
}

// judgeRequest builds the request of one spawn over cells.
// Each cell carries its exact old and new value and the covering entry's why.
// The request also lists the open ranges that cover a cell, and the sealed expectation and the amendments as text.
// An added case names its file in the current capture, and a removed case names its file in the baseline.
// The request also lists the unruled files of the review range and the range's commit messages.
func (p *DriftVerify) judgeRequest(spawnID string, rec Records, report CheckReport, cells []ClassifiedCell, listed listedFiles) (JudgeRequest, error) {
	d := p.deps
	baseHead, err := baselineHead(d)
	if err != nil {
		return JudgeRequest{}, err
	}
	seal, err := CheckSeal(d, rec, baseHead)
	if err != nil {
		return JudgeRequest{}, err
	}
	request := JudgeRequest{SpawnID: spawnID, Mode: DiffLines, Expectation: string(seal.Sealed), Amendments: amendmentsText(rec.Scope.Amendments)}
	request.Files, request.CommitMessages = listed.judgeFiles(), listed.messages

	listedRanges := map[[2]string]bool{}
	for i, classified := range cells {
		cell := classified.Cell
		judged := JudgeCell{
			ID:        judgeCellID(i),
			Case:      cell.Case,
			Identity:  JudgeIdentity{Key: cell.Identity.Key, N: cell.Identity.N, Line: cell.Identity.Line},
			Kind:      cell.Kind,
			Old:       cell.Old,
			New:       cell.New,
			Why:       classified.Coverage.Why,
			OpenRange: classified.Coverage.Open,
		}
		switch cell.Kind {
		case CellAddedCase:
			judged.File = filepath.Join(report.Capture.Dir, filepath.FromSlash(cell.Case))
		case CellRemovedCase:
			judged.File = filepath.Join(d.Paths.BaselineDir, filepath.FromSlash(cell.Case))
		}
		request.Cells = append(request.Cells, judged)

		if !classified.Coverage.Open {
			continue
		}
		entry := coveringLine(report.Expectation, cell)
		if key := [2]string{cell.Case, entry.Range}; !listedRanges[key] {
			listedRanges[key] = true
			request.OpenRanges = append(request.OpenRanges, JudgeOpenRange{Case: cell.Case, Range: entry.Range, Why: entry.Why})
		}
	}
	return request, nil
}

// coveringLine returns the first line entry of the expectation that covers the unkeyed cell.
// It returns a zero LineEntry when none does.
func coveringLine(expectation Expectation, cell Cell) LineEntry {
	for _, entry := range expectation.Cases[cell.Case].Lines {
		if covers, _ := coversLine(cell.Case, entry, cell); covers {
			return entry
		}
	}
	return LineEntry{}
}

// amendmentsText renders the parent's amendments in order.
// Each amendment is a comment line naming its kind, caller and time, followed by its entries if any.
// It is empty when there are no amendments.
func amendmentsText(amendments []Amendment) string {
	var text strings.Builder
	for _, amendment := range amendments {
		fmt.Fprintf(&text, "# amendment %s by %s at %s\n", amendment.Kind, amendment.By, amendment.At)
		if amendment.Entries == "" {
			continue
		}
		text.WriteString(amendment.Entries)
		if !strings.HasSuffix(amendment.Entries, "\n") {
			text.WriteString("\n")
		}
	}
	return text.String()
}

// openJudgeSession starts a judge session.
// It removes a stale judgement.yaml and writes the request.
// It takes the snapshot of both checkouts and of scratchDir, and it opens the session mark.
// The snapshot comes first,
// so an open mark always has one and a pass that dies between the two leaves no session to recover.
func (p *DriftVerify) openJudgeSession(request JudgeRequest, scratchDir string) error {
	d := p.deps
	if err := p.removeJudgement(); err != nil {
		return err
	}
	if err := WriteJudgeRequest(d.Paths, request); err != nil {
		return err
	}
	if _, err := TakeJudgeSnapshot(d, scratchDir); err != nil {
		return err
	}
	return WriteRecords(d, func(scope *ScopeRecord, _ *JudgedRecord) error {
		scope.JudgeSession = SessionMark{Open: true, OpenedAt: stamp(p.now()), SpawnID: request.SpawnID}
		return nil
	})
}

// resumeJudgeSession rewrites the request of a session an earlier entry left open.
// When the records say its snapshot was lost, it clears the snapshot-lost flag and then takes a new snapshot,
// so the snapshot holds the write that cleared it.
func (p *DriftVerify) resumeJudgeSession(rec Records, request JudgeRequest, scratchDir string) error {
	d := p.deps
	if err := WriteJudgeRequest(d.Paths, request); err != nil {
		return err
	}
	if !rec.Scope.SnapshotLost {
		return nil
	}
	err := WriteRecords(d, func(scope *ScopeRecord, _ *JudgedRecord) error {
		scope.SnapshotLost = false
		return nil
	})
	if err != nil {
		return err
	}
	_, err = TakeJudgeSnapshot(d, scratchDir)
	return err
}

// removeJudgement removes judgement.yaml, which is not a record.
// A file that is already gone is not an error.
func (p *DriftVerify) removeJudgement() error {
	if err := os.Remove(p.deps.Paths.Judgement); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("drift: %s: remove the judgement file: %w", p.name, err)
	}
	return nil
}

// settleOpenSession returns why the session mark, which an earlier entry left open, may not go on;
// it returns empty when the session may.
// A snapshot that is gone guards nothing once no chair is live,
// so it is recorded as lost and the pass goes on to take a new one;
// under a live chair the reason is snapshotGoneReason, since what that session changed cannot be told.
func (p *DriftVerify) settleOpenSession(mark SessionMark) (string, error) {
	d := p.deps
	snapshot, found, err := ReadJudgeSnapshot(d)
	if err != nil {
		return "", err
	}
	if found {
		return p.changedSince(snapshot, mark, false)
	}
	table, err := p.tableFor()
	if err != nil {
		return "", err
	}
	live, err := d.Seats.Probe(table)
	if err != nil {
		return "", fmt.Errorf("drift: %s: probe the judge's chair: %w", p.name, err)
	}
	if err := p.recordSnapshotLost(); err != nil {
		return "", err
	}
	if live.Chair != nil {
		return snapshotGoneReason, nil
	}
	return "", nil
}

// judgeSessionChanged returns why the checkouts no longer match the snapshot the returned judge was spawned under;
// it returns empty when they match.
// A snapshot that went while that judge ran is recorded as lost, so the parent's resume takes a new one.
func (p *DriftVerify) judgeSessionChanged() (string, error) {
	snapshot, found, err := ReadJudgeSnapshot(p.deps)
	if err != nil {
		return "", err
	}
	if !found {
		return snapshotGoneReason, p.recordSnapshotLost()
	}
	rec, err := ReadRecords(p.deps)
	if err != nil {
		return "", err
	}
	return p.changedSince(snapshot, rec.Scope.JudgeSession, true)
}

// recordSnapshotLost marks the session's snapshot lost, so the next judge step takes a new one.
func (p *DriftVerify) recordSnapshotLost() error {
	return WriteRecords(p.deps, func(scope *ScopeRecord, _ *JudgedRecord) error {
		scope.SnapshotLost = true
		return nil
	})
}

// changedSince returns why the checkouts no longer match snapshot, and empty when they match.
// mark is the open judge-session mark, and returned says the judge session has just returned.
// The reason names each change with its file's modification time against the judge session's window,
// so a change made after the guard halted a returned session reads as another process's.
// It names the directory that holds the snapshot's copies and the way to restore.
// It tells the parent or the operator to keep that directory, since the resume compares against it.
// A check after a recorded guard check names what still differs and what was restored since the last check.
// The check is recorded on the mark through recordGuardHalt.
func (p *DriftVerify) changedSince(snapshot JudgeSnapshot, mark SessionMark, returned bool) (string, error) {
	d := p.deps
	changes, err := snapshot.Changes(d)
	if err != nil {
		return "", err
	}
	now := p.now()
	texts := make([]string, len(changes))
	described := make([]string, len(changes))
	for i, change := range changes {
		texts[i] = change.Text
		described[i] = change.Text + changeTime(change.Path, mark, now)
	}
	if err := p.recordGuardHalt(mark, texts, returned, now); err != nil {
		return "", err
	}
	if len(changes) == 0 {
		return "", nil
	}

	lead := "the judge session changed what it may not: " + strings.Join(described, "; ")
	if mark.GuardHalt != nil {
		var restored []string
		for _, earlier := range mark.GuardHalt.Changes {
			if !slices.Contains(texts, earlier) {
				restored = append(restored, earlier)
			}
		}
		restoredText := "none"
		if len(restored) > 0 {
			restoredText = strings.Join(restored, "; ")
		}
		lead = fmt.Sprintf("the resume re-checked the judge guard against the snapshot, and what the judge session changed still differs: %s; restored since the last check: %s", strings.Join(described, "; "), restoredText)
	}
	deletes := "an added path"
	if snapshot.ScratchDir != "" {
		deletes += " and the current capture's scratch directory " + snapshot.ScratchDir
	}
	return fmt.Sprintf("%s; the snapshot taken before it ran, with a copy of each untracked file, is in %s; way forward: the parent or the operator resets a moved branch to its snapshot HEAD, restores a changed tracked file from its HEAD and an untracked path from the snapshot's copy, deletes %s, keeps the snapshot directory, which the resume compares against, then runs lym loom resume", lead, d.JudgeSnapshotDir, deletes), nil
}

// changeTime words when the file at path was last modified, against the judge session's window, the mark records.
// The window runs from the mark's opening to the guard's first halt of the returned session, or to now before one.
// A file modified after that halt was changed by another process, since the judge session had returned.
// It returns empty for a change with no file, such as a moved HEAD or a deleted path.
func changeTime(path string, mark SessionMark, now time.Time) string {
	if path == "" {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	modified := info.ModTime()
	windowEnd := stamp(now)
	if mark.GuardHalt != nil && mark.GuardHalt.At != "" {
		windowEnd = mark.GuardHalt.At
		// The halt's stamp drops its fraction of a second, so a second's grace keeps a judge edit inside the window.
		if halted, err := time.Parse(time.RFC3339, mark.GuardHalt.At); err == nil && modified.After(halted.Add(time.Second)) {
			return fmt.Sprintf(" (modified at %s, after the guard halted the returned judge session at %s, so by another process)", stamp(modified), mark.GuardHalt.At)
		}
	}
	if opened, err := time.Parse(time.RFC3339, mark.OpenedAt); err == nil && modified.Before(opened) {
		return fmt.Sprintf(" (modified at %s, before the judge session opened at %s)", stamp(modified), mark.OpenedAt)
	}
	return fmt.Sprintf(" (modified at %s, inside the judge session's window from %s to %s, so by the judge session or a process working beside it)", stamp(modified), mark.OpenedAt, windowEnd)
}

// recordGuardHalt records a guard check on the open judge-session mark, given the changes it named.
// A check of a returned session that names changes records its time as the halt's, the first time, and the changes.
// A check of a session that has not returned records the changes with no time,
// so the next check words what was restored; the first check after the session returns sets the time.
// A later check updates the changes of a recorded halt, and one that names none clears the halt.
// Over records that differ from their commit it writes nothing, since the guard names that edit itself.
func (p *DriftVerify) recordGuardHalt(mark SessionMark, changes []string, returned bool, now time.Time) error {
	var halt *GuardHalt
	switch {
	case len(changes) == 0 && mark.GuardHalt == nil:
		return nil
	case len(changes) == 0:
	case mark.GuardHalt != nil:
		at := mark.GuardHalt.At
		if at == "" && returned {
			at = stamp(now)
		}
		halt = &GuardHalt{At: at, Changes: changes}
	case returned:
		halt = &GuardHalt{At: stamp(now), Changes: changes}
	default:
		halt = &GuardHalt{Changes: changes}
	}
	records, err := ReadRecords(p.deps)
	if err != nil {
		return err
	}
	if records.Mismatch {
		return nil
	}
	return WriteRecords(p.deps, func(scope *ScopeRecord, _ *JudgedRecord) error {
		scope.JudgeSession.GuardHalt = halt
		return nil
	})
}

// tableFor builds the judge's table over the request file, gated by the row's judgement entry.
func (p *DriftVerify) tableFor() (seatengine.Table, error) {
	table, err := p.deps.JudgeTable(p.deps.Paths.JudgeRequest)
	if err != nil {
		return seatengine.Table{}, err
	}
	table.Gate = shuttleengine.GateSpec{p.judgement}
	return table, nil
}

// tableWithParent builds the judge's table for request's spawn:
// when the judge asks the parent, the spawn's parent agreement entry follows the judgement entry.
func (p *DriftVerify) tableWithParent(request JudgeRequest) (seatengine.Table, error) {
	table, err := p.tableFor()
	if err != nil {
		return seatengine.Table{}, err
	}
	agreement, asks, err := p.parentAgreement(request)
	if err != nil {
		return seatengine.Table{}, err
	}
	if asks {
		table.Gate = append(table.Gate, agreement)
	}
	return table, nil
}

// parentAgreement returns the parent agreement entry of request's spawn, and whether the judge's table carries it.
// It does under judge_ask_parent, and once the spawn holds an ask whatever the key now reads.
// A row told no parent seat carries none.
func (p *DriftVerify) parentAgreement(request JudgeRequest) (shuttleengine.GateEntry, bool, error) {
	seat := p.deps.JudgeParent
	if seat == nil {
		return shuttleengine.GateEntry{}, false, nil
	}
	cfg, err := p.deps.Config()
	if err != nil {
		return shuttleengine.GateEntry{}, false, err
	}
	if !cfg.JudgeAskParent {
		held, err := seat.heldJudgeAsk(request.SpawnID)
		if err != nil || !held {
			return shuttleengine.GateEntry{}, false, err
		}
	}
	wait := time.Duration(cfg.JudgeParentTimeoutMin) * time.Minute
	return ParentAgreementGateEntry(seat, request.want(), p.deps.Paths.Judgement, wait, judgeAskLimit(p.judgeStartedAt), p.now), true, nil
}

const (
	// reaskWaitBase is the first wait of the resume's wait for the parent's answer.
	// The wait returns to it when the ask record changes.
	reaskWaitBase = time.Second
	// reaskWaitCeiling is the longest single wait of the resume's wait for the parent's answer.
	reaskWaitCeiling = time.Minute
)

// askStanding is the part of a judge ask that the resume's wait watches for a change.
type askStanding struct {
	number int
	state  burlerengine.ParentAskState
}

// judgeAsks returns the asks the judge spawn spawnID made, oldest first.
// It returns none when the row is told no parent seat.
func (p *DriftVerify) judgeAsks(spawnID string) ([]burlerengine.ParentAsk, error) {
	seat := p.deps.JudgeParent
	if seat == nil {
		return nil, nil
	}
	record, _, err := burlerengine.ReadParentAsks(seat.Files)
	if err != nil {
		return nil, err
	}
	seed := judgeAskSeed(spawnID)
	var asks []burlerengine.ParentAsk
	for _, ask := range record.Asks {
		if ask.Seed == seed {
			asks = append(asks, ask)
		}
	}
	return asks, nil
}

// endedUnanswered answers the spawn's latest ask when it is not cleared and ended without the parent's answer:
// timeout, not-sent, no-parent or undelivered.
// Otherwise, and for a row with no parent seat, it answers false.
func (p *DriftVerify) endedUnanswered(spawnID string) (burlerengine.ParentAsk, bool, error) {
	asks, err := p.judgeAsks(spawnID)
	if err != nil || len(asks) == 0 {
		return burlerengine.ParentAsk{}, false, err
	}
	latest := asks[len(asks)-1]
	switch latest.State {
	case burlerengine.ParentAskTimeout, burlerengine.ParentAskNotSent, burlerengine.ParentAskNoParent, burlerengine.ParentAskUndelivered:
		if !latest.Cleared {
			return latest, true, nil
		}
	}
	return burlerengine.ParentAsk{}, false, nil
}

// reaskWaiting answers the spawn's latest ask when it is open and an earlier ask of the same spawn is cleared:
// the ask an earlier resume sent and was still waiting on.
func (p *DriftVerify) reaskWaiting(spawnID string) (burlerengine.ParentAsk, bool, error) {
	asks, err := p.judgeAsks(spawnID)
	if err != nil || len(asks) < 2 {
		return burlerengine.ParentAsk{}, false, err
	}
	latest := asks[len(asks)-1]
	cleared := slices.ContainsFunc(asks[:len(asks)-1], func(ask burlerengine.ParentAsk) bool { return ask.Cleared })
	if latest.State.Ended() || !cleared {
		return burlerengine.ParentAsk{}, false, nil
	}
	return latest, true, nil
}

// haltOnEndedAsk returns the Stuck for a judge ask that ended without the parent's answer under the run's autonomy.
// Its way forward is the resume, which asks the parent again over the same rulings.
func haltOnEndedAsk(ask burlerengine.ParentAsk) (shedengine.Outcome, shedengine.OutputPointer, error) {
	ended := string(ask.State)
	if ask.Reason != "" {
		ended += ": " + ask.Reason
	}
	return haltForParent(fmt.Sprintf("under autonomy the judge's ask %d to the parent ended %s, and the rulings stand only with the parent's agreement; way forward: lym loom resume, which asks the parent again over the same rulings", ask.Number, ended))
}

// askToRepeat answers the ask of the spawn spawnID that a session left open makes the resume ask the parent about again.
// It answers one only under the run's autonomy.
// The session must be open, its snapshot not lost, and its judgement.yaml present.
// The ask is the spawn's ended unanswered ask, or its open ask that an earlier resume followed a cleared one with.
func (p *DriftVerify) askToRepeat(rec Records, cfg Config, spawnID string) (burlerengine.ParentAsk, bool, error) {
	if !cfg.Autonomous || !rec.Scope.JudgeSession.Open || rec.Scope.SnapshotLost {
		return burlerengine.ParentAsk{}, false, nil
	}
	if _, present, err := readRecordFile(p.deps.Paths.Judgement); err != nil || !present {
		return burlerengine.ParentAsk{}, false, err
	}
	if ask, ended, err := p.endedUnanswered(spawnID); err != nil || ended {
		return ask, ended, err
	}
	return p.reaskWaiting(spawnID)
}

// runJudgeSession opens or resumes the judge session of request and runs the judge.
// scratchDir is the capture directory it guards.
// It returns the session only when the judge's rulings may be stored.
// Under the run's autonomy a session left open over an ask the parent has not answered asks the parent again.
// It spawns no judge then.
// A judge that ends Done with its latest ask ended unanswered halts instead, so the rulings stay unstored.
func (p *DriftVerify) runJudgeSession(ctx context.Context, rec Records, request JudgeRequest, scratchDir string) (shedengine.Outcome, shedengine.OutputPointer, *judgeSession, error) {
	cfg, err := p.deps.Config()
	if err != nil {
		return "", shedengine.OutputPointer{}, nil, err
	}
	held, repeat, err := p.askToRepeat(rec, cfg, request.SpawnID)
	if err != nil {
		return "", shedengine.OutputPointer{}, nil, err
	}
	if repeat {
		outcome, pointer, session, err := p.reaskParent(ctx, request, held)
		if session != nil || outcome != "" || err != nil {
			return outcome, pointer, session, err
		}
	}

	if rec.Scope.JudgeSession.Open {
		err = p.resumeJudgeSession(rec, request, scratchDir)
	} else {
		err = p.openJudgeSession(request, scratchDir)
	}
	if err != nil {
		return "", shedengine.OutputPointer{}, nil, err
	}
	outcome, pointer, session, err := p.runAgent(ctx, request)
	if session == nil || !cfg.Autonomous {
		return outcome, pointer, session, err
	}
	ask, ended, err := p.endedUnanswered(request.SpawnID)
	if err != nil {
		return "", shedengine.OutputPointer{}, nil, err
	}
	if ended {
		outcome, pointer, err := haltOnEndedAsk(ask)
		return outcome, pointer, nil, err
	}
	return outcome, pointer, session, nil
}

// reaskParent asks the parent about the stored rulings of the open session of request and waits for the answer.
// No judge is spawned.
// held is the spawn's ask that askToRepeat answered.
// An ended one is cleared.
// The agreement entry then sends a new ask of the next round over the unchanged rulings, with no limit.
// An open one is waited on as it stands.
// The wait goes through Deps.Sleep from one second and doubles to a minute while the ask record is unchanged.
// A cancelled context ends it with the context's error.
// A satisfied answer returns the stored rulings as a session with no strand, since no judge ran.
// An ask that ends unanswered again returns the same halt as a judge run would.
// Findings in the parent's report return no session, no outcome and no error: the judge has to rewrite its rulings.
func (p *DriftVerify) reaskParent(ctx context.Context, request JudgeRequest, held burlerengine.ParentAsk) (shedengine.Outcome, shedengine.OutputPointer, *judgeSession, error) {
	if err := requireSeams(p.name, seam{"Sleep", p.deps.Sleep != nil}); err != nil {
		return "", shedengine.OutputPointer{}, nil, err
	}
	// No judge ran in this pass, so the ask has no judge run to end before.
	p.judgeStartedAt = time.Time{}
	entry, asks, err := p.parentAgreement(request)
	if err != nil || !asks {
		return "", shedengine.OutputPointer{}, nil, err
	}
	if held.State.Ended() {
		if err := burlerengine.ClearParentAsk(p.deps.JudgeParent.Files, held.Number); err != nil {
			return "", shedengine.OutputPointer{}, nil, err
		}
	}

	delay, seen := reaskWaitBase, askStanding{}
	for {
		result, err := entry.Gate()
		if err != nil {
			return "", shedengine.OutputPointer{}, nil, err
		}
		spawnAsks, err := p.judgeAsks(request.SpawnID)
		if err != nil {
			return "", shedengine.OutputPointer{}, nil, err
		}
		latest := spawnAsks[len(spawnAsks)-1]
		switch {
		case result.Passed && latest.State == burlerengine.ParentAskDone:
			judgement, err := p.storedJudgement(request)
			if err != nil {
				return "", shedengine.OutputPointer{}, nil, err
			}
			return shedengine.Done, shedengine.OutputPointer{}, &judgeSession{Judgement: judgement}, nil
		case result.Passed:
			outcome, pointer, err := haltOnEndedAsk(latest)
			return outcome, pointer, nil, err
		case latest.State == burlerengine.ParentAskDone:
			return "", shedengine.OutputPointer{}, nil, nil
		}

		if standing := (askStanding{latest.Number, latest.State}); standing == seen {
			delay = min(2*delay, reaskWaitCeiling)
		} else {
			delay, seen = reaskWaitBase, standing
		}
		if err := p.deps.Sleep(ctx, delay); err != nil {
			return "", shedengine.OutputPointer{}, nil, err
		}
	}
}

// storedJudgement reads the rulings in judgement.yaml against request.
func (p *DriftVerify) storedJudgement(request JudgeRequest) (Judgement, error) {
	data, present, err := readRecordFile(p.deps.Paths.Judgement)
	if err != nil {
		return Judgement{}, err
	}
	if !present {
		return Judgement{}, fmt.Errorf("drift: %s: the parent agreed with rulings that have no judgement file at %s", p.name, p.deps.Paths.Judgement)
	}
	judgement, err := ParseJudgement(data, request.want())
	if err != nil {
		return Judgement{}, fmt.Errorf("drift: %s: read the stored rulings: %w", p.name, err)
	}
	return judgement, nil
}

// judgeSession is the rulings a judge session wrote and the strand that gave them.
type judgeSession struct {
	// Judgement is the parsed judgement.yaml.
	Judgement Judgement
	// Strand is the guid of the judge's strand, the one identity a seat result carries.
	Strand string
}

// strandRecorder passes every call to a seat runner and keeps the strand guid of the last chair it ran or resumed.
type strandRecorder struct {
	inner  shedadapters.SeatRunner
	strand string
}

func (r *strandRecorder) Probe(table seatengine.Table) (seatengine.LiveTable, error) {
	return r.inner.Probe(table)
}

func (r *strandRecorder) Resume(table seatengine.Table, live seatengine.LiveTable) (seatengine.Result, error) {
	result, err := r.inner.Resume(table, live)
	r.strand = result.Chair.StrandGUID
	return result, err
}

func (r *strandRecorder) Run(table seatengine.Table) (seatengine.Result, error) {
	result, err := r.inner.Run(table)
	r.strand = result.Chair.StrandGUID
	return result, err
}

// runAgent runs the judge, resuming its live chair, and reads its rulings.
// It returns the session only when the judge ended Done;
// both checkouts still match the snapshot, and the baseline still matches its digest.
// Otherwise it returns the outcome to end the pass with, which leaves the session mark open,
// so the next entry resumes the judge.
func (p *DriftVerify) runAgent(ctx context.Context, request JudgeRequest) (shedengine.Outcome, shedengine.OutputPointer, *judgeSession, error) {
	d := p.deps
	// The judge of either diff mode runs here, and the table's parent agreement caps its ask by this start.
	p.judgeStartedAt = p.now()
	seats := &strandRecorder{inner: d.Seats}
	table := func() (seatengine.Table, error) { return p.tableWithParent(request) }
	agent := shedadapters.NewMultiLLMProducerSourced(p.name, table, seats, d.Now, nil).KeepGateFindings(d.GateFindingsFile)
	outcome, pointer, err := agent.Call(ctx)
	if err != nil || outcome != shedengine.Done {
		return outcome, pointer, nil, err
	}

	reason, err := p.judgeSessionChanged()
	if err != nil {
		return "", shedengine.OutputPointer{}, nil, err
	}
	if reason == "" {
		baseline, err := CheckBaseline(d)
		if err != nil {
			return "", shedengine.OutputPointer{}, nil, err
		}
		reason = baseline.Fault
	}
	if reason != "" {
		outcome, pointer, err := haltForParent(reason)
		return outcome, pointer, nil, err
	}

	data, present, err := readRecordFile(d.Paths.Judgement)
	if err != nil {
		return "", shedengine.OutputPointer{}, nil, err
	}
	if !present {
		return "", shedengine.OutputPointer{}, nil, fmt.Errorf("drift: %s: the judge ended Done with no judgement file at %s", p.name, d.Paths.Judgement)
	}
	judgement, err := ParseJudgement(data, request.want())
	if err != nil {
		return "", shedengine.OutputPointer{}, nil, fmt.Errorf("drift: %s: read the judge's rulings: %w", p.name, err)
	}
	return shedengine.Done, pointer, &judgeSession{Judgement: judgement, Strand: seats.strand}, nil
}

// storeRulings stores the judge's rulings in judged.json as exact tuples:
// each listed open range that the request covers is stored with the request's why and the judge's sentence.
// It closes the session mark and records the check in one write; then it removes judgement.yaml.
// It also stores each file ruling as a JudgedFile and records every loosened check on the check.
// Any ruling other than matches is a Stuck naming the cells and their reasons.
// So is a loosened check with no measured cause, whose reason follows the cells'.
// All matches with every loosened check caused records the pass and is Done.
func (p *DriftVerify) storeRulings(report CheckReport, cells []ClassifiedCell, request JudgeRequest, session *judgeSession, listed listedFiles, reused int, head string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	at := stamp(p.now())
	fileRulings, freshChecks := listed.newRulings(session.Judgement, session.Strand, at)
	checks := listed.allChecks(freshChecks)
	uncaused := uncausedChecks(checks)
	byID := make(map[string]ClassifiedCell, len(cells))
	for i, cell := range cells {
		byID[judgeCellID(i)] = cell
	}

	var tuples []JudgedTuple
	var notMatching []ruledCell
	matched := 0
	for _, ruling := range session.Judgement.Cells {
		cell := byID[ruling.ID]
		tuples = append(tuples, JudgedTuple{
			Case:     cell.Cell.Case,
			Identity: cell.Cell.Identity,
			Kind:     cell.Cell.Kind,
			Old:      cell.Cell.Old,
			New:      cell.Cell.New,
			Ruling:   ruling.Ruling,
			Reason:   ruling.Reason,
			Strand:   session.Strand,
			At:       at,
		})
		if ruling.Ruling == RulingMatches {
			matched++
			continue
		}
		notMatching = append(notMatching, ruledCell{cell: cell, reason: ruling.Reason})
	}

	whys := make(map[[2]string]string, len(request.OpenRanges))
	for _, open := range request.OpenRanges {
		whys[[2]string{open.Case, open.Range}] = open.Why
	}
	var ranges []JudgedRange
	for _, note := range session.Judgement.OpenRanges {
		why, covers := whys[[2]string{note.Case, note.Range}]
		if !covers {
			continue
		}
		ranges = append(ranges, JudgedRange{Case: note.Case, Range: note.Range, Why: why, Checked: note.Checked, Strand: session.Strand, At: at})
	}

	result := p.checkResult(report, head, len(notMatching) == 0 && len(uncaused) == 0, matched, reused)
	result.Loosened = checks
	err := p.recordCheck(result, func(scope *ScopeRecord, judged *JudgedRecord) {
		closeJudgeSession(scope, judged)
		for _, tuple := range tuples {
			judged.Tuples = replaceOrAppend(judged.Tuples, tuple, func(held JudgedTuple) bool {
				return held.Case == tuple.Case && held.Identity == tuple.Identity && held.Kind == tuple.Kind && held.Old == tuple.Old && held.New == tuple.New
			})
		}
		for _, note := range ranges {
			judged.Ranges = replaceOrAppend(judged.Ranges, note, func(held JudgedRange) bool {
				return held.Case == note.Case && held.Range == note.Range
			})
		}
		storeFileRulings(judged, fileRulings)
	})
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := p.removeJudgement(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	noMatch := ""
	if len(notMatching) > 0 {
		if noMatch, err = p.cellsNoMatchReason("the judge ruled that a change does not match its expectation", notMatching); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
	}
	if reason := haltReasons(noMatch, uncaused); reason != "" {
		return haltForParent(reason)
	}
	return doneBecause("the check passed: every change is expected and ruled to match")
}

// passCommand maps the report of a check under diff command, whose declared compare already ran, to an outcome:
// an exit 0 records the pass and is Done with no judge;
// a failing compare at a code head with a stored non-matching case ruling and no amendment since is a Stuck naming them,
// and no judge runs;
// otherwise one judge, spawned under a snapshot guard, rules on the cases, and its rulings are stored.
// The review range's files are listed before the exit is read:
// an exit 0 with a file no stored ruling covers spawns the judge for the files alone.
// Under the run's autonomy, a judge ask that ended unanswered halts the pass before the rulings are stored,
// and a session left open over such an ask asks the parent again instead of spawning a judge.
func (p *DriftVerify) passCommand(ctx context.Context, rec Records, report CheckReport) (shedengine.Outcome, shedengine.OutputPointer, error) {
	d := p.deps
	head, err := d.Head()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("drift: %s: read the code HEAD: %w", p.name, err)
	}
	listed, err := p.listFiles(rec)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if report.Compare.Passed && len(listed.unruled) == 0 {
		return p.passWithoutJudge(p.commandResult(report, head, true), listed, "the check passed: the compare exited 0")
	}
	if stored := storedCaseRulings(rec, head, report.Metadata); !report.Compare.Passed && len(stored) > 0 {
		if err := p.recordCheck(p.commandResult(report, head, false), nil); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		reason, err := p.noMatchReason("a case the judge already ruled does not match its expectation", stored, stored)
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		return haltForParent(reason)
	}

	mark := rec.Scope.JudgeSession
	spawnID := mark.SpawnID
	if !mark.Open {
		if spawnID, err = newSpawnID(); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
	}
	baseHead, err := baselineHead(d)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	seal, err := CheckSeal(d, rec, baseHead)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	request := JudgeRequest{SpawnID: spawnID, Mode: DiffCommand, Expectation: string(seal.Sealed), Amendments: amendmentsText(rec.Scope.Amendments)}
	request.Files, request.CommitMessages = listed.judgeFiles(), listed.messages
	if !report.Compare.Passed {
		request.CompareLog = report.Compare.Log
	}

	outcome, pointer, session, err := p.runJudgeSession(ctx, rec, request, "")
	if session == nil {
		return outcome, pointer, err
	}
	return p.storeCaseRulings(report, session, listed, head)
}

// commandResult builds the record of a check under diff command over report at head, with the compare's exit and log.
func (p *DriftVerify) commandResult(report CheckReport, head string, passed bool) CheckResult {
	result := p.checkResult(report, head, passed, 0, 0)
	result.CompareExit, result.CompareLog = report.Compare.ExitCode, report.Compare.Log
	return result
}

// describeCaseRuling words a ruling of does not match or unexpected on a case with its reason.
func describeCaseRuling(caseName string, ruling Ruling, reason string) string {
	return fmt.Sprintf("case %q ruled %q: %s", caseName, ruling, reason)
}

// storedCaseRulings returns each stored non-matching ruling at head that no amendment was recorded after.
// A ruling on a case the metadata marks name is left out, since such a case is no drift.
// Each string names the case, its ruling and its reason.
// Rulings and amendments both carry UTC stamps, so their strings order by time;
// an amendment stamped in the same second as a ruling counts as after it.
func storedCaseRulings(rec Records, head string, metadata Volatile) []string {
	var ruled []string
	for _, stored := range rec.Judged.Cases {
		if stored.CodeHead != head || stored.Ruling == RulingMatches || metadata.holdsCase(stored.Case) {
			continue
		}
		amended := slices.ContainsFunc(rec.Scope.Amendments, func(amendment Amendment) bool { return amendment.At >= stored.At })
		if !amended {
			ruled = append(ruled, describeCaseRuling(stored.Case, stored.Ruling, stored.Reason))
		}
	}
	return ruled
}

// storeCaseRulings stores the judge's rulings on the cases of the failing compare.
// It closes the session mark and records the check in one write; then it removes judgement.yaml.
// A ruling of matches, and any ruling on a case the report's metadata marks name, is not stored.
// It drops an earlier stored ruling of the same case at head.
// Each other ruling is stored as a JudgedCase keyed by head and case.
// It also stores each file ruling as a JudgedFile and records every loosened check on the check.
// Any ruling other than matches is a Stuck naming the cases and their reasons.
// So is a loosened check with no measured cause, whose reason follows the cases'.
// All matches with every loosened check caused records the pass and is Done.
func (p *DriftVerify) storeCaseRulings(report CheckReport, session *judgeSession, listed listedFiles, head string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	at := stamp(p.now())
	fileRulings, freshChecks := listed.newRulings(session.Judgement, session.Strand, at)
	checks := listed.allChecks(freshChecks)
	uncaused := uncausedChecks(checks)
	var ruled []JudgedCase
	var matching, notMatching []string
	for _, ruling := range session.Judgement.Cases {
		if ruling.Ruling == RulingMatches || report.Metadata.holdsCase(ruling.Case) {
			matching = append(matching, ruling.Case)
			continue
		}
		ruled = append(ruled, JudgedCase{CodeHead: head, Case: ruling.Case, Ruling: ruling.Ruling, Reason: ruling.Reason, Strand: session.Strand, At: at})
		notMatching = append(notMatching, describeCaseRuling(ruling.Case, ruling.Ruling, ruling.Reason))
	}

	result := p.commandResult(report, head, len(notMatching) == 0 && len(uncaused) == 0)
	result.Matched = len(matching)
	result.Loosened = checks
	err := p.recordCheck(result, func(scope *ScopeRecord, judged *JudgedRecord) {
		closeJudgeSession(scope, judged)
		judged.Cases = slices.DeleteFunc(judged.Cases, func(held JudgedCase) bool {
			return held.CodeHead == head && slices.Contains(matching, held.Case)
		})
		for _, stored := range ruled {
			judged.Cases = replaceOrAppend(judged.Cases, stored, func(held JudgedCase) bool {
				return held.CodeHead == stored.CodeHead && held.Case == stored.Case
			})
		}
		storeFileRulings(judged, fileRulings)
	})
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := p.removeJudgement(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	noMatch := ""
	if len(notMatching) > 0 {
		if noMatch, err = p.noMatchReason("the judge ruled that the failing compare does not match its expectation", notMatching, notMatching); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
	}
	if reason := haltReasons(noMatch, uncaused); reason != "" {
		return haltForParent(reason)
	}
	if report.Compare.Passed {
		return doneBecause("the check passed: the compare exited 0 and every loosened check has a measured cause")
	}
	return doneBecause("the check passed: the judge ruled every case of the failing compare to match")
}

// replaceOrAppend replaces the first item of items that same accepts with item, and appends item when none is.
func replaceOrAppend[T any](items []T, item T, same func(held T) bool) []T {
	for i := range items {
		if same(items[i]) {
			items[i] = item
			return items
		}
	}
	return append(items, item)
}
