// Package envkit builds the fully filled seam structs the recipe tests share: a `shedrecipe.Env`, a `landingshed.Deps` and a `loomshed.PRReworkDeps`.
//
// A new required field is added to the kit once, instead of to every package's private copy.
// Builders return plain values the caller mutates; there are no option structs, and a builder fails the test only on its own setup.
// NilSeams reports and never asserts, so the CLI wiring tests and this kit's own test assert on its answer.
//
// `shedrecipe` and `landingshed` cannot import the kit without a cycle, so each keeps one local builder.
package envkit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"gopkg.in/yaml.v3"
)

// nilLegal holds the dotted paths NilSeams skips: seams whose nil is a documented default rather than a missing wiring.
var nilLegal = map[string]bool{
	"Now":                              true,
	"VerifyMergeBase":                  true,
	"ReviewBase":                       true,
	"VerifyTreeScope":                  true,
	"DiscussionHistory":                true,
	"WebsterRecord":                    true,
	"Pawl.Watch":                       true,
	"NotifyParent":                     true,
	"Landing.CommitStatus":             true,
	"Landing.CommitParentRecords":      true,
	"Landing.MergeState":               true,
	"Landing.AbortMerge":               true,
	"Landing.StopConflictSession":      true,
	"Landing.MarkTaskDone":             true,
	"Landing.ConfigChanges":            true,
	"Landing.Notify":                   true,
	"Landing.Reconcile":                true,
	"Landing.ParentHoldsTaskWork":      true,
	"Landing.ParentOwner":              true,
	"Landing.AdoptSubject":             true,
	"Landing.LandBlockers":             true,
	"Landing.OriginParentHoldsWork":    true,
	"Landing.ParentCommitsOriginLacks": true,
	"InnerRun.OwnerCheck":              true,
	"InnerRun.PauseOwned":              true,
	"InnerRun.ParentHoldsTaskWork":     true,
	"InnerRun.Restepped":               true,
	"InnerRun.HandBack":                true,
	"Landing.VerifyCommand":            true,
	"Landing.VerifyWaitMark":           true,
	"Landing.FailingTests":             true,
	"Landing.TraceFailure":             true,
	"Landing.Registry":                 true,
	"Landing.Config.EngineTrailers":    true,
	"WebsterDeps.Batcher":              true,
	"WebsterDeps.Roles":                true,
	"WebsterDeps.ParentBranch":         true,
	"WebsterDeps.InstructionFiles":     true,
	"WebsterDeps.Geom.Git":             true,
	"WebsterDeps.Geom.GateSlots":       true,
	"WebsterDeps.Geom.GateConfig":      true,
	"InnerRun.ReviveAPIError":          true,
	"InnerRun.Now":                     true,
	"InnerRun.Awake":                   true,
	"InnerRun.Sleeps":                  true,
	"InnerRun.Pause":                   true,
	"InnerRun.Notify":                  true,
	"InnerRun.PairDir":                 true,
	"InnerRun.ChildDriverLockHeld":     true,
	"InnerRun.TacklerInFlight":         true,
	"InnerRun.ArbiterStop":             true,
	"InnerRun.ChildBuildChanged":       true,
	"InnerRun.ChildDirtyPaths":         true,
	"Land.Sleep":                       true,
	"Land.Now":                         true,
	"Land.LandLock.Sleep":              true,
	"Land.Notices.Notify":              true,
	"Land.Notices.Sleep":               true,
	"PrimeLock.Sleep":                  true,
	"SegmentBounces":                   true,
	"Segments":                         true,
	"Setting":                          true,
	"Command.Setting":                  true,
	"Command.ExecutableSetting":        true,
	"Drift.Seats":                      true,
	"Drift.Now":                        true,
	"DocumentSubject":                  true,
	"CodeSubject":                      true,
	"MergeInOnly":                      true,
	"RecordHead":                       true,
	"ReviewInput":                      true,
}

