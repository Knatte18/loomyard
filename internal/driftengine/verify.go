// verify.go — the Drift-Verify producer: the mechanical compare, then one strong agent on the drift it finds, and the decision halts that put drift in a task meant to change nothing to the run's parent.

package driftengine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// The opening texts of the halts that record gate_halt_at, which follow shedadapters.DecisionHaltPrefix where it applies.
// They are for the operator's reading; no code reads them back.
const (
	dirtyHaltOpening   = "drift verify halt: the tracked tree is dirty; "
	gateHaltOpening    = "drift verify halt: the verify gate failed; "
	compareHaltOpening = "drift verify halt: the compare still shows drift; "
	generalHaltOpening = "drift verify halt: "
)

// The names of the gate entries of the agent's table; the told verify entry keeps its own name.
const (
	dirtyGateName   = "dirty"
	compareGateName = "compare"
)

// dirtyGateAttempts is how many times the agent is asked to commit or clean its tree.
const dirtyGateAttempts = 3

// VerifyTold carries what the Drift-Verify table renders: the failing compare's log and the commit the baseline was captured at.
type VerifyTold struct {
	// CompareLog is the log of the compare that showed the drift.
	CompareLog string
	// BaselineHead is the counted capture's head.
	BaselineHead string
}

// DriftVerify is the Drift-Verify ShedProducer.
type DriftVerify struct {
	name   string
	row    Row
	verify shuttleengine.GateEntry
	deps   Deps
}

var _ shedengine.ShedProducer = (*DriftVerify)(nil)

// NewDriftVerify returns the Drift-Verify producer identified as name.
// verify is the row's resolved test gate entry, which the agent's table runs only once the session committed code.
func NewDriftVerify(name string, row Row, verify shuttleengine.GateEntry, deps Deps) *DriftVerify {
	return &DriftVerify{name: name, row: row, verify: verify, deps: deps}
}

// Call runs one Drift-Verify pass and commits the durable drift directory on every way out.
// A pass with no scope record, scope off or a waiver is Done with no compare.
// Otherwise it settles the agent session an interrupted entry left and rules the acceptances the parent stood by.
// It halts for the parent on an unruled acceptance under scope full and on a missing baseline.
// It checks that the tree is clean and that the agent's commits pass the verify gate, then runs the compare.
// A passing compare is Done; a failing one spawns one agent whose table gates on a clean tree, on the verify entry and on a final compare.
// Every Stuck under scope full, and under scope partial the missing-baseline halt and a failing final compare while a test is withdrawn, opens with shedadapters.DecisionHaltPrefix.
func (p *DriftVerify) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	d := p.deps
	err := requireSeams(p.name,
		seam{"Config", d.Config != nil},
		seam{"Status", d.Status != nil},
		seam{"Head", d.Head != nil},
		seam{"DirtyPaths", d.DirtyPaths != nil},
		seam{"Commit", d.Commit != nil},
		seam{"Seats", d.Seats != nil},
		seam{"VerifyTable", d.VerifyTable != nil},
		seam{"Compare", d.Compare != nil},
	)
	if err == nil && p.verify.Gate == nil {
		err = fmt.Errorf("drift: %s is wired without a verify gate entry", p.name)
	}
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	outcome, pointer, err := p.pass(ctx)
	return commitOnExit(p.name, d.Commit, []string{DirRel()}, outcome, pointer, err)
}

// doneBecause returns a Done carrying why no agent was needed.
func doneBecause(why string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return shedengine.Done, shedengine.OutputPointer{Reason: why}, nil
}

