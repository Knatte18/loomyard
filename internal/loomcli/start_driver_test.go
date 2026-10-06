package loomcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestMustUseLLMDriverArm asserts the go driver value and an empty driver value both select the
// detached-spawn (go) arm, and the llm value selects the strand launch -- the two-value switch step
// 5 branches on, with the go driver as both the default and the zero-config answer.
//
//testtiming:keep pins the go and empty driver values selecting the detached-spawn arm and the llm value the strand launch; its covering test runs the llm arm without asserting the switch
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

// stubDriverHandle is a minimal driverHandle a fakeDriverStarter returns.
type stubDriverHandle struct {
	guid   string
	runDir string
}

func (h stubDriverHandle) StrandGUID() string { return h.guid }
func (h stubDriverHandle) RunDir() string     { return h.runDir }

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
	stencilsDir := filepath.Join(dir, "stencils")
	stencilkit.SeedInto(t, stencilsDir)
	return &loomCLI{
		location:        &lyxcwd.Location{HubPath: dir, WorktreeName: "pair", AnchorRel: "."},
		runDeps:         websterengine.RunDeps{Geom: websterengine.Geometry{StencilsDir: stencilsDir}},
		parentName:      "hub:orch",
		cfg:             loomengine.Config{},
		registry:        modelspec.Registry{},
		driverStarter:   starter,
		driverPaneProbe: probe,
	}
}

