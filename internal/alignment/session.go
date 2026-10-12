// session.go implements the alignment agent's lifecycle, shared by both points: spawn once per key, poll without blocking, close, and record what Go applied.
// The session never blocks its caller, since the parent-review gate polls it on each re-evaluation.

package alignment

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// The causes an Outcome carries when the session, not the agent's own valid verdict, ended an attempt.
const (
	// CauseTimeout means the agent ran past its deadline.
	CauseTimeout = "timeout"
	// CauseDied means the agent's run ended without a verdict, or no agent is live for the attempt.
	CauseDied = "died"
	// CauseMissing means the agent finished without writing a verdict.
	CauseMissing = "missing"
	// CauseInvalid means the agent's verdict file is invalid.
	CauseInvalid = "invalid"
	// CauseClosed means the attempt was closed, so its verdict is never read.
	CauseClosed = "closed"
)

// Run is the handle of a started or probed agent.
type Run interface {
	// Wait blocks until the agent ends and classifies how.
	Wait() (shuttleengine.Result, error)
	// StrandGUID names the agent's strand.
	StrandGUID() string
}

// Shuttle is the part of shuttle the session drives.
type Shuttle interface {
	// Start launches spec and returns its handle.
	Start(spec shuttleengine.Spec) (Run, error)
	// Probe returns the live run matching spec's output files, and false when none is live.
	Probe(spec shuttleengine.Spec) (Run, bool, error)
	// StopStrand ends the strand guid names and records the stop.
	StopStrand(guid string) error
}

// RunnerShuttle adapts the shuttle runner to Shuttle, probing with no gate.
func RunnerShuttle(r *shuttleengine.Runner) Shuttle {
	return runnerShuttle{runner: r}
}

type runnerShuttle struct {
	runner *shuttleengine.Runner
}

func (s runnerShuttle) Start(spec shuttleengine.Spec) (Run, error) {
	run, err := s.runner.Start(spec)
	if err != nil {
		return nil, err
	}
	return run, nil
}

func (s runnerShuttle) Probe(spec shuttleengine.Spec) (Run, bool, error) {
	run, found, err := s.runner.ProbeGated(spec, shuttleengine.GateSpec{})
	if err != nil || !found {
		return nil, false, err
	}
	return run, true, nil
}

func (s runnerShuttle) StopStrand(guid string) error {
	return s.runner.StopStrand(guid)
}

// SessionConfig carries what a Session works over.
// Root is one point's attempt directory, and StencilsDir is where the agent's stencil is read from.
// Now is the clock, where nil means time.Now.
// Token draws an attempt's random token, where nil means eight random bytes as hex.
type SessionConfig struct {
	Root        string
	StencilsDir string
	Shuttle     Shuttle
	Now         func() time.Time
	Token       func() string
}

// Session runs the agent's attempts at one point of a run.
type Session struct {
	cfg SessionConfig

	mu      sync.Mutex
	watches map[int]*watch
}

// watch is the in-process wait on one attempt's run: result and err are set before done is closed.
type watch struct {
	done   chan struct{}
	result shuttleengine.Result
	err    error
}

// NewSession returns a Session over cfg.
func NewSession(cfg SessionConfig) *Session {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Token == nil {
		cfg.Token = randomToken
	}
	return &Session{cfg: cfg, watches: map[int]*watch{}}
}

// randomToken returns eight random bytes as hex.
func randomToken() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("alignment: draw a token: %v", err))
	}
	return hex.EncodeToString(buf)
}

// SpawnInput carries what Begin spawns an attempt from.
// Prompt's verdict and report paths are filled by Begin.
type SpawnInput struct {
	Key      string
	Prompt   PromptInput
	Settings Settings
	Snapshot string
	PRNumber int
	HeadSHA  string
}

// Outcome is what Poll found.
// Done is false while the agent is pending.
// Cause is empty for the agent's own valid verdict, and otherwise one of the Cause constants with Detail saying why.
type Outcome struct {
	Done    bool
	Verdict Verdict
	Cause   string
	Detail  string
}

