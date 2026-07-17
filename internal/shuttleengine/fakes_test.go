// fakes_test.go implements the hermetic test doubles the rest of
// shuttleengine's tests drive the run loop against: fakeMux (a MuxOps
// double that records every call and lets a test script Status
// liveness/CapturePane content per call and inject a per-method error) and
// fakeEngine (an Engine double with a canned Launch, a trivial fixture
// format for ParseEvents, a scriptable Startup sequence, fixed
// InterruptSequence/ComposeSend choreographies, and a scriptable
// AuditForks/AuditForksIncremental canned result/error). Neither fake asserts
// anything itself — tests inspect their recorded calls.

package shuttleengine

import (
	"fmt"
	"strings"
	"sync"

	"github.com/Knatte18/loomyard/internal/muxengine"
)

// fakeMux is a hermetic MuxOps double: it never touches tmux. CallLog
// records every call across all six methods, in invocation order, as a
// short formatted tag — the single source tests use to assert
// cross-method choreography (e.g. Interrupt/Send's exact key/text
// sequence). Status and CapturePane are scripted via a FIFO queue: each
// call consumes the next queued value, and the last queued value sticks
// once the queue is drained, so a script shorter than the actual number of
// polls still returns a stable final answer. Every method's error is
// injected independently via its own field, so a test can force exactly one
// call to fail without touching the others.
type fakeMux struct {
	mu sync.Mutex

	// CallLog is the ordered, cross-method call log described above.
	CallLog []string

	AddStrandCalls  []muxengine.AddSpec
	AddStrandResult muxengine.Strand
	AddStrandErr    error

	RemoveStrandCalls []struct {
		GUID      string
		Recursive bool
	}
	RemoveStrandErr error

	// StatusQueue is consumed FIFO by Status; see the type doc for the
	// draining rule. StatusErr, when set, makes every Status call fail
	// regardless of StatusQueue.
	StatusQueue []muxengine.StatusResult
	StatusErr   error

	// CaptureQueue behaves like StatusQueue but for CapturePane.
	CaptureQueue []string
	CaptureErr   error

	SendTextCalls []struct {
		GUID   string
		Text   string
		Submit bool
	}
	SendTextErr error

	SendKeyCalls []struct{ GUID, Key string }
	SendKeyErr   error
}

func (m *fakeMux) AddStrand(spec muxengine.AddSpec) (muxengine.Strand, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CallLog = append(m.CallLog, "AddStrand")
	m.AddStrandCalls = append(m.AddStrandCalls, spec)
	if m.AddStrandErr != nil {
		return muxengine.Strand{}, m.AddStrandErr
	}
	return m.AddStrandResult, nil
}

func (m *fakeMux) RemoveStrand(guid string, recursive bool) (muxengine.Removed, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CallLog = append(m.CallLog, "RemoveStrand")
	m.RemoveStrandCalls = append(m.RemoveStrandCalls, struct {
		GUID      string
		Recursive bool
	}{guid, recursive})
	if m.RemoveStrandErr != nil {
		return muxengine.Removed{}, m.RemoveStrandErr
	}
	return muxengine.Removed{}, nil
}

func (m *fakeMux) Status() (muxengine.StatusResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CallLog = append(m.CallLog, "Status")
	if m.StatusErr != nil {
		return muxengine.StatusResult{}, m.StatusErr
	}
	if len(m.StatusQueue) == 0 {
		return muxengine.StatusResult{}, nil
	}
	result := m.StatusQueue[0]
	if len(m.StatusQueue) > 1 {
		m.StatusQueue = m.StatusQueue[1:]
	}
	return result, nil
}

func (m *fakeMux) SendText(guid, text string, submit bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CallLog = append(m.CallLog, "SendText:"+text)
	m.SendTextCalls = append(m.SendTextCalls, struct {
		GUID   string
		Text   string
		Submit bool
	}{guid, text, submit})
	return m.SendTextErr
}

