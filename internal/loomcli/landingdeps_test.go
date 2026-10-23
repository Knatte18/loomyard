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
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
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
	recordPath := filepath.Join(t.TempDir(), "pause.json")
	runner.SetPause(pauserecord.Geometry{RecordPath: recordPath, Slug: loc.WorktreeName})
	now := time.Now()
	if _, err := pauserecord.Update(recordPath, now, func(record *pauserecord.Record) error {
		record.Live = append(record.Live, pauserecord.Entry{Scope: loc.WorktreeName, PausedAt: now.Add(-time.Hour), GraceEnd: now.Add(-time.Minute)})
		return nil
	}); err != nil {
		t.Fatalf("write the pause record: %v", err)
	}
	cfg := landingshed.Config{Squash: true}
	languageSkills := []string{"scribe:golang-testing"}
	lock := parentLockDeps(loc, shedrun.SelfRunID, "", "")
	lock.Store.Alive = func(int, string) bool { return true }

	build := func(recipe string) landingshed.Deps {
		return landingDeps(
			loc,
			recipe,
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
			lock,
			time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		)
	}
	deps := build(shedrun.RecipeLoom)

	// A darn run has no plan to trace a failure to.
	if build(shedrun.RecipeDarn).TraceFailure != nil {
		t.Error("landingDeps(...).TraceFailure is set for the darn recipe; want nil")
	}

	// The slug entry stops the verify until the run holds its parent lock, which exempts the slug's entries.
	if !deps.VerifyHold.PastGrace() {
		t.Error("landingDeps(...).VerifyHold.PastGrace() = false with a slug entry past its grace end and no lock held; want true")
	}
	owner := parentlock.Owner{Slug: loc.WorktreeName, Pair: "pair", RunID: "run", PID: 1, StartTime: "t1"}
	if _, err := lock.Store.TryAcquire("main", owner); err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if deps.VerifyHold.PastGrace() {
		t.Error("landingDeps(...).VerifyHold.PastGrace() = true while the run holds its parent lock; want false")
	}
	if _, err := lock.Store.Release("main", owner); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if !deps.VerifyHold.PastGrace() {
		t.Error("landingDeps(...).VerifyHold.PastGrace() = false after the lock is released; want true")
	}

	if deps.ParentName != "parent-session" {
		t.Errorf("landingDeps(...).ParentName = %q; want the told parent name", deps.ParentName)
	}
	if !slices.Equal(deps.LanguageSkills, languageSkills) {
		t.Errorf("landingDeps(...).LanguageSkills = %v; want the told language skills %v", deps.LanguageSkills, languageSkills)
	}

	if deps.Slug != "pair" {
		t.Errorf("landingDeps(...).Slug = %q; want the worktree's slug %q", deps.Slug, "pair")
	}

	v := reflect.ValueOf(deps)
	typ := v.Type()
	// The loom source leaves the Publish skip and failure classifier seams nil, so the walk skips them.
	if deps.PublishSkip != nil {
		t.Error("landingDeps(...).PublishSkip is set for loom's verify source; want nil")
	}
	if deps.ClassifyVerifyFailure != nil {
		t.Error("landingDeps(...).ClassifyVerifyFailure is set for loom's verify source; want nil")
	}
	// FixerSeats is the one field wireLanding fills, from the seat engine wire built.
	unfilled := map[string]bool{
		"PublishSkip": true, "ClassifyVerifyFailure": true, "FixerSeats": true,
	}
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if unfilled[typ.Field(i).Name] {
			continue
		}
		if field.IsZero() {
			t.Errorf("landingDeps(...).%s is the zero value; want it populated", typ.Field(i).Name)
		}
	}
}

// TestRowEntry asserts rowEntry answers the history index just after the last entry another producer made.
func TestRowEntry(t *testing.T) {
	t.Parallel()

	entry := func(producer string) shedengine.HistoryEntry { return shedengine.HistoryEntry{Producer: producer} }
	tests := []struct {
		name    string
		history []shedengine.HistoryEntry
		want    int
	}{
		{"EmptyHistory", nil, 0},
		{"OwnResumedEntries", []shedengine.HistoryEntry{entry("Webster"), entry("Merge-In"), entry("Merge-In")}, 1},
		{"ReachedAgainFromAnotherRow", []shedengine.HistoryEntry{entry("Merge-In"), entry("Finalize"), entry("Merge-In")}, 2},
		{"OnlyOwnEntries", []shedengine.HistoryEntry{entry("Merge-In"), entry("Merge-In")}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := rowEntry(tt.history, "Merge-In"); got != tt.want {
				t.Errorf("rowEntry(%v, Merge-In) = %d; want %d", tt.history, got, tt.want)
			}
		})
	}
}

