// Package shuttlefake fakes the two seams a shuttle consumer is built over: shuttleengine.Engine and shuttleengine.ReedOps.
//
// Engine is inert until a field or func override scripts it, and records what Prepare was handed.
// Reed keeps a mutex-guarded strand table, so a test sees the liveness a real reed would report.
// Neither fake asserts anything or takes a testing.TB; a test reads the recorded fields and asserts itself.
//
// internal/shuttleengine's own tests keep their richer in-package fake, because the import cycle forbids this kit there.
package shuttlefake

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Engine is a shuttleengine.Engine whose every answer is inert unless scripted.
// Startup answers StartupReady by default, and a consumer that needs another state sets StartupFn.
// A smoke engine that launches a real shell embeds Engine and overrides Prepare only.
// Fields are read after the run under test has returned; the fake locks only its own writes.
type Engine struct {
	mu sync.Mutex

	// PrepareLaunch is the Launch Prepare returns when PrepareFn is nil and PrepareErr is nil.
	// The zero value answers a canned launch command and session id.
	PrepareLaunch *shuttleengine.Launch
	PrepareErr    error
	// PrepareCalls counts Prepare calls, and LastSpec and LastPrompt hold the latest call's Spec and its Prompt.
	PrepareCalls int
	LastSpec     shuttleengine.Spec
	LastPrompt   string

	// Events and EventsErr are what ParseEvents answers when ParseEventsFn is nil.
	Events    []shuttleengine.Event
	EventsErr error
	// Audit and AuditErr are what AuditForks and AuditForksIncremental answer when their func is nil.
	Audit    shuttleengine.ForkAudit
	AuditErr error

	PrepareFn               func(runDir string, spec shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error)
	ParseEventsFn           func(data []byte) ([]shuttleengine.Event, error)
	StartupFn               func(capture string) shuttleengine.StartupState
	InterruptSequenceFn     func() []shuttleengine.PaneInput
	TrustDismissSequenceFn  func(capture string) []shuttleengine.PaneInput
	ComposeSendFn           func(text string) []shuttleengine.PaneInput
	ModelSwitchSequenceFn   func(model string) []shuttleengine.PaneInput
	SkillLoadMessageFn      func(skills []string) string
	ClassifySkillLoadFn     func(turnEnd shuttleengine.Event, skills []string) shuttleengine.SkillLoadReport
	AuditForksFn            func(sessionID, workdir string) (shuttleengine.ForkAudit, error)
	AuditForksIncrementalFn func(sessionID, workdir string, seenTranscripts map[string]bool) (shuttleengine.ForkAudit, error)
}

var (
	_ shuttleengine.Engine      = (*Engine)(nil)
	_ shuttleengine.SkillLoader = (*Engine)(nil)
)

// Prepare records the call and answers PrepareFn, else PrepareErr, else PrepareLaunch, else a canned launch.
func (e *Engine) Prepare(runDir string, spec shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	e.mu.Lock()
	e.PrepareCalls++
	e.LastSpec = spec
	e.LastPrompt = spec.Prompt
	e.mu.Unlock()
	if e.PrepareFn != nil {
		return e.PrepareFn(runDir, spec, cfg)
	}
	if e.PrepareErr != nil {
		return shuttleengine.Launch{}, e.PrepareErr
	}
	if e.PrepareLaunch != nil {
		return *e.PrepareLaunch, nil
	}
	return shuttleengine.Launch{Cmd: "fake-launch-cmd", SessionID: "fake-session"}, nil
}

// ParseEvents answers ParseEventsFn, else EventsErr, else Events.
func (e *Engine) ParseEvents(data []byte) ([]shuttleengine.Event, error) {
	if e.ParseEventsFn != nil {
		return e.ParseEventsFn(data)
	}
	if e.EventsErr != nil {
		return nil, e.EventsErr
	}
	return e.Events, nil
}

// Startup answers StartupFn, else StartupReady.
func (e *Engine) Startup(capture string) shuttleengine.StartupState {
	if e.StartupFn != nil {
		return e.StartupFn(capture)
	}
	return shuttleengine.StartupReady
}

// InterruptSequence answers InterruptSequenceFn, else no inputs.
func (e *Engine) InterruptSequence() []shuttleengine.PaneInput {
	if e.InterruptSequenceFn != nil {
		return e.InterruptSequenceFn()
	}
	return nil
}