// pass is Call without the commit.
func (p *DriftVerify) pass(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	d := p.deps
	scope, found, err := ReadScope(d.Paths)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	switch {
	case !found:
		return doneBecause("no scope is recorded, so there is nothing to compare")
	case scope.Scope == ScopeOff:
		return doneBecause("the scope is off")
	case scope.WaivedAt != "":
		return doneBecause("the compare is waived: " + scope.WaivedReason)
	}

	status, err := d.Status()
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	open, err := AgentSessionOpen(d.Paths)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if open {
		live, told, err := p.probeInterruptedAgent(ctx)
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		if live {
			return p.runAgent(ctx, told)
		}
		if err := CloseAgentSession(d.Paths); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
	}

	gateResume := p.resumedFrom(scope.GateHaltAt, status)
	if scope.GateHaltAt != "" && !gateResume {
		if scope, err = p.updateScope(func(cur *ScopeRecord) { cur.GateHaltAt = "" }); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
	}
	if scope.Scope == ScopeFull && p.resumedFrom(scope.HaltNamedAt, status) {
		scope, err = p.updateScope(func(cur *ScopeRecord) {
			for i, acceptance := range cur.Accepted {
				if slices.Contains(cur.HaltNamed, acceptance.Test) && acceptance.Ruling == RulingUnruled {
					cur.Accepted[i].Ruling = RulingRuled
				}
			}
			cur.HaltNamed, cur.HaltNamedAt = nil, ""
		})
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
	}

	if scope.Scope == ScopeFull && len(scope.Unruled()) > 0 {
		return p.halt(false, generalHaltOpening, "the Drift-Verify agent accepted drift that only the run's parent may rule on")
	}

	capture, captureFound, err := ReadCapture(d.Paths)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if !CountedCapture(scope, capture, captureFound, d.AwaitRow) {
		return p.halt(true, generalHaltOpening, "no baseline capture counts, so there is nothing to compare against; way forward: lym drift waive --reason <text> followed by lym loom resume")
	}

	cfg, err := d.Config()
	if err == nil {
		_, err = cfg.Step(stepCompare)
	}
	if err != nil {
		return p.halt(false, generalHaltOpening, fmt.Sprintf("%v; then lym loom resume", err))
	}

	if outcome, pointer, stopped, err := p.guardCommits(gateResume); stopped || err != nil {
		return outcome, pointer, err
	}

	result, err := d.Compare(ctx)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if result.Passed {
		return doneBecause("the compare passed")
	}
	return p.runAgent(ctx, VerifyTold{CompareLog: result.Log, BaselineHead: capture.Head})
}

// probeInterruptedAgent reports whether the agent an earlier entry spawned still has a live chair, and the table text that entry was told.
func (p *DriftVerify) probeInterruptedAgent(ctx context.Context) (live bool, told VerifyTold, err error) {
	d := p.deps
	capture, captureFound, err := ReadCapture(d.Paths)
	if err != nil {
		return false, VerifyTold{}, err
	}
	if !captureFound {
		return false, VerifyTold{}, nil
	}
	told = VerifyTold{CompareLog: newestCompareLog(d.Paths.LogsDir), BaselineHead: capture.Head}
	table, err := p.tableFor(ctx, told)
	if err != nil {
		return false, told, err
	}
	probed, err := d.Seats.Probe(table)
	if err != nil {
		return false, told, fmt.Errorf("drift: %s: probe the interrupted agent: %w", p.name, err)
	}
	return probed.Chair != nil, told, nil
}

// newestCompareLog returns the path of the newest compare log under logsDir, and "" when there is none.
func newestCompareLog(logsDir string) string {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return ""
	}
	newest := ""
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "compare-") && strings.HasSuffix(name, ".log") && name > newest {
			newest = name
		}
	}
	if newest == "" {
		return ""
	}
	return filepath.Join(logsDir, newest)
}

// resumedFrom reports whether this entry resumes from a halt the producer recorded at mark: the run's last history entry is a Stuck of this row whose latest time is at or after mark.
// Both times compare as instants at second precision, since the shed stamps its history after Call returns.
func (p *DriftVerify) resumedFrom(mark string, status shedengine.Status) bool {
	if mark == "" || len(status.History) == 0 {
		return false
	}
	last := status.History[len(status.History)-1]
	if last.Producer != p.name || last.Outcome != shedengine.Stuck {
		return false
	}
	latest := last.LastAt
	if latest == "" {
		latest = last.At
	}
	at, atErr := time.Parse(time.RFC3339, latest)
	marked, markErr := time.Parse(time.RFC3339, mark)
	if atErr != nil || markErr != nil {
		return false
	}
	return !at.Truncate(time.Second).Before(marked.Truncate(time.Second))
}

