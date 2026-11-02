// rowgate.go implements the wrap that hands a row gate's failure to a cold fixer at the writer's idle boundary and re-runs the gate over the fixer's finished tree.

package gatefixer

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// awaitSlice is how long one call to the fixer's wait blocks before the wrap asks again.
const awaitSlice = time.Minute

// RowFixer is what a row's gate wrap needs to hand a failure to a cold fixer.
type RowFixer struct {
	// Fixer spawns, waits on and stops the fixer strands.
	Fixer Fixer
	// VisitDir returns the directory of the row visit, under which the wrap keeps its records.
	VisitDir func() (string, error)
	// Spec builds the fixer's launch from the report the fixer reads and the reply file it writes.
	Spec func(reportPath, replyPath string) (Spec, error)
	// Timeout bounds each fixer's run.
	Timeout time.Duration
	// Forked tells whether the row's run evaluates the entries' Forked closures, which the row decides when it is built.
	Forked bool
	// SkipForked, when non-nil, reports whether a failing Forked evaluation's findings are for the writer alone, so no fixer is spawned; nil skips nothing.
	SkipForked func(findings string) bool
}

// fixerOutcome is what one wait on a fixer ended with.
type fixerOutcome struct {
	status     string
	deadReason string
	reply      string
}

// note is the text a failing gate's findings carry for this outcome: the fixer's reply, or why it ended without one.
func (o fixerOutcome) note() string {
	if o.status == StatusDone {
		return o.reply
	}
	return fmt.Sprintf("The cold fixer ended without a reply (%s); any change it left uncommitted is in the worktree.", o.deadReason)
}

// rowWrap is the state of one wrapped gate: the entry that spawns fixers, its re-prompt budget and the strands this wrap spawned.
type rowWrap struct {
	rf       RowFixer
	entry    string
	attempts int

	mu      sync.Mutex
	spawned map[string]bool
}

// WrapRowGate returns a copy of spec whose entry named entry hands a failure to a cold fixer, and whose other entries wait for a running fixer before they judge.
// Every entry keeps its name, attempts, pass-on-cap flag and re-prompt line.
//
// The named entry's gate, and its forked gate when set, run the inner closure once;
// a failure spawns one fixer with the findings as its report, waits for it, runs the closure again and returns that verdict, with the fixer's reply appended to a failure.
// No fixer is spawned for a passing, pending, terminal or erroring result, for a forked failure SkipForked reports, or once the entry's set holds attempts plus one records.
// A fixer that cannot be started leaves the inner failure standing with a line saying no fixer ran.
// A live record in the set is waited on instead of spawning, so a restarted driver finds the fixer it left running.
// The record turns terminal when the entry that spawns fixers returns a result judged over the fixer's finished tree, or when any other entry fails over it;
// an entry that passes over a live record leaves it live, so a later entry still folds the fixer's reply into its findings.
// The entry's final closure never spawns; it only waits for a live record, then judges once.
// An entry naming no entry of spec is refused.
func WrapRowGate(spec shuttleengine.GateSpec, entry string, rf RowFixer) (shuttleengine.GateSpec, error) {
	at := slices.IndexFunc(spec, func(candidate shuttleengine.GateEntry) bool { return candidate.Name == entry })
	if at < 0 {
		return nil, fmt.Errorf("gatefixer: the gate has no entry %q to hand a fixer; way forward: name the entry that carries the fixer element", entry)
	}
	wrap := &rowWrap{rf: rf, entry: entry, attempts: spec[at].Attempts, spawned: map[string]bool{}}

	wrapped := make(shuttleengine.GateSpec, len(spec))
	for i, candidate := range spec {
		if i == at {
			wrapped[i] = wrap.fixerEntry(candidate)
			continue
		}
		candidate.Gate = wrap.waitOnly(candidate.Gate, false)
		candidate.Forked = wrap.waitOnly(candidate.Forked, false)
		wrapped[i] = candidate
	}
	return wrapped, nil
}

// fixerEntry returns the entry that spawns fixers, built from the original so the inner closures stay the row's own.
func (w *rowWrap) fixerEntry(original shuttleengine.GateEntry) shuttleengine.GateEntry {
	finalInner := original.Gate
	if w.rf.Forked && original.Forked != nil {
		finalInner = original.Forked
	}
	wrapped := original
	wrapped.Gate = w.spawning(original.Gate, false)
	if original.Forked != nil {
		wrapped.Forked = w.spawning(original.Forked, true)
	}
	wrapped.MayHold = true
	wrapped.Final = w.waitOnly(finalInner, true)
	wrapped.Expired = w.expired
	return wrapped
}

func (w *rowWrap) openStore() (rowStore, error) {
	visitDir, err := w.rf.VisitDir()
	if err != nil {
		return rowStore{}, fmt.Errorf("gatefixer: resolve the row visit directory: %w", err)
	}
	return newRowStore(visitDir, w.entry), nil
}

// waitOnly returns inner behind a wait on the set's live fixer, and nil for a nil inner.
// With no live record it runs inner at once.
// markOnPass says whether a passing or pending inner marks the live record terminal; a failing one always does.
func (w *rowWrap) waitOnly(inner shuttleengine.Gate, markOnPass bool) shuttleengine.Gate {
	if inner == nil {
		return nil
	}
	return func() (shuttleengine.GateResult, error) {
		store, live, err := w.liveRecord()
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if live == nil {
			return inner()
		}
		return w.waitThenRun(store, live, inner, markOnPass)
	}
}

