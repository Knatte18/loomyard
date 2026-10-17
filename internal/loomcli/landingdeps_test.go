// landingdeps_test.go drift-guards landingDeps: it asserts every field of the returned
// landingshed.Deps is populated, via a reflection-based walk rather than an enumerated list of field
// assertions, so a newly added field is caught automatically rather than silently passing. This test
// needs no fixture, no hubforge, and no git init -- landingDeps performs no I/O -- so it stays Tier 1.

package loomcli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/alignment"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestLandingDeps_EveryFieldPopulated asserts landingDeps populates every field of the
// landingshed.Deps it returns, walking the struct by reflection so a field added later is
// caught automatically rather than silently passing an enumerated list of assertions.
//
//testtiming:keep drift guard walking every landingshed.Deps field by reflection so a field added later must be populated; the covering mark-done test populates only the fields it uses
func TestLandingDeps_EveryFieldPopulated(t *testing.T) {
	t.Parallel()

	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
	geom := websterengine.Geometry{
		WebsterDir:  "/webster",
		StencilsDir: "/stencils",
	}
	pushBranch := func() error { return nil }
	registry := modelspec.Registry{"claude/sonnet-5": {}}
	runner := &shuttleengine.Runner{}
	cfg := landingshed.Config{Squash: true}
	languageSkills := []string{"scribe:golang-testing"}

	deps := landingDeps(
		loc,
		shedrun.RecipeLoom,
		geom,
		"task/foo",
		"https://example.com/origin.git",
		"main",
		true,
		pushBranch,
		registry,
		runner,
		cfg,
		"parent-session",
		languageSkills,
		func(string, time.Time) error { return nil },
		func() (string, error) { return "", nil },
		planVerifySource(loc),
		parentLockDeps(loc, shedrun.SelfRunID, "", ""),
		time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	)

	if deps.ParentName != "parent-session" {
		t.Errorf("landingDeps(...).ParentName = %q; want the told parent name", deps.ParentName)
	}
	if !slices.Equal(deps.LanguageSkills, languageSkills) {
		t.Errorf("landingDeps(...).LanguageSkills = %v; want the told language skills %v", deps.LanguageSkills, languageSkills)
	}

	v := reflect.ValueOf(deps)
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if field.IsZero() {
			t.Errorf("landingDeps(...).%s is the zero value; want it populated", typ.Field(i).Name)
		}
	}
}

// TestDriverWaitMark asserts the callback marks the driver strand only while it is live and not retiring, is a no-op returning nil for a gone, dead or retiring one, and passes a failing strand read or mark op through.
func TestDriverWaitMark(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 3, 9, 15, 0, 0, time.UTC)
	driver := func(live, retiring bool) reedengine.StrandStatus {
		return reedengine.StrandStatus{GUID: "driver-guid", Name: loomengine.LoomDriverStrandName, Live: live, Retiring: retiring}
	}
	other := reedengine.StrandStatus{GUID: "other-guid", Name: "someone:else", Live: true}
	markErr := errors.New("tmux gone")
	statusErr := errors.New("reed down")

	tests := []struct {
		name      string
		strands   []reedengine.StrandStatus
		statusErr error
		markErr   error
		wantGUID  string
		wantErr   error
	}{
		{name: "live driver is marked", strands: []reedengine.StrandStatus{other, driver(true, false)}, wantGUID: "driver-guid"},
		{name: "gone driver is a no-op", strands: []reedengine.StrandStatus{other}},
		{name: "dead driver is a no-op", strands: []reedengine.StrandStatus{driver(false, false)}},
		{name: "retiring driver is a no-op", strands: []reedengine.StrandStatus{driver(true, true)}},
		{name: "mark failure is passed through", strands: []reedengine.StrandStatus{driver(true, false)}, markErr: markErr, wantGUID: "driver-guid", wantErr: markErr},
		{name: "status failure is passed through", statusErr: statusErr, wantErr: statusErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotGUID, gotLabel string
			var gotStart time.Time
			mark := driverWaitMark(
				func() (reedengine.StatusResult, error) {
					return reedengine.StatusResult{Strands: tt.strands}, tt.statusErr
				},
				func(guid, label string, started time.Time) error {
					gotGUID, gotLabel, gotStart = guid, label, started
					return tt.markErr
				},
			)
			err := mark("verify Publish", start)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if gotGUID != tt.wantGUID {
				t.Fatalf("marked strand = %q, want %q", gotGUID, tt.wantGUID)
			}
			if tt.wantGUID != "" && (gotLabel != "verify Publish" || !gotStart.Equal(start)) {
				t.Errorf("mark = (%q, %v), want (%q, %v)", gotLabel, gotStart, "verify Publish", start)
			}
		})
	}
}