// noFindingsIndex is a planindex.Index that answers no findings, so the packages built on the kit do not link the resolve-backed index.
// Its Delta panics: a test that needs a delta supplies its own index.
type noFindingsIndex struct{ planindex.Index }

func (noFindingsIndex) ValidateFormat(*planparser.Plan, string, []planparser.Card) ([]planindex.Finding, error) {
	return nil, nil
}

func (noFindingsIndex) ValidateRework(*planparser.Plan, string, int) ([]planindex.Finding, error) {
	return nil, nil
}

// nilFabricOpener is a typed-nil fabric handle with a nil error.
// The Parent-Align and Finalize constructors only nil-check the interface the handle is stored behind, and never dereference it.
func nilFabricOpener() (*fabricengine.Fabric, error) {
	return nil, nil
}

// noopFixerSeats is a fixer seat runner that starts nothing and ends at once.
type noopFixerSeats struct{}

func (noopFixerSeats) Run(seatengine.Table) (seatengine.Result, error) {
	return seatengine.Result{}, nil
}

// noopSessions is a session runner that starts nothing and stops nothing, since the fixture's landing takes the local path and opens no session.
type noopSessions struct{}

func (noopSessions) StartSession(shuttleengine.Spec) (shuttleengine.OpenResult, error) {
	return shuttleengine.OpenResult{}, errors.New("envkit: the fixture's landing starts no session")
}

func (noopSessions) StopStrand(string) error { return nil }

// LandingDeps returns a `landingshed.Deps` with every field the Parent-Align and Finalize constructors require, filled with told values derived from dir.
func LandingDeps(dir string) landingshed.Deps {
	return landingshed.Deps{
		GateKey:          "parent-align",
		WorktreeRoot:     dir,
		TaskBranch:       "task-branch",
		ParentBranch:     "fixture-parent",
		DescriptionPath:  summaryparser.Path(dir),
		StencilsDir:      dir,
		ParentName:       "fixture-parent-session",
		ScratchDir:       filepath.Join(dir, "landing-scratch"),
		OpenFabric:       nilFabricOpener,
		OpenParentFabric: nilFabricOpener,
		Shuttle:          &shedfake.MergeShuttle{},
		EnginesRun:       func() ([]string, error) { return nil, nil },
		TaskHead:         func() (string, error) { return "head", nil },
		BaseCommand:      func() (string, error) { return "lym gate test --wait --tier 0 --all", nil },
		GateTiers:        fixedGateTiers,
		VerifyHold:       verifytree.NewPauseHold(pauserecord.Geometry{}, nil),
		Lock: landingshed.ParentLockDeps{
			Store:          &parentlock.Store{Dir: filepath.Join(dir, "parent-lock")},
			Owner:          func() (parentlock.Owner, error) { return parentlock.Owner{Slug: "fixture", RunID: "fixture"}, nil },
			WaitPath:       parentlock.WaitRecordPath(filepath.Join(dir, "landing-scratch")),
			PauseRequested: func() (bool, error) { return false, nil },
		},
		ReadParentTips: func(bool) (fabricengine.ParentTips, error) {
			return fabricengine.ParentTips{Local: "tip", Upstream: "tip", Relation: fabricengine.ParentEqual}, nil
		},
		HoldsWork:          func(string, string) (bool, error) { return false, nil },
		ChangedPaths:       func(string, string) ([]string, error) { return nil, nil },
		MergeBase:          func(string) (string, error) { return "base", nil },
		MergeRecordPath:    landingshed.MergeRecordPath(filepath.Join(dir, "landing-scratch")),
		FixerSeats:         noopFixerSeats{},
		DecisionRecordPath: filepath.Join(dir, "decision-record.md"),
		PlanDir:            filepath.Join(dir, "plan"),
		LineChange:         func(string, string) (fabricengine.LineChange, error) { return fabricengine.LineChange{}, nil },
		IsTestFile:         func(string) bool { return false },
		RowEntry:           func(string) (int, error) { return 1, nil },
		TaskRemoteTip:      func() (string, error) { return "", nil },
		CodeReviewOff:      func() (bool, error) { return false, nil },
		RerouteReviewed:    func() (bool, error) { return true, nil },
		// ParentMode answers off and Origin is empty, so a landing through this deps takes the local path and calls no GitHub.
		PullRequest: landingshed.PullRequestDeps{
			RecordPath:     filepath.Join(dir, "landing-scratch", "pull-request.json"),
			RecordLockPath: filepath.Join(dir, "landing-scratch", "pull-request.lock"),
			ParentMode:     func() (string, string, error) { return "off", "fixture-parent", nil },
			EntryField:     func() (string, error) { return "", nil },
			Origin:         func() (landingshed.Origin, error) { return landingshed.Origin{}, nil },
			API: func(string) (pullrequest.API, error) {
				return nil, errors.New("envkit: the fixture's landing reaches no GitHub")
			},
			PushTask:        func() error { return nil },
			CommitsMissing:  func(string, string) ([]string, error) { return nil, nil },
			DirtyPaths:      func() ([]string, error) { return nil, nil },
			MailAgent:       func(string, string) error { return nil },
			PauseCovered:    func() (bool, error) { return false, nil },
			Now:             time.Now,
			Sleep:           func(context.Context, time.Duration) error { return nil },
			Sessions:        noopSessions{},
			SessionLive:     func(string) (bool, error) { return false, nil },
			RegisterSession: func(string) error { return nil },
			BoardEntryPath:  filepath.Join(dir, "landing-scratch", "board-entry.md"),
		},
	}
}

