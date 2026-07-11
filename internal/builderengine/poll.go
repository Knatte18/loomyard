// poll.go implements the `poll` verb's engine core: Classify, the pure
// decision function that re-derives an in-flight implementer's cross-
// process terminal state (nobody in poll's process holds the shuttle Run
// handle — spawn-batch exits right after Start — so this is re-derived from
// files and a live mux query every tick, never from an in-process handle);
// the two impure gatherers Classify's caller feeds from (turnEnded,
// strandLive), both riding shuttle's provider-invariant seams per the
// Shuttle Provider-Seam Invariant — builderengine never parses event
// grammar or pane state itself; and PollUntilTerminal, the blocking
// long-poll loop that re-runs a caller-supplied gather function on a fixed
// tick until a batch reaches a terminal classification or the wait budget
// elapses. The long-poll IS the notification (the discussion's `poll`
// semantics decision): the loop blocks inside Go, costing the orchestrator
// nothing per tick, and returns the instant the batch terminates.

package builderengine

import (
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ClassifyInputs carries every signal Classify's decision needs. Changed,
// Scope, and Dirty are the digest-computation inputs (git diff / scope
// prefixes / worktree cleanliness) and MUST be filled only when Report is
// non-nil — every caller-side gather implementation checks for the report
// FIRST and runs the gitquery helpers exclusively inside that
// report-present branch, since a running snapshot never touches git (the
// discussion: drift judgment on a half-done batch is noise, and a literal
// every-tick diff would be a defect at the 1s poll tick).
type ClassifyInputs struct {
	// BatchNumber and BatchSlug together name the batch being classified,
	// used to compose the NN-<batch-slug> identifier a non-report Digest
	// carries (a report-present Digest instead carries the report's own
	// "batch:" field verbatim, via Distill).
	BatchNumber int
	BatchSlug   string

	// ReportPath is the batch-report file path Classify's caller checked
	// for; it is informational only (Classify itself branches on Report,
	// never re-reads ReportPath), kept here so a caller building
	// ClassifyInputs has a natural place to record what it looked at.
	ReportPath string
	// Report is the parsed batch-report when the caller found one, nil
	// when it did not. A non-nil Report always wins classification,
	// regardless of every other field's value.
	Report *Report

	// TurnEnded reports whether the implementer's turn has ended (a Stop
	// event was observed) — see the package-level turnEnded gatherer.
	TurnEnded bool
	// StrandLive reports whether the implementer's mux strand is still
	// live — see the package-level strandLive gatherer.
	StrandLive bool

	// Elapsed is the wall-clock duration since this batch's implementer
	// was spawned.
	Elapsed time.Duration
	// BatchTimeout is the configured ceiling (builder.yaml's
	// batch_timeout_min) Elapsed is compared against.
	BatchTimeout time.Duration

	// Changed, Scope, and Dirty are Distill's inputs, populated only when
	// Report is non-nil (see the type doc above).
	Changed []string
	Scope   []string
	Dirty   bool
}

// Classify decides a batch's classification from ins, in the discussion's
// pinned decision order, returning the Digest to report and whether that
// classification is terminal:
//
//  1. Report present: terminal, via Distill(in.Report, in.Changed,
//     in.Scope, in.Dirty) — status is done or stuck per the report itself.
//  2. No report, TurnEnded: terminal dead, DeadReasonAsking — the
//     implementer ended its turn without ever satisfying the file
//     contract, which is respawn/recover material, same as a crash.
//  3. No report, Elapsed > BatchTimeout: terminal dead, DeadReasonTimeout.
//  4. No report, turn still in progress (TurnEnded false), strand pane
//     gone (StrandLive false): terminal dead, DeadReasonDied.
//  5. Otherwise: non-terminal running snapshot carrying only batch,
//     status, and ElapsedS — no digest field beyond those three is
//     populated, since a running snapshot never computes drift.
func Classify(in ClassifyInputs) (Digest, bool) {
	batch := fmt.Sprintf("%02d-%s", in.BatchNumber, in.BatchSlug)

	if in.Report != nil {
		return Distill(in.Report, in.Changed, in.Scope, in.Dirty), true
	}

	if in.TurnEnded {
		return Digest{Batch: batch, Status: DigestStatusDead, DeadReason: DeadReasonAsking}, true
	}

	if in.Elapsed > in.BatchTimeout {
		return Digest{Batch: batch, Status: DigestStatusDead, DeadReason: DeadReasonTimeout}, true
	}

	if !in.StrandLive {
		return Digest{Batch: batch, Status: DigestStatusDead, DeadReason: DeadReasonDied}, true
	}

	return Digest{Batch: batch, Status: DigestStatusRunning, ElapsedS: int(in.Elapsed.Seconds())}, false
}

// turnEnded reports whether the implementer's turn has already ended
// without ever satisfying the file contract: it reads eventsPath's raw
// bytes and delegates all event-grammar parsing to engine.ParseEvents (the
// Shuttle Provider-Seam Invariant — builderengine never parses event
// grammar itself), reporting true only when at least one returned Event
// carries Kind == shuttleengine.EventStop. A not-yet-created events file
// (the implementer has not appended its first line yet) is not an error:
// it reports (false, nil) unchanged. A ParseEvents error propagates — an
// unreadable event grammar leaves no classifiable turn-end signal at all,
// which the caller must treat as a poll-tick failure, never a silently
// assumed "still running".
func turnEnded(eventsPath string, engine shuttleengine.Engine) (bool, error) {
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("builder: read events file %s: %w", eventsPath, err)
	}

	events, err := engine.ParseEvents(data)
	if err != nil {
		return false, fmt.Errorf("builder: parse events %s: %w", eventsPath, err)
	}

	for _, e := range events {
		if e.Kind == shuttleengine.EventStop {
			return true, nil
		}
	}
	return false, nil
}

