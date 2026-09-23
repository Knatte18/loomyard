package loomcli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// TestMustUseLLMDriverArm asserts the go driver value and an empty driver value both select the
// detached-spawn (go) arm, and the llm value selects the strand launch -- the two-value switch step
// 5 branches on, with the go driver as both the default and the zero-config answer.
func TestMustUseLLMDriverArm(t *testing.T) {
	tests := []struct {
		name   string
		driver string
		want   bool
	}{
		{"Go_SelectsDetachedSpawnArm", shedrun.DriverGo, false},
		{"Empty_SelectsDetachedSpawnArm", "", false},
		{"LLM_SelectsStrandLaunch", shedrun.DriverLLM, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mustUseLLMDriverArm(tt.driver); got != tt.want {
				t.Errorf("mustUseLLMDriverArm(%q) = %v; want %v", tt.driver, got, tt.want)
			}
		})
	}
}

// stubDriverHandle is a minimal driverHandle a fakeDriverStarter returns. ready, awaitErr and
// awaitCalls configure its AwaitStarted method; existing literals that set only guid/runDir keep
// compiling, since all three are zero-value-safe (ready false, awaitErr nil, awaitCalls nil).
type stubDriverHandle struct {
	guid       string
	runDir     string
	ready      bool
	awaitErr   error
	awaitCalls *int
}

func (h stubDriverHandle) StrandGUID() string { return h.guid }
func (h stubDriverHandle) RunDir() string     { return h.runDir }

// AwaitStarted implements driverHandle by returning the stub's configured ready/awaitErr pair,
// incrementing *awaitCalls first when non-nil so a test can assert how many times it was called.
func (h stubDriverHandle) AwaitStarted() (bool, error) {
	if h.awaitCalls != nil {
		*h.awaitCalls++
	}
	return h.ready, h.awaitErr
}

// fakeDriverStarter records the spec it was started with and returns a canned handle or error.
type fakeDriverStarter struct {
	gotSpec  shuttleengine.Spec
	called   bool
	handle   driverHandle
	startErr error
}

func (f *fakeDriverStarter) StartDriver(spec shuttleengine.Spec) (driverHandle, error) {
	f.called = true
	f.gotSpec = spec
	if f.startErr != nil {
		return nil, f.startErr
	}
	return f.handle, nil
}

// fakeDriverPaneProbeForArm is a driverPaneProbe stub recording whether/when RemoveDriverStrand was
// called, so tests can assert removal-before-start ordering.
type fakeDriverPaneProbeForArm struct {
	removeCalled bool
	removeGUID   string
	removeErr    error
	order        *[]string
}

func (f *fakeDriverPaneProbeForArm) Strands() ([]reedengine.StrandStatus, error) {
	return nil, nil
}

func (f *fakeDriverPaneProbeForArm) RemoveDriverStrand(guid string) error {
	f.removeCalled = true
	f.removeGUID = guid
	if f.order != nil {
		*f.order = append(*f.order, "remove")
	}
	return f.removeErr
}

// newTestLLMArmReceiver builds a *loomCLI bare enough to drive startLLMDriverArm: a real
// *lyxcwd.Location (a plain value, no filesystem behind it beyond t.TempDir()), a zero-value config
// (so loomengine.ResolveDriver defers to the engine default and never touches the registry), and the
// given fakes wired as the two driver seams.
func newTestLLMArmReceiver(t *testing.T, starter driverStarter, probe driverPaneProbe) *loomCLI {
	t.Helper()
	dir := t.TempDir()
	return &loomCLI{
		location:        &lyxcwd.Location{HubPath: dir, WorktreeName: "warp", AnchorRel: "."},
		cfg:             loomengine.Config{},
		registry:        modelspec.Registry{},
		driverStarter:   starter,
		driverPaneProbe: probe,
	}
}