// TestStartLLMDriverArm drives the llm arm through the two seams with a fake starter and probe.
//
//testtiming:keep pins the llm arm composing the started spec and addressing the run by slug, removing a dead corpse before the start, and propagating a starter or corpse-removal error without reaching the starter; the covering tests drive the arm through the whole bootstrap without asserting the spec
func TestStartLLMDriverArm(t *testing.T) {
	// With no corpse to remove (driverStrandNone) the started spec carries the composed prompt, report
	// path and the driver strand's own name; the literal "self" never reaches the prompt, because the
	// arm resolves it to the worktree slug for both the prompt and the report path.
	t.Run("no corpse composes the spec, addresses the run by slug and starts", func(t *testing.T) {
		starter := &fakeDriverStarter{handle: stubDriverHandle{guid: "g-new", runDir: "/run/dir"}}
		probe := &fakeDriverPaneProbeForArm{}
		c := newTestLLMArmReceiver(t, starter, probe)

		handle, err := c.startLLMDriverArm(driverStrandNone, "", shedrun.SelfRunID)
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
		prompt := starter.gotSpec.Prompt
		if !strings.Contains(prompt, "run `pair`") {
			t.Errorf("prompt = %q; want it to name the slug run-id \"pair\"", prompt)
		}
		if strings.Contains(prompt, `"self"`) || strings.Contains(prompt, string(filepath.Separator)+"self"+string(filepath.Separator)) {
			t.Errorf("prompt = %q; want no literal self run-id or report path segment", prompt)
		}
		if !strings.Contains(prompt, "lyx reed remove --name driver --detach") {
			t.Errorf("prompt = %q; want the teardown command", prompt)
		}
	})

	// A relaunch that started first and removed second would leave two strands under one name,
	// which reed's add has no upsert semantics to reconcile.
	t.Run("a dead corpse is removed before the start", func(t *testing.T) {
		var order []string
		probe := &fakeDriverPaneProbeForArm{order: &order}
		starter := &fakeDriverStarter{handle: stubDriverHandle{guid: "g-new", runDir: "/run/dir"}}
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
	})

	t.Run("the starter error propagates", func(t *testing.T) {
		wantErr := errors.New("start failed")
		c := newTestLLMArmReceiver(t, &fakeDriverStarter{startErr: wantErr}, &fakeDriverPaneProbeForArm{})

		_, err := c.startLLMDriverArm(driverStrandNone, "", "self")
		if !errors.Is(err, wantErr) {
			t.Errorf("startLLMDriverArm() error = %v; want %v", err, wantErr)
		}
	})

	t.Run("a corpse-removal error propagates and never reaches the starter", func(t *testing.T) {
		wantErr := errors.New("remove failed")
		starter := &fakeDriverStarter{handle: stubDriverHandle{guid: "g-new"}}
		c := newTestLLMArmReceiver(t, starter, &fakeDriverPaneProbeForArm{removeErr: wantErr})

		_, err := c.startLLMDriverArm(driverStrandDead, "g-dead", "self")
		if !errors.Is(err, wantErr) {
			t.Errorf("startLLMDriverArm() error = %v; want %v", err, wantErr)
		}
		if starter.called {
			t.Error("startLLMDriverArm() called the starter seam despite the corpse removal failing")
		}
	})
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

// fakeDriverPaneProbeFull is a fully configurable driverPaneProbe stub for driving
// runDriverSpawnAndWait's llm-arm failure sites and success path.
type fakeDriverPaneProbeFull struct {
	strandsFn    func() ([]reedengine.StrandStatus, error)
	removeErr    error
	removeCalled bool
	removeGUID   string
	order        *[]string
}

func (f *fakeDriverPaneProbeFull) Strands() ([]reedengine.StrandStatus, error) {
	return f.strandsFn()
}

func (f *fakeDriverPaneProbeFull) RemoveDriverStrand(guid string) error {
	f.removeCalled = true
	f.removeGUID = guid
	if f.order != nil {
		*f.order = append(*f.order, "remove")
	}
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
	worktreeDir := filepath.Join(dir, "pair")
	if err := os.MkdirAll(worktreeDir, 0o755); err != nil {
		t.Fatalf("mkdir worktree dir: %v", err)
	}
	// Deliberately NOT shedrun.ScratchDir(loc, "self") -- the mkdir-failure case below needs that
	// exact path free to occupy with a blocker file.
	runLockDir := filepath.Join(worktreeDir, ".lyx", "run-lock-dir")
	if err := os.MkdirAll(runLockDir, 0o755); err != nil {
		t.Fatalf("mkdir run lock dir: %v", err)
	}
	stencilsDir := filepath.Join(dir, "stencils")
	stencilkit.SeedInto(t, stencilsDir)
	c := &loomCLI{
		location:        &lyxcwd.Location{HubPath: dir, WorktreeName: "pair", AnchorRel: "."},
		runDeps:         websterengine.RunDeps{Geom: websterengine.Geometry{StencilsDir: stencilsDir}},
		runID:           "self",
		cfg:             loomengine.Config{},
		registry:        modelspec.Registry{},
		driverStarter:   starter,
		driverPaneProbe: probe,
		midMerge: func(*lyxcwd.Location) (fabricengine.MidMergeState, error) {
			return fabricengine.MidMergeState{Kind: fabricengine.MidMergeNone}, nil
		},
		shedPaths: shedbuild.ShedPaths{
			LockPath:       filepath.Join(runLockDir, "run.lock"),
			StatusPath:     filepath.Join(runLockDir, "status.json"),
			StatusLockPath: filepath.Join(runLockDir, "status.json.lock"),
		},
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

// TestRunDriverSpawnAndWait_LLMArm_ReleasesLockOnFailure asserts every failure site of the llm arm
// returns false and releases the bootstrap lock: a leaked lock wedges every later start in the
// worktree and is invisible until the second invocation, so each site is checked individually.
func TestRunDriverSpawnAndWait_LLMArm_ReleasesLockOnFailure(t *testing.T) {
	deadStrand := func() ([]reedengine.StrandStatus, error) {
		return []reedengine.StrandStatus{{GUID: "g-dead", Name: driverStrandDisplayName, Live: false}}, nil
	}
	tests := []struct {
		name string
		// strands, removeErr and startErr configure the probe and starter seams; a nil strands answers no strands.
		strands   func() ([]reedengine.StrandStatus, error)
		removeErr error
		startErr  error
		// mutate breaks the receiver, or the disk it works on, before the call.
		mutate            func(t *testing.T, c *loomCLI)
		wantRemoveCalled  bool
		wantStarterCalled bool
		wantOutput        string
	}{
		{
			// modelspec.Parse rejects whitespace.
			name:   "driver settings resolution",
			mutate: func(_ *testing.T, c *loomCLI) { c.cfg.Driver = "bad spec with a space" },
		},
		{
			name:    "strand read",
			strands: func() ([]reedengine.StrandStatus, error) { return nil, errors.New("strand read failed") },
		},
		{
			// The corpse removal happens before the run start when the strand action is dead, so the
			// starter must never be reached when the removal itself fails.
			name:             "corpse removal",
			strands:          deadStrand,
			removeErr:        errors.New("remove failed"),
			wantRemoveCalled: true,
		},
		{
			// A plain FILE at the path the report's parent directory (shedrun.DriveReportsDir) must
			// occupy makes os.MkdirAll there fail with "not a directory".
			name: "report directory mkdir",
			mutate: func(t *testing.T, c *loomCLI) {
				scratchDir := shedrun.DriveReportsDir(c.location, c.runID)
				if err := os.MkdirAll(filepath.Dir(scratchDir), 0o755); err != nil {
					t.Fatalf("mkdir scratch dir's parent: %v", err)
				}
				if err := os.WriteFile(scratchDir, []byte("blocker"), 0o644); err != nil {
					t.Fatalf("write blocker file at %q: %v", scratchDir, err)
				}
			},
		},
		{
			name:              "run start",
			startErr:          errors.New("start failed"),
			wantStarterCalled: true,
		},
		{
			// StartDriver blocks through shuttle's startup probe and returns the not-ready error
			// directly, so a not-ready provider surfaces as an ordinary starter-seam error that the
			// failed-start refusal reports on the envelope unchanged.
			name:              "not-ready start",
			startErr:          errors.New("shuttle: start: the provider never became ready (run dir /run/dir, strand g-run)"),
			wantStarterCalled: true,
			wantOutput:        "shuttle: start: the provider never became ready (run dir /run/dir, strand g-run)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strands := tt.strands
			if strands == nil {
				strands = noStrands
			}
			starter := &fakeDriverStarter{startErr: tt.startErr}
			probe := &fakeDriverPaneProbeFull{strandsFn: strands, removeErr: tt.removeErr}
			c, bootstrapLockPath := newTestSpawnAndWaitReceiver(t, starter, probe)
			if tt.mutate != nil {
				tt.mutate(t, c)
			}

			bootstrapLock := acquireTestBootstrapLock(t, bootstrapLockPath)
			var out bytes.Buffer
			ok := c.runDriverSpawnAndWait(context.Background(), &out, shedrun.DriverLLM, bootstrapLock, noopLockHeld)

			if ok {
				t.Error("runDriverSpawnAndWait() = true; want false")
			}
			if probe.removeCalled != tt.wantRemoveCalled {
				t.Errorf("corpse removal attempted = %v; want %v", probe.removeCalled, tt.wantRemoveCalled)
			}
			if starter.called != tt.wantStarterCalled {
				t.Errorf("starter seam reached = %v; want %v", starter.called, tt.wantStarterCalled)
			}
			if !strings.Contains(out.String(), tt.wantOutput) {
				t.Errorf("envelope output = %q; want it to contain %q", out.String(), tt.wantOutput)
			}
			assertBootstrapLockReleased(t, bootstrapLockPath)
		})
	}
}

// TestRunDriverSpawnAndWait_LiveRetiringDriverIsRemovedThenReplaced pins that a live driver strand marked retiring is removed by guid before a fresh driver starts, rather than adopted.
func TestRunDriverSpawnAndWait_LiveRetiringDriverIsRemovedThenReplaced(t *testing.T) {
	var order []string
	retiring := func() ([]reedengine.StrandStatus, error) {
		return []reedengine.StrandStatus{{GUID: "g-retiring", Name: driverStrandDisplayName, PaneID: "%0", Live: true, Retiring: true}}, nil
	}
	probe := &fakeDriverPaneProbeFull{strandsFn: retiring, order: &order}
	starter := &fakeDriverStarter{handle: stubDriverHandle{guid: "g-new", runDir: "/run/dir"}}
	c, bootstrapLockPath := newTestSpawnAndWaitReceiver(t, orderingStarter{starter: starter, order: &order}, probe)

	bootstrapLock := acquireTestBootstrapLock(t, bootstrapLockPath)
	ok := c.runDriverSpawnAndWait(context.Background(), &bytes.Buffer{}, shedrun.DriverLLM, bootstrapLock, noopLockHeld)
	defer func() { _ = bootstrapLock.Release() }()

	if !ok {
		t.Fatal("runDriverSpawnAndWait() = false; want true")
	}
	if probe.removeGUID != "g-retiring" {
		t.Errorf("RemoveDriverStrand called with guid %q; want %q", probe.removeGUID, "g-retiring")
	}
	if len(order) != 2 || order[0] != "remove" || order[1] != "start" {
		t.Errorf("call order = %v; want [remove start]", order)
	}
}

// TestRunDriverSpawnAndWait_LLMArm_NeverConsultsTheRunLockHandshake asserts the run-lock handshake is
// not reached on the llm arm: the test's own lock-held seam carries a counter, and a successful
// llm-arm run through this function must leave that counter at zero. This is the assertion pinning
// the deliberate asymmetry between the two arms -- a later refactor that "unifies" them into a shared
// helper reaching the handshake fails here rather than silently reintroducing the race the handshake
// decision exists to avoid.
//
//testtiming:keep pins the llm arm never consulting the go arm's run-lock handshake, the deliberate asymmetry between the arms that a later unifying refactor would break; the covering test counts no handshake calls
func TestRunDriverSpawnAndWait_LLMArm_NeverConsultsTheRunLockHandshake(t *testing.T) {
	starter := &fakeDriverStarter{handle: stubDriverHandle{guid: "g-run", runDir: "/run/dir"}}
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
	_ = bootstrapLock.Release()
}

// parkedStrands answers a table holding one driver strand, live or dead.
func parkedStrands(live bool) func() ([]reedengine.StrandStatus, error) {
	return func() ([]reedengine.StrandStatus, error) {
		return []reedengine.StrandStatus{{GUID: "g-drv", Name: driverStrandDisplayName, Live: live}}, nil
	}
}

// newResumeBranchReceiver builds a runDriverSpawnAndWait receiver carrying a sender, a counting wait and a park marker on disk.
func newResumeBranchReceiver(t *testing.T, starter driverStarter, probe driverPaneProbe, sender *fakeDriverSender) (*loomCLI, string, string) {
	t.Helper()
	c, bootstrapLockPath := newTestSpawnAndWaitReceiver(t, starter, probe)
	c.driverSender = sender
	c.driverResumeWait = func() {}
	marker := shedrun.ParkMarker(c.location, shedrun.ResolveRunID(c.location, c.runID))
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		t.Fatalf("mkdir marker dir: %v", err)
	}
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	return c, bootstrapLockPath, marker
}

// voucherOnDisk reads the handoff voucher the receiver's location names, reporting whether one exists.
func voucherOnDisk(t *testing.T, c *loomCLI) (handoffVoucher, bool) {
	t.Helper()
	if _, err := os.Stat(loomengine.LoomHandoffVoucher(c.location)); errors.Is(err, os.ErrNotExist) {
		return handoffVoucher{}, false
	}
	voucher, found, err := state.ReadJSONStrict[handoffVoucher](loomengine.LoomHandoffVoucher(c.location), loomengine.LoomHandoffVoucherLock(c.location))
	if err != nil {
		t.Fatalf("read handoff voucher: %v", err)
	}
	return voucher, found
}

func TestRunDriverSpawnAndWait_SpawnVouchesForTheResume(t *testing.T) {
	tests := []struct {
		name        string
		tierTwoOn   bool
		priorVouch  bool
		wantVoucher bool
	}{
		{"tier 2 on writes the voucher", true, false, true},
		{"tier 2 on replaces an earlier voucher", true, true, true},
		{"tier 2 off writes none", false, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			starter := &fakeDriverStarter{handle: stubDriverHandle{guid: "g-new", runDir: "/run/dir"}}
			c, lockPath := newTestSpawnAndWaitReceiver(t, starter, &fakeDriverPaneProbeFull{strandsFn: noStrands})
			if tc.tierTwoOn {
				c.frictionDir = t.TempDir()
			}
			history := []shedengine.HistoryEntry{{Producer: "plan"}, {Producer: "implement"}}
			if err := state.WriteJSON(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, shedengine.Status{State: shedengine.StateRunning, History: history}); err != nil {
				t.Fatalf("write status: %v", err)
			}
			if tc.priorVouch {
				recordHandoffVoucher(loomengine.LoomHandoffVoucher(c.location), loomengine.LoomHandoffVoucherLock(c.location), 99, shedengine.StateFailed)
			}

			bootstrapLock := acquireTestBootstrapLock(t, lockPath)
			ok := c.runDriverSpawnAndWait(context.Background(), &bytes.Buffer{}, shedrun.DriverLLM, bootstrapLock, noopLockHeld)
			_ = bootstrapLock.Release()

			if !ok || !starter.called {
				t.Fatalf("runDriverSpawnAndWait() = %v, starter called = %v; want a spawn", ok, starter.called)
			}
			voucher, found := voucherOnDisk(t, c)
			if found != tc.wantVoucher {
				t.Fatalf("voucher present = %v; want %v", found, tc.wantVoucher)
			}
			if tc.wantVoucher && (voucher.HistoryLength != len(history) || voucher.State != string(shedengine.StateRunning)) {
				t.Errorf("voucher = %+v; want history length %d, state %q", voucher, len(history), shedengine.StateRunning)
			}
		})
	}
}