// Begin spawns the agent for in.Key, or returns the attempt that already holds the key and starts nothing, closed or not.
// A new attempt is numbered one past the highest, its attempt.json is written before the spawn,
// and any file already at its verdict or report path is removed, so only a verdict written after this spawn can be read.
// A failed start closes the attempt with the start error as its reason and returns the error.
// A failed strand record stops the strand before returning, so nothing runs untracked.
func (s *Session) Begin(in SpawnInput) (Attempt, error) {
	attempts, err := ListAttempts(s.cfg.Root)
	if err != nil {
		return Attempt{}, err
	}
	number := 1
	for _, existing := range attempts {
		if existing.Key == in.Key {
			return existing, nil
		}
		number = existing.Number + 1
	}

	now := s.cfg.Now()
	attempt := Attempt{
		Number:    number,
		Key:       in.Key,
		Point:     in.Prompt.Point,
		Token:     s.cfg.Token(),
		SpawnedAt: now,
		Deadline:  now.Add(in.Settings.Timeout),
		Snapshot:  in.Snapshot,
		PRNumber:  in.PRNumber,
		HeadSHA:   in.HeadSHA,
		Dir:       filepath.Join(s.cfg.Root, "attempt-"+strconv.Itoa(number)),
	}
	if err := WriteAttempt(attempt); err != nil {
		return Attempt{}, err
	}
	for _, stale := range []string{attempt.VerdictPath(), attempt.ReportPath()} {
		if err := os.Remove(stale); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return Attempt{}, fmt.Errorf("alignment: clear %q before the spawn: %w", stale, err)
		}
	}

	promptInput := in.Prompt
	promptInput.VerdictPath = attempt.VerdictPath()
	promptInput.ReportPath = attempt.ReportPath()
	prompt, err := Prompt(s.cfg.StencilsDir, promptInput)
	if err != nil {
		return Attempt{}, s.failBegin(attempt, "compose the prompt", err)
	}
	if err := os.WriteFile(attempt.PromptPath(), []byte(prompt), 0o644); err != nil {
		return Attempt{}, s.failBegin(attempt, "write the prompt", err)
	}

	logger.Info("alignment: spawn", "point", string(attempt.Point), "key", attempt.Key, "attempt", attempt.Number, "deadline", attempt.Deadline)
	run, err := s.cfg.Shuttle.Start(Spec(prompt, attempt, in.Settings))
	if err != nil {
		return Attempt{}, s.failBegin(attempt, "start the agent", err)
	}
	attempt.StrandGUID = run.StrandGUID()
	if err := WriteAttempt(attempt); err != nil {
		if stopErr := s.cfg.Shuttle.StopStrand(attempt.StrandGUID); stopErr != nil {
			return Attempt{}, fmt.Errorf("%w; stopping the agent strand %s also failed: %v", err, attempt.StrandGUID, stopErr)
		}
		return Attempt{}, err
	}
	s.watchRun(attempt.Number, run)
	return attempt, nil
}

// failBegin closes attempt, which never got a live agent, with the failure as its reason, and returns the failure.
func (s *Session) failBegin(attempt Attempt, step string, cause error) error {
	failure := fmt.Errorf("alignment: %s: %w", step, cause)
	attempt.Closed = true
	attempt.ClosedReason = failure.Error()
	if err := WriteAttempt(attempt); err != nil {
		return fmt.Errorf("%w; closing the attempt also failed: %v", failure, err)
	}
	return failure
}

// watchRun waits on run in the background and stores its result for Poll.
func (s *Session) watchRun(number int, run Run) {
	w := &watch{done: make(chan struct{})}
	s.mu.Lock()
	s.watches[number] = w
	s.mu.Unlock()
	go func() {
		w.result, w.err = run.Wait()
		close(w.done)
	}()
}

// watchDone returns the channel closed when attempt number's run has ended and its result is stored, or a closed one when this process holds no watch for it.
func (s *Session) watchDone(number int) <-chan struct{} {
	s.mu.Lock()
	w := s.watches[number]
	s.mu.Unlock()
	if w == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return w.done
}