// strandLive reports whether guid names a strand mux currently tracks as
// live: it calls mux.Status() and scans the returned Strands for guid's
// Live field. guid absent from the result reports (false, nil) — mux no
// longer tracks it, which the caller treats identically to a pane that
// died. Liveness is NEVER read from persisted mux state
// (muxengine.LoadState carries no liveness field at all); only this live
// Status() query can answer "is the pane actually there right now".
func strandLive(mux shuttleengine.MuxOps, guid string) (bool, error) {
	status, err := mux.Status()
	if err != nil {
		return false, fmt.Errorf("builder: mux status: %w", err)
	}
	for _, s := range status.Strands {
		if s.GUID == guid {
			return s.Live, nil
		}
	}
	return false, nil
}

// clock abstracts time.Now/time.Sleep so PollUntilTerminal's wait loop runs
// instantly under test, mirroring shuttleengine's wait.go seam
// (internal/shuttleengine/wait.go): a fake clock's Sleep advances a virtual
// "now" rather than blocking, letting a test replay a whole poll sequence
// instantly.
type clock interface {
	Now() time.Time
	Sleep(d time.Duration)
}

// realClock is the production clock: real wall-clock time, real sleeping.
type realClock struct{}

func (realClock) Now() time.Time        { return time.Now() }
func (realClock) Sleep(d time.Duration) { time.Sleep(d) }

// pollTick is PollUntilTerminal's fixed re-run interval. The long-poll IS
// the notification (the discussion's `poll` semantics decision): blocking
// inside Go on a short tick costs the orchestrator nothing, so 1 second
// keeps the loop responsive without hammering gather's own I/O.
const pollTick = 1 * time.Second

// PollUntilTerminal repeatedly calls gather on pollTick's cadence until it
// reports terminal or wait elapses, timing itself via clk (realClock{} in
// production; a fake clock replays an entire poll sequence instantly under
// test). A terminal gather result returns immediately. If wait elapses
// first, PollUntilTerminal returns gather's last non-terminal ("running")
// digest with a nil error — the snapshot the orchestrator's next poll call
// re-polls from; a deadline is an ordinary long-poll return, never a
// failure. A gather error propagates immediately: a tick that cannot even
// determine whether the batch is terminal yet has nothing safe to report.
func PollUntilTerminal(gather func() (Digest, bool, error), wait time.Duration, clk clock) (Digest, error) {
	deadline := clk.Now().Add(wait)

	for {
		digest, terminal, err := gather()
		if err != nil {
			return Digest{}, err
		}
		if terminal {
			return digest, nil
		}
		if clk.Now().After(deadline) {
			return digest, nil
		}
		clk.Sleep(pollTick)
	}
}