// TestFixerTestFileMatcher asserts the matcher a darn run gets is the union over languages, and a loom run's follows the live plan's language.
func TestFixerTestFileMatcher(t *testing.T) {
	t.Parallel()

	writePlan := func(t *testing.T, language string) string {
		t.Helper()
		planDir := planparser.PlanDir(t.TempDir())
		plankit.Write(t, planDir, plankit.Plan{
			Language: language,
			Cards: []plankit.Card{{
				Number:        1,
				Slug:          "first-card",
				Summary:       "the first card",
				Groups:        []plankit.Group{{Label: "Edit", Targets: []string{"docs/a.md"}}},
				Intent:        "change the doc.",
				ImpactSummary: "none",
			}},
		})
		return planDir
	}
	unparseable := t.TempDir()

	tests := []struct {
		name    string
		recipe  string
		planDir string
		path    string
		want    bool
	}{
		{"DarnUnionGoTest", shedrun.RecipeDarn, "", "a/b_test.go", true},
		{"DarnUnionCsharpTest", shedrun.RecipeDarn, "", "a/BTests.cs", true},
		{"DarnUnionSource", shedrun.RecipeDarn, "", "a/b.go", false},
		{"LoomNoLanguageReadsGo", shedrun.RecipeLoom, writePlan(t, ""), "a/b_test.go", true},
		{"LoomNoLanguageSourceIsNotATest", shedrun.RecipeLoom, writePlan(t, ""), "a/b.go", false},
		{"LoomCsharpPlanIgnoresGoTests", shedrun.RecipeLoom, writePlan(t, "csharp"), "a/b_test.go", false},
		{"LoomCsharpPlanMatchesCsharpTests", shedrun.RecipeLoom, writePlan(t, "csharp"), "a/BTests.cs", true},
		{"LoomUnparseablePlanCountsEveryFile", shedrun.RecipeLoom, unparseable, "a/b.go", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := fixerTestFileMatcher(tt.recipe, tt.planDir)(tt.path); got != tt.want {
				t.Errorf("fixerTestFileMatcher(%s, %q)(%q) = %t; want %t", tt.recipe, tt.planDir, tt.path, got, tt.want)
			}
		})
	}
}

// TestHoldsParentLock asserts a nil store reads not held, and a store whose directory is a file reads not held across two calls with one warning.
//
// It stays serial because logcapture swaps the process-global logger.
func TestHoldsParentLock(t *testing.T) {
	if holdsParentLock(nil, "pair")() {
		t.Error("holdsParentLock(nil, pair)() = true; want false")
	}

	buf := logcapture.Capture(t)
	dir := filepath.Join(t.TempDir(), "locks")
	if err := os.WriteFile(dir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	holds := holdsParentLock(&parentlock.Store{Dir: dir}, "pair")
	if holds() || holds() {
		t.Error("holdsParentLock over an unlistable store = true; want false")
	}
	if got := strings.Count(buf.String(), "the parent locks cannot be listed"); got != 1 {
		t.Errorf("warnings over two calls = %d; want 1", got)
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

// TestConflictSessionStopper asserts the seam stops every non-retiring conflict strand of the run by guid, numbered forms included, when it is live or an own-window strand with a present pane.
// It stops nothing for a retiring or absent one, or one with neither a live nor a present pane, and passes a failing table read or stop through with the guid the failure belongs to.
func TestConflictSessionStopper(t *testing.T) {
	t.Parallel()

	conflict := func(guid, role string, live, retiring bool) reedengine.StrandStatus {
		return reedengine.StrandStatus{GUID: guid, Name: "hub:slug:" + role, Live: live, Retiring: retiring}
	}
	// shellOnly is an own-window conflict strand whose engine exited: its pane is present and not live.
	shellOnly := conflict("c1", "conflict", false, false)
	shellOnly.OwnWindow, shellOnly.Present = true, true
	// plainPresent is a window 0 strand with a present pane that is not live, which reed's reconcile removes at its next op.
	plainPresent := conflict("c1", "conflict", false, false)
	plainPresent.Present = true
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
		{name: "present shell-only own-window conflict strand is stopped", strands: []reedengine.StrandStatus{shellOnly}, wantStops: []string{"c1"}},
		{name: "present not-live window 0 conflict strand stops nothing", strands: []reedengine.StrandStatus{plainPresent}},
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

func TestReportOfRunKeepsOnlyTheRunsOwnEntry(t *testing.T) {
	t.Parallel()
	report := fabricengine.OriginReport{
		Unreachable: true,
		Branches:    []fabricengine.BranchOutcome{{Branch: "parent"}},
		Entries: []fabricengine.EntryOutcome{
			{Slug: "parent", State: fabricengine.EntryStopped},
			{Slug: "child", State: fabricengine.EntryActive},
		},
	}

	got := reportOfRun(report, "child")

	if len(got.Entries) != 1 || got.Entries[0].Slug != "child" {
		t.Errorf("reportOfRun(report, %q).Entries = %+v; want only the child entry", "child", got.Entries)
	}
	if !got.Unreachable || len(got.Branches) != 1 {
		t.Errorf("reportOfRun(report, %q) = %+v; want the other fields unchanged", "child", got)
	}
}

func TestMergeableTips(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		tips        fabricengine.ParentTips
		pushSkipped bool
		want        []string
	}{
		{"SkippedPushDropsTheUpstreamTip", fabricengine.ParentTips{Local: "local", Upstream: "upstream"}, true, []string{"local"}},
		{"UnskippedPushKeepsBothTips", fabricengine.ParentTips{Local: "local", Upstream: "upstream"}, false, []string{"local", "upstream"}},
		{"EmptyUpstreamIsDropped", fabricengine.ParentTips{Local: "local"}, false, []string{"local"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := mergeableTips(tt.tips, tt.pushSkipped); !slices.Equal(got, tt.want) {
				t.Errorf("mergeableTips(%+v, %v) = %v; want %v", tt.tips, tt.pushSkipped, got, tt.want)
			}
		})
	}
}
