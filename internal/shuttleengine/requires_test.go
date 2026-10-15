// requires_test.go covers Spec.Requires: the refusals at Start and at attach, and the runs that pass the check.

package shuttleengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
)

// supportingEngine is a fakeEngine plus the RequirementSupporter capability over a scripted requirement set.
type supportingEngine struct {
	*fakeEngine

	supported []Requirement
}

func (e supportingEngine) Supports(req Requirement) bool { return slices.Contains(e.supported, req) }

// requiresRig is a Runner over a set holding "full" (the default, meeting every requirement), "wake" (meeting background-wake only) and "plain" (no RequirementSupporter).
type requiresRig struct {
	t    *testing.T
	fx   *fixture
	reed *fakeReed
}

func newRequiresRig(t *testing.T) *requiresRig {
	t.Helper()
	full := supportingEngine{fakeEngine: &fakeEngine{}, supported: []Requirement{RequirementForks, RequirementPeerMessaging, RequirementBackgroundWake}}
	wake := supportingEngine{fakeEngine: &fakeEngine{}, supported: []Requirement{RequirementBackgroundWake}}
	plain := &fakeEngine{}
	engines := NewEngineSet("full", map[string]Engine{"full": full, "wake": wake, "plain": plain})
	reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}, CaptureQueue: []string{"IDLE"}}
	for _, engine := range []*fakeEngine{full.fakeEngine, wake.fakeEngine, plain} {
		readyStart(reed, engine)
	}
	fx := newFixture(t, reed, full, withEngines(engines), withConfig(fastConfig), withSeparateRunDir())
	return &requiresRig{t: t, fx: fx, reed: reed}
}

// start starts a run of spec on the rig's runner.
func (r *requiresRig) start(spec Spec) (*Run, error) {
	spec.Prompt = "x"
	spec.OutputFiles = []string{"out.md"}
	return r.fx.Runner.Start(spec)
}

// startRecord starts a plain-engine run without requirements and rewrites its record to carry forkSubagents.
func (r *requiresRig) startRecord(forkSubagents bool) (runDir string) {
	r.t.Helper()
	if _, err := r.start(Spec{Engine: "plain"}); err != nil {
		r.t.Fatalf("Start(plain) error: %v", err)
	}
	state, runDir, err := FindRun(r.fx.Runner.cfg, r.fx.Anchor, "strand-1")
	if err != nil {
		r.t.Fatalf("FindRun(strand-1): %v", err)
	}
	state.ForkSubagents = forkSubagents
	if err := saveRunState(runDir, state); err != nil {
		r.t.Fatalf("saveRunState: %v", err)
	}
	seedPresentReedState(r.t, r.fx.DotLyx)
	return runDir
}

func (r *requiresRig) outputFile() string { return filepath.Join(r.fx.Worktree, "out.md") }