// TestStartLLMDriverArm_ComposesAndStarts drives the llm arm through the two seams with a fake
// starter: no corpse to remove (driverStrandNone), and the started spec carries the composed prompt,
// report path, and the driver strand's own name.
func TestStartLLMDriverArm_ComposesAndStarts(t *testing.T) {
	starter := &fakeDriverStarter{handle: stubDriverHandle{guid: "g-new", runDir: "/run/dir"}}
	probe := &fakeDriverPaneProbeForArm{}
	c := newTestLLMArmReceiver(t, starter, probe)

	handle, err := c.startLLMDriverArm(driverStrandNone, "", "self")
	if err != nil {
		t.Fatalf("startLLMDriverArm() error = %v; want nil", err)
	}
	if !starter.called {
		t.Fatal("startLLMDriverArm() did not call the driverStarter seam")
	}
	if probe.removeCalled {
		t.Error("startLLMDriverArm() called RemoveDriverStrand with no corpse to remove (driverStrandNone); want it untouched")
	}
	if starter.gotSpec.NameOverride != driverStrandDisplayName {
		t.Errorf("started spec.NameOverride = %q; want %q", starter.gotSpec.NameOverride, driverStrandDisplayName)
	}
	if len(starter.gotSpec.OutputFiles) != 1 {
		t.Fatalf("started spec.OutputFiles = %v; want a single-entry slice", starter.gotSpec.OutputFiles)
	}
	if handle.StrandGUID() != "g-new" {
		t.Errorf("startLLMDriverArm() handle.StrandGUID() = %q; want %q", handle.StrandGUID(), "g-new")
	}
}

// TestStartLLMDriverArm_RemovesCorpseBeforeStart asserts the corpse removal happens before the run
// start when the strand action is dead: a relaunch that started first and removed second would leave
// two strands under one name, which reed's add has no upsert semantics to reconcile.
func TestStartLLMDriverArm_RemovesCorpseBeforeStart(t *testing.T) {
	var order []string
	probe := &fakeDriverPaneProbeForArm{order: &order}
	starter := &fakeDriverStarter{handle: stubDriverHandle{guid: "g-new", runDir: "/run/dir"}}
	// Wrap starter.StartDriver to also record ordering, via a thin adapter.
	c := newTestLLMArmReceiver(t, orderingStarter{starter: starter, order: &order}, probe)

	if _, err := c.startLLMDriverArm(driverStrandDead, "g-dead", "self"); err != nil {
		t.Fatalf("startLLMDriverArm() error = %v; want nil", err)
	}
	if !probe.removeCalled {
		t.Fatal("startLLMDriverArm() did not remove the dead driver strand")
	}
	if probe.removeGUID != "g-dead" {
		t.Errorf("RemoveDriverStrand called with guid %q; want %q", probe.removeGUID, "g-dead")
	}
	if len(order) != 2 || order[0] != "remove" || order[1] != "start" {
		t.Errorf("call order = %v; want [remove start]", order)
	}
}

// orderingStarter wraps a fakeDriverStarter and additionally appends to a shared order slice, so a
// test can assert relative ordering against a second seam's own recorded calls.
type orderingStarter struct {
	starter *fakeDriverStarter
	order   *[]string
}

func (o orderingStarter) StartDriver(spec shuttleengine.Spec) (driverHandle, error) {
	*o.order = append(*o.order, "start")
	return o.starter.StartDriver(spec)
}

// TestStartLLMDriverArm_NoCorpseRemovalWhenActionNone asserts the corpse removal does not happen at
// all when the strand action is none -- nothing to remove, and removing when there is no dead strand
// present is a bug in the other direction.
func TestStartLLMDriverArm_NoCorpseRemovalWhenActionNone(t *testing.T) {
	starter := &fakeDriverStarter{handle: stubDriverHandle{guid: "g-new", runDir: "/run/dir"}}
	probe := &fakeDriverPaneProbeForArm{}
	c := newTestLLMArmReceiver(t, starter, probe)

	if _, err := c.startLLMDriverArm(driverStrandNone, "", "self"); err != nil {
		t.Fatalf("startLLMDriverArm() error = %v; want nil", err)
	}
	if probe.removeCalled {
		t.Error("startLLMDriverArm() called RemoveDriverStrand when the strand action was none; want it untouched")
	}
}

