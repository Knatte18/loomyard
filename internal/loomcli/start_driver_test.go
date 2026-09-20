package loomcli

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/reedengine"
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
