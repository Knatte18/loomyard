// engine_test.go tables Engine.Run against a same-package fakeShuttle whose handles block until the test releases them and a fakeRemover that ends a removed handle's Wait as died.
// It covers spec construction for both halves (including the ClusterFan -> Spec.ForkSubagents wiring and the concurrent start) and the cluster audit policy wiring.
// It covers every shuttleengine.Outcome of either half, the fixer's gate outcomes, the review-parse gate's re-prompts and the marker lifecycle.
// It covers the round's failure rules (every failure row of join, the start failures and ErrHalfNotStopped).
// It covers the per-round instruction-file materialization step, and the PATTERN-directive transposition detector for the pattern.Directive call site.
// Every Geometry built here points WorktreeRoot at a test temp dir, so materialization lands there
// rather than in the real package source tree. TestEngine_Run_MaterializesInstructionFiles is the
// one site that keeps WorktreeRoot and AnchorPath distinct on purpose.

package burlerengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// halfScript scripts one half of a fake round.
type halfScript struct {
	// result is what Wait returns once it is let through.
	result shuttleengine.Result
	// err, when set, is what Wait returns instead of result.
	err error
	// startErr, when set, makes StartGated fail with it and return no handle.
	startErr error
	// onStart runs inside StartGated, after the half is registered and before it returns.
	onStart func()
	// hold, when non-nil, blocks Wait until it is closed or the half is stopped.
	hold chan struct{}
	// waitForMarker blocks Wait until the ready marker exists or the half is stopped, as a fixer awaiting its review does.
	waitForMarker bool
	// writes is the content Wait writes to the half's one output file before it reports done.
	writes string
	// beforeReturn runs after writes, before Wait reports done.
	beforeReturn func()
	// rewrites holds the review file content the reviewer writes at each gate re-prompt, in order.
	rewrites []string
}

// fakeShuttle is a same-package Shuttle double: it starts the half its spec's role names, records the spec and gate it was given, and hands back a fakeHandle scripted from that half's halfScript.
type fakeShuttle struct {
	review, fix halfScript
	// markerPath is the round's absolute ready marker path, which waitForMarker polls.
	markerPath string

	mu sync.Mutex
	// started holds the roles of the halves that were started, in start order.
	started []string
	// markerAtStart records, per started role, whether the ready marker existed when the half was started.
	markerAtStart map[string]bool
	specs         map[string]shuttleengine.Spec
	gates         map[string]shuttleengine.GateSpec
	handles       map[string]*fakeHandle
	// gateFindings collects the findings of every failed gate evaluation.
	gateFindings []string
}

// fakeHandle is one started fake half.
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
}

func (h *fakeHandle) StrandGUID() string { return h.guid }

func (h *fakeHandle) RunDir() string { return h.runDir }

// stop ends the handle's blocked Wait as died.
func (h *fakeHandle) stop() {
	h.stopOnce.Do(func() { close(h.stopped) })
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

// Wait blocks as its script says, then reports the scripted result, first writing the half's output file for a done outcome and evaluating the half's gate like the shuttle wait loop would.
func (h *fakeHandle) Wait() (shuttleengine.Result, error) {
	defer h.returned.Store(true)

	if h.script.hold != nil {
		select {
		case <-h.script.hold:
		case <-h.stopped:
			return h.diedResult(), nil
		}
	}
	if h.script.waitForMarker && !h.waitForMarker() {
		return h.diedResult(), nil
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
	if len(h.gate) > 0 {
		gate, err := h.evaluateGate()
		if err != nil {
			return result, err
		}
		result.Gate = gate
	}
	return result, nil
}

// waitForMarker polls for the ready marker, reporting false when the half is stopped first.
// A marker that never appears fails the test by the deadline rather than hanging it.
func (h *fakeHandle) waitForMarker() bool {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(h.shuttle.markerPath); err == nil {
			return true
		}
		select {
		case <-h.stopped:
			return false
		case <-time.After(time.Millisecond):
		}
	}
	return false
}

// evaluateGate runs the handle's gate spec like the shuttle wait loop:
// each entry's closure runs in list order, skipping off entries.
// A failing entry is re-prompted up to its Attempts, each re-prompt first writing the next rewrites content to the output file as the half would,
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

// StartGated registers the half its spec's role names and returns a handle scripted from that half's halfScript, or its start error.
func (f *fakeShuttle) StartGated(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (Handle, error) {
	script := f.fix
	if spec.Role == burlerReviewRole {
		script = f.review
	}

	f.mu.Lock()
	if f.specs == nil {
		f.specs = map[string]shuttleengine.Spec{}
		f.gates = map[string]shuttleengine.GateSpec{}
		f.handles = map[string]*fakeHandle{}
		f.markerAtStart = map[string]bool{}
	}
	f.started = append(f.started, spec.Role)
	f.specs[spec.Role] = spec
	f.gates[spec.Role] = gate
	_, statErr := os.Stat(f.markerPath)
	f.markerAtStart[spec.Role] = statErr == nil
	var handle *fakeHandle
	if script.startErr == nil {
		handle = &fakeHandle{
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
	}
	f.mu.Unlock()

	if script.onStart != nil {
		script.onStart()
	}
	if handle == nil {
		return nil, script.startErr
	}
	return handle, nil
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

// fakeRemover is a same-package StrandRemover double: it records every guid it is asked to remove, ends the removed handle's Wait as died, and fails when told to.
type fakeRemover struct {
	shuttle *fakeShuttle
	err     error

	mu      sync.Mutex
	removed []string
}

func (r *fakeRemover) RemoveStrandIfLive(guid string) error {
	r.mu.Lock()
	r.removed = append(r.removed, guid)
	r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.shuttle.stop(guid)
	return nil
}

// doneRound scripts a round whose reviewer writes review and whose fixer waits for the ready marker, then writes its report.
func doneRound(review string) *fakeShuttle {
	return &fakeShuttle{
		review: halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, writes: review},
		fix:    halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, waitForMarker: true, writes: "nothing fixed"},
	}
}

// newEngineTestProfile builds a minimal valid Profile (relative paths — the
// engine resolves them against root via validate) rooted at a fresh temp dir.
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
		ReadyMarkerPath: "review.md.ready",
	}
	return root, p
}

// newEngineForTest wires an Engine to shuttle and a fresh fakeRemover over a Geometry rooted at root.
func newEngineForTest(t *testing.T, root string, shuttle *fakeShuttle) (*Engine, *fakeRemover) {
	t.Helper()
	return newEngineWith(t, Geometry{WorktreeRoot: root, AnchorPath: root}, Config{}, shuttle)
}