// TestStartLLMDriverArm_StarterErrorPropagates asserts the starter seam's error propagates.
func TestStartLLMDriverArm_StarterErrorPropagates(t *testing.T) {
	wantErr := errors.New("start failed")
	starter := &fakeDriverStarter{startErr: wantErr}
	probe := &fakeDriverPaneProbeForArm{}
	c := newTestLLMArmReceiver(t, starter, probe)

	_, err := c.startLLMDriverArm(driverStrandNone, "", "self")
	if !errors.Is(err, wantErr) {
		t.Errorf("startLLMDriverArm() error = %v; want %v", err, wantErr)
	}
}

// TestStartLLMDriverArm_RemoveCorpseErrorPropagates asserts a corpse-removal failure propagates and
// never reaches the starter seam.
func TestStartLLMDriverArm_RemoveCorpseErrorPropagates(t *testing.T) {
	wantErr := errors.New("remove failed")
	starter := &fakeDriverStarter{handle: stubDriverHandle{guid: "g-new"}}
	probe := &fakeDriverPaneProbeForArm{removeErr: wantErr}
	c := newTestLLMArmReceiver(t, starter, probe)

	_, err := c.startLLMDriverArm(driverStrandDead, "g-dead", "self")
	if !errors.Is(err, wantErr) {
		t.Errorf("startLLMDriverArm() error = %v; want %v", err, wantErr)
	}
	if starter.called {
		t.Error("startLLMDriverArm() called the starter seam despite the corpse removal failing")
	}
}

// fakeDriverPaneProbeFull is a fully configurable driverPaneProbe stub for driving
// runDriverSpawnAndWait's llm-arm failure sites and success path.
type fakeDriverPaneProbeFull struct {
	strandsFn    func() ([]reedengine.StrandStatus, error)
	removeErr    error
	removeCalled bool
}

func (f *fakeDriverPaneProbeFull) Strands() ([]reedengine.StrandStatus, error) {
	return f.strandsFn()
}

func (f *fakeDriverPaneProbeFull) RemoveDriverStrand(guid string) error {
	f.removeCalled = true
	return f.removeErr
}

// noStrands is a fakeDriverPaneProbeFull strandsFn returning an empty, error-free slice -- the
// common "nothing tracked yet" answer most of the seven failure-site cases below need for every
// consumer except the one under test.
func noStrands() ([]reedengine.StrandStatus, error) { return nil, nil }

// newTestSpawnAndWaitReceiver builds a *loomCLI bare enough to drive runDriverSpawnAndWait, plus the
// real bootstrap lock path it should acquire and release against.
func newTestSpawnAndWaitReceiver(t *testing.T, starter driverStarter, probe driverPaneProbe) (*loomCLI, string) {
	t.Helper()
	dir := t.TempDir()
	worktreeDir := filepath.Join(dir, "warp")
	if err := os.MkdirAll(worktreeDir, 0o755); err != nil {
		t.Fatalf("mkdir worktree dir: %v", err)
	}
	// Deliberately NOT shedrun.ScratchDir(loc, "self") -- the mkdir-failure case below needs that
	// exact path free to occupy with a blocker file.
	runLockDir := filepath.Join(worktreeDir, ".lyx", "run-lock-dir")
	if err := os.MkdirAll(runLockDir, 0o755); err != nil {
		t.Fatalf("mkdir run lock dir: %v", err)
	}
	c := &loomCLI{
		location:        &lyxcwd.Location{HubPath: dir, WorktreeName: "warp", AnchorRel: "."},
		runID:           "self",
		cfg:             loomengine.Config{},
		registry:        modelspec.Registry{},
		driverStarter:   starter,
		driverPaneProbe: probe,
		shedPaths:       shedbuild.ShedPaths{LockPath: filepath.Join(runLockDir, "run.lock")},
	}
	bootstrapLockPath := filepath.Join(dir, "bootstrap.lock")
	return c, bootstrapLockPath
}

// acquireTestBootstrapLock acquires the bootstrap lock a test drives runDriverSpawnAndWait against.
func acquireTestBootstrapLock(t *testing.T, path string) *lock.FileLock {
	t.Helper()
	l, err := lock.AcquireWriteLock(path)
	if err != nil {
		t.Fatalf("acquire test bootstrap lock: %v", err)
	}
	return l
}