// TestConflictSessionStopper asserts the seam stops every live, non-retiring conflict strand of the run by guid, numbered forms included.
// It stops nothing for a dead, retiring or absent one, and passes a failing table read or stop through with the guid the failure belongs to.
func TestConflictSessionStopper(t *testing.T) {
	t.Parallel()

	conflict := func(guid, role string, live, retiring bool) reedengine.StrandStatus {
		return reedengine.StrandStatus{GUID: guid, Name: "hub:slug:" + role, Live: live, Retiring: retiring}
	}
	other := reedengine.StrandStatus{GUID: "other-guid", Name: "hub:slug:webster", Live: true}
	statusErr := errors.New("reed down")
	stopErr := errors.New("tmux gone")

	tests := []struct {
		name      string
		strands   []reedengine.StrandStatus
		statusErr error
		stopErr   error
		wantStops []string
		wantGUID  string
		wantErr   error
	}{
		{name: "live conflict strand is stopped", strands: []reedengine.StrandStatus{other, conflict("c1", "conflict", true, false)}, wantStops: []string{"c1"}},
		{name: "live numbered conflict strand is stopped", strands: []reedengine.StrandStatus{conflict("c2", "conflict-2", true, false)}, wantStops: []string{"c2"}},
		{name: "every live conflict strand is stopped", strands: []reedengine.StrandStatus{conflict("c1", "conflict", true, false), conflict("c2", "conflict-2", true, false)}, wantStops: []string{"c1", "c2"}},
		{name: "dead conflict strand stops nothing", strands: []reedengine.StrandStatus{conflict("c1", "conflict", false, false)}},
		{name: "retiring conflict strand stops nothing", strands: []reedengine.StrandStatus{conflict("c1", "conflict", true, true)}},
		{name: "no conflict strand stops nothing", strands: []reedengine.StrandStatus{other}},
		{name: "table read failure is passed through with an empty guid", statusErr: statusErr, wantErr: statusErr},
		{name: "stop failure ends the pass with the failing guid", strands: []reedengine.StrandStatus{conflict("c1", "conflict", true, false), conflict("c2", "conflict-2", true, false)}, stopErr: stopErr, wantStops: []string{"c1"}, wantGUID: "c1", wantErr: stopErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotStops []string
			stopper := conflictSessionStopper(
				func() (reedengine.StatusResult, error) {
					return reedengine.StatusResult{Strands: tt.strands}, tt.statusErr
				},
				func(guid string) error {
					gotStops = append(gotStops, guid)
					return tt.stopErr
				},
			)
			guid, err := stopper()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if guid != tt.wantGUID {
				t.Errorf("guid = %q, want %q", guid, tt.wantGUID)
			}
			if !reflect.DeepEqual(gotStops, tt.wantStops) {
				t.Errorf("stopped = %v, want %v", gotStops, tt.wantStops)
			}
		})
	}
}

// TestLoopLockOwner asserts the lock's owner process is the run's live loop, and the calling step process for a dead loop, the teardown's mark and no record at all.
func TestLoopLockOwner(t *testing.T) {
	t.Parallel()

	parentPID := os.Getppid()
	parentStart, _ := proc.StartTime(parentPID)
	if parentStart == "" {
		t.Skip("this platform reports no process start time")
	}
	liveLoop := proc.TreeRecord{PID: parentPID, StartTime: parentStart}
	self := proc.SelfRecord()

	tests := []struct {
		name    string
		record  *shedverbs.LoopPIDRecord
		wantPID int
	}{
		{name: "a live loop is the owner process", record: &shedverbs.LoopPIDRecord{LoopID: "loop-1", Loop: liveLoop}, wantPID: parentPID},
		{name: "a dead loop falls back to the step", record: &shedverbs.LoopPIDRecord{LoopID: "loop-1", Loop: proc.TreeRecord{PID: parentPID, StartTime: "not-its-start-time"}}, wantPID: self.PID},
		{name: "the teardown mark counts as no loop", record: &shedverbs.LoopPIDRecord{LoopID: shedverbs.TeardownLoopID, Loop: liveLoop}, wantPID: self.PID},
		{name: "no record is the step", wantPID: self.PID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
			if tt.record != nil {
				if err := shedverbs.WriteLoopPIDRecord(shedrun.LoopPIDFile(loc, shedrun.SelfRunID), *tt.record); err != nil {
					t.Fatalf("WriteLoopPIDRecord() error = %v", err)
				}
			}

			owner, err := loopLockOwner(loc, shedrun.SelfRunID)()
			if err != nil {
				t.Fatalf("loopLockOwner()() error = %v", err)
			}
			if owner.PID != tt.wantPID {
				t.Errorf("owner.PID = %d; want %d", owner.PID, tt.wantPID)
			}
			if owner.Slug != "pair" || owner.Pair != loc.WorktreePath() || owner.RunID != "pair" {
				t.Errorf("owner = %+v; want the run named for the worktree %q at %q", owner, "pair", loc.WorktreePath())
			}
		})
	}
}