func (m *fakeMux) SendKey(guid, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CallLog = append(m.CallLog, "SendKey:"+key)
	m.SendKeyCalls = append(m.SendKeyCalls, struct{ GUID, Key string }{guid, key})
	return m.SendKeyErr
}

func (m *fakeMux) CapturePane(guid string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CallLog = append(m.CallLog, "CapturePane")
	if m.CaptureErr != nil {
		return "", m.CaptureErr
	}
	if len(m.CaptureQueue) == 0 {
		return "", nil
	}
	result := m.CaptureQueue[0]
	if len(m.CaptureQueue) > 1 {
		m.CaptureQueue = m.CaptureQueue[1:]
	}
	return result, nil
}

// var _ MuxOps = (*fakeMux)(nil) is the compile-time proof fakeMux satisfies
// the seam it doubles for.
var _ MuxOps = (*fakeMux)(nil)

// fakeEngine is a hermetic Engine double. Prepare returns a canned Launch
// without writing any real files — tests that need pollEventsTick to see
// something seed events.jsonl directly. ParseEvents splits a trivial
// newline-delimited fixture format: a line of the form "STOP:<message>"
// becomes an EventStop carrying <message> as Message, a line of the form
// "ASK:<question>" becomes an EventAsk carrying <question> as Message, and
// any other line (blank, or missing either prefix) is skipped — mirroring
// the leniency a real engine's parser must have for partial appends. Startup replays
// StartupScript in FIFO order (the last entry stays once the script drains,
// same rule as fakeMux's queues; an empty script always reports
// StartupPending). InterruptSequence/ComposeSend return fixed, inspectable
// choreographies rather than scripted ones — Interrupt/Send tests assert
// against these canonical sequences directly.
type fakeEngine struct {
	mu sync.Mutex

	PrepareLaunch Launch
	PrepareErr    error
	// PrepareHook, when set, runs inside Prepare with the run directory
	// before it returns — a test uses it to plant on-disk state (e.g. a
	// run.json directory that makes the later saveRunState fail) without a
	// bespoke Engine double.
	PrepareHook  func(runDir string)
	PrepareCalls []struct {
		RunDir string
		Spec   Spec
		Cfg    Config
	}

	// ParseEventsErr, when set, makes every ParseEvents call fail.
	// ParseEventsFailCount, when > 0, makes only the first N calls fail
	// (ParseEventsErr's error, or a generic fixed error if ParseEventsErr is
	// nil) before ParseEvents starts succeeding normally on the SAME bytes —
	// the fake used to prove pollEventsTick re-reads and re-parses unconsumed
	// bytes after a transient parse failure instead of discarding them.
	ParseEventsErr       error
	ParseEventsFailCount int
	parseEventsCallsSeen int

	StartupScript []StartupState
	StartupCalls  []string

	// AuditForksResult is returned by AuditForks whenever AuditForksErr is
	// nil; AuditForksCalls records every (sessionID, workdir) pair AuditForks
	// was called with, so a test can assert it was — or was NOT — called.
	AuditForksResult ForkAudit
	AuditForksErr    error
	AuditForksCalls  []struct {
		SessionID string
		Workdir   string
	}
}

func (e *fakeEngine) Prepare(runDir string, spec Spec, cfg Config) (Launch, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.PrepareCalls = append(e.PrepareCalls, struct {
		RunDir string
		Spec   Spec
		Cfg    Config
	}{runDir, spec, cfg})
	if e.PrepareErr != nil {
		return Launch{}, e.PrepareErr
	}
	if e.PrepareHook != nil {
		e.PrepareHook(runDir)
	}
	return e.PrepareLaunch, nil
}