// assertBootstrapLockReleased asserts the bootstrap lock at path can be re-acquired -- the only
// observable proof, from outside the function under test, that it released the lock rather than
// leaking it. A leaked bootstrap lock wedges every subsequent start in that worktree and is invisible
// until the second invocation, which is why this assertion exists for each of the seven failure sites
// individually rather than being left to review.
func assertBootstrapLockReleased(t *testing.T, path string) {
	t.Helper()
	fl, ok, err := lock.TryAcquireWriteLock(path)
	if err != nil {
		t.Fatalf("re-acquire bootstrap lock at %q: %v", path, err)
	}
	if !ok {
		t.Errorf("bootstrap lock at %q was not released", path)
		return
	}
	_ = fl.Release()
}

// noopLockHeld is the go arm's handshake seam for every llm-arm failure-site test below: the llm arm
// never reaches it, so its return value is never observed, and it exists purely to satisfy
// runDriverSpawnAndWait's signature.
func noopLockHeld() (bool, error) { return false, nil }

// TestRunDriverSpawnAndWait_LLMArm_ReleasesLockOnDriverSettingsResolutionFailure covers failure site
// 1 of 7: the driver-settings resolution.
func TestRunDriverSpawnAndWait_LLMArm_ReleasesLockOnDriverSettingsResolutionFailure(t *testing.T) {
	starter := &fakeDriverStarter{}
	probe := &fakeDriverPaneProbeFull{strandsFn: noStrands}
	c, bootstrapLockPath := newTestSpawnAndWaitReceiver(t, starter, probe)
	c.cfg.Driver = "bad spec with a space" // modelspec.Parse rejects whitespace

	bootstrapLock := acquireTestBootstrapLock(t, bootstrapLockPath)
	ok := c.runDriverSpawnAndWait(context.Background(), &bytes.Buffer{}, shedrun.DriverLLM, bootstrapLock, noopLockHeld)

	if ok {
		t.Error("runDriverSpawnAndWait() = true; want false (driver-settings resolution must fail)")
	}
	if starter.called {
		t.Error("runDriverSpawnAndWait() reached the starter seam despite a driver-settings resolution failure")
	}
	assertBootstrapLockReleased(t, bootstrapLockPath)
}

// TestRunDriverSpawnAndWait_LLMArm_ReleasesLockOnStrandReadFailure covers failure site 2 of 7: the
// strand read.
func TestRunDriverSpawnAndWait_LLMArm_ReleasesLockOnStrandReadFailure(t *testing.T) {
	wantErr := errors.New("strand read failed")
	starter := &fakeDriverStarter{}
	probe := &fakeDriverPaneProbeFull{strandsFn: func() ([]reedengine.StrandStatus, error) { return nil, wantErr }}
	c, bootstrapLockPath := newTestSpawnAndWaitReceiver(t, starter, probe)

	bootstrapLock := acquireTestBootstrapLock(t, bootstrapLockPath)
	ok := c.runDriverSpawnAndWait(context.Background(), &bytes.Buffer{}, shedrun.DriverLLM, bootstrapLock, noopLockHeld)

	if ok {
		t.Error("runDriverSpawnAndWait() = true; want false (the strand read must fail)")
	}
	if starter.called {
		t.Error("runDriverSpawnAndWait() reached the starter seam despite a strand-read failure")
	}
	assertBootstrapLockReleased(t, bootstrapLockPath)
}

// TestRunDriverSpawnAndWait_LLMArm_ReleasesLockOnCorpseRemovalFailure covers failure site 3 of 7: the
// corpse removal. It also pins that the corpse removal happens before the run start when the strand
// action is dead: the starter seam must never be reached when the corpse removal itself fails.
func TestRunDriverSpawnAndWait_LLMArm_ReleasesLockOnCorpseRemovalFailure(t *testing.T) {
	wantErr := errors.New("remove failed")
	deadStrand := func() ([]reedengine.StrandStatus, error) {
		return []reedengine.StrandStatus{{GUID: "g-dead", Name: driverStrandDisplayName, Live: false}}, nil
	}
	starter := &fakeDriverStarter{}
	probe := &fakeDriverPaneProbeFull{strandsFn: deadStrand, removeErr: wantErr}
	c, bootstrapLockPath := newTestSpawnAndWaitReceiver(t, starter, probe)

	bootstrapLock := acquireTestBootstrapLock(t, bootstrapLockPath)
	ok := c.runDriverSpawnAndWait(context.Background(), &bytes.Buffer{}, shedrun.DriverLLM, bootstrapLock, noopLockHeld)

	if ok {
		t.Error("runDriverSpawnAndWait() = true; want false (the corpse removal must fail)")
	}
	if !probe.removeCalled {
		t.Error("runDriverSpawnAndWait() never attempted the corpse removal")
	}
	if starter.called {
		t.Error("runDriverSpawnAndWait() reached the starter seam despite the corpse removal failing -- a relaunch that starts before removing would leave two strands under one name")
	}
	assertBootstrapLockReleased(t, bootstrapLockPath)
}