// newEngineWith is newEngineForTest over a told geometry and config.
func newEngineWith(t *testing.T, geom Geometry, cfg Config, shuttle *fakeShuttle) (*Engine, *fakeRemover) {
	t.Helper()
	shuttle.markerPath = filepath.Join(geom.WorktreeRoot, "review.md.ready")
	remover := &fakeRemover{shuttle: shuttle}
	return New(shuttle, remover, geom, cfg, newTestStencilsDir(t), ""), remover
}

const (
	approvedReview = "---\nverdict: APPROVED\n---\nlooks good\n"
	blockingReview = "---\nverdict: BLOCKING\nfindings:\n" +
		"  - id: F1\n    severity: BLOCKING\n    class: design\n    location: target.txt:1\n    summary: colors do not match\n" +
		"---\nfound a mismatch\n"
	malformedReview = "not frontmatter at all\n"

	reviewRole = burlerReviewRole
	fixRole    = burlerFixRole
)

// clusterTestConfig is a one-lens, one-fan config, so a cluster profile resolves to exactly one fork.
var clusterTestConfig = Config{
	Lenses: map[string]string{"style": "style prose"},
	Fans:   map[string][]string{"standard": {"style"}},
}

// TestEngine_Run_SpecConstruction proves Run starts both halves before either finishes and builds each half's shuttle Spec as the round driver pins it:
// the reviewer then the fixer, each with a non-empty prompt, its own role and one output file resolved absolute, the round's timeout, round and skills, its own model choice, and Interactive left false.
func TestEngine_Run_SpecConstruction(t *testing.T) {
	root, p := newEngineTestProfile(t)
	shuttle := doneRound(approvedReview)
	// The reviewer cannot finish before the fixer has started.
	reviewHold := make(chan struct{})
	shuttle.review.hold = reviewHold
	shuttle.fix.onStart = func() { close(reviewHold) }
	e, _ := newEngineForTest(t, root, shuttle)

	opts := RunOpts{
		Review:  ModelChoice{Model: "haiku", Effort: "low", Version: "5"},
		Fix:     ModelChoice{Model: "opus", Effort: "high"},
		Timeout: 5 * time.Minute,
		Round:   "1",
	}
	if _, err := e.Run(p, opts); err != nil {
		t.Fatalf("Run() = %v; want nil error", err)
	}
	if want := []string{reviewRole, fixRole}; !slices.Equal(shuttle.started, want) {
		t.Fatalf("started halves = %v; want %v, both before either finished", shuttle.started, want)
	}

	tests := []struct {
		role       string
		wantOutput string
		wantChoice ModelChoice
	}{
		{reviewRole, filepath.Join(root, "review.md"), opts.Review},
		{fixRole, filepath.Join(root, "fixer-report.md"), opts.Fix},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			spec := shuttle.specs[tt.role]
			if spec.Prompt == "" {
				t.Errorf("spec.Prompt = \"\"; want non-empty")
			}
			if got := spec.OutputFiles; len(got) != 1 || got[0] != tt.wantOutput {
				t.Errorf("spec.OutputFiles = %v; want [%q]", got, tt.wantOutput)
			}
			if spec.Role != tt.role {
				t.Errorf("spec.Role = %q; want %q", spec.Role, tt.role)
			}
			if got, want := spec.Skills, []string{"scribe:prose", "scribe:code-quality", "scribe:testing"}; !slices.Equal(got, want) {
				t.Errorf("spec.Skills = %v; want %v", got, want)
			}
			if spec.Model != tt.wantChoice.Model || spec.Effort != tt.wantChoice.Effort || spec.Version != tt.wantChoice.Version {
				t.Errorf("spec model = (%q, %q, %q); want %+v", spec.Model, spec.Effort, spec.Version, tt.wantChoice)
			}
			if spec.Timeout != opts.Timeout {
				t.Errorf("spec.Timeout = %v; want %v", spec.Timeout, opts.Timeout)
			}
			if spec.Round != opts.Round {
				t.Errorf("spec.Round = %q; want %q", spec.Round, opts.Round)
			}
			if spec.Interactive {
				t.Errorf("spec.Interactive = true; want false (autonomous default)")
			}
		})
	}
	if shuttle.specs[reviewRole].Prompt == shuttle.specs[fixRole].Prompt {
		t.Errorf("both halves received the same prompt; want each its own orchestrator")
	}
}

// TestEngine_Run_ForkSubagentsSpecWiring proves the reviewer's Spec.ForkSubagents mirrors Profile.ClusterFan
// exactly: true only when a fan actually resolved, false for a plain (non-cluster) profile — the
// sole trigger for a cluster round's fork authorization — and the fixer's is false either way.
func TestEngine_Run_ForkSubagentsSpecWiring(t *testing.T) {
	tests := []struct {
		name        string
		clusterFan  string
		wantReviews bool
	}{
		{"cluster profile", "standard", true},
		{"plain profile", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, p := newEngineTestProfile(t)
			p.ClusterFan = tt.clusterFan
			shuttle := doneRound(approvedReview)
			// "standard" resolves to exactly one lens (style) — the audit requires exactly one clean fork report to pass.
			shuttle.review.result.ForkAudit = &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{{TranscriptPath: "fork-1", ReportReturned: true}},
			}
			e, _ := newEngineWith(t, Geometry{WorktreeRoot: root, AnchorPath: root}, clusterTestConfig, shuttle)

			if _, err := e.Run(p, RunOpts{}); err != nil {
				t.Fatalf("Run() = %v; want nil error", err)
			}
			if got := shuttle.specs[reviewRole].ForkSubagents; got != tt.wantReviews {
				t.Errorf("reviewer spec.ForkSubagents = %v; want %v", got, tt.wantReviews)
			}
			if shuttle.specs[fixRole].ForkSubagents {
				t.Errorf("fixer spec.ForkSubagents = true; want false")
			}
		})
	}
}

