// rowgate.go wraps a row gate so that its failure goes to a cold fixer at the writer's idle boundary,
// and the gate re-runs over the fixer's finished tree.

package gatefixer

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// awaitSlice is how long one call to the fixer's wait blocks before the wrap asks again.
const awaitSlice = time.Minute

// RowFixer holds what a row gate's wrap needs to hand a failure to a cold fixer.
type RowFixer struct {
	// Fixer spawns, waits on and stops the fixer strands.
	Fixer Fixer
	// VisitDir returns the directory of the row visit, under which the wrap keeps its records.
	VisitDir func() (string, error)
	// Spec builds the fixer's launch from the report the fixer reads and the reply file it writes.
	Spec func(reportPath, replyPath string) (Spec, error)
	// Timeout bounds each fixer's run.
	Timeout time.Duration
	// Forked reports whether the row's run evaluates the entries' Forked closures; the row decides this when it is built.
	Forked bool
	// SkipForked, when non-nil, reports whether a forked failure's findings are for the writer alone, so no fixer runs.
	SkipForked func(findings string) bool
}

// fixerOutcome is what one wait on a fixer ended with.
type fixerOutcome struct {
	status     string
	deadReason string
	reply      string
	// replaceErr is why the replacement of an interrupted fixer could not be started.
	replaceErr error
}

// note is the text a failing gate's findings carry for this outcome: the fixer's reply, or why it ended without one.
func (o fixerOutcome) note() string {
	switch {
	case o.status == StatusDone:
		return o.reply
	case o.deadReason == DeadReasonInterrupted:
		cause := "The cold fixer was interrupted, by a machine stop or a killed pane, before it replied"
		if o.replaceErr != nil {
			cause += fmt.Sprintf(", and its replacement could not be started (%v)", o.replaceErr)
		}
		return cause + "; any change it left uncommitted is in the worktree."
	}
	return fmt.Sprintf("The cold fixer ended without a reply (%s); any change it left uncommitted is in the worktree.", o.deadReason)
}

// rowWrap is the state of one wrapped gate: the entry that spawns fixers, its re-prompt budget and the strands it knows.
type rowWrap struct {
	rf       RowFixer
	entry    string
	attempts int
	// rerun is the fixer entry's own mark, set while the gate runs again after a fixer; nil when the entry has none.
	rerun *shuttleengine.RerunMark

	mu sync.Mutex
	// known holds the strands this wrap spawned and those it left live after they ended interrupted.
	known map[string]bool
}

// WrapRowGate returns a copy of spec whose entry named entry hands a failure to a cold fixer.
// Its other entries wait for a running fixer before they judge.
// Every entry keeps its name, attempts, pass-on-cap flag and re-prompt line.
//
// The named entry's gate, and its forked gate when set, run the inner closure once;
// a failure spawns one fixer with the findings as its report, and waits for it;
// the closure then runs again, and its verdict is returned with the fixer's reply appended to a failure.
// No fixer is spawned for a passing, pending, terminal, writer-only or erroring result;
// a writer-only failure is returned as it is.
// The run after the fixer goes through the entry's re-run mark,
// and a writer-only verdict of it carries the fixer's reply.
// It is not spawned for a forked failure SkipForked reports, or once the entry's set holds attempts plus one records.
// A record of an interrupted fixer does not count toward that bound.
// A fixer that cannot be started leaves the inner failure standing with a line saying no fixer ran.
// A live record in the set is waited on instead of spawning, so a restarted driver finds the fixer it left running.
// The named entry, its final closure included, may wait on a fixer that ends interrupted.
// It then spawns a replacement on the same report and reply files, and judges once the replacement ends.
// The replacement resumes the interrupted session where shuttle can.
// The set may come to hold more than attempts plus one interrupted records.
// A further interruption then stands as the fixer's outcome.
// The record turns terminal when the spawning entry returns a result judged over the fixer's finished tree.
// It also turns terminal when any other entry fails over it, unless the fixer ended interrupted:
// that record stays live for the named entry to replace, and Expired no longer reports it.
// An entry that passes over a live record leaves it live;
// a later entry still folds the fixer's reply into its findings.
// The entry's final closure spawns no fixer for a failure of its own.
// It waits for a live record, then judges once, and that run goes through the entry's re-run mark.
// An entry naming no entry of spec is refused.
func WrapRowGate(spec shuttleengine.GateSpec, entry string, rf RowFixer) (shuttleengine.GateSpec, error) {
	at := slices.IndexFunc(spec, func(candidate shuttleengine.GateEntry) bool { return candidate.Name == entry })
	if at < 0 {
		return nil, fmt.Errorf("gatefixer: the gate has no entry %q to hand a fixer; way forward: name the entry that carries the fixer element", entry)
	}
	wrap := &rowWrap{rf: rf, entry: entry, attempts: spec[at].Attempts, known: map[string]bool{}}

	wrapped := make(shuttleengine.GateSpec, len(spec))
	for i, candidate := range spec {
		if i == at {
			wrapped[i] = wrap.fixerEntry(candidate)
			continue
		}
		candidate.Gate = wrap.waitOnly(candidate.Gate, false, false)
		candidate.Forked = wrap.waitOnly(candidate.Forked, false, false)
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
	w.rerun = original.Rerun
	wrapped := original
	wrapped.Gate = w.spawning(original.Gate, false)
	if original.Forked != nil {
		wrapped.Forked = w.spawning(original.Forked, true)
	}
	wrapped.MayHold = true
	wrapped.Final = w.waitOnly(finalInner, true, true)
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
// markOnPass says whether a passing or pending inner marks the live record terminal.
// A failing one does, except on an entry that does not carry the fixer, over a fixer that ended interrupted.
// afterFixer says whether the entry is the one that carries the fixer:
// its run over a live fixer's finished tree goes through the re-run mark, and it replaces an interrupted fixer.
func (w *rowWrap) waitOnly(inner shuttleengine.Gate, markOnPass, afterFixer bool) shuttleengine.Gate {
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
		judge := inner
		if afterFixer {
			judge = func() (shuttleengine.GateResult, error) { return w.rerun.Run(inner) }
		}
		return w.waitThenRun(store, live, judge, markOnPass, afterFixer)
	}
}

// spawning returns inner behind the fixer hand-off.
// It waits on a live record, and on a failure of its own it spawns a fixer and judges again.
func (w *rowWrap) spawning(inner shuttleengine.Gate, forked bool) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		store, live, err := w.liveRecord()
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if live != nil {
			return w.waitThenRun(store, live, func() (shuttleengine.GateResult, error) { return w.rerun.Run(inner) }, true, true)
		}
		failed, err := inner()
		if err != nil || failed.Passed || failed.Pending || failed.Terminal || failed.WriterOnly {
			return failed, err
		}
		spent, _, err := store.budget()
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if spent >= w.attempts+1 || (forked && w.rf.SkipForked != nil && w.rf.SkipForked(failed.Findings)) {
			return failed, nil
		}
		return w.spawnAndJudgeAgain(store, spent+1, inner, failed)
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

// spawnAndJudgeAgain writes failed's findings as the report of the fixer numbered n;
// it spawns that fixer and returns inner's verdict once the fixer has ended, inner run under the entry's re-run mark.
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
		w.markKnown(rec.StrandGUID)
	}
	return w.waitThenRun(store, rec, func() (shuttleengine.GateResult, error) { return w.rerun.Run(inner) }, true, true)
}