// updateScope applies change to the scope record and returns the record as written.
func (p *DriftVerify) updateScope(change func(cur *ScopeRecord)) (ScopeRecord, error) {
	var written ScopeRecord
	err := UpdateScope(p.deps.Paths, func(cur ScopeRecord, _ bool) (ScopeRecord, error) {
		change(&cur)
		written = cur
		return cur, nil
	})
	return written, err
}

// now returns the told clock, or the system clock when none was told.
func (p *DriftVerify) now() time.Time {
	if p.deps.Now == nil {
		return time.Now()
	}
	return p.deps.Now()
}

// markGateHalt records gate_halt_at when it is unset, so a resume's halt keeps the time it read.
func (p *DriftVerify) markGateHalt() error {
	return UpdateScope(p.deps.Paths, func(cur ScopeRecord, _ bool) (ScopeRecord, error) {
		if cur.GateHaltAt == "" {
			cur.GateHaltAt = stamp(p.now())
		}
		return cur, nil
	})
}

// guardCommits runs the handoff's first two checks again: a clean tracked tree and, on a resume from a gate halt, the verify gate over the agent's commits.
// stopped is true when it returned a Stuck.
// It clears gate_halt_at once the tree is clean and the gate passed or was not due.
func (p *DriftVerify) guardCommits(gateResume bool) (outcome shedengine.Outcome, pointer shedengine.OutputPointer, stopped bool, err error) {
	d := p.deps
	dirty, err := d.DirtyPaths()
	if err != nil {
		return "", shedengine.OutputPointer{}, false, err
	}
	if len(dirty) > 0 {
		if err := p.markGateHalt(); err != nil {
			return "", shedengine.OutputPointer{}, false, err
		}
		outcome, pointer, err = p.halt(false, dirtyHaltOpening, fmt.Sprintf("%s; way forward: commit or clean them, then lym loom resume", strings.Join(dirty, ", ")))
		return outcome, pointer, true, err
	}

	if gateResume {
		attempt, attemptFound, err := AttemptHead(d.Paths)
		if err != nil {
			return "", shedengine.OutputPointer{}, false, err
		}
		head, err := d.Head()
		if err != nil {
			return "", shedengine.OutputPointer{}, false, err
		}
		if attemptFound && attempt != head {
			result, err := p.verify.Gate()
			if err != nil {
				return "", shedengine.OutputPointer{}, false, err
			}
			if !result.Passed {
				outcome, pointer, err = p.halt(false, gateHaltOpening, fmt.Sprintf("the verify gate failed on the agent's commits; %s; way forward: fix the tests and commit, then lym loom resume", p.keepFindings(result.Findings)))
				return outcome, pointer, true, err
			}
		}
		if _, err := p.updateScope(func(cur *ScopeRecord) { cur.GateHaltAt = "" }); err != nil {
			return "", shedengine.OutputPointer{}, false, err
		}
	}
	return "", shedengine.OutputPointer{}, false, nil
}

// keepFindings writes findings to the gate findings file and returns the clause naming it, or the findings inline when none can be kept.
func (p *DriftVerify) keepFindings(findings string) string {
	file := p.deps.GateFindingsFile
	if file != "" {
		err := os.MkdirAll(filepath.Dir(file), 0o755)
		if err == nil {
			err = os.WriteFile(file, []byte(findings), 0o644)
		}
		if err == nil {
			return "findings: " + file
		}
	}
	return "findings: " + strings.Join(strings.Fields(findings), " ")
}

// halt returns a Stuck whose reason is opening followed by detail.
// The reason opens with shedadapters.DecisionHaltPrefix when always is set or the scope is full.
// Under scope full, while the scope record holds unruled acceptances, the reason also names them with the parent's ways forward for them, and their names and the time go into the scope record.
func (p *DriftVerify) halt(always bool, opening, detail string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	reason, err := p.haltReason(always, opening, detail)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
}