// TestEngine_Run_ClusterAuditPolicy proves Run wires the reviewer's ForkAudit through
// auditClusterRound for a done cluster round: a violating audit fails Run with the populated-so-far
// Result carrying the raw ForkAudit and stops the fixer, a clean-but-warning audit passes with Result.ClusterWarnings
// copied through, and a non-cluster profile never invokes the policy at all — no
// ForkAudit/ClusterWarnings on the Result even when the fake shuttle's scripted Result carries one.
func TestEngine_Run_ClusterAuditPolicy(t *testing.T) {
	violatingAudit := &shuttleengine.ForkAudit{
		Forks: []shuttleengine.ForkReport{{TranscriptPath: "fork-1", ReportReturned: true, WriteCalls: 1}},
	}
	cleanAudit := &shuttleengine.ForkAudit{
		Forks: []shuttleengine.ForkReport{{TranscriptPath: "fork-1", ReportReturned: false}},
	}

	t.Run("violating audit fails the round and stops the fixer", func(t *testing.T) {
		root, p := newEngineTestProfile(t)
		p.ClusterFan = "standard"
		shuttle := doneRound(approvedReview)
		shuttle.review.result.ForkAudit = violatingAudit
		e, remover := newEngineWith(t, Geometry{WorktreeRoot: root, AnchorPath: root}, clusterTestConfig, shuttle)

		got, err := e.Run(p, RunOpts{})
		if err == nil {
			t.Fatalf("Run() error = nil; want a cluster audit policy failure")
		}
		if !strings.Contains(err.Error(), "write/edit tool call") {
			t.Errorf("Run() error = %q; want it to carry the underlying audit violation", err.Error())
		}
		if got.ForkAudit != violatingAudit {
			t.Errorf("Result.ForkAudit = %v; want the raw audit copied through even on a policy failure", got.ForkAudit)
		}
		if want := []string{fixRole + "-guid"}; !slices.Equal(remover.removed, want) {
			t.Errorf("removed strands = %v; want the fixer %v", remover.removed, want)
		}
	})

	t.Run("clean audit with a warning passes and copies warnings through", func(t *testing.T) {
		root, p := newEngineTestProfile(t)
		p.ClusterFan = "standard"
		shuttle := doneRound(approvedReview)
		shuttle.review.result.ForkAudit = cleanAudit
		e, _ := newEngineWith(t, Geometry{WorktreeRoot: root, AnchorPath: root}, clusterTestConfig, shuttle)

		got, err := e.Run(p, RunOpts{})
		if err != nil {
			t.Fatalf("Run() = %v; want nil error", err)
		}
		if len(got.ClusterWarnings) != 1 {
			t.Fatalf("Result.ClusterWarnings = %v; want exactly one warning", got.ClusterWarnings)
		}
		if got.ForkAudit != cleanAudit {
			t.Errorf("Result.ForkAudit = %v; want the reviewer's audit", got.ForkAudit)
		}
	})

	t.Run("non-cluster profile never invokes the policy", func(t *testing.T) {
		root, p := newEngineTestProfile(t)
		// A non-cluster profile carries no ClusterFan; a scripted ForkAudit
		// on the reviewer's Result must be ignored entirely — the
		// policy is not even consulted, so it must never surface on
		// Result even though the shuttle "returned" one.
		shuttle := doneRound(approvedReview)
		shuttle.review.result.ForkAudit = &shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{{WriteCalls: 99}}}
		e, _ := newEngineWith(t, Geometry{WorktreeRoot: root, AnchorPath: root}, clusterTestConfig, shuttle)

		got, err := e.Run(p, RunOpts{})
		if err != nil {
			t.Fatalf("Run() = %v; want nil error (policy never invoked for a non-cluster profile)", err)
		}
		if got.ClusterWarnings != nil {
			t.Errorf("Result.ClusterWarnings = %v; want nil for a non-cluster profile", got.ClusterWarnings)
		}
	})
}

// TestEngine_Run_ShuttleOutcomes table-drives Run over every outcome of either half and the review-file parse path.
// A non-done outcome of the reviewer, or of the fixer after the handoff, carries through to Result.Outcome with an empty Verdict and a nil error.
// A done round parses its review file into VerdictBlocking with its findings or VerdictApproved with none, and fails loud -- never defaulting a verdict -- on a review file that was never written (a fake-shuttle-only scenario; the real shuttle's file-contract polling makes it impossible in production) or whose frontmatter is malformed;
// a hard shuttle start error is wrapped, not swallowed.
// Whatever the outcome, each half's identities and kept RunDir pass through to the Result unchanged: the RunDir passthrough is what lets a caller point at the kept shuttle run dir for a died or timed-out half.
func TestEngine_Run_ShuttleOutcomes(t *testing.T) {
	t.Parallel()

	died := shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}
	timeout := shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout}
	done := shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}

	tests := []struct {
		name           string
		shuttle        *fakeShuttle
		wantOutcome    shuttleengine.Outcome
		wantErr        bool
		errSubstr      string
		wantVerdict    Verdict
		wantFindingIDs []string
	}{
		{
			name: "reviewer died",
			shuttle: &fakeShuttle{
				review: halfScript{result: died},
				fix:    halfScript{result: done, waitForMarker: true},
			},
			wantOutcome: shuttleengine.OutcomeDied,
		},
		{
			name: "reviewer timeout",
			shuttle: &fakeShuttle{
				review: halfScript{result: timeout},
				fix:    halfScript{result: done, waitForMarker: true},
			},
			wantOutcome: shuttleengine.OutcomeTimeout,
		},
		{
			name: "fixer died after the handoff",
			shuttle: &fakeShuttle{
				review: halfScript{result: done, writes: approvedReview},
				fix:    halfScript{result: died, waitForMarker: true},
			},
			wantOutcome: shuttleengine.OutcomeDied,
		},
		{
			name: "fixer timeout after the handoff",
			shuttle: &fakeShuttle{
				review: halfScript{result: done, writes: approvedReview},
				fix:    halfScript{result: timeout, waitForMarker: true},
			},
			wantOutcome: shuttleengine.OutcomeTimeout,
		},
		{
			name:           "done with a BLOCKING review",
			shuttle:        doneRound(blockingReview),
			wantOutcome:    shuttleengine.OutcomeDone,
			wantVerdict:    VerdictBlocking,
			wantFindingIDs: []string{"F1"},
		},
		{
			name:        "done with an APPROVED review",
			shuttle:     doneRound(approvedReview),
			wantOutcome: shuttleengine.OutcomeDone,
			wantVerdict: VerdictApproved,
		},
		{
			// The reviewer is done but the fake never writes the review file.
			name:        "done with a missing review file",
			shuttle:     doneRound(""),
			wantOutcome: shuttleengine.OutcomeDone,
			wantErr:     true,
			errSubstr:   "read review file",
		},
		{
			name:        "done with a malformed review file",
			shuttle:     doneRound(malformedReview),
			wantOutcome: shuttleengine.OutcomeDone,
			wantErr:     true,
			errSubstr:   "frontmatter",
		},
		{
			name: "hard shuttle start error",
			shuttle: &fakeShuttle{
				review: halfScript{startErr: errors.New("reed: add strand failed")},
			},
			wantErr:   true,
			errSubstr: "reed: add strand failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p := newEngineTestProfile(t)
			tt.shuttle.review.result.RunDir = "/kept/review-run"
			tt.shuttle.fix.result.RunDir = "/kept/fix-run"
			e, _ := newEngineForTest(t, root, tt.shuttle)

			got, err := e.Run(p, RunOpts{})

			if tt.wantErr {
				if err == nil {
					t.Fatalf("Run() error = nil; want an error")
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("Run() error = %q; want it to carry %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() = %v; want nil error", err)
			}

			if got.Outcome != tt.wantOutcome {
				t.Errorf("Result.Outcome = %q; want %q", got.Outcome, tt.wantOutcome)
			}
			if got.Verdict != tt.wantVerdict {
				t.Errorf("Result.Verdict = %q; want %q", got.Verdict, tt.wantVerdict)
			}
			var gotFindingIDs []string
			for _, finding := range got.Findings {
				gotFindingIDs = append(gotFindingIDs, finding.ID)
			}
			if !slices.Equal(gotFindingIDs, tt.wantFindingIDs) {
				t.Errorf("Result.Findings ids = %v; want %v", gotFindingIDs, tt.wantFindingIDs)
			}
			if got.Review.SessionID != reviewRole+"-session" || got.Review.StrandGUID != reviewRole+"-guid" {
				t.Errorf("Result.Review identities = (%q, %q); want the reviewer's", got.Review.SessionID, got.Review.StrandGUID)
			}
			if got.Fix.SessionID != fixRole+"-session" || got.Fix.StrandGUID != fixRole+"-guid" {
				t.Errorf("Result.Fix identities = (%q, %q); want the fixer's", got.Fix.SessionID, got.Fix.StrandGUID)
			}
			if got.Review.RunDir != "/kept/review-run" {
				t.Errorf("Result.Review.RunDir = %q; want %q", got.Review.RunDir, "/kept/review-run")
			}
			if got.NotStarted {
				t.Errorf("Result.NotStarted = true; want false for halves that started")
			}
		})
	}
}