// ReworkDeps returns a `loomshed.PRReworkDeps` whose directories exist under dir and whose closures are no-ops.
func ReworkDeps(t testing.TB, dir string) loomshed.PRReworkDeps {
	t.Helper()
	return loomshed.PRReworkDeps{
		PlanDir:        mustMkdir(t, filepath.Join(dir, "rework-plan")),
		ReworkDir:      mustMkdir(t, filepath.Join(dir, "rework-rounds")),
		ReworkDirRel:   "_lym/loom/rework",
		ReviewsDir:     mustMkdir(t, filepath.Join(dir, "rework-reviews")),
		AmendmentPath:  filepath.Join(dir, "rework-amendment.yaml"),
		ReadCommitted:  func(string) ([]byte, bool, error) { return nil, false, nil },
		ArchiveWebster: func(string) error { return nil },
	}
}

// TemplateSetting answers any registered config module's key from the module's embedded template.
// It errors naming the module when it is unregistered, and naming the key when the template lacks it.
// A recipe that names a new setting key needs no kit change.
func TemplateSetting(module, key string) (any, error) {
	registered, ok := configreg.Lookup(module)
	if !ok {
		return nil, fmt.Errorf("envkit: module %q is not a registered config module", module)
	}
	var template map[string]any
	if err := yaml.Unmarshal([]byte(registered.Template()), &template); err != nil {
		return nil, fmt.Errorf("envkit: decode %s template: %w", module, err)
	}
	value, ok := template[key]
	if !ok {
		return nil, fmt.Errorf("envkit: key %q is not in the %s template", key, module)
	}
	return value, nil
}

// fixedGateLists are the tier lists of the gate keys the kit's seams answer from, so a resolved gate builds without a hub's gate.yaml.
var fixedGateLists = map[string][]int{
	"card":            {0, 1},
	"batch":           {0, 1},
	"webster":         {0, 1},
	"parent-align":    {0, 1},
	"finalize":        {0, 1},
	"adopt":           {0, 1},
	"webster-fix":     {0, 1},
	"darn-write":      {0, 1},
	"code-burler-fix": {0, 1},
	"drift-verify":    {0, 1},
	"pr-review":       {0, 1},
}

// fixedGateTiers answers a named gate's tier list from fixedGateLists, and errors naming a gate the kit holds no list for.
func fixedGateTiers(gate string) ([]int, error) {
	list, ok := fixedGateLists[gate]
	if !ok {
		return nil, fmt.Errorf("envkit: no fixed list for gate %q", gate)
	}
	return list, nil
}