//testtiming:keep pins a live parked driver being resumed with one line, its marker removed, no handoff voucher written and no refusal recorded, without spawning; the covering tests assert the refusal and spawn paths
func TestRunDriverSpawnAndWait_LiveParkedDriverIsResumedNotSpawned(t *testing.T) {
	starter := &fakeDriverStarter{}
	sender := &fakeDriverSender{}
	c, lockPath, marker := newResumeBranchReceiver(t, starter, &fakeDriverPaneProbeFull{strandsFn: parkedStrands(true)}, sender)
	c.frictionDir = t.TempDir()
	writeTestRunState(t, c, shedengine.StateAwaiting)

	var out bytes.Buffer
	bootstrapLock := acquireTestBootstrapLock(t, lockPath)
	ok := c.runDriverSpawnAndWait(context.Background(), &out, shedrun.DriverLLM, bootstrapLock, noopLockHeld)

	if !ok {
		t.Fatalf("runDriverSpawnAndWait() = false; want true (output %q)", out.String())
	}
	if len(sender.texts) != 1 {
		t.Errorf("SendDriver calls = %d; want 1", len(sender.texts))
	}
	if _, found := voucherOnDisk(t, c); found {
		t.Error("a parked-driver resume wrote a handoff voucher")
	}
	if starter.called {
		t.Error("the starter was called on a live parked driver")
	}
	if markerExists(marker) {
		t.Error("park marker still on disk after the resume")
	}
	if out.Len() != 0 {
		t.Errorf("a refusal was recorded: %q", out.String())
	}
	_ = bootstrapLock.Release()
}