// TestEngine_Run_GateOutcomes table-drives Run over a round's gate list.
// The reviewer's gate spec is the review-parse entry alone, unwrapped:
// its findings name the review file but not the fixer report.
// The fixer's gate spec is the caller's list, every entry wrapped by repairReportBeforeGate, so a round carrying the zero GateSpec leaves the fixer ungated.
// A round whose fixer gate fails returns a Result with Gate populated and Passed false, Verdict and Findings left empty, Outcome still OutcomeDone and a nil error.
// The failing entry's closure the shuttle received is the wrapped one the round actually ran; re-invoking it (the told closure is pure) recovers the findings text the failing attempt produced, which must name this round's own fixer-report path.
// The caller's gate list is left as it was.
func TestEngine_Run_GateOutcomes(t *testing.T) {
	t.Parallel()

	failing := func(name, findings string) shuttleengine.GateEntry {
		return shuttleengine.GateEntry{Name: name, Attempts: 3, Gate: func() (shuttleengine.GateResult, error) {
			return shuttleengine.GateResult{Passed: false, Findings: findings}, nil
		}}
	}
	passing := shuttleengine.GateEntry{Name: "first", Attempts: 3, Gate: func() (shuttleengine.GateResult, error) {
		return shuttleengine.GateResult{Passed: true}, nil
	}}

	tests := []struct {
		name string
		gate shuttleengine.GateSpec
		// failingIndex is the position of the fixer entry that fails, -1 when none does.
		failingIndex int
	}{
		{name: "zero gate spec", gate: nil, failingIndex: -1},
		{name: "single failing entry", gate: shuttleengine.GateSpec{failing("", "the widget is the wrong color")}, failingIndex: 0},
		{name: "failing second entry", gate: shuttleengine.GateSpec{passing, failing("second", "the housing is the wrong color")}, failingIndex: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p := newEngineTestProfile(t)
			shuttle := doneRound(approvedReview)
			e, _ := newEngineForTest(t, root, shuttle)
			var namesBefore []string
			for _, entry := range tt.gate {
				namesBefore = append(namesBefore, entry.Name)
			}

			got, err := e.Run(p, RunOpts{Gate: tt.gate})
			if err != nil {
				t.Fatalf("Run() = %v; want nil error", err)
			}
			var namesAfter []string
			for _, entry := range tt.gate {
				namesAfter = append(namesAfter, entry.Name)
			}
			if !slices.Equal(namesBefore, namesAfter) {
				t.Fatalf("gate entry names = %v; want the caller's list left as it was, %v", namesAfter, namesBefore)
			}

			reviewGate := shuttle.gates[reviewRole]
			if len(reviewGate) != 1 {
				t.Fatalf("reviewer gate spec has %d entries; want the review entry alone", len(reviewGate))
			}
			reviewEntry := reviewGate[0]
			if reviewEntry.Name != "review" || reviewEntry.Attempts != reviewGateAttempts || !reviewEntry.PassOnCap {
				t.Errorf("reviewer gate entry = {Name: %q, Attempts: %d, PassOnCap: %v}; want the review entry with %d attempts that passes on cap", reviewEntry.Name, reviewEntry.Attempts, reviewEntry.PassOnCap, reviewGateAttempts)
			}
			if len(shuttle.gates[fixRole]) != len(tt.gate) {
				t.Fatalf("fixer gate spec has %d entries; want the caller's %d", len(shuttle.gates[fixRole]), len(tt.gate))
			}
			if err := os.Remove(filepath.Join(root, "review.md")); err != nil && !os.IsNotExist(err) {
				t.Fatalf("Remove(review) = %v; want nil", err)
			}
			reviewResult, gerr := reviewEntry.Gate()
			if gerr != nil || reviewResult.Passed {
				t.Fatalf("review entry on a missing file = (%+v, %v); want a failed result and nil error", reviewResult, gerr)
			}
			if !strings.Contains(reviewResult.Findings, filepath.Join(root, "review.md")) || strings.Contains(reviewResult.Findings, filepath.Join(root, "fixer-report.md")) {
				t.Errorf("review entry findings = %q; want them to name the review file only", reviewResult.Findings)
			}

			if tt.failingIndex < 0 {
				if got.Gate != nil {
					t.Errorf("Result.Gate = %+v; want nil for an ungated fixer", got.Gate)
				}
				return
			}
			if got.Gate == nil || got.Gate.Passed {
				t.Fatalf("Result.Gate = %+v; want populated with Passed false", got.Gate)
			}
			if got.Verdict != "" {
				t.Errorf("Result.Verdict = %q; want empty", got.Verdict)
			}
			if len(got.Findings) != 0 {
				t.Errorf("Result.Findings = %+v; want none", got.Findings)
			}
			if got.Outcome != shuttleengine.OutcomeDone {
				t.Errorf("Result.Outcome = %q; want %q", got.Outcome, shuttleengine.OutcomeDone)
			}

			gateResult, gerr := shuttle.gates[fixRole][tt.failingIndex].Gate()
			if gerr != nil {
				t.Fatalf("fixer gate[%d].Gate() = %v; want nil error", tt.failingIndex, gerr)
			}
			if !strings.Contains(gateResult.Findings, filepath.Join(root, "fixer-report.md")) {
				t.Errorf("gate findings = %q; want them to name the fixer report", gateResult.Findings)
			}
			if strings.Contains(gateResult.Findings, filepath.Join(root, "review.md")) {
				t.Errorf("gate findings = %q; want them not to name the review file, which the fixer does not write", gateResult.Findings)
			}
		})
	}
}