// haltReason builds the reason halt returns, and records the unruled acceptances it names.
func (p *DriftVerify) haltReason(always bool, opening, detail string) (string, error) {
	scope, _, err := ReadScope(p.deps.Paths)
	if err != nil {
		return "", err
	}
	reason := opening + detail
	if always || scope.Scope == ScopeFull {
		reason = shedadapters.DecisionHaltPrefix + reason
	}
	if scope.Scope != ScopeFull {
		return reason, nil
	}
	unruled := scope.Unruled()
	if len(unruled) == 0 {
		return reason, nil
	}
	named := make([]string, len(unruled))
	for i, acceptance := range unruled {
		named[i] = acceptance.Test
	}
	_, err = p.updateScope(func(cur *ScopeRecord) {
		cur.HaltNamed, cur.HaltNamedAt = named, stamp(p.now())
	})
	if err != nil {
		return "", err
	}
	return reason + fmt.Sprintf("; the agent's acceptances awaiting the parent's ruling: %s; for them: lym loom resume to stand by them, after any lym drift accept of your own, or lym drift withdraw <test> followed by lym loom resume to reject one", strings.Join(named, ", ")), nil
}

// recordingSeats passes every call to a seat runner and keeps the result of the last run or resume.
type recordingSeats struct {
	inner shedadapters.SeatRunner
	last  *seatengine.Result
}

func (r *recordingSeats) Probe(table seatengine.Table) (seatengine.LiveTable, error) {
	return r.inner.Probe(table)
}

func (r *recordingSeats) Resume(table seatengine.Table, live seatengine.LiveTable) (seatengine.Result, error) {
	result, err := r.inner.Resume(table, live)
	r.last = &result
	return result, err
}

func (r *recordingSeats) Run(table seatengine.Table) (seatengine.Result, error) {
	result, err := r.inner.Run(table)
	r.last = &result
	return result, err
}

// failedGate returns the name of the gate entry the chair's gate ended on in state failed, and "" when none did.
func (r *recordingSeats) failedGate() string {
	if r.last == nil || r.last.Chair.Gate == nil {
		return ""
	}
	for _, entry := range r.last.Chair.Gate.Entries {
		if entry.State == shuttleengine.GateEntryFailed {
			return entry.Name
		}
	}
	return ""
}

// tableFor builds the agent's table for told, gated on a clean tree, the told verify entry and a final compare.
func (p *DriftVerify) tableFor(ctx context.Context, told VerifyTold) (seatengine.Table, error) {
	table, err := p.deps.VerifyTable(told)
	if err != nil {
		return seatengine.Table{}, err
	}
	table.Gate = shuttleengine.GateSpec{p.dirtyEntry(), p.verifyEntry(), p.compareEntry(ctx)}
	return table, nil
}

// dirtyEntry fails while the code checkout's tracked tree is dirty, asking the session to commit or clean it.
func (p *DriftVerify) dirtyEntry() shuttleengine.GateEntry {
	return shuttleengine.GateEntry{
		Name:     dirtyGateName,
		Attempts: dirtyGateAttempts,
		Gate: func() (shuttleengine.GateResult, error) {
			dirty, err := p.deps.DirtyPaths()
			if err != nil {
				return shuttleengine.GateResult{}, err
			}
			if len(dirty) > 0 {
				return shuttleengine.GateResult{Findings: fmt.Sprintf("The tracked tree is dirty: %s.\nCommit these changes, or restore them to HEAD, and end your turn again.", strings.Join(dirty, ", "))}, nil
			}
			return shuttleengine.GateResult{Passed: true}, nil
		},
	}
}

// verifyEntry is the told verify entry, passing without running while HEAD is still the commit the attempt started at.
func (p *DriftVerify) verifyEntry() shuttleengine.GateEntry {
	entry := p.verify
	entry.Gate = p.onceCommitted(entry.Gate)
	entry.Forked = p.onceCommitted(entry.Forked)
	entry.Final = p.onceCommitted(entry.Final)
	return entry
}

// onceCommitted wraps gate to pass without running while HEAD equals the attempt head; a nil gate stays nil.
func (p *DriftVerify) onceCommitted(gate shuttleengine.Gate) shuttleengine.Gate {
	if gate == nil {
		return nil
	}
	return func() (shuttleengine.GateResult, error) {
		attempt, attemptFound, err := AttemptHead(p.deps.Paths)
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		head, err := p.deps.Head()
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if attemptFound && attempt == head {
			return shuttleengine.GateResult{Passed: true}, nil
		}
		return gate()
	}
}