// TestRunDriverSpawnAndWait_LLMArm_ReleasesLockOnReportDirMkdirFailure covers failure site 4 of 7:
// the report directory's mkdir-all. It forces the failure by pre-creating a plain FILE at the exact
// path the report's parent directory (shedrun.ScratchDir) must occupy, so os.MkdirAll there fails
// with "not a directory".
func TestRunDriverSpawnAndWait_LLMArm_ReleasesLockOnReportDirMkdirFailure(t *testing.T) {
	starter := &fakeDriverStarter{}
	probe := &fakeDriverPaneProbeFull{strandsFn: noStrands}
	c, bootstrapLockPath := newTestSpawnAndWaitReceiver(t, starter, probe)

	scratchDir := shedrun.ScratchDir(c.location, c.runID)
	if err := os.MkdirAll(filepath.Dir(scratchDir), 0o755); err != nil {
		t.Fatalf("mkdir scratch dir's parent: %v", err)
	}
	if err := os.WriteFile(scratchDir, []byte("blocker"), 0o644); err != nil {
		t.Fatalf("write blocker file at %q: %v", scratchDir, err)
	}

	bootstrapLock := acquireTestBootstrapLock(t, bootstrapLockPath)
	ok := c.runDriverSpawnAndWait(context.Background(), &bytes.Buffer{}, shedrun.DriverLLM, bootstrapLock, noopLockHeld)

	if ok {
		t.Error("runDriverSpawnAndWait() = true; want false (the report directory's mkdir-all must fail)")
	}
	if starter.called {
		t.Error("runDriverSpawnAndWait() reached the starter seam despite the report directory's mkdir-all failing")
	}
	assertBootstrapLockReleased(t, bootstrapLockPath)
}

// TestRunDriverSpawnAndWait_LLMArm_ReleasesLockOnRunStartFailure covers failure site 5 of 7: the run
// start.
func TestRunDriverSpawnAndWait_LLMArm_ReleasesLockOnRunStartFailure(t *testing.T) {
	wantErr := errors.New("start failed")
	starter := &fakeDriverStarter{startErr: wantErr}
	probe := &fakeDriverPaneProbeFull{strandsFn: noStrands}
	c, bootstrapLockPath := newTestSpawnAndWaitReceiver(t, starter, probe)

	bootstrapLock := acquireTestBootstrapLock(t, bootstrapLockPath)
	ok := c.runDriverSpawnAndWait(context.Background(), &bytes.Buffer{}, shedrun.DriverLLM, bootstrapLock, noopLockHeld)

	if ok {
		t.Error("runDriverSpawnAndWait() = true; want false (the run start must fail)")
	}
	if !starter.called {
		t.Error("runDriverSpawnAndWait() never reached the starter seam")
	}
	assertBootstrapLockReleased(t, bootstrapLockPath)
}