// TestEngine_Run_ReviewGateRepairsUnparseableReview drives Run with a first review file that fails ParseReview.
// The review entry fails with the parse error and the quoting hint in its findings,
// and the fake shuttle's reviewer rewrites the file at each re-prompt.
// A rewrite that parses passes the entry,
// and Run returns the verdict of the repaired file.
// A file still invalid after the entry's budget is let through,
// and Run fails with the strict parse error, after exactly one failed evaluation per attempt plus the capping one.
func TestEngine_Run_ReviewGateRepairsUnparseableReview(t *testing.T) {
	t.Parallel()

	const quotedFragmentReview = "---\nverdict: BLOCKING\nfindings:\n" +
		"  - id: F1\n    severity: BLOCKING\n    class: design\n    location: target.txt:1\n    summary: \"capital\" is misspelled as \"captial\"\n" +
		"---\nfound a mismatch\n"

	tests := []struct {
		name           string
		rewrites       []string
		wantErrSubstr  string
		wantVerdict    Verdict
		wantEvaluation int
	}{
		{name: "repaired on the first re-prompt", rewrites: []string{blockingReview}, wantVerdict: VerdictBlocking, wantEvaluation: 1},
		{name: "still invalid after the budget", rewrites: []string{quotedFragmentReview, quotedFragmentReview}, wantErrSubstr: "round reached done but its review file is invalid", wantEvaluation: reviewGateAttempts + 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p := newEngineTestProfile(t)
			shuttle := doneRound(quotedFragmentReview)
			shuttle.review.rewrites = tt.rewrites
			e, _ := newEngineForTest(t, root, shuttle)

			got, err := e.Run(p, RunOpts{})

			if len(shuttle.gateFindings) != tt.wantEvaluation {
				t.Fatalf("failed review gate evaluations = %d; want %d", len(shuttle.gateFindings), tt.wantEvaluation)
			}
			for _, findings := range shuttle.gateFindings {
				for _, want := range []string{"frontmatter is not valid YAML", "ONE double-quoted string", filepath.Join(root, "review.md")} {
					if !strings.Contains(findings, want) {
						t.Errorf("review gate findings = %q; want them to carry %q", findings, want)
					}
				}
			}
			if tt.wantErrSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Fatalf("Run() error = %v; want it to carry %q", err, tt.wantErrSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() = %v; want nil error", err)
			}
			if got.Verdict != tt.wantVerdict {
				t.Errorf("Result.Verdict = %q; want %q", got.Verdict, tt.wantVerdict)
			}
		})
	}
}

// TestEngine_Run_MarkerLifecycle table-drives the ready marker's lifecycle.
// A stale marker is removed before either half starts, and the reviewer is held until the fixer has started so the marker is observably absent while the reviewer runs.
// The marker exists afterwards only when the reviewer reached done with a parsing review and, for a cluster round, a passing audit.
func TestEngine_Run_MarkerLifecycle(t *testing.T) {
	t.Parallel()

	done := shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}
	passingAudit := &shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{{TranscriptPath: "fork-1", ReportReturned: true}}}
	violatingAudit := &shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{{TranscriptPath: "fork-1", ReportReturned: true, WriteCalls: 1}}}

	tests := []struct {
		name        string
		review      halfScript
		cluster     bool
		staleMarker bool
		wantMarker  bool
		wantErr     bool
	}{
		{name: "parsing review writes the marker", review: halfScript{result: done, writes: approvedReview}, wantMarker: true},
		{name: "stale marker is removed and the fresh one written", review: halfScript{result: done, writes: approvedReview}, staleMarker: true, wantMarker: true},
		{name: "unparseable review leaves no marker", review: halfScript{result: done, writes: malformedReview}, staleMarker: true, wantErr: true},
		{name: "died reviewer leaves no marker", review: halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}}, staleMarker: true},
		{name: "cluster round with a passing audit writes the marker", review: halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, ForkAudit: passingAudit}, writes: approvedReview}, cluster: true, wantMarker: true},
		{name: "cluster round with a violating audit leaves no marker", review: halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, ForkAudit: violatingAudit}, writes: approvedReview}, cluster: true, staleMarker: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p := newEngineTestProfile(t)
			if tt.cluster {
				p.ClusterFan = "standard"
			}
			shuttle := doneRound("")
			shuttle.review = tt.review
			reviewHold := make(chan struct{})
			shuttle.review.hold = reviewHold
			shuttle.fix.onStart = func() { close(reviewHold) }
			e, _ := newEngineWith(t, Geometry{WorktreeRoot: root, AnchorPath: root}, clusterTestConfig, shuttle)
			markerPath := filepath.Join(root, "review.md.ready")
			if tt.staleMarker {
				if err := os.WriteFile(markerPath, []byte("stale"), 0o644); err != nil {
					t.Fatalf("WriteFile(stale marker) = %v; want nil", err)
				}
			}

			_, err := e.Run(p, RunOpts{})

			if (err != nil) != tt.wantErr {
				t.Fatalf("Run() error = %v; wantErr %v", err, tt.wantErr)
			}
			for role, existed := range shuttle.markerAtStart {
				if existed {
					t.Errorf("the marker existed when the %s half started; want it removed or not yet written", role)
				}
			}
			_, statErr := os.Stat(markerPath)
			if gotMarker := statErr == nil; gotMarker != tt.wantMarker {
				t.Errorf("marker exists after the round = %v; want %v", gotMarker, tt.wantMarker)
			}
		})
	}
}