// TrustDismissSequence answers TrustDismissSequenceFn, else no inputs.
func (e *Engine) TrustDismissSequence(capture string) []shuttleengine.PaneInput {
	if e.TrustDismissSequenceFn != nil {
		return e.TrustDismissSequenceFn(capture)
	}
	return nil
}

// ComposeSend answers ComposeSendFn, else no inputs.
func (e *Engine) ComposeSend(text string) []shuttleengine.PaneInput {
	if e.ComposeSendFn != nil {
		return e.ComposeSendFn(text)
	}
	return nil
}

// SkillLoadMessage answers SkillLoadMessageFn, else the skills joined by commas.
func (e *Engine) SkillLoadMessage(skills []string) string {
	if e.SkillLoadMessageFn != nil {
		return e.SkillLoadMessageFn(skills)
	}
	return strings.Join(skills, ",")
}

// ClassifySkillLoad answers ClassifySkillLoadFn, else a verified report with every skill loaded.
func (e *Engine) ClassifySkillLoad(turnEnd shuttleengine.Event, skills []string) shuttleengine.SkillLoadReport {
	if e.ClassifySkillLoadFn != nil {
		return e.ClassifySkillLoadFn(turnEnd, skills)
	}
	return shuttleengine.SkillLoadReport{Verified: true, Loaded: skills}
}

// DefaultSkillLoadTimeout answers a bound of one second.
func (e *Engine) DefaultSkillLoadTimeout() time.Duration {
	return time.Second
}

// ModelSwitchSequence answers ModelSwitchSequenceFn, else no inputs.
func (e *Engine) ModelSwitchSequence(model string) []shuttleengine.PaneInput {
	if e.ModelSwitchSequenceFn != nil {
		return e.ModelSwitchSequenceFn(model)
	}
	return nil
}

// AuditForks answers AuditForksFn, else AuditErr, else Audit.
func (e *Engine) AuditForks(sessionID, workdir string) (shuttleengine.ForkAudit, error) {
	if e.AuditForksFn != nil {
		return e.AuditForksFn(sessionID, workdir)
	}
	return e.Audit, e.AuditErr
}

// AuditForksIncremental answers AuditForksIncrementalFn, else AuditErr, else Audit.
func (e *Engine) AuditForksIncremental(sessionID, workdir string, seenTranscripts map[string]bool) (shuttleengine.ForkAudit, error) {
	if e.AuditForksIncrementalFn != nil {
		return e.AuditForksIncrementalFn(sessionID, workdir, seenTranscripts)
	}
	return e.Audit, e.AuditErr
}

// Reed is a shuttleengine.ReedOps over a strand table.
// AddStrand mints a GUID and marks the strand live, RemoveStrand retires it, and Status reports what is left.
// Every method is guarded by one mutex, so concurrent AddStrand and Status calls are safe.
// Fields are read after the run under test has returned.
type Reed struct {
	mu   sync.Mutex
	next int

	// Strands is the table Status reports, in add order.
	// A test seeds it directly to script a strand it did not add.
	Strands []reedengine.StrandStatus
	// RemovedGUIDs records every GUID RemoveStrand retired, in call order.
	RemovedGUIDs []string
	// AddedSpecs records every AddSpec AddStrand was handed, in call order.
	AddedSpecs []reedengine.AddSpec

	// StatusErr makes Status fail; StatusFn replaces it entirely.
	StatusErr error
	// RemoveErr makes RemoveStrand fail without retiring the strand.
	RemoveErr error
	// AddErr makes AddStrand fail without minting a strand.
	AddErr error

	SendTextCalls []SendTextCall
	SendKeyCalls  []SendKeyCall
	CaptureCalls  []string
	// WaitMarkCalls records every SetWaitMark call, in call order.
	WaitMarkCalls []WaitMarkCall
	// WaitMarkErr makes SetWaitMark fail after recording the call.
	WaitMarkErr error

	AddStrandFn    func(spec reedengine.AddSpec) (reedengine.Strand, error)
	RemoveStrandFn func(guid string, recursive bool) (reedengine.Removed, error)
	StatusFn       func() (reedengine.StatusResult, error)
	SendTextFn     func(guid, text string, submit bool) error
	SendKeyFn      func(guid, key string) error
	CapturePaneFn  func(guid string) (string, error)
}

// SendTextCall is one recorded SendText call.
type SendTextCall struct {
	GUID   string
	Text   string
	Submit bool
}