func (w *rowWrap) markKnown(guid string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.known[guid] = true
}

// waitThenRun waits for rec's fixer and runs inner once.
// The record is marked terminal only after inner has returned a result,
// so a driver restarted during the run, or after an inner error, still finds the fixer live.
// A passing or pending result judges nothing over the finished tree;
// when markOnPass is false, it leaves the record live.
// replace says whether an interrupted fixer is replaced before inner runs.
// Without it, the record of a fixer that ended interrupted is never marked terminal here.
// A failing verdict carries the fixer's reply or dead note.
func (w *rowWrap) waitThenRun(store rowStore, rec *Record, inner shuttleengine.Gate, markOnPass, replace bool) (shuttleengine.GateResult, error) {
	outcome, rec, err := w.waitReplacingInterrupted(store, rec, replace)
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
	if outcome.deadReason == DeadReasonInterrupted && !replace {
		// The record stays live while the writer reworks this failure,
		// so it must not read as expired and finalize the writer mid-turn.
		w.markKnown(rec.StrandGUID)
	} else if err := store.markTerminal(rec.StrandGUID, outcome.status, outcome.deadReason); err != nil {
		return shuttleengine.GateResult{}, err
	}
	if result.Passed || result.Pending || result.Terminal {
		return result, nil
	}
	result.Findings += "\n\n## Cold fixer reply\n\n" + outcome.note()
	return result, nil
}

// waitReplacingInterrupted waits for rec's fixer and returns its outcome and the record the wait ended on.
// With replace set, a fixer that ends interrupted is marked terminal,
// and a replacement is spawned on its report and reply files and waited on in turn.
// The replacement's output file is the interrupted run's own, so shuttle resumes the interrupted session where it can.
// The interruption stands as the outcome when a replacement cannot be started.
// It also stands once the set holds more than attempts plus one interrupted records.
func (w *rowWrap) waitReplacingInterrupted(store rowStore, rec *Record, replace bool) (fixerOutcome, *Record, error) {
	for {
		outcome, err := w.wait(store, rec)
		if err != nil || !replace || outcome.deadReason != DeadReasonInterrupted {
			return outcome, rec, err
		}
		if err := store.markTerminal(rec.StrandGUID, outcome.status, outcome.deadReason); err != nil {
			return fixerOutcome{}, nil, err
		}
		_, interrupted, err := store.budget()
		if err != nil {
			return fixerOutcome{}, nil, err
		}
		if interrupted > w.attempts+1 {
			return outcome, rec, nil
		}
		spec, err := w.rf.Spec(rec.ReportPath, rec.ReplyPath)
		if err != nil {
			return fixerOutcome{}, nil, fmt.Errorf("gatefixer: build the interrupted fixer's replacement launch: %w", err)
		}
		next, spawned, err := w.rf.Fixer.SpawnOrAttach(store, spec)
		if err != nil {
			// Logged here, since a passing judgment carries no findings to say it in.
			logger.Warn("gatefixer: the interrupted fixer's replacement could not be started", "strandGUID", rec.StrandGUID, "error", err)
			outcome.replaceErr = err
			return outcome, rec, nil
		}
		if spawned {
			w.markKnown(next.StrandGUID)
		}
		rec = next
	}
}

// wait asks the fixer to wait until its answer is terminal.
// A wait that fails stops the strand and marks the record dead,
// so a fixer whose engine or events can no longer be read never blocks the visit;
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

// expired reports whether the set holds a live record whose strand this wrap does not know;
// only a driver restarted onto the visit sees such a record.
func (w *rowWrap) expired() (bool, error) {
	_, live, err := w.liveRecord()
	if err != nil || live == nil {
		return false, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return !w.known[live.StrandGUID], nil
}