// Poll reports where the attempt stands, re-reading its record, and never blocks.
// A closed attempt is done with CauseClosed and its verdict file is never read.
// A verdict file present decides: valid is the agent's own verdict, invalid is CauseInvalid.
// Otherwise a run that ended without a verdict, or the deadline, ends the attempt with CauseTimeout, CauseDied or CauseMissing, after stopping a strand that may still be live.
// After a restart, when this process holds no watch, Poll probes for a live run once: a found run is watched and pending, and none is CauseDied.
// Anything else is pending.
func (s *Session) Poll(a Attempt) (Outcome, error) {
	current, err := ReadAttempt(a.Dir)
	if err != nil {
		return Outcome{}, err
	}
	if current.Closed {
		return Outcome{Done: true, Cause: CauseClosed, Detail: current.ClosedReason}, nil
	}
	verdict, found, err := ReadVerdict(current)
	if err != nil {
		return Outcome{Done: true, Cause: CauseInvalid, Detail: err.Error()}, nil
	}
	if found {
		return Outcome{Done: true, Verdict: verdict}, nil
	}

	s.mu.Lock()
	w := s.watches[current.Number]
	s.mu.Unlock()
	if w != nil {
		select {
		case <-w.done:
			return s.endWithoutVerdict(current, w)
		default:
		}
	}

	now := s.cfg.Now()
	if !now.Before(current.Deadline) {
		return s.stopAndEnd(current, CauseTimeout, "the agent ran past its deadline")
	}
	if w != nil {
		return Outcome{}, nil
	}

	spec := Spec("", current, Settings{Timeout: current.Deadline.Sub(now)})
	run, live, err := s.cfg.Shuttle.Probe(spec)
	if err != nil {
		return Outcome{}, fmt.Errorf("alignment: probe for a live agent of attempt %d: %w", current.Number, err)
	}
	if !live {
		return s.stopAndEnd(current, CauseDied, "no agent is live for the attempt and it left no verdict")
	}
	s.watchRun(current.Number, run)
	return Outcome{}, nil
}

// endWithoutVerdict classifies a finished watch whose run left no verdict.
func (s *Session) endWithoutVerdict(a Attempt, w *watch) (Outcome, error) {
	switch {
	case w.err != nil:
		return s.stopAndEnd(a, CauseDied, fmt.Sprintf("waiting on the agent failed: %v", w.err))
	case w.result.Outcome == shuttleengine.OutcomeTimeout:
		return s.stopAndEnd(a, CauseTimeout, "the agent ran past its deadline")
	case w.result.Outcome == shuttleengine.OutcomeDone:
		return s.stopAndEnd(a, CauseMissing, "the agent finished without writing a verdict")
	default:
		return s.stopAndEnd(a, CauseDied, fmt.Sprintf("the agent ended %s without a verdict", w.result.Outcome))
	}
}

// stopAndEnd stops the attempt's strand, which may still be live, and answers a finished outcome with cause.
// A stop that fails is returned as an error, since the agent may still be live and nothing may proceed beside it.
func (s *Session) stopAndEnd(a Attempt, cause, detail string) (Outcome, error) {
	if err := s.stopStrand(a); err != nil {
		return Outcome{}, err
	}
	logger.Warn("alignment: attempt ended without a verdict", "point", string(a.Point), "attempt", a.Number, "cause", cause)
	return Outcome{Done: true, Cause: cause, Detail: detail}, nil
}

// stopStrand stops the strand recorded on a; an attempt that recorded none has nothing to stop.
func (s *Session) stopStrand(a Attempt) error {
	if a.StrandGUID == "" {
		return nil
	}
	if err := s.cfg.Shuttle.StopStrand(a.StrandGUID); err != nil {
		return fmt.Errorf("alignment: stop the agent strand %s: %w", a.StrandGUID, err)
	}
	logger.Info("alignment: stopped the agent strand", "point", string(a.Point), "attempt", a.Number, "strand", a.StrandGUID)
	return nil
}

// Close stops a strand that may still be live, then marks the attempt closed with reason.
// A stop that fails is returned and the attempt stays open, since an agent must not run beside a fallback.
// Closing an attempt that is already closed changes nothing.
func (s *Session) Close(a Attempt, reason string) error {
	current, err := ReadAttempt(a.Dir)
	if err != nil {
		return err
	}
	if current.Closed {
		return nil
	}
	if err := s.stopStrand(current); err != nil {
		return err
	}
	current.Closed = true
	current.ClosedReason = reason
	return WriteAttempt(current)
}

// MarkApplied records the verdict Go applied for the attempt and whether the parent was notified of it.
func (s *Session) MarkApplied(a Attempt, applied string, notified bool) error {
	current, err := ReadAttempt(a.Dir)
	if err != nil {
		return err
	}
	current.Applied = applied
	current.Notified = notified
	return WriteAttempt(current)
}

// Attempts lists every attempt under the session's root, sorted by number.
func (s *Session) Attempts() ([]Attempt, error) {
	return ListAttempts(s.cfg.Root)
}