func TestRunner_Requires(t *testing.T) {
	t.Parallel()

	startWayForward := "way forward: give the role a model alias of an engine that meets them"
	startRefusals := []struct {
		name string
		spec Spec
		want []string
	}{
		{"an unknown requirement", Spec{Requires: []Requirement{"telepathy"}},
			[]string{`shuttle: start: unknown requirement "telepathy"; known requirements: forks, peer-messaging, background-wake`}},
		{"a requirement the engine does not meet names the engine, the requirement and the meeting engine", Spec{Engine: "wake", Requires: []Requirement{RequirementBackgroundWake, RequirementPeerMessaging}},
			[]string{`engine "wake"`, "peer-messaging", "engines that meet them all: full", startWayForward}},
		{"a ForkSubagents spec on an engine without forks", Spec{Engine: "wake", ForkSubagents: true},
			[]string{`engine "wake"`, "forks", "engines that meet them all: full", startWayForward}},
		{"an engine without the capability meets nothing", Spec{Engine: "plain", Requires: []Requirement{RequirementBackgroundWake}},
			[]string{`engine "plain"`, "background-wake", "engines that meet them all: full, wake", startWayForward}},
	}
	for _, tt := range startRefusals {
		t.Run("Start_refuses_"+tt.name, func(t *testing.T) {
			t.Parallel()
			rig := newRequiresRig(t)
			_, err := rig.start(tt.spec)
			if !errors.Is(err, ErrRequirementsUnmet) {
				t.Fatalf("Start() error = %v; want it to wrap ErrRequirementsUnmet", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Start() error = %q; want it to contain %q", err, want)
				}
			}
			if len(rig.reed.AddStrandCalls) != 0 {
				t.Errorf("AddStrand calls = %d; want none for a refused start", len(rig.reed.AddStrandCalls))
			}
		})
	}

	t.Run("Start_names_none_when_no_registered_engine_meets_the_requirement", func(t *testing.T) {
		t.Parallel()
		rig := newRequiresRig(t)
		noneMeets := NewEngineSet("wake", map[string]Engine{"wake": supportingEngine{fakeEngine: &fakeEngine{}, supported: []Requirement{RequirementBackgroundWake}}})
		runner := NewRunner(rig.reed, noneMeets, rig.fx.Anchor, rig.fx.Worktree, rig.fx.Runner.cfg)
		_, err := runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}, Requires: []Requirement{RequirementForks}})
		if !errors.Is(err, ErrRequirementsUnmet) || !strings.Contains(err.Error(), "engines that meet them all: none") {
			t.Errorf("Start() error = %v; want ErrRequirementsUnmet naming none", err)
		}
	})

	t.Run("Start_runs_on_an_engine_meeting_every_requirement_with_duplicates_and_forks_beside_ForkSubagents", func(t *testing.T) {
		t.Parallel()
		rig := newRequiresRig(t)
		spec := Spec{ForkSubagents: true, Requires: []Requirement{RequirementForks, RequirementBackgroundWake, RequirementForks}}
		if _, err := rig.start(spec); err != nil {
			t.Fatalf("Start() error: %v", err)
		}
	})

	attachSpec := func(rig *requiresRig, requires ...Requirement) Spec {
		return Spec{OutputFiles: []string{rig.outputFile()}, Timeout: time.Minute, Requires: requires}
	}
	attachWayForward := func(rig *requiresRig, runDir string) []string {
		minAge := 2 * time.Duration(rig.fx.Runner.cfg.StartupTimeoutS) * time.Second
		return []string{
			fmt.Sprintf(`lyx reed remove %s`, "strand-1"),
			runDir,
			fmt.Sprintf("older than twice startup_timeout_s (%s)", minAge),
		}
	}

	attachRefusals := []struct {
		name           string
		forkSubagents  bool
		requires       []Requirement
		want           []string
		wantWayForward bool
	}{
		{name: "a spec requirement the recorded engine does not meet", requires: []Requirement{RequirementBackgroundWake}, want: []string{`engine "plain"`, "background-wake"}, wantWayForward: true},
		{name: "a record carrying ForkSubagents on an engine without forks", forkSubagents: true, want: []string{`engine "plain"`, "forks"}, wantWayForward: true},
		{name: "an unknown requirement", requires: []Requirement{"telepathy"}, want: []string{`shuttle: attach: unknown requirement "telepathy"; known requirements: forks, peer-messaging, background-wake`}},
	}
	for _, tt := range attachRefusals {
		t.Run("attach_refuses_"+tt.name, func(t *testing.T) {
			t.Parallel()
			rig := newRequiresRig(t)
			runDir := rig.startRecord(tt.forkSubagents)
			_, found, err := rig.fx.Runner.ProbeGated(attachSpec(rig, tt.requires...), GateSpec{})
			if found || !errors.Is(err, ErrRequirementsUnmet) {
				t.Fatalf("ProbeGated() found = %v, error = %v; want a refusal wrapping ErrRequirementsUnmet", found, err)
			}
			wants := tt.want
			if tt.wantWayForward {
				wants = append(wants, attachWayForward(rig, runDir)...)
			}
			for _, want := range wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("ProbeGated() error = %q; want it to contain %q", err, want)
				}
			}
		})
	}

	t.Run("attach_ignores_a_ForkSubagents_on_the_spec_alone", func(t *testing.T) {
		t.Parallel()
		rig := newRequiresRig(t)
		rig.startRecord(false)
		spec := attachSpec(rig)
		spec.ForkSubagents = true
		if _, found, err := rig.fx.Runner.ProbeGated(spec, GateSpec{}); err != nil || !found {
			t.Errorf("ProbeGated() found = %v, error = %v; want the run attached with no refusal", found, err)
		}
	})

	t.Run("attach_harvests_a_finished_run_without_the_check", func(t *testing.T) {
		t.Parallel()
		rig := newRequiresRig(t)
		rig.startRecord(true)
		touchOutputFile(t, rig.outputFile())
		if _, found, err := rig.fx.Runner.ProbeGated(attachSpec(rig, RequirementBackgroundWake), GateSpec{}); err != nil || !found {
			t.Errorf("ProbeGated() found = %v, error = %v; want the finished run harvested with no refusal", found, err)
		}
	})
}