// WaitMarkCall is one recorded SetWaitMark call; an empty Label is a clear.
type WaitMarkCall struct {
	GUID  string
	Label string
}

// SendKeyCall is one recorded SendKey call.
type SendKeyCall struct {
	GUID string
	Key  string
}

var _ shuttleengine.ReedOps = (*Reed)(nil)

// AddStrand answers AddStrandFn, else AddErr, else mints a live strand named after the spec.
func (r *Reed) AddStrand(spec reedengine.AddSpec) (reedengine.Strand, error) {
	r.mu.Lock()
	r.AddedSpecs = append(r.AddedSpecs, spec)
	r.mu.Unlock()
	if r.AddStrandFn != nil {
		return r.AddStrandFn(spec)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.AddErr != nil {
		return reedengine.Strand{}, r.AddErr
	}
	r.next++
	name := spec.NameOverride
	if name == "" {
		name = spec.Role
	}
	strand := reedengine.Strand{
		GUID:      fmt.Sprintf("fake-guid-%d", r.next),
		Name:      name,
		Parent:    spec.Parent,
		Cmd:       spec.Cmd,
		ResumeCmd: spec.ResumeCmd,
		SessionID: spec.SessionID,
		PaneID:    fmt.Sprintf("%%%d", r.next),
		Display:   spec.Display,
	}
	r.Strands = append(r.Strands, reedengine.StrandStatus{GUID: strand.GUID, Name: strand.Name, PaneID: strand.PaneID, Live: true})
	return strand, nil
}

// RemoveStrand answers RemoveStrandFn, else RemoveErr, else retires the strand and records its GUID.
// An unknown GUID is recorded and answers an empty Removed, as a real reed's cascade over nothing does.
func (r *Reed) RemoveStrand(guid string, recursive bool) (reedengine.Removed, error) {
	if r.RemoveStrandFn != nil {
		return r.RemoveStrandFn(guid, recursive)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.RemoveErr != nil {
		return reedengine.Removed{}, r.RemoveErr
	}
	r.RemovedGUIDs = append(r.RemovedGUIDs, guid)
	var removed reedengine.Removed
	kept := r.Strands[:0:0]
	for _, s := range r.Strands {
		if s.GUID == guid {
			removed.Strands = append(removed.Strands, struct{ GUID, Name string }{s.GUID, s.Name})
			continue
		}
		kept = append(kept, s)
	}
	r.Strands = kept
	return removed, nil
}

// Status answers StatusFn, else StatusErr, else the strand table.
func (r *Reed) Status() (reedengine.StatusResult, error) {
	if r.StatusFn != nil {
		return r.StatusFn()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.StatusErr != nil {
		return reedengine.StatusResult{}, r.StatusErr
	}
	return reedengine.StatusResult{Strands: append([]reedengine.StrandStatus(nil), r.Strands...)}, nil
}

// SendText records the call and answers SendTextFn, else nil.
func (r *Reed) SendText(guid, text string, submit bool) error {
	r.mu.Lock()
	r.SendTextCalls = append(r.SendTextCalls, SendTextCall{GUID: guid, Text: text, Submit: submit})
	r.mu.Unlock()
	if r.SendTextFn != nil {
		return r.SendTextFn(guid, text, submit)
	}
	return nil
}

// SendKey records the call and answers SendKeyFn, else nil.
func (r *Reed) SendKey(guid, key string) error {
	r.mu.Lock()
	r.SendKeyCalls = append(r.SendKeyCalls, SendKeyCall{GUID: guid, Key: key})
	r.mu.Unlock()
	if r.SendKeyFn != nil {
		return r.SendKeyFn(guid, key)
	}
	return nil
}

// SetWaitMark records the call and answers WaitMarkErr.
func (r *Reed) SetWaitMark(guid, label string, start time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.WaitMarkCalls = append(r.WaitMarkCalls, WaitMarkCall{GUID: guid, Label: label})
	return r.WaitMarkErr
}

// CapturePane records the call and answers CapturePaneFn, else an empty capture.
func (r *Reed) CapturePane(guid string) (string, error) {
	r.mu.Lock()
	r.CaptureCalls = append(r.CaptureCalls, guid)
	r.mu.Unlock()
	if r.CapturePaneFn != nil {
		return r.CapturePaneFn(guid)
	}
	return "", nil
}