// writeTestRunState persists a status file in state st at the receiver's status path.
func writeTestRunState(t *testing.T, c *loomCLI, st shedengine.State) {
	t.Helper()
	if err := state.WriteJSON(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, shedengine.Status{State: st}); err != nil {
		t.Fatalf("write status: %v", err)
	}
}

// TestRunDriverSpawnAndWait_LiveDriverWithoutParkMarker asserts a live driver strand without a park marker, and without the retiring mark, is neither removed, replaced nor sent to:
// over a run with no status or a running run the call is a no-op that succeeds, and over a run handed back to the driver it refuses retryably until the driver parks.
func TestRunDriverSpawnAndWait_LiveDriverWithoutParkMarker(t *testing.T) {
	tests := []struct {
		name string
		// runState is the persisted run state; empty writes no status file.
		runState    shedengine.State
		wantRefusal bool
	}{
		{name: "no status file"},
		{name: "running run is a no-op", runState: shedengine.StateRunning},
		{name: "awaiting run refuses retryably", runState: shedengine.StateAwaiting, wantRefusal: true},
		{name: "blocked run refuses retryably", runState: shedengine.StateBlocked, wantRefusal: true},
		{name: "paused run refuses retryably", runState: shedengine.StatePaused, wantRefusal: true},
		{name: "failed run refuses retryably", runState: shedengine.StateFailed, wantRefusal: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			starter := &fakeDriverStarter{}
			sender := &fakeDriverSender{}
			probe := &fakeDriverPaneProbeFull{strandsFn: parkedStrands(true)}
			c, lockPath, marker := newResumeBranchReceiver(t, starter, probe, sender)
			if err := os.Remove(marker); err != nil {
				t.Fatalf("remove marker: %v", err)
			}
			if tt.runState != "" {
				c.frictionDir = t.TempDir()
				writeTestRunState(t, c, tt.runState)
			}

			var out bytes.Buffer
			bootstrapLock := acquireTestBootstrapLock(t, lockPath)
			ok := c.runDriverSpawnAndWait(context.Background(), &out, shedrun.DriverLLM, bootstrapLock, noopLockHeld)

			if len(sender.texts) != 0 || starter.called || probe.removeCalled {
				t.Errorf("sent %d line(s), starter called = %v, strand removed = %v; want none", len(sender.texts), starter.called, probe.removeCalled)
			}
			if _, found := voucherOnDisk(t, c); found {
				t.Error("the live-strand call wrote a handoff voucher")
			}
			if !tt.wantRefusal {
				if !ok || out.Len() != 0 {
					t.Errorf("runDriverSpawnAndWait() = %v, output %q; want true and no output", ok, out.String())
				}
				_ = bootstrapLock.Release()
				return
			}
			if ok {
				t.Fatal("runDriverSpawnAndWait() = true; want a refusal while the driver has not parked")
			}
			var envelope struct {
				OK    bool   `json:"ok"`
				Kind  string `json:"kind"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("envelope %q: %v", out.String(), err)
			}
			if envelope.OK || envelope.Kind != shedrun.StartNotParkedKind {
				t.Errorf("envelope = %+v; want ok=false kind=%q", envelope, shedrun.StartNotParkedKind)
			}
			for _, want := range []string{"not parked yet", "lyx loom start"} {
				if !strings.Contains(envelope.Error, want) {
					t.Errorf("error = %q; want substring %q", envelope.Error, want)
				}
			}
			assertBootstrapLockReleased(t, lockPath)
		})
	}
}

//testtiming:keep pins a dead driver strand with a stale park marker removing the marker and spawning a fresh driver with no resume line sent; the covering tests neither park a marker nor assert its removal
func TestRunDriverSpawnAndWait_DeadDriverWithStaleMarkerRemovesItAndSpawns(t *testing.T) {
	starter := &fakeDriverStarter{handle: stubDriverHandle{guid: "g-new", runDir: "/run/dir"}}
	sender := &fakeDriverSender{}
	c, lockPath, marker := newResumeBranchReceiver(t, starter, &fakeDriverPaneProbeFull{strandsFn: parkedStrands(false)}, sender)

	bootstrapLock := acquireTestBootstrapLock(t, lockPath)
	ok := c.runDriverSpawnAndWait(context.Background(), &bytes.Buffer{}, shedrun.DriverLLM, bootstrapLock, noopLockHeld)

	if !ok {
		t.Fatal("runDriverSpawnAndWait() = false; want true")
	}
	if markerExists(marker) {
		t.Error("stale park marker survived a fresh spawn")
	}
	if !starter.called || len(sender.texts) != 0 {
		t.Errorf("starter called = %v, sent %d line(s); want a spawn and no send", starter.called, len(sender.texts))
	}
	_ = bootstrapLock.Release()
}

func TestRunDriverSpawnAndWait_ResumeNeverReadyRefusesAndKeepsMarker(t *testing.T) {
	starter := &fakeDriverStarter{}
	sender := &fakeDriverSender{repeatErr: notReady()}
	c, lockPath, marker := newResumeBranchReceiver(t, starter, &fakeDriverPaneProbeFull{strandsFn: parkedStrands(true)}, sender)

	var out bytes.Buffer
	bootstrapLock := acquireTestBootstrapLock(t, lockPath)
	ok := c.runDriverSpawnAndWait(context.Background(), &out, shedrun.DriverLLM, bootstrapLock, noopLockHeld)

	if ok {
		t.Error("runDriverSpawnAndWait() = true; want a refusal")
	}
	if !markerExists(marker) {
		t.Error("park marker was removed despite the failed resume")
	}
	if starter.called {
		t.Error("the starter was called despite a live driver")
	}
	if !strings.Contains(out.String(), "lyx loom start") {
		t.Errorf("envelope %q does not name the retry", out.String())
	}
	assertBootstrapLockReleased(t, lockPath)
}
