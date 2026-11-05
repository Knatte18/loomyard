// verify.go — the Drift-Verify producer: containment of the current code's captured changes by the sealed expectation, and one judge on the expected changes no stored ruling covers.

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

	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// The recipe rows that edit code, which a Stuck's way forward goes back to.
const (
	darnWriteRow = "Darn-Write"
	websterRow   = "Webster"
)

// The reasons of the Stuck passes that name no changed value.
const (
	recordsMismatchReason = "the drift records on disk differ from their newest committed copy; way forward: the parent runs lym drift amend <slug> --restore, then lym loom resume"
	neverCountedReason    = "the baseline capture never counted, so there is no baseline to check against; way forward: the parent runs lym drift defer, then lym loom resume"
)

// DriftVerify is the Drift-Verify ShedProducer; the Drift-Recheck row runs the same engine.
type DriftVerify struct {
	name      string
	row       Row
	judgement shuttleengine.GateEntry
	deps      Deps
}

var _ shedengine.ShedProducer = (*DriftVerify)(nil)

// NewDriftVerify returns the Drift-Verify producer identified as name.
// judgement is the row's resolved `judgement` gate entry, which the judge's table runs at each turn end.
func NewDriftVerify(name string, row Row, judgement shuttleengine.GateEntry, deps Deps) *DriftVerify {
	return &DriftVerify{name: name, row: row, judgement: judgement, deps: deps}
}

// Call runs one Drift-Verify pass.
// With drift switched off it is Done at once.
// Otherwise it first ends the live chairs of the removed drift-baseline and drift-verify agents, and then runs these steps in order, each ending the pass when it answers:
// no scope record, scope off, a deferred capture, a waiver and a head already checked with no amendment since are Done;
// a records mismatch, a capture that never counted and a baseline that is missing or edited are a Stuck for the run's parent;
// an open judge session whose snapshot is gone or whose checkouts changed since is a Stuck, and one that is intact is resumed;
// a failed current capture, a seal fault and any change the expectation does not cover are a Stuck;
// a change whose exact old and new value has a stored ruling reuses it, and a stored ruling of does not match is a Stuck.
// With no change left to judge the check passes; otherwise one judge rules on the rest, and its rulings are stored.
// Under a recorded diff of command the declared compare replaces the capture and the diff.
// An exit 0 is Done, and a failing compare is ruled on case by case by one judge.
// A stored ruling of the same code head stands until an amendment follows it.
// Every Stuck for the parent opens with shedadapters.DecisionHaltPrefix, and one that names `lym drift defer` carries DeferHaltPrefix right after it.
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

// pass is Call after the switch check and the end of the removed agents' chairs.
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
		reason, err := p.judgeSessionChanged()
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		if reason != "" {
			return haltForParent(reason)
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

// recordDiffMode records the config's diff mode and key patterns in a scope record that holds none, which keeps them for the run, and returns the records as written.
func (p *DriftVerify) recordDiffMode() (Records, error) {
	d := p.deps
	cfg, err := d.Config()
	if err != nil {
		return Records{}, fmt.Errorf("drift: %s: load the drift config: %w", p.name, err)
	}
	err = WriteRecords(d, func(scope *ScopeRecord, _ *JudgedRecord) error {
		scope.Diff, scope.KeyPatterns = cfg.Diff, cfg.KeyPatterns
		return nil
	})
	if err != nil {
		return Records{}, err
	}
	return ReadRecords(d)
}

// recordUnsealed records Unsealed once for a run that has neither a seal commit nor an expected.yaml, so containment reads the empty expectation.
// A run with an expected.yaml and no seal commit is left unmarked, so the seal check faults on it, and so is a run whose row is not past Drift-Expect, which the check refuses.
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

// checkAndJudge runs the drift check and maps its report: a fault or an unexpected change is a Stuck, stored rulings are reused, and the changes left over go to the judge.
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
	var ruledNoMatch []string
	reused := 0
	for _, cell := range report.Cells {
		tuple, stored := storedRuling(rec.Judged, cell.Cell)
		switch {
		case !stored:
			toJudge = append(toJudge, cell)
		case tuple.Ruling == RulingMatches:
			reused++
		default:
			ruledNoMatch = append(ruledNoMatch, fmt.Sprintf("%s: %s", describeCell(cell), tuple.Reason))
		}
	}
	if len(ruledNoMatch) > 0 {
		if err := p.recordCheck(p.checkResult(report, head, false, 0, reused), nil); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		return haltForParent(p.noMatchReason("a change the judge already ruled does not match its expectation", ruledNoMatch))
	}
	if len(toJudge) == 0 {
		err := p.recordCheck(p.checkResult(report, head, true, 0, reused), closeJudgeSession)
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		return doneBecause("the check passed: every change is expected and ruled to match")
	}
	return p.judge(ctx, rec, report, toJudge, reused, head)
}

