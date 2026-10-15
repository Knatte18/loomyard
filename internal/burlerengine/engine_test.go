// engine_test.go holds the same-package fakeShuttle, a seatengine.Shuttle whose handles block until the test releases them or stops them, a stop ending the handle's Wait as died, and the profile and engine builders review_test.go and fix_test.go drive the real seats engine over it with.
// Every Geometry built here points WorktreeRoot at a test temp dir, so materialization lands there
// rather than in the real package source tree.

package burlerengine

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// halfScript scripts one seat of a fake half.
type halfScript struct {
	// result is what Wait returns once it is let through.
	result shuttleengine.Result
	// err, when set, is what Wait returns instead of result.
	err error
	// startErr, when set, makes StartGated fail with it and return no handle.
	startErr error
	// onStart runs inside StartGated, after the seat is registered and before it returns.
	onStart func()
	// hold, when non-nil, blocks Wait until it is closed or the seat is stopped.
	hold chan struct{}
	// writes is the content Wait writes to the seat's one output file before it reports done.
	writes string
	// beforeReturn runs after writes, before Wait reports done.
	beforeReturn func()
	// rewrites holds the review file content the reviewer writes at each gate re-prompt, in order.
	rewrites []string
	// pendingPrompt makes the seat's handle report that it delivered a pending task prompt, and deliverErr makes that delivery fail.
	pendingPrompt bool
	deliverErr    error
}

// fakeStrandPrefix is the shortname and slug every fake strand name is formed under.
const fakeStrandPrefix = "tst:slug:"

// fakeShuttle is a same-package seatengine.Shuttle double: it starts the seat its spec's role names, records the spec and gate it was given, and hands back a fakeHandle scripted from that seat's halfScript.
// review scripts the review half's chair and fix the fix half's chair.
type fakeShuttle struct {
	review, fix halfScript

	mu sync.Mutex
	// started holds the roles of the seats that were started, in start order.
	started []string
	specs   map[string]shuttleengine.Spec
	gates   map[string]shuttleengine.GateSpec
	handles map[string]*fakeHandle
	// gateFindings collects the findings of every failed gate evaluation.
	gateFindings []string

	// live holds the roles of the seats ProbeGated finds live; a role absent from it is found nowhere.
	live map[string]bool
	// probeErr holds, per role, the error ProbeGated answers instead of a result.
	probeErr map[string]error
	// probeSpecs holds, per role, the spec ProbeGated was last asked about.
	probeSpecs map[string]shuttleengine.Spec

	// stopErr makes a handle's Stop fail, for every handle or only the one named by stopErrGuid when that is set.
	stopErr     error
	stopErrGuid string
	// stoppedGUIDs holds the strand guid of every Stop call, in call order.
	stoppedGUIDs []string
}

// stopCalls returns the strand guids of the Stop calls so far, in call order.
func (f *fakeShuttle) stopCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.stoppedGUIDs)
}

// fakeHandle is one started fake seat.
type fakeHandle struct {
	shuttle  *fakeShuttle
	role     string
	guid     string
	runDir   string
	spec     shuttleengine.Spec
	gate     shuttleengine.GateSpec
	script   halfScript
	stopped  chan struct{}
	stopOnce sync.Once
	returned atomic.Bool
	// deliveries counts the DeliverPendingPrompt calls.
	deliveries atomic.Int32
}

// DeliverPendingPrompt counts the call and answers from the seat's script.
func (h *fakeHandle) DeliverPendingPrompt() (bool, error) {
	h.deliveries.Add(1)
	return h.script.pendingPrompt, h.script.deliverErr
}

func (h *fakeHandle) StrandGUID() string { return h.guid }

func (h *fakeHandle) StrandName() string { return fakeStrandPrefix + h.role }

func (h *fakeHandle) RunDir() string { return h.runDir }

// SessionState answers an idle zero reading.
func (h *fakeHandle) SessionState() (shuttleengine.RunSessionState, error) {
	return shuttleengine.RunSessionState{}, nil
}

// Send lands every line.
func (h *fakeHandle) Send(string) error { return nil }

// stop ends the handle's blocked Wait as died.
func (h *fakeHandle) stop() {
	h.stopOnce.Do(func() { close(h.stopped) })
}

// Stop records the call and, unless the shuttle was told to fail it, ends the handle's Wait as died.
func (h *fakeHandle) Stop() error {
	h.shuttle.mu.Lock()
	h.shuttle.stoppedGUIDs = append(h.shuttle.stoppedGUIDs, h.guid)
	failed := h.shuttle.stopErr != nil && (h.shuttle.stopErrGuid == "" || h.shuttle.stopErrGuid == h.guid)
	err := h.shuttle.stopErr
	h.shuttle.mu.Unlock()
	if failed {
		return err
	}
	h.stop()
	return nil
}