// compareEntry runs the compare once more and ends the session at once when drift remains, so no fix goes uncompared.
func (p *DriftVerify) compareEntry(ctx context.Context) shuttleengine.GateEntry {
	return shuttleengine.GateEntry{
		Name:     compareGateName,
		Attempts: 1,
		Gate: func() (shuttleengine.GateResult, error) {
			result, err := p.deps.Compare(ctx)
			if err != nil {
				return shuttleengine.GateResult{}, err
			}
			if result.Passed {
				return shuttleengine.GateResult{Passed: true}, nil
			}
			return shuttleengine.GateResult{Findings: "the compare still shows drift: " + result.Reason, Terminal: true}, nil
		},
	}
}

// runAgent spawns the Drift-Verify agent, or resumes the live one, and maps its end.
// The agent's end closes the agent-session mark, unless the context ended, which leaves a possibly live agent for the next entry to reattach to.
func (p *DriftVerify) runAgent(ctx context.Context, told VerifyTold) (shedengine.Outcome, shedengine.OutputPointer, error) {
	d := p.deps
	seats := &recordingSeats{inner: d.Seats}
	tables := func() (seatengine.Table, error) { return p.tableFor(ctx, told) }
	prepare := func() (string, error) {
		head, err := d.Head()
		if err != nil {
			return "", err
		}
		return "", ResetRuns(d.Paths, head)
	}
	agent := shedadapters.NewMultiLLMProducerSourced(p.name, tables, seats, d.Now, prepare).KeepGateFindings(d.GateFindingsFile)
	outcome, pointer, err := agent.Call(ctx)
	if ctx.Err() == nil {
		err = errors.Join(err, CloseAgentSession(d.Paths))
	}
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	scope, _, err := ReadScope(d.Paths)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	switch outcome {
	case shedengine.Done:
		if scope.Scope == ScopeFull && len(scope.Unruled()) > 0 {
			return p.haltFrom(pointer, false, generalHaltOpening, "the Drift-Verify agent accepted drift that only the run's parent may rule on")
		}
		return shedengine.Done, pointer, nil
	case shedengine.Stuck:
		return p.stuckAfterAgent(pointer, seats.failedGate(), scope)
	}
	return outcome, pointer, nil
}

// haltFrom is halt keeping the agent's output pointer.
func (p *DriftVerify) haltFrom(pointer shedengine.OutputPointer, always bool, opening, detail string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	reason, err := p.haltReason(always, opening, detail)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	pointer.Reason = reason
	return shedengine.Stuck, pointer, nil
}

// stuckAfterAgent words the Stuck the agent's gate ended on.
// A compare that still fails names the report and the ways forward; a dirty tree or a failing verify entry that spent its attempts keeps the gate's findings, records gate_halt_at and opens with its halt text.
func (p *DriftVerify) stuckAfterAgent(pointer shedengine.OutputPointer, failed string, scope ScopeRecord) (shedengine.Outcome, shedengine.OutputPointer, error) {
	switch failed {
	case compareGateName:
		detail := fmt.Sprintf("%s; the agent's report is %s; way forward: lym drift accept <test> --reason <text> followed by lym loom resume, or fix the drift and lym loom resume", pointer.Reason, path.Join(DirRel(), filepath.Base(p.deps.Paths.VerifyReport)))
		return p.haltFrom(pointer, scope.Scope == ScopePartial && len(scope.Withdrawn) > 0, compareHaltOpening, detail)
	case dirtyGateName:
		if err := p.markGateHalt(); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		return p.haltFrom(pointer, false, dirtyHaltOpening, pointer.Reason+"; way forward: commit or clean the tree, then lym loom resume")
	case "":
		return p.haltFrom(pointer, false, generalHaltOpening, pointer.Reason)
	}
	if err := p.markGateHalt(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	return p.haltFrom(pointer, false, gateHaltOpening, pointer.Reason+"; way forward: fix the tests and commit, then lym loom resume")
}