// spawning returns inner behind the fixer hand-off: it waits on a live record, and on a failure of its own spawns a fixer and judges again.
func (w *rowWrap) spawning(inner shuttleengine.Gate, forked bool) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		store, live, err := w.liveRecord()
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if live != nil {
			return w.waitThenRun(store, live, inner, true)
		}
		failed, err := inner()
		if err != nil || failed.Passed || failed.Pending || failed.Terminal {
			return failed, err
		}
		held, err := store.count()
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if held >= w.attempts+1 || (forked && w.rf.SkipForked != nil && w.rf.SkipForked(failed.Findings)) {
			return failed, nil
		}
		return w.spawnAndJudgeAgain(store, held+1, inner, failed)
	}
}

func (w *rowWrap) liveRecord() (rowStore, *Record, error) {
	store, err := w.openStore()
	if err != nil {
		return rowStore{}, nil, err
	}
	live, err := store.Live()
	if err != nil {
		return rowStore{}, nil, err
	}
	return store, live, nil
}

// spawnAndJudgeAgain writes failed's findings as the report of the fixer that takes place n, spawns it and returns inner's verdict once the fixer has ended.
func (w *rowWrap) spawnAndJudgeAgain(store rowStore, n int, inner shuttleengine.Gate, failed shuttleengine.GateResult) (shuttleengine.GateResult, error) {
	reportPath, replyPath := store.reportPath(n), store.replyPath(n)
	if err := os.MkdirAll(store.dir, 0o755); err != nil {
		return shuttleengine.GateResult{}, fmt.Errorf("gatefixer: create the fixer directory %s: %w", store.dir, err)
	}
	if err := os.WriteFile(reportPath, []byte(failed.Findings), 0o644); err != nil {
		return shuttleengine.GateResult{}, fmt.Errorf("gatefixer: write the fixer report %s: %w", reportPath, err)
	}
	spec, err := w.rf.Spec(reportPath, replyPath)
	if err != nil {
		return shuttleengine.GateResult{}, fmt.Errorf("gatefixer: build the fixer's launch: %w", err)
	}
	rec, spawned, err := w.rf.Fixer.SpawnOrAttach(store, spec)
	if err != nil {
		failed.Findings += fmt.Sprintf("\n\nThe cold fixer could not be started (%v); no fixer ran.", err)
		return failed, nil
	}
	if spawned {
		w.mu.Lock()
		w.spawned[rec.StrandGUID] = true
		w.mu.Unlock()
	}
	return w.waitThenRun(store, rec, inner, true)
}

// waitThenRun waits for rec's fixer and runs inner once.
// The record is marked terminal only after inner has returned a result, so a driver restarted during the run, or after an inner error, still finds the fixer live;
// a passing or pending result, which judges nothing over the finished tree, leaves it live when markOnPass is false.
// A failing verdict carries the fixer's reply or dead note.
func (w *rowWrap) waitThenRun(store rowStore, rec *Record, inner shuttleengine.Gate, markOnPass bool) (shuttleengine.GateResult, error) {
	outcome, err := w.wait(store, rec)
	if err != nil {
		return shuttleengine.GateResult{}, err
	}
	result, err := inner()
	if err != nil {
		return shuttleengine.GateResult{}, err
	}
	if (result.Passed || result.Pending) && !markOnPass {
		return result, nil
	}
	if err := store.markTerminal(rec.StrandGUID, outcome.status, outcome.deadReason); err != nil {
		return shuttleengine.GateResult{}, err
	}
	if result.Passed || result.Pending || result.Terminal {
		return result, nil
	}
	result.Findings += "\n\n## Cold fixer reply\n\n" + outcome.note()
	return result, nil
}

// wait asks the fixer to wait until its answer is terminal.
// A wait that fails stops the strand and marks the record dead, so a fixer whose engine or events can no longer be read never blocks the visit;
// when that stop fails too, the record stays live and the joined error is returned.
func (w *rowWrap) wait(store rowStore, rec *Record) (fixerOutcome, error) {
	for {
		result, err := w.rf.Fixer.Await(rec, w.rf.Timeout, awaitSlice)
		if err != nil {
			if stopErr := w.rf.Fixer.Stop(rec); stopErr != nil {
				return fixerOutcome{}, errors.Join(err, stopErr)
			}
			if markErr := store.markTerminal(rec.StrandGUID, StatusDead, DeadReasonDied); markErr != nil {
				return fixerOutcome{}, errors.Join(err, markErr)
			}
			return fixerOutcome{status: StatusDead, deadReason: DeadReasonDied}, nil
		}
		if result.Status != StatusRunning {
			return fixerOutcome{status: result.Status, deadReason: result.DeadReason, reply: result.Reply}, nil
		}
	}
}

// expired reports whether the set holds a live record whose strand this wrap did not spawn, which only a driver restarted onto the visit sees.
func (w *rowWrap) expired() (bool, error) {
	_, live, err := w.liveRecord()
	if err != nil || live == nil {
		return false, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return !w.spawned[live.StrandGUID], nil
}