func (e *fakeEngine) ParseEvents(data []byte) ([]Event, error) {
	e.mu.Lock()
	e.parseEventsCallsSeen++
	callNum := e.parseEventsCallsSeen
	e.mu.Unlock()

	if e.ParseEventsFailCount > 0 && callNum <= e.ParseEventsFailCount {
		if e.ParseEventsErr != nil {
			return nil, e.ParseEventsErr
		}
		return nil, fmt.Errorf("fakeEngine: scripted ParseEvents failure %d/%d", callNum, e.ParseEventsFailCount)
	}
	if e.ParseEventsErr != nil {
		return nil, e.ParseEventsErr
	}

	var events []Event
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if msg, ok := strings.CutPrefix(trimmed, "STOP:"); ok {
			events = append(events, Event{Kind: EventStop, Message: msg, Raw: []byte(trimmed)})
			continue
		}
		if msg, ok := strings.CutPrefix(trimmed, "ASK:"); ok {
			events = append(events, Event{Kind: EventAsk, Message: msg, Raw: []byte(trimmed)})
			continue
		}
	}
	return events, nil
}

func (e *fakeEngine) Startup(capture string) StartupState {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.StartupCalls = append(e.StartupCalls, capture)
	if len(e.StartupScript) == 0 {
		return StartupPending
	}
	next := e.StartupScript[0]
	if len(e.StartupScript) > 1 {
		e.StartupScript = e.StartupScript[1:]
	}
	return next
}

// InterruptSequence returns the canonical single-Escape interrupt
// choreography — fixed, not scripted, so tests assert against it directly.
func (e *fakeEngine) InterruptSequence() []PaneInput {
	return []PaneInput{{Key: "Escape"}}
}

// TrustDismissSequence returns the canonical single-Enter trust dismissal —
// fixed, not scripted, so tests assert against it directly.
func (e *fakeEngine) TrustDismissSequence() []PaneInput {
	return []PaneInput{{Key: "Enter"}}
}

// ComposeSend returns the canonical Escape-then-submit-text choreography —
// fixed, not scripted, so tests assert against it directly.
func (e *fakeEngine) ComposeSend(text string) []PaneInput {
	return []PaneInput{
		{Key: "Escape"},
		{Text: text, Submit: true},
	}
}

// AuditForks records the (sessionID, workdir) it was called with and returns
// AuditForksResult/AuditForksErr — a test scripts the canned audit a
// fork-mode done classification should attach to Result.ForkAudit. It forwards
// to AuditForksIncremental with a nil seenTranscripts map, mirroring the
// real engine's AuditForks-in-terms-of-AuditForksIncremental relationship.
func (e *fakeEngine) AuditForks(sessionID, workdir string) (ForkAudit, error) {
	return e.AuditForksIncremental(sessionID, workdir, nil)
}

// AuditForksIncremental records the (sessionID, workdir, seenTranscripts) it was
// called with and returns AuditForksResult/AuditForksErr, filtering
// AuditForksResult.Forks down to reports whose TranscriptPath is not a key of
// seenTranscripts (nil map == no filtering) — a test scripts the canned audit an
// incremental caller should observe, and can assert the seen-set was applied.
func (e *fakeEngine) AuditForksIncremental(sessionID, workdir string, seenTranscripts map[string]bool) (ForkAudit, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.AuditForksCalls = append(e.AuditForksCalls, struct {
		SessionID string
		Workdir   string
	}{sessionID, workdir})
	if e.AuditForksErr != nil {
		return ForkAudit{}, e.AuditForksErr
	}

	result := e.AuditForksResult
	if seenTranscripts == nil {
		return result, nil
	}
	filtered := make([]ForkReport, 0, len(result.Forks))
	for _, report := range result.Forks {
		if seenTranscripts[report.TranscriptPath] {
			continue
		}
		filtered = append(filtered, report)
	}
	result.Forks = filtered
	return result, nil
}

// var _ Engine = (*fakeEngine)(nil) is the compile-time proof fakeEngine
// satisfies the seam it doubles for.
var _ Engine = (*fakeEngine)(nil)