// TestParentLockDeps_PauseAfterTheWaitingRowEndsTheWait writes a status running at Publish and asserts the pause seam reports false with no condition, false with pause_after naming another row, true with it naming Publish, and false with only pause_requested set.
func TestParentLockDeps_PauseAfterTheWaitingRowEndsTheWait(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		apply func(st *shedengine.Status)
		want  bool
	}{
		{"no condition", func(*shedengine.Status) {}, false},
		{"pause_after naming another row", func(st *shedengine.Status) { st.PauseAfter = "Finalize" }, false},
		{"pause_after naming the waiting row", func(st *shedengine.Status) { st.PauseAfter = "Publish" }, true},
		{"pause_requested alone", func(st *shedengine.Status) { st.PauseRequested = true }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			statusPath := filepath.Join(dir, "status.json")
			statusLockPath := filepath.Join(dir, "status.json.lock")
			st := shedengine.Status{CurrentProducer: "Publish", State: shedengine.StateRunning}
			tt.apply(&st)
			writeStatusFixture(t, statusPath, statusLockPath, st)

			loc := &lyxcwd.Location{HubPath: dir, WorktreeName: "pair", AnchorRel: "."}
			got, err := parentLockDeps(loc, shedrun.SelfRunID, statusPath, statusLockPath).PauseRequested()
			if err != nil || got != tt.want {
				t.Errorf("PauseRequested() = %v, %v; want %v, nil", got, err, tt.want)
			}
		})
	}
}