// FullEnv returns a `shedrecipe.Env` with every path field an absolute path under one `t.TempDir()` and every seam any registered engine requires filled.
// A seam added to the registry's requirements must be filled here, which NilSeams(FullEnv(t)) catches.
func FullEnv(t testing.TB) shedrecipe.Env {
	t.Helper()
	dir := t.TempDir()
	return shedrecipe.Env{
		Cwd:                mustMkdir(t, filepath.Join(dir, "cwd")),
		AnchorPath:         mustMkdir(t, filepath.Join(dir, "anchor")),
		ConfigDirs:         configengine.Dirs{Anchor: filepath.Join(dir, "anchor")},
		WorktreeRoot:       mustMkdir(t, filepath.Join(dir, "worktree")),
		VerifyDir:          mustMkdir(t, filepath.Join(dir, "verify")),
		VerifyCommand:      func(string) (string, error) { return "go test ./...", nil },
		VerifyTreeScope:    func(string) (string, bool, error) { return "", false, nil },
		VerifyHold:         verifytree.NewPauseHold(pauserecord.Geometry{}, nil),
		StatusPath:         filepath.Join(dir, "status.json"),
		StatusLockPath:     filepath.Join(dir, "status.json.lock"),
		StencilsDir:        mustMkdir(t, filepath.Join(dir, "stencils")),
		SpecsDir:           mustMkdir(t, filepath.Join(dir, "specs")),
		RunRoot:            mustMkdir(t, filepath.Join(dir, "run-root")),
		RunScratchDir:      mustMkdir(t, filepath.Join(dir, "run-scratch")),
		GateFindingsFile:   func(row string) string { return filepath.Join(dir, "gate-findings", row+".md") },
		DecisionRecordPath: filepath.Join(dir, "decision-record.md"),
		PatternRecordPath:  filepath.Join(dir, "decision-record.md"),
		RefuseRaw:          []string{"go test"},
		SupportLogPath:     filepath.Join(dir, "support-log.md"),
		RelevantPath:       filepath.Join(dir, "relevant.md"),
		DescriptionPath:    filepath.Join(dir, "description.md"),
		Shuttle:            &shedfake.Shuttle{},
		Burler:             &shedfake.BurlerHalfRunner{},
		Seats:              &shedfake.SeatRunner{},
		Models:             modelspec.Registry{"opus": {Engine: "claude", Model: "claude-opus-test"}},
		Setting:            TemplateSetting,
		SettingKnown:       configreg.CheckSettingKey,
		Resume:             "lym loom resume",
		ExecutableSetting:  TemplateSetting,
		Segments: map[string]shedrecipe.SegmentSettings{
			"Discussion-Review": fullEnvSegment(3, "loom.yaml"),
			"Plan-Review":       fullEnvSegment(3, "loom.yaml"),
			"Code-Review":       fullEnvSegment(3, "loom.yaml"),
		},
		WebsterRun: func(websterengine.RunDeps, websterengine.RunOptions) (websterengine.RunResult, error) {
			return websterengine.RunResult{}, nil
		},
		WebsterDeps:   shedfake.WebsterSeams(),
		CommitWebster: func() error { return nil },
		Landing:       LandingDeps(mustMkdir(t, filepath.Join(dir, "landing"))),
		DiscussionTable: func() (seatengine.Table, error) {
			return seatengine.Table{
				RolePrefix: "discussion",
				Segment:    segmentcolor.Discussion,
				Seats: []seatengine.Seat{{
					Stencil: "loom-template-discussion-chair",
					Outputs: []string{filepath.Join(dir, "discussion-output.md")},
				}},
			}, nil
		},
		CommitDiscussion: func() error { return nil },
		DescribeTable: func() (seatengine.Table, error) {
			return seatengine.Table{
				RolePrefix: "describe",
				Segment:    segmentcolor.Describe,
				Seats: []seatengine.Seat{{
					Stencil: "landing-template-describe",
					Outputs: []string{filepath.Join(dir, "description.md")},
				}},
			}, nil
		},
		CommitDescription: func() error { return nil },
		BeamDescribe: landingshed.BeamDescribeDeps{
			Slug:            "demo",
			DescriptionPath: filepath.Join(dir, "beam-description.md"),
			Entry:           func() (string, string, error) { return "Title", "Brief", nil },
			CommitSubjects:  func() ([]string, error) { return nil, nil },
			Commit:          func() error { return nil },
		},
		Pawl: pawlengine.Deps{
			Slug:  "demo",
			RunID: "run-1",
			Paths: pawlengine.Paths{
				LandRequest:     filepath.Join(dir, "land-request.json"),
				DobbyStopMarker: filepath.Join(dir, "dobby-stop"),
				AwaitMarker:     filepath.Join(dir, "await-entered"),
			},
			SetStatus:           func(string) error { return nil },
			EntryProducer:       func() (string, error) { return "", nil },
			OwnedElsewhere:      func() (bool, error) { return false, nil },
			MarkBeam:            func() error { return nil },
			SpawnPush:           func() error { return nil },
			RegenerateWorkspace: func() error { return nil },
			Steps:               []pawlengine.Step{{Name: "noop", Run: func() error { return nil }}},
			StopDobby:           func() error { return nil },
			StartDobby:          func() error { return nil },
			DobbyLive:           func() (bool, error) { return false, nil },
			Recheck:             func() ([]string, error) { return nil, nil },
			WriteSnapshot:       func() error { return nil },
			Notify:              func(string) error { return nil },
			Down:                func() error { return nil },
			Now:                 time.Now,
			Sleep:               func(context.Context, time.Duration) error { return nil },
		},
		PlanTable: func() (seatengine.Table, error) {
			return seatengine.Table{
				RolePrefix: "plan",
				Segment:    segmentcolor.Plan,
				Seats: []seatengine.Seat{{
					Stencil: "loom-template-plan",
					Outputs: []string{filepath.Join(dir, "plan-output.md")},
				}},
			}, nil
		},
		CommitPlan:     func() error { return nil },
		ApprovePlan:    func() error { return nil },
		SkipPlanReview: func() (bool, error) { return false, nil },
		CarryOver:      func(discussionparser.CarryOver) error { return nil },
		DocumentSubject: func([]string) (shedadapters.Subject, bool, error) {
			return shedadapters.Subject{Kind: shedadapters.SubjectDocument, Hash: "document-hash"}, true, nil
		},
		CodeSubject: func() (shedadapters.Subject, bool, error) {
			return shedadapters.Subject{Kind: shedadapters.SubjectCode, MergeBase: "merge-base", Head: "head", FullHash: "full-hash", PatchID: "patch-id"}, true, nil
		},
		MergeInOnly:        func(string) (bool, error) { return false, nil },
		RecordHead:         func() (string, error) { return "record-head", nil },
		ReviewInput:        func() (string, time.Time, bool, error) { return "", time.Time{}, false, nil },
		ReviewBase:         func() (string, error) { return "review-base", nil },
		ReflectFriction:    func() string { return "skipped" },
		Rework:             ReworkDeps(t, dir),
		DarnVerifyAttempts: 3,
		DarnTable: func(loomshed.DarnTold) (seatengine.Table, error) {
			return seatengine.Table{
				RolePrefix: "darn",
				Segment:    segmentcolor.Darn,
				Seats: []seatengine.Seat{{
					Stencil: "darn-template-write",
					Outputs: []string{filepath.Join(dir, "darn-description.md")},
				}},
			}, nil
		},
		Darn: loomshed.DarnDeps{
			Commit:        func() error { return nil },
			LatestOutcome: func() (loomshed.DarnOutcome, bool, error) { return loomshed.DarnOutcome{}, false, nil },
		},
		Drift:     driftDeps(mustMkdir(t, filepath.Join(dir, "drift"))),
		PlanIndex: noFindingsIndex{},

		Slug:                     "test-slug",
		ReviewCirclingCheckpoint: 3,

		ScratchDir:     mustMkdir(t, filepath.Join(dir, "scratch")),
		CreateWorktree: func(context.Context) error { return nil },
		InnerRun: battenshed.InnerRunDeps{
			Spawn: func(context.Context) error { return nil },
			ResolveStatus: func() (string, string, error) {
				return filepath.Join(dir, "loomrun-status.json"), filepath.Join(dir, "loomrun-status.json.lock"), nil
			},
			ReadStatus: func(string, string) (shedengine.Status, bool, error) {
				return shedengine.Status{}, false, nil
			},
			ReadDecision: func() (battenshed.ChildDecision, bool, error) {
				return battenshed.ChildDecision{}, false, nil
			},
			DriverAlive: func(context.Context) (bool, error) { return false, nil },
			DriverStrand: func(context.Context) (battenshed.ChildDriverStrand, error) {
				return battenshed.ChildDriverNone, nil
			},
			ChildRunLockHeld: func() (bool, error) { return false, nil },
			ReviveStrands:    func(context.Context) error { return nil },
			ArbiterInFlight: func(context.Context) (battenshed.ChildArbiter, bool, error) {
				return battenshed.ChildArbiter{}, false, nil
			},
			PauseRequested:     func() (bool, error) { return false, nil },
			MarkWatched:        func(context.Context) (bool, error) { return false, nil },
			OrchStrandRecorded: func() (bool, error) { return true, nil },
			ParentLock:         &battenshed.ChildParentLock{},
			StopReport:         func() (string, time.Time, bool, error) { return "", time.Time{}, false, nil },
			HandBack:           func(shedengine.Status) bool { return false },
			Activity: func(context.Context) ([]battenshed.AgentActivity, bool, error) {
				return nil, false, nil
			},
		},
		Teardown: battenshed.TeardownDeps{
			Shutdown: func(context.Context) (string, error) { return "", nil },
			Remove:   func(context.Context) error { return nil },
		},
		Land: battenshed.LandDeps{
			ParentBranch: func() (string, error) { return "main", nil },
			PrimeBranch:  func() (string, error) { return "main", nil },
			PrimeDirty:   func() ([]string, error) { return nil, nil },
			Pull:         func(context.Context) (string, error) { return "", nil },
			Landing:      func() (battenshed.LandingSettings, error) { return battenshed.LandingSettings{}, nil },
			RunDeploy: func(context.Context, string, string, time.Duration) (battenshed.DeployResult, error) {
				return battenshed.DeployResult{}, nil
			},
			PruneBoard:     func() error { return nil },
			PauseRequested: func() (bool, error) { return false, nil },
			LandLock: battenshed.PrimeLock{
				Path: filepath.Join(dir, "land.lock"),
				Acquire: func() (func() error, bool, error) {
					return func() error { return nil }, true, nil
				},
			},
			Notices: battenshed.RowNoticeDeps{
				OrchStrandRecorded: func() (bool, error) { return true, nil },
				BattenHistoryLen:   func() (int, error) { return 0, nil },
			},
		},
		PrimeLock: battenshed.PrimeLock{
			Path: filepath.Join(dir, "prime.lock"),
			Acquire: func() (func() error, bool, error) {
				return func() error { return nil }, true, nil
			},
		},
		SeedChild: battenshed.SeedChildDeps{
			ReadBoardType:        func(context.Context) (string, error) { return "loom", nil },
			ChildDriver:          func(string) (string, error) { return "claude", nil },
			ReadRunSettings:      func(context.Context) (battenshed.RunSettings, error) { return battenshed.RunSettings{}, nil },
			PrepareRunSettings:   func(battenshed.RunSettings) (map[string][]byte, error) { return nil, nil },
			WriteLocalOverride:   func(context.Context, string, []byte) error { return nil },
			RemoveLocalOverrides: func(context.Context, []string) ([]string, error) { return nil, nil },
			WriteSeed:            func(context.Context, string, string) error { return nil },
			CommitSeed:           func(context.Context) error { return nil },
			PushSeed:             func(context.Context) error { return nil },
			MarkWatched:          func(context.Context) (bool, error) { return false, nil },
		},
		Command: shedadapters.CommandDeps{
			Lym:     "lym",
			Slug:    "fullenv-slug",
			TaskDir: func() (string, error) { return "", nil },
			Dir:     dir,
			LogPath: func(string) string { return "" },
			Run: func(context.Context, string, string, string, time.Duration) (shedadapters.CommandResult, error) {
				return shedadapters.CommandResult{}, nil
			},
			Rerun: "lym batten run fullenv-slug",
		},
	}
}