// TestRunDriverSpawnAndWait_LLMArm_ReleasesLockOnReadinessRefusing covers failure site 6 of 7: the
// readiness await reporting not-ready. The handle's AwaitStarted returns (false, nil), which is the
// pane-died-or-startup-timed-out answer; this no longer spends a real ~5s budget the way the old
// pane-liveness probe's own bounded poll did, since the handle is a fake under this test's direct
// control.
func TestRunDriverSpawnAndWait_LLMArm_ReleasesLockOnReadinessRefusing(t *testing.T) {
	awaitCalls := 0
	starter := &fakeDriverStarter{handle: stubDriverHandle{guid: "g-run", runDir: "/run/dir", ready: false, awaitCalls: &awaitCalls}}
	probe := &fakeDriverPaneProbeFull{strandsFn: noStrands}
	c, bootstrapLockPath := newTestSpawnAndWaitReceiver(t, starter, probe)

	bootstrapLock := acquireTestBootstrapLock(t, bootstrapLockPath)
	var out bytes.Buffer
	ok := c.runDriverSpawnAndWait(context.Background(), &out, shedrun.DriverLLM, bootstrapLock, noopLockHeld)

	if ok {
		t.Error("runDriverSpawnAndWait() = true; want false (the readiness await must refuse)")
	}
	if awaitCalls != 1 {
		t.Errorf("AwaitStarted was called %d time(s); want 1", awaitCalls)
	}
	for _, want := range []string{"driver strand did not come up", "/run/dir", "g-run"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("envelope output = %q; want it to contain %q", out.String(), want)
		}
	}
	assertBootstrapLockReleased(t, bootstrapLockPath)
}

// TestRunDriverSpawnAndWait_LLMArm_ReleasesLockOnReadinessError covers failure site 7 of 7: the
// readiness await erroring, which AwaitStarted's own contract reserves for reed's liveness check
// failing repeatedly -- a case the old pane-liveness probe had no equivalent for, since it never
// returned an error on its own polling loop.
func TestRunDriverSpawnAndWait_LLMArm_ReleasesLockOnReadinessError(t *testing.T) {
	wantErr := errors.New("liveness check failed")
	starter := &fakeDriverStarter{handle: stubDriverHandle{guid: "g-run", runDir: "/run/dir", awaitErr: wantErr}}
	probe := &fakeDriverPaneProbeFull{strandsFn: noStrands}
	c, bootstrapLockPath := newTestSpawnAndWaitReceiver(t, starter, probe)

	bootstrapLock := acquireTestBootstrapLock(t, bootstrapLockPath)
	var out bytes.Buffer
	ok := c.runDriverSpawnAndWait(context.Background(), &out, shedrun.DriverLLM, bootstrapLock, noopLockHeld)

	if ok {
		t.Error("runDriverSpawnAndWait() = true; want false (the readiness await must error)")
	}
	if !strings.Contains(out.String(), wantErr.Error()) {
		t.Errorf("envelope output = %q; want it to contain %q", out.String(), wantErr.Error())
	}
	assertBootstrapLockReleased(t, bootstrapLockPath)
}

// TestRunDriverSpawnAndWait_LLMArm_NeverConsultsTheRunLockHandshake asserts the run-lock handshake is
// not reached on the llm arm: the test's own lock-held seam carries a counter, and a successful
// llm-arm run through this function must leave that counter at zero. This is the assertion pinning
// the deliberate asymmetry between the two arms -- a later refactor that "unifies" them into a shared
// helper reaching the handshake fails here rather than silently reintroducing the race the handshake
// decision exists to avoid.
func TestRunDriverSpawnAndWait_LLMArm_NeverConsultsTheRunLockHandshake(t *testing.T) {
	awaitCalls := 0
	starter := &fakeDriverStarter{handle: stubDriverHandle{guid: "g-run", runDir: "/run/dir", ready: true, awaitCalls: &awaitCalls}}
	probe := &fakeDriverPaneProbeFull{strandsFn: noStrands}
	c, bootstrapLockPath := newTestSpawnAndWaitReceiver(t, starter, probe)

	lockHeldCalls := 0
	countingLockHeld := func() (bool, error) {
		lockHeldCalls++
		return true, nil
	}

	bootstrapLock := acquireTestBootstrapLock(t, bootstrapLockPath)
	ok := c.runDriverSpawnAndWait(context.Background(), &bytes.Buffer{}, shedrun.DriverLLM, bootstrapLock, countingLockHeld)

	if !ok {
		t.Fatal("runDriverSpawnAndWait() = false; want true (this scenario is a clean success)")
	}
	if lockHeldCalls != 0 {
		t.Errorf("the go arm's lockHeld seam was consulted %d time(s) on an llm-seeded bootstrap; want 0", lockHeldCalls)
	}
	if awaitCalls != 1 {
		t.Errorf("AwaitStarted was called %d time(s); want 1", awaitCalls)
	}
	_ = bootstrapLock.Release()
}