// TestLandingAlignmentDeps_Config drives the real Config closure over a temp location with loom.yaml seeded:
// skip_alignment and a hub with no board dir read Off, and otherwise it reads the resolved settings and alignment_pr_approve.
// This pins the PR-side half of the criterion that skip_alignment or no board dir leaves both alignment points as before.
func TestLandingAlignmentDeps_Config(t *testing.T) {
	t.Parallel()

	recipes := []string{shedrun.RecipeLoom, shedrun.RecipeDarn}
	depsOf := func(t *testing.T, loc *lyxcwd.Location, recipe string) landingshed.AlignmentDeps {
		t.Helper()
		registry, err := modelspec.LoadRegistry(fabricengine.ConfigDirs(loc))
		if err != nil {
			t.Fatal(err)
		}
		return landingAlignmentDeps(loc, recipe, t.TempDir(), registry, nil, pauserecord.Geometry{})
	}
	configOf := func(t *testing.T, loc *lyxcwd.Location, recipe string) landingshed.AlignmentConfig {
		t.Helper()
		cfg, err := depsOf(t, loc, recipe).Config()
		if err != nil {
			t.Fatalf("Config() = %v; want nil", err)
		}
		return cfg
	}

	for _, recipe := range recipes {
		t.Run(recipe+": skip_alignment reads off", func(t *testing.T) {
			t.Parallel()
			loc := hubLocation(t, "pair", ".")
			seedLoomConfigWithKeys(t, loc.AnchorPath(), map[string]string{"skip_alignment": "true"})
			if cfg := configOf(t, loc, recipe); !cfg.Off {
				t.Errorf("Config() = %+v; want Off", cfg)
			}
		})
	}

	t.Run("no board dir reads off", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		if err := os.RemoveAll(fabricengine.BoardDir(loc.HubPath)); err != nil {
			t.Fatal(err)
		}
		if cfg := configOf(t, loc, shedrun.RecipeLoom); !cfg.Off {
			t.Errorf("Config() = %+v; want Off", cfg)
		}
	})

	t.Run("the template reads on with pr approve and the resolved default settings", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		cfg := configOf(t, loc, shedrun.RecipeLoom)
		if cfg.Off || !cfg.PRApprove || cfg.Settings.Model == "" || cfg.Settings.Timeout != 20*time.Minute {
			t.Errorf("Config() = %+v; want on, pr approve and the opus[high] settings with a 20 minute timeout", cfg)
		}
	})

	for _, recipe := range recipes {
		t.Run(recipe+": alignment_pr_approve false reads pr approve false", func(t *testing.T) {
			t.Parallel()
			loc := hubLocation(t, "pair", ".")
			seedLoomConfigWithKeys(t, loc.AnchorPath(), map[string]string{"alignment_pr_approve": "false"})
			if cfg := configOf(t, loc, recipe); cfg.Off || cfg.PRApprove {
				t.Errorf("Config() = %+v; want on with pr approve false", cfg)
			}
		})
	}

	t.Run("each recipe tells its own prompt and a record appended while the darn agent runs changes its snapshot", func(t *testing.T) {
		t.Parallel()
		loc := &lyxcwd.Location{HubPath: boardkit.HubWithBoardConfig(t), WorktreeName: "pair", AnchorRel: "."}
		registry, err := modelspec.LoadRegistry(fabricengine.ConfigDirs(loc))
		if err != nil {
			t.Fatal(err)
		}
		record, supportLog := loomengine.DiscussionDecisionRecord(loc), loomengine.DiscussionSupportLog(loc)

		loomDeps := landingAlignmentDeps(loc, shedrun.RecipeLoom, t.TempDir(), registry, nil, pauserecord.Geometry{})
		darnDeps := landingAlignmentDeps(loc, shedrun.RecipeDarn, t.TempDir(), registry, nil, pauserecord.Geometry{})
		for recipe, prompt := range map[string]alignment.PromptInput{shedrun.RecipeLoom: loomDeps.Prompt, shedrun.RecipeDarn: darnDeps.Prompt} {
			if prompt.DecisionRecordPath != record || prompt.SupportLogPath != supportLog {
				t.Errorf("%s prompt paths = %q, %q; want the loomengine accessors' %q, %q", recipe, prompt.DecisionRecordPath, prompt.SupportLogPath, record, supportLog)
			}
		}
		if got, want := loomDeps.Prompt.PlanDir, planparser.PlanDir(loc.AnchorPath()); got != want || loomDeps.Prompt.MeasureStencil != alignment.MeasureLoom {
			t.Errorf("loom prompt = PlanDir %q, MeasureStencil %q; want %q, %q", got, loomDeps.Prompt.MeasureStencil, want, alignment.MeasureLoom)
		}
		if darnDeps.Prompt.PlanDir != "" || darnDeps.Prompt.MeasureStencil != alignment.MeasureDarn {
			t.Errorf("darn prompt = PlanDir %q, MeasureStencil %q; want an empty plan directory and %q", darnDeps.Prompt.PlanDir, darnDeps.Prompt.MeasureStencil, alignment.MeasureDarn)
		}

		var sections strings.Builder
		for _, heading := range []string{"Goal", "Scope", "Decisions", "Constraints", "Auto-mode assumptions", "Open risks", "Acceptance criteria"} {
			sections.WriteString("## " + heading + "\n\nbody.\n\n")
		}
		if err := os.MkdirAll(filepath.Dir(record), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(record, []byte(sections.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(supportLog, []byte("log"), 0o644); err != nil {
			t.Fatal(err)
		}
		before, err := darnDeps.Snapshot()
		if err != nil {
			t.Fatalf("Snapshot() = %v; want nil", err)
		}
		added := discussionparser.AddedDecision{By: "parent", Title: "Keep it", Decision: "Keep it.", Rationale: "It races.", Date: time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)}
		if findings, err := discussionparser.AppendDecision(record, supportLog, added); err != nil || len(findings) != 0 {
			t.Fatalf("AppendDecision() = %v, %v; want no findings and nil error", findings, err)
		}
		after, err := darnDeps.Snapshot()
		if err != nil {
			t.Fatalf("Snapshot() = %v; want nil", err)
		}
		if before == after {
			t.Errorf("Snapshot() = %q before and after a recorded decision; want it changed", after)
		}
	})
}