// settled reports whether the handle is no longer blocked: its Wait returned, or it was stopped.
func (h *fakeHandle) settled() bool {
	if h.returned.Load() {
		return true
	}
	select {
	case <-h.stopped:
		return true
	default:
		return false
	}
}

// diedResult is what a stopped handle's Wait reports.
func (h *fakeHandle) diedResult() shuttleengine.Result {
	return shuttleengine.Result{Outcome: shuttleengine.OutcomeDied, StrandGUID: h.guid, SessionID: h.role + "-session", RunDir: h.runDir}
}

// Wait blocks as its script says, then reports the scripted result, first writing the seat's output file for a done outcome and evaluating the seat's gate like the shuttle wait loop would.
func (h *fakeHandle) Wait() (shuttleengine.Result, error) {
	defer h.returned.Store(true)

	if h.script.hold != nil {
		select {
		case <-h.script.hold:
		case <-h.stopped:
			return h.diedResult(), nil
		}
	}
	if h.script.err != nil {
		return shuttleengine.Result{}, h.script.err
	}

	result := h.script.result
	if result.StrandGUID == "" {
		result.StrandGUID = h.guid
	}
	if result.SessionID == "" {
		result.SessionID = h.role + "-session"
	}
	if result.RunDir == "" {
		result.RunDir = h.runDir
	}
	if result.Outcome != shuttleengine.OutcomeDone {
		return result, nil
	}

	if h.script.writes != "" {
		if err := os.WriteFile(h.spec.OutputFiles[0], []byte(h.script.writes), 0o644); err != nil {
			return shuttleengine.Result{}, err
		}
	}
	if h.script.beforeReturn != nil {
		h.script.beforeReturn()
	}
	// A scripted gate outcome stands in for the evaluation, as a row that needs a failed review gate cannot get one from the real entry, which passes on cap.
	if len(h.gate) > 0 && result.Gate == nil {
		gate, err := h.evaluateGate()
		if err != nil {
			return result, err
		}
		result.Gate = gate
	}
	return result, nil
}

// evaluateGate runs the handle's gate spec like the shuttle wait loop:
// each entry's closure runs in list order, skipping off entries.
// A failing entry is re-prompted up to its Attempts, each re-prompt first writing the next rewrites content to the output file as the seat would,
// and a PassOnCap entry that spent its budget is let through.
// Any other entry that spent its budget fails the gate and stops the evaluation.
// A closure's error is returned.
func (h *fakeHandle) evaluateGate() (*shuttleengine.GateOutcome, error) {
	outcome := &shuttleengine.GateOutcome{Passed: true}
	for _, entry := range h.gate {
		if entry.Attempts <= 0 {
			continue
		}
		for reprompts := 0; ; reprompts++ {
			gateResult, err := entry.Gate()
			if err != nil {
				return nil, err
			}
			if gateResult.Passed {
				break
			}
			h.shuttle.mu.Lock()
			h.shuttle.gateFindings = append(h.shuttle.gateFindings, gateResult.Findings)
			h.shuttle.mu.Unlock()
			if reprompts == entry.Attempts {
				if !entry.PassOnCap {
					outcome.Passed = false
				}
				break
			}
			if reprompts < len(h.script.rewrites) {
				if err := os.WriteFile(h.spec.OutputFiles[0], []byte(h.script.rewrites[reprompts]), 0o644); err != nil {
					return nil, err
				}
			}
		}
		if !outcome.Passed {
			break
		}
	}
	return outcome, nil
}

// scriptFor is the halfScript of the seat spec's role names: the review script for the review half's chair under any role prefix, the fix script for every other seat.
func (f *fakeShuttle) scriptFor(spec shuttleengine.Spec) halfScript {
	if strings.HasSuffix(spec.Role, reviewHalfRole) {
		return f.review
	}
	return f.fix
}

// initLocked allocates the recording maps on first use; the caller holds f.mu.
func (f *fakeShuttle) initLocked() {
	if f.specs != nil {
		return
	}
	f.specs = map[string]shuttleengine.Spec{}
	f.gates = map[string]shuttleengine.GateSpec{}
	f.handles = map[string]*fakeHandle{}
	f.probeSpecs = map[string]shuttleengine.Spec{}
}