// driftDeps returns the Drift rows' seams over root with no-op seams, leaving the seat runner and the clock the registry entries fill from the Env.
func driftDeps(root string) driftengine.Deps {
	table := func() (seatengine.Table, error) { return seatengine.Table{}, nil }
	return driftengine.Deps{
		Paths:         driftengine.NewPaths(root),
		WorktreeRoot:  root,
		Config:        func() (driftengine.Config, error) { return driftengine.Config{}, nil },
		Status:        func() (shedengine.Status, error) { return shedengine.Status{}, nil },
		Head:          func() (string, error) { return "head", nil },
		BranchPoint:   func() (string, error) { return "branch-point", nil },
		DirtyPaths:    func() ([]string, error) { return nil, nil },
		Commit:        func([]string) error { return nil },
		BaselineTable: func(string) (seatengine.Table, error) { return table() },
		StartCapture:  func() error { return nil },
		RunCapture: func(context.Context) (driftengine.CaptureRecord, error) {
			return driftengine.CaptureRecord{}, nil
		},
		Sleep:       func(context.Context, time.Duration) error { return nil },
		VerifyTable: func(driftengine.VerifyTold) (seatengine.Table, error) { return table() },
		Compare: func(context.Context) (driftengine.CompareResult, error) {
			return driftengine.CompareResult{Passed: true}, nil
		},
	}
}