// TestEngine_Run_RoundFailureRules table-drives the round's failure rules, one row each.
// The rows cover every failure row of join (the stopped half's guid, the deciding half's outcome or error, and no fake handle left blocked), the start failures of either half, and a half that cannot be stopped.
func TestEngine_Run_RoundFailureRules(t *testing.T) {
	t.Parallel()

	done := shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}
	notStarted := fmt.Errorf("shuttle: start run: %w", shuttleengine.ErrNotStarted)
	plainStartErr := errors.New("reed: add strand failed")

	tests := []struct {
		name    string
		shuttle *fakeShuttle
		// removeErr makes the remover fail every removal.
		removeErr   error
		wantOutcome shuttleengine.Outcome
		wantErrIs   error
		wantErrText string
		wantRemoved []string
		wantStarted []string
		// leftLive names a row whose failed stop leaves a half running by design.
		leftLive bool
		// editReview, when set, is written over the review file by the fixer before it reports done.
		editReview  string
		check       func(t *testing.T, got Result)
		noVerdict   bool
		wantVerdict Verdict
	}{
		{
			name: "a reviewer that died stops the fixer and reports the reviewer's outcome",
			shuttle: &fakeShuttle{
				review: halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}},
				fix:    halfScript{result: done, waitForMarker: true},
			},
			wantOutcome: shuttleengine.OutcomeDied,
			wantRemoved: []string{fixRole + "-guid"},
			wantStarted: []string{reviewRole, fixRole},
			noVerdict:   true,
		},
		{
			name: "an unparseable review stops the fixer and reports the strict parse error",
			shuttle: &fakeShuttle{
				review: halfScript{result: done, writes: malformedReview},
				fix:    halfScript{result: done, waitForMarker: true},
			},
			wantErrText: "round reached done but its review file is invalid",
			wantRemoved: []string{fixRole + "-guid"},
			wantStarted: []string{reviewRole, fixRole},
		},
		{
			name: "a fixer that timed out while the reviewer runs stops the reviewer and reports the fixer's outcome",
			shuttle: &fakeShuttle{
				review: halfScript{result: done, writes: approvedReview, hold: make(chan struct{})},
				fix:    halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout}},
			},
			wantOutcome: shuttleengine.OutcomeTimeout,
			wantRemoved: []string{reviewRole + "-guid"},
			wantStarted: []string{reviewRole, fixRole},
			noVerdict:   true,
			check: func(t *testing.T, got Result) {
				if got.Fix.StrandGUID != fixRole+"-guid" || got.Review.StrandGUID != reviewRole+"-guid" {
					t.Errorf("per-half identities = (%q, %q); want both halves named", got.Review.StrandGUID, got.Fix.StrandGUID)
				}
			},
		},
		{
			name: "a fixer that is done before the marker stops the reviewer and reports the skipped handoff",
			shuttle: &fakeShuttle{
				review: halfScript{result: done, writes: approvedReview, hold: make(chan struct{})},
				fix:    halfScript{result: done, writes: "nothing fixed"},
			},
			wantErrText: "before the review was handed off",
			wantRemoved: []string{reviewRole + "-guid"},
			wantStarted: []string{reviewRole, fixRole},
		},
		{
			name: "a review changed after the handoff is an error",
			shuttle: &fakeShuttle{
				review: halfScript{result: done, writes: approvedReview},
				fix:    halfScript{result: done, waitForMarker: true, writes: "nothing fixed"},
			},
			editReview:  blockingReview,
			wantErrText: "changed after it was handed off",
			wantStarted: []string{reviewRole, fixRole},
		},
		{
			name: "a fixer-report with a Disputed section completes like any other",
			shuttle: &fakeShuttle{
				review: halfScript{result: done, writes: blockingReview},
				fix:    halfScript{result: done, waitForMarker: true, writes: "## Disputed\n\n- F1: the cited code contradicts the premise\n"},
			},
			wantOutcome: shuttleengine.OutcomeDone,
			wantVerdict: VerdictBlocking,
			wantStarted: []string{reviewRole, fixRole},
		},
		{
			name: "a half that cannot be stopped is ErrHalfNotStopped with the way forward",
			shuttle: &fakeShuttle{
				review: halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}},
				fix:    halfScript{result: done, waitForMarker: true},
			},
			removeErr:   errors.New("reed unreachable"),
			wantErrIs:   ErrHalfNotStopped,
			wantErrText: `way forward: run "lyx reed remove ` + fixRole + `-guid", then re-step the row`,
			wantRemoved: []string{fixRole + "-guid"},
			wantStarted: []string{reviewRole, fixRole},
			leftLive:    true,
		},
		{
			name: "a reviewer that never started leaves the fixer unstarted",
			shuttle: &fakeShuttle{
				review: halfScript{startErr: notStarted},
			},
			wantOutcome: shuttleengine.OutcomeDied,
			wantStarted: []string{reviewRole},
			noVerdict:   true,
			check: func(t *testing.T, got Result) {
				if !got.NotStarted || got.Review.StartError != notStarted.Error() || got.Review.StrandGUID != "" || got.Review.SessionID != "" || got.Review.RunDir != "" {
					t.Errorf("Result = %+v; want NotStarted, the reviewer's StartError text and no reviewer identity", got)
				}
			},
		},
		{
			name: "a fixer that never started stops the reviewer first",
			shuttle: &fakeShuttle{
				review: halfScript{result: done, writes: approvedReview, hold: make(chan struct{})},
				fix:    halfScript{startErr: notStarted},
			},
			wantOutcome: shuttleengine.OutcomeDied,
			wantRemoved: []string{reviewRole + "-guid"},
			wantStarted: []string{reviewRole, fixRole},
			noVerdict:   true,
			check: func(t *testing.T, got Result) {
				if !got.NotStarted || got.Fix.StartError != notStarted.Error() || got.Fix.StrandGUID != "" || got.Review.StrandGUID != reviewRole+"-guid" || got.Review.StartError != "" {
					t.Errorf("Result = %+v; want NotStarted, the fixer's StartError text, no fixer identity and the reviewer's identity", got)
				}
			},
		},
		{
			name: "a reviewer whose start fails with a plain error is returned and the fixer never started",
			shuttle: &fakeShuttle{
				review: halfScript{startErr: plainStartErr},
			},
			wantErrIs:   plainStartErr,
			wantErrText: "burler: shuttle run:",
			wantStarted: []string{reviewRole},
		},
		{
			name: "a fixer whose start fails with a plain error stops the reviewer and is returned",
			shuttle: &fakeShuttle{
				review: halfScript{result: done, writes: approvedReview, hold: make(chan struct{})},
				fix:    halfScript{startErr: plainStartErr},
			},
			wantErrIs:   plainStartErr,
			wantErrText: "burler: shuttle run:",
			wantRemoved: []string{reviewRole + "-guid"},
			wantStarted: []string{reviewRole, fixRole},
		},
		{
			name: "a fixer whose start fails with a plain error and a reviewer that cannot be stopped is ErrHalfNotStopped",
			shuttle: &fakeShuttle{
				review: halfScript{result: done, writes: approvedReview, hold: make(chan struct{})},
				fix:    halfScript{startErr: plainStartErr},
			},
			removeErr:   errors.New("reed unreachable"),
			wantErrIs:   ErrHalfNotStopped,
			wantErrText: `way forward: run "lyx reed remove ` + reviewRole + `-guid", then re-step the row`,
			wantRemoved: []string{reviewRole + "-guid"},
			wantStarted: []string{reviewRole, fixRole},
			leftLive:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p := newEngineTestProfile(t)
			if tt.editReview != "" {
				tt.shuttle.fix.beforeReturn = func() {
					if err := os.WriteFile(filepath.Join(root, "review.md"), []byte(tt.editReview), 0o644); err != nil {
						t.Errorf("WriteFile(review) = %v; want nil", err)
					}
				}
			}
			e, remover := newEngineForTest(t, root, tt.shuttle)
			remover.err = tt.removeErr

			got, err := e.Run(p, RunOpts{})

			wantErr := tt.wantErrIs != nil || tt.wantErrText != ""
			if wantErr != (err != nil) {
				t.Fatalf("Run() error = %v; wantErr %v", err, wantErr)
			}
			if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
				t.Errorf("Run() error = %v; want it to wrap %v", err, tt.wantErrIs)
			}
			if tt.wantErrText != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErrText)) {
				t.Errorf("Run() error = %v; want it to carry %q", err, tt.wantErrText)
			}
			if tt.wantErrIs == ErrHalfNotStopped && (err == nil || !strings.HasSuffix(err.Error(), tt.wantErrText)) {
				t.Errorf("Run() error = %v; want it to end with the way forward %q", err, tt.wantErrText)
			}
			if !wantErr {
				if got.Outcome != tt.wantOutcome {
					t.Errorf("Result.Outcome = %q; want %q", got.Outcome, tt.wantOutcome)
				}
				if got.Verdict != tt.wantVerdict {
					t.Errorf("Result.Verdict = %q; want %q", got.Verdict, tt.wantVerdict)
				}
			}
			if tt.noVerdict && (got.Verdict != "" || len(got.Findings) != 0) {
				t.Errorf("Result verdict = (%q, %v); want empty", got.Verdict, got.Findings)
			}
			if !slices.Equal(remover.removed, tt.wantRemoved) {
				t.Errorf("removed strands = %v; want %v", remover.removed, tt.wantRemoved)
			}
			if !slices.Equal(tt.shuttle.started, tt.wantStarted) {
				t.Errorf("started halves = %v; want %v", tt.shuttle.started, tt.wantStarted)
			}
			if tt.leftLive {
				for _, guid := range tt.wantRemoved {
					tt.shuttle.stop(guid)
				}
			} else if blocked := tt.shuttle.blocked(); len(blocked) != 0 {
				t.Errorf("halves left blocked: %v; want every started half stopped or finished", blocked)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

// TestEngine_Run_MaterializesInstructionFiles proves Run writes exactly four instruction files to
// a fresh per-round directory under lyxdirs.DotLyxDirName, bakes their absolute
// paths into the two orchestrator prompts it hands the shuttle, and that a rendered file's content
// reflects a filled marker from the profile.
func TestEngine_Run_MaterializesInstructionFiles(t *testing.T) {
	root, p := newEngineTestProfile(t)
	shuttle := doneRound(approvedReview)
	// worktreeRoot and anchorPath are two distinct directories here — anchorPath sits under
	// worktreeRoot at the same "sub/dir" subpath a real AnchorRel would produce — so the instruction
	// dir's AnchorPath anchoring (as opposed to WorktreeRoot) is actually observable, and a swapped
	// constructor is caught directly by the two assertions below rather than surfacing downstream as
	// an unrelated file-not-found.
	worktreeRoot := root
	anchorPath := filepath.Join(worktreeRoot, "sub", "dir")
	e, _ := newEngineWith(t, Geometry{WorktreeRoot: worktreeRoot, AnchorPath: anchorPath}, Config{}, shuttle)

	if _, err := e.Run(p, RunOpts{}); err != nil {
		t.Fatalf("Run() = %v; want nil error", err)
	}

	burlerDir := filepath.Join(anchorPath, lyxdirs.DotLyxDirName, "burler")
	matches, err := filepath.Glob(filepath.Join(burlerDir, "round-*", "instruction-*.md"))
	if err != nil {
		t.Fatalf("filepath.Glob() = %v; want nil", err)
	}
	if len(matches) != 4 {
		t.Fatalf("materialized instruction files = %v; want exactly 4", matches)
	}

	// The round directory must land under AnchorPath, never under WorktreeRoot — a swapped
	// constructor (WorktreeRoot/AnchorPath transposed) must fail here, at the construction boundary,
	// rather than surfacing downstream as a file-not-found.
	worktreeBurlerDir := filepath.Join(worktreeRoot, lyxdirs.DotLyxDirName, "burler")
	if _, err := os.Stat(worktreeBurlerDir); !os.IsNotExist(err) {
		t.Errorf("os.Stat(%q) = %v; want it NOT to exist (the round dir must land under AnchorPath, not WorktreeRoot)", worktreeBurlerDir, err)
	}

	roundDir := filepath.Dir(matches[0])
	wantPaths := map[string][]string{
		reviewRole: {"instruction-1-explore-reviewer.md", "instruction-2-review.md"},
		fixRole:    {"instruction-1-explore-fixer.md", "instruction-2-review.md", "instruction-3-fix.md"},
	}
	for role, names := range wantPaths {
		for _, name := range names {
			if path := filepath.Join(roundDir, name); !strings.Contains(shuttle.specs[role].Prompt, path) {
				t.Errorf("%s prompt does not contain materialized instruction path %q", role, path)
			}
		}
	}

	// The rubric is the profile's, so it should show up in the explore file, proving the file
	// actually carries a filled marker rather than empty/leftover template syntax.
	content, err := os.ReadFile(filepath.Join(roundDir, "instruction-1-explore-reviewer.md"))
	if err != nil {
		t.Fatalf("ReadFile(instruction-1-explore-reviewer.md) = %v; want nil", err)
	}
	if !strings.Contains(string(content), p.Rubric) {
		t.Errorf("instruction-1-explore-reviewer.md content = %q; want it to contain the profile's rubric %q", content, p.Rubric)
	}
}

// TestEngine_Run_PatternDirectiveReachesInstruction1 closes the one pattern.Directive call site with no behavioural coverage at all before this task.
// Engine.Run passes the directive into composePrompt as patternDirective, and composePrompt fills it into the explore files' values map only, through stencil.FillOptional with pattern_directive in the optional set.
// The review and fix files therefore never receive it, and the orchestrator prompts never carry it.
// Each sub-test plants PATTERN.md at the repo root, or somewhere else for the negative cases,
// before Run, and asserts on both explore files' disk content for the inlined overview text.
// The subpath-anchored cases tell WorktreeRoot and AnchorPath a subdirectory of the repo root, as a
// hub with a non-"." AnchorRel does, so a directive read through either of them instead of RepoRoot
// fails here.
// The pair of active and inactive cases is what makes the assertion
// meaningful, since a presence-only assertion cannot distinguish a working read from a template that
// hardcodes the text.
func TestEngine_Run_PatternDirectiveReachesInstruction1(t *testing.T) {
	const overview = "# PATTERN\n\n- PATTERN-demo: the demo rule\n"
	tests := []struct {
		name       string
		patternAt  string // directory under the repo root holding PATTERN.md, "" for none
		anchorRel  string // the told WorktreeRoot and AnchorPath, relative to the repo root
		wantPinned bool
	}{
		{name: "PATTERN active", patternAt: ".", anchorRel: ".", wantPinned: true},
		{name: "PATTERN inactive", patternAt: "", anchorRel: ".", wantPinned: false},
		{name: "PATTERN active, subpath-anchored", patternAt: ".", anchorRel: "sub/dir", wantPinned: true},
		{name: "PATTERN only under the anchor path", patternAt: "sub/dir", anchorRel: "sub/dir", wantPinned: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, p := newEngineTestProfile(t)
			anchorPath := filepath.Join(root, filepath.FromSlash(tt.anchorRel))
			if tt.anchorRel != "." {
				// The profile's relative paths resolve against WorktreeRoot, which is the anchor path here.
				if err := os.MkdirAll(anchorPath, 0o755); err != nil {
					t.Fatalf("MkdirAll(%q) = %v; want nil", anchorPath, err)
				}
				for _, name := range []string{"target.txt", "fasit.txt"} {
					if err := os.WriteFile(filepath.Join(anchorPath, name), []byte(name), 0o644); err != nil {
						t.Fatalf("WriteFile(%s) = %v; want nil", name, err)
					}
				}
			}
			if tt.patternAt != "" {
				patternPath := filepath.Join(root, filepath.FromSlash(tt.patternAt), "PATTERN.md")
				if err := os.WriteFile(patternPath, []byte(overview), 0o644); err != nil {
					t.Fatalf("WriteFile(PATTERN.md) = %v; want nil", err)
				}
			}
			shuttle := doneRound(approvedReview)
			e, _ := newEngineWith(t, Geometry{WorktreeRoot: anchorPath, AnchorPath: anchorPath, RepoRoot: root}, Config{}, shuttle)

			if _, err := e.Run(p, RunOpts{}); err != nil {
				t.Fatalf("Run() = %v; want nil error", err)
			}

			burlerDir := filepath.Join(anchorPath, lyxdirs.DotLyxDirName, "burler")
			entries, err := os.ReadDir(burlerDir)
			if err != nil {
				t.Fatalf("ReadDir(%q) = %v; want nil", burlerDir, err)
			}
			if len(entries) != 1 {
				t.Fatalf("ReadDir(%q) = %v; want exactly one round entry", burlerDir, entries)
			}

			for _, name := range []string{"instruction-1-explore-reviewer.md", "instruction-1-explore-fixer.md"} {
				content, err := os.ReadFile(filepath.Join(burlerDir, entries[0].Name(), name))
				if err != nil {
					t.Fatalf("ReadFile(%s) = %v; want nil", name, err)
				}
				if gotPinned := strings.Contains(string(content), overview); gotPinned != tt.wantPinned {
					t.Errorf("%s contains the PATTERN overview = %v; want %v", name, gotPinned, tt.wantPinned)
				}
			}
		})
	}
}