// registerLocked builds the handle of spec's seat from script and records it; the caller holds f.mu.
func (f *fakeShuttle) registerLocked(spec shuttleengine.Spec, gate shuttleengine.GateSpec, script halfScript) *fakeHandle {
	handle := &fakeHandle{
		shuttle: f,
		role:    spec.Role,
		guid:    spec.Role + "-guid",
		runDir:  "/kept/" + spec.Role,
		spec:    spec,
		gate:    gate,
		script:  script,
		stopped: make(chan struct{}),
	}
	f.handles[spec.Role] = handle
	return handle
}

// StartGated registers the seat its spec's role names and returns a handle scripted from that seat's halfScript, or its start error.
func (f *fakeShuttle) StartGated(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (seatengine.Handle, error) {
	script := f.scriptFor(spec)

	f.mu.Lock()
	f.initLocked()
	var handle *fakeHandle
	if script.startErr == nil {
		handle = f.registerLocked(spec, gate, script)
	}
	f.started = append(f.started, spec.Role)
	f.specs[spec.Role] = spec
	f.gates[spec.Role] = gate
	f.mu.Unlock()

	if script.onStart != nil {
		script.onStart()
	}
	if handle == nil {
		return nil, script.startErr
	}
	return handle, nil
}

// ProbeGated records spec and answers the seat its role names: its scripted probe error, a handle scripted like a started seat when the role is live, and otherwise not found.
func (f *fakeShuttle) ProbeGated(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (seatengine.Handle, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.initLocked()
	f.probeSpecs[spec.Role] = spec
	if err := f.probeErr[spec.Role]; err != nil {
		return nil, false, err
	}
	if !f.live[spec.Role] {
		return nil, false, nil
	}
	return f.registerLocked(spec, gate, f.scriptFor(spec)), true, nil
}

// stop ends the blocked Wait of the handle with guid, if one was started.
func (f *fakeShuttle) stop(guid string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, h := range f.handles {
		if h.guid == guid {
			h.stop()
		}
	}
}

// blocked returns the roles of started handles that are neither done waiting nor stopped.
func (f *fakeShuttle) blocked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var roles []string
	for role, h := range f.handles {
		if !h.settled() {
			roles = append(roles, role)
		}
	}
	return roles
}

// newEngineTestProfile builds a minimal valid Profile (relative paths — the
// engine resolves them against root when a half validates) rooted at a fresh temp dir.
func newEngineTestProfile(t *testing.T) (root string, p Profile) {
	t.Helper()
	root = t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "target.txt"), []byte("target"), 0o644); err != nil {
		t.Fatalf("WriteFile(target) = %v; want nil", err)
	}
	if err := os.WriteFile(filepath.Join(root, "fasit.txt"), []byte("fasit"), 0o644); err != nil {
		t.Fatalf("WriteFile(fasit) = %v; want nil", err)
	}

	p = Profile{
		Target:          FileSet{Paths: []string{"target.txt"}},
		Fasit:           FileSet{Paths: []string{"fasit.txt"}},
		Rubric:          "the widget's color must match the housing's color",
		FixScope:        FixScopeSource,
		ReviewPath:      "review.md",
		FixerReportPath: "fixer-report.md",
	}
	return root, p
}

// newEngineForTest wires an Engine to shuttle over a Geometry rooted at root.
func newEngineForTest(t *testing.T, root string, shuttle *fakeShuttle) *Engine {
	t.Helper()
	return newEngineWith(t, Geometry{WorktreeRoot: root, AnchorPath: root}, Config{}, shuttle)
}

// newEngineWith is newEngineForTest over a told geometry and config; a geometry without a shortname is given the fake strand prefix's.
func newEngineWith(t *testing.T, geom Geometry, cfg Config, shuttle *fakeShuttle) *Engine {
	t.Helper()
	if geom.Shortname == "" {
		geom.Shortname, geom.Slug = "tst", "slug"
	}
	return New(shuttle, geom, cfg, newTestStencilsDir(t), "")
}

const (
	approvedReview = "---\nverdict: APPROVED\n---\nlooks good\n"
	blockingReview = "---\nverdict: BLOCKING\nfindings:\n" +
		"  - id: F1\n    severity: BLOCKING\n    class: design\n    location: target.txt:1\n    summary: colors do not match\n    fix: change the color in target.txt:1\n" +
		"---\n### [BLOCKING] colors do not match\n**Location:** target.txt:1\n**Issue:** found a mismatch\n"
	malformedReview = "not frontmatter at all\n"

	reviewHalfRole = reviewRolePrefix
	fixHalfRole    = fixRolePrefix
)

// clusterTestConfig is a one-lens, one-fan config, so a cluster profile resolves to exactly one fork.
var clusterTestConfig = Config{
	Lenses: map[string]string{"style": "style prose"},
	Fans:   map[string][]string{"standard": {"style"}},
}