// fullEnvSegment returns a review segment's settings with empty models and the given bounce budget, resolved from review_max_bounces in configFile.
func fullEnvSegment(maxBounces int, configFile string) shedrecipe.SegmentSettings {
	return shedrecipe.SegmentSettings{
		MaxBounces:      maxBounces,
		MaxBouncesKey:   "review_max_bounces",
		ConfigFile:      configFile,
		AmendMaxBounces: 1,
	}
}

// NilSeams returns the dotted path of every func, interface, pointer, map and slice-of-func field of env that is nil, recursing into nested structs.
// It skips the paths in nilLegal, whose nil is a documented default.
// It reports and never asserts.
func NilSeams(env shedrecipe.Env) []string {
	var nils []string
	walkNil(reflect.ValueOf(env), "", &nils)
	return nils
}

func walkNil(v reflect.Value, prefix string, nils *[]string) {
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		path := field.Name
		if prefix != "" {
			path = prefix + "." + field.Name
		}
		if nilLegal[path] {
			continue
		}
		fv := v.Field(i)
		switch fv.Kind() {
		case reflect.Struct:
			walkNil(fv, path, nils)
		case reflect.Func, reflect.Interface, reflect.Pointer, reflect.Map:
			if fv.IsNil() {
				*nils = append(*nils, path)
			}
		case reflect.Slice:
			if fv.IsNil() && fv.Type().Elem().Kind() == reflect.Func {
				*nils = append(*nils, path)
			}
		}
	}
}

func mustMkdir(t testing.TB, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	return path
}

// ResolveGlyphs answers every glyph found with no files.
func (noFindingsIndex) ResolveGlyphs(_ string, glyphs []string) ([]planindex.GlyphAnswer, error) {
	answers := make([]planindex.GlyphAnswer, len(glyphs))
	for i, glyph := range glyphs {
		answers[i] = planindex.GlyphAnswer{Glyph: glyph, Verdict: planindex.GlyphFound}
	}
	return answers, nil
}