// TestEngine_Run_MaterializeFailure proves a materialization failure (the .lyx parent directory
// cannot be created) returns a hard error naming the failure, before the shuttle is ever invoked.
func TestEngine_Run_MaterializeFailure(t *testing.T) {
	root, p := newEngineTestProfile(t)

	// A regular file at <root>/.lyx makes os.MkdirAll(<root>/.lyx/burler)
	// fail: ".lyx" cannot be traversed as a directory component. The burler
	// dir join is AnchorPath()-anchored (lyxdirs.DotLyxDirName); this
	// fixture leaves AnchorRel unset (AnchorRel "."), so AnchorPath()
	// equals WorktreePath() and the file sits directly at WorktreePath()/
	// .lyx. validate() resolves target.txt/fasit.txt against the same
	// WorktreePath() root and is unaffected.
	notdir := filepath.Join(root, lyxdirs.DotLyxDirName)
	if err := os.WriteFile(notdir, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile(notdir) = %v; want nil", err)
	}

	shuttle := doneRound(approvedReview)
	e, _ := newEngineForTest(t, root, shuttle)

	_, err := e.Run(p, RunOpts{})
	if err == nil {
		t.Fatalf("Run() error = nil; want a materialization failure")
	}
	if !strings.Contains(err.Error(), "materialize instruction files") {
		t.Errorf("Run() error = %q; want it to name the materialization failure", err.Error())
	}
	if len(shuttle.started) != 0 {
		t.Errorf("started halves = %v; want none on a materialization failure", shuttle.started)
	}
}