// faultReason returns the reason of the Stuck a fault of the report is, and empty when it holds none.
// A failed current capture names its reason, its log and three ways forward.
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

// noMatchReason words a Stuck over changes ruled not to match, naming each with its reason, the parent's amendment and the route that fixes code.
func (p *DriftVerify) noMatchReason(lead string, ruled []string) string {
	return fmt.Sprintf("%s: %s; way forward: the parent runs lym drift amend <slug> --entries <file> with entries that explain the change, then lym loom resume; or fix the code with %s", lead, strings.Join(ruled, "; "), p.fixRoute())
}

// fixRoute returns the parent's route to the row that edits code: Darn-Write when the run's rows hold it, else Webster, whose Merriam meets the cells in its run-level drift entry.
func (p *DriftVerify) fixRoute() string {
	row := websterRow
	if slices.Contains(p.deps.Rows, darnWriteRow) {
		row = darnWriteRow
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

// checkResult builds the record of a check over report at head, counting the cells the judge ruled matching and the cells a stored ruling decided.
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

// recordCheck writes result as the last check through the records guard, with change applied first when it is not nil.
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

// closeJudgeSession closes the judge-session mark and clears the snapshot-lost mark, since a closed session needs no snapshot.
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

// judge spawns the judge on cells, which no stored ruling covers, and stores its rulings.
// A fresh session removes a stale judgement.yaml, writes the request under a new spawn id, opens the session mark and takes the snapshot of both checkouts.
// A session an earlier entry left open keeps its spawn id and its snapshot, and takes a new snapshot only when the records say the old one was lost.
func (p *DriftVerify) judge(ctx context.Context, rec Records, report CheckReport, cells []ClassifiedCell, reused int, head string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	mark := rec.Scope.JudgeSession
	spawnID := mark.SpawnID
	if !mark.Open {
		var err error
		if spawnID, err = newSpawnID(); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
	}
	request, err := p.judgeRequest(spawnID, rec, report, cells)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	if mark.Open {
		err = p.resumeJudgeSession(rec, request, report.Capture.Dir)
	} else {
		err = p.openJudgeSession(request, report.Capture.Dir)
	}
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	outcome, pointer, session, err := p.runAgent(ctx, request)
	if session == nil {
		return outcome, pointer, err
	}
	return p.storeRulings(report, cells, request, session, reused, head)
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

// judgeRequest builds the request of one spawn over cells: each cell with its exact old and new value and covering why, the open ranges that cover a cell, and the sealed expectation and the amendments as text.
// An added case names its file in the current capture and a removed case its file in the baseline.
func (p *DriftVerify) judgeRequest(spawnID string, rec Records, report CheckReport, cells []ClassifiedCell) (JudgeRequest, error) {
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

	listed := map[[2]string]bool{}
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
		if key := [2]string{cell.Case, entry.Range}; !listed[key] {
			listed[key] = true
			request.OpenRanges = append(request.OpenRanges, JudgeOpenRange{Case: cell.Case, Range: entry.Range, Why: entry.Why})
		}
	}
	return request, nil
}

// coveringLine returns the first line entry of the expectation that covers the unkeyed cell, the entry Cover reports.
func coveringLine(expectation Expectation, cell Cell) LineEntry {
	for _, entry := range expectation.Cases[cell.Case].Lines {
		if covers, _ := coversLine(cell.Case, entry, cell); covers {
			return entry
		}
	}
	return LineEntry{}
}

// amendmentsText renders the parent's amendments in order, each as a comment line naming its kind, caller and time and then its entries; it is empty when there is none.
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

// openJudgeSession starts a judge session: it removes a stale judgement.yaml, writes the request, opens the session mark and takes the snapshot of both checkouts and of scratchDir.
func (p *DriftVerify) openJudgeSession(request JudgeRequest, scratchDir string) error {
	d := p.deps
	if err := p.removeJudgement(); err != nil {
		return err
	}
	if err := WriteJudgeRequest(d.Paths, request); err != nil {
		return err
	}
	err := WriteRecords(d, func(scope *ScopeRecord, _ *JudgedRecord) error {
		scope.JudgeSession = SessionMark{Open: true, OpenedAt: stamp(p.now()), SpawnID: request.SpawnID}
		return nil
	})
	if err != nil {
		return err
	}
	_, err = TakeJudgeSnapshot(d, scratchDir)
	return err
}

// resumeJudgeSession rewrites the request of a session an earlier entry left open and, when the records say its snapshot was lost, takes a new one and clears that mark.
func (p *DriftVerify) resumeJudgeSession(rec Records, request JudgeRequest, scratchDir string) error {
	d := p.deps
	if err := WriteJudgeRequest(d.Paths, request); err != nil {
		return err
	}
	if !rec.Scope.SnapshotLost {
		return nil
	}
	if _, err := TakeJudgeSnapshot(d, scratchDir); err != nil {
		return err
	}
	return WriteRecords(d, func(scope *ScopeRecord, _ *JudgedRecord) error {
		scope.SnapshotLost = false
		return nil
	})
}

// removeJudgement removes judgement.yaml, which is no record, and is content with it already gone.
func (p *DriftVerify) removeJudgement() error {
	if err := os.Remove(p.deps.Paths.Judgement); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("drift: %s: remove the judgement file: %w", p.name, err)
	}
	return nil
}

// judgeSessionChanged returns why the checkouts no longer match the snapshot the judge was spawned under, and empty when they match.
// A snapshot that is gone is recorded as lost first, so the parent's resume takes a new one.
// The reason names each change, the directory the snapshot's copies are in and the way to restore.
func (p *DriftVerify) judgeSessionChanged() (string, error) {
	d := p.deps
	snapshot, found, err := ReadJudgeSnapshot(d)
	if err != nil {
		return "", err
	}
	if !found {
		err := WriteRecords(d, func(scope *ScopeRecord, _ *JudgedRecord) error {
			scope.SnapshotLost = true
			return nil
		})
		if err != nil {
			return "", err
		}
		return "the snapshot of both checkouts the judge was spawned under is gone, so what its session changed cannot be told; way forward: the parent or the operator checks both checkouts for the judge's edits, then runs lym loom resume, which takes a new snapshot and resumes the judge", nil
	}
	changes, err := snapshot.Changes(d)
	if err != nil {
		return "", err
	}
	if len(changes) == 0 {
		return "", nil
	}
	return fmt.Sprintf("the judge session changed what it may not: %s; the snapshot taken before it ran, with a copy of each untracked file, is in %s; way forward: the parent or the operator resets a moved branch to its snapshot HEAD, restores a changed tracked file from its HEAD and an untracked path from the snapshot's copy, deletes an added path and the changed scratch directory, then runs lym loom resume", strings.Join(changes, "; "), d.JudgeSnapshotDir), nil
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
// It returns the session only when the judge ended Done, both checkouts still match the snapshot and the baseline still matches its digest;
// otherwise it returns the outcome to end the pass with, which leaves the session mark open, so the next entry resumes the judge.
func (p *DriftVerify) runAgent(ctx context.Context, request JudgeRequest) (shedengine.Outcome, shedengine.OutputPointer, *judgeSession, error) {
	d := p.deps
	seats := &strandRecorder{inner: d.Seats}
	agent := shedadapters.NewMultiLLMProducerSourced(p.name, p.tableFor, seats, d.Now, nil).KeepGateFindings(d.GateFindingsFile)
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

// storeRulings stores the judge's rulings in judged.json as exact tuples, each listed open range with its covering why and the judge's sentence, closes the session mark and records the check, all in one write, and then removes judgement.yaml.
// Any ruling of does not match is a Stuck naming the cells and their reasons; all matches records the pass and is Done.
func (p *DriftVerify) storeRulings(report CheckReport, cells []ClassifiedCell, request JudgeRequest, session *judgeSession, reused int, head string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	at := stamp(p.now())
	byID := make(map[string]ClassifiedCell, len(cells))
	for i, cell := range cells {
		byID[judgeCellID(i)] = cell
	}

	var tuples []JudgedTuple
	var notMatching []string
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
		notMatching = append(notMatching, fmt.Sprintf("%s: %s", describeCell(cell), ruling.Reason))
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

	result := p.checkResult(report, head, len(notMatching) == 0, matched, reused)
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
	})
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := p.removeJudgement(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if len(notMatching) > 0 {
		return haltForParent(p.noMatchReason("the judge ruled that a change does not match its expectation", notMatching))
	}
	return doneBecause("the check passed: every change is expected and ruled to match")
}

// passCommand maps the report of a check under diff command, whose declared compare already ran, to an outcome:
// an exit 0 records the pass and is Done with no judge;
// a failing compare at a code head whose stored case rulings include a does not match or an unexpected, with no amendment recorded since, is a Stuck naming them, with no new judge;
// otherwise one judge, spawned under the same snapshot guard as under diff lines, rules on the cases, and its rulings are stored.
func (p *DriftVerify) passCommand(ctx context.Context, rec Records, report CheckReport) (shedengine.Outcome, shedengine.OutputPointer, error) {
	d := p.deps
	head, err := d.Head()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("drift: %s: read the code HEAD: %w", p.name, err)
	}
	if report.Compare.Passed {
		if err := p.recordCheck(p.commandResult(report, head, true), closeJudgeSession); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		return doneBecause("the check passed: the compare exited 0")
	}
	if stored := storedCaseRulings(rec, head); len(stored) > 0 {
		if err := p.recordCheck(p.commandResult(report, head, false), nil); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		return haltForParent(p.noMatchReason("a case the judge already ruled does not match its expectation", stored))
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
	request := JudgeRequest{SpawnID: spawnID, Mode: DiffCommand, CompareLog: report.Compare.Log, Expectation: string(seal.Sealed), Amendments: amendmentsText(rec.Scope.Amendments)}

	if mark.Open {
		err = p.resumeJudgeSession(rec, request, "")
	} else {
		err = p.openJudgeSession(request, "")
	}
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	outcome, pointer, session, err := p.runAgent(ctx, request)
	if session == nil {
		return outcome, pointer, err
	}
	return p.storeCaseRulings(report, session, head)
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

// storedCaseRulings returns, worded by describeCaseRuling, each stored ruling of does not match or unexpected at head that no amendment was recorded after.
// Rulings and amendments both carry UTC stamps, so their strings order by time; an amendment stamped in the same second as a ruling counts as after it.
func storedCaseRulings(rec Records, head string) []string {
	var ruled []string
	for _, stored := range rec.Judged.Cases {
		if stored.CodeHead != head || stored.Ruling == RulingMatches {
			continue
		}
		amended := slices.ContainsFunc(rec.Scope.Amendments, func(amendment Amendment) bool { return amendment.At >= stored.At })
		if !amended {
			ruled = append(ruled, describeCaseRuling(stored.Case, stored.Ruling, stored.Reason))
		}
	}
	return ruled
}

// storeCaseRulings stores the judge's rulings on the cases of the failing compare, closes the session mark and records the check in one write, and then removes judgement.yaml.
// A ruling of matches is not stored, and drops an earlier stored ruling of the same case at head; each other ruling is stored as a JudgedCase keyed by head and case.
// Any ruling other than matches is a Stuck naming the cases and their reasons; all matches records the pass and is Done.
func (p *DriftVerify) storeCaseRulings(report CheckReport, session *judgeSession, head string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	at := stamp(p.now())
	var ruled []JudgedCase
	var matching, notMatching []string
	for _, ruling := range session.Judgement.Cases {
		if ruling.Ruling == RulingMatches {
			matching = append(matching, ruling.Case)
			continue
		}
		ruled = append(ruled, JudgedCase{CodeHead: head, Case: ruling.Case, Ruling: ruling.Ruling, Reason: ruling.Reason, Strand: session.Strand, At: at})
		notMatching = append(notMatching, describeCaseRuling(ruling.Case, ruling.Ruling, ruling.Reason))
	}

	result := p.commandResult(report, head, len(notMatching) == 0)
	result.Matched = len(matching)
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
	})
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := p.removeJudgement(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if len(notMatching) > 0 {
		return haltForParent(p.noMatchReason("the judge ruled that the failing compare does not match its expectation", notMatching))
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
