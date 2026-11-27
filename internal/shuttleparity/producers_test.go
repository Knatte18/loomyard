// producers_test.go lists every launch-spec producer outside the seat tables and holds its Codex launch to Claude's.

package shuttleparity

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/frictionengine"
	"github.com/Knatte18/loomyard/internal/gatefixer"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomcli"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/samplerengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedarbiter"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/treadleengine"
	"github.com/Knatte18/loomyard/internal/weaverengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// producer is one production function that holds a launch-spec literal.
type producer struct {
	// Function is the glyph of the function holding the literal.
	Function string
	// Specs returns the producer's spec with its prompt rendered for, and launching on, engine.
	Specs func(engine string) (shuttleengine.Spec, error)
}

// producerSpec is what one producer's literal sets that decides what a session of it receives.
type producerSpec struct {
	function string
	// literal holds the literal's role, config role, fork and requirement fields and its posture.
	literal shuttleengine.Spec
	// languageSkills says the literal sets the hub's language skills.
	languageSkills bool
	render         func(engine string) (string, error)
}

// producers returns one entry per production function outside seatengine that holds a launch-spec literal.
// Each renders the producer's prompt on fx's stencils with stand-in inputs.
// Only a call of an entry's Specs reads fx.
func producers(fx parityFixture) []producer {
	env := fx.env
	worktree := func() string { return env.Location.WorktreePath() }
	standIn := func(name string) string { return filepath.Join(env.Dir, name) }
	batch := batcher.Batch{Cards: []planparser.Card{{Number: 1, Slug: "alpha", SourcePath: "_lym/plan/01-alpha.md"}}}
	judgeInputs := func(engine string) treadleengine.JudgePromptInputs {
		return treadleengine.JudgePromptInputs{
			Engine: engine, StencilsDir: env.StencilsDir, ParentName: env.ParentName, Round: 1, HardCap: 3,
			PriorReviews: []string{standIn("review.md")}, VerdictPath: standIn("verdict.md"), HandoffPath: standIn("handoff.md"),
		}
	}
	bouncer := func() (*shedadapters.Bouncer, error) {
		runDir := standIn("bouncer")
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			return nil, err
		}
		return shedadapters.NewBouncer(shedadapters.BouncerConfig{
			Name:               "stand-in",
			RunDir:             runDir,
			ArtifactPaths:      []string{standIn("artifact.md")},
			ReportName:         func(int) string { return "report.md" },
			StencilsDir:        env.StencilsDir,
			SpecsDir:           env.SpecsDir,
			RubricStencil:      "loom-rubric-code-review",
			ParentName:         env.ParentName,
			CirclingCheckpoint: 1,
			Shuttle:            struct{ shedadapters.Shuttle }{},
		})
	}

	entries := []producerSpec{
		{
			function: "internal/sheddriver#tacklerSpec",
			literal:  shuttleengine.Spec{Role: sheddriver.TacklerRole, ConfigRole: agentconfig.RoleTackler},
			render: func(engine string) (string, error) {
				return sheddriver.RenderTacklerPrompt(env.StencilsDir, engine, sheddriver.TacklerPromptInput{
					RunID: "run", Slug: env.Slug, Recipe: "a recipe", GotoRows: "none", Worktree: worktree(),
					EnvelopePath: standIn("envelope.json"), TraceCopy: standIn("trace"), StderrPath: standIn("stderr"),
					TraceDir: standIn("traces"), DriverLog: standIn("driver.log"), VerdictPath: standIn("verdict.md"),
					CaseBriefPath: standIn("case.md"),
				})
			},
		},
		{
			function: "internal/treadleengine#runJudgeCall",
			literal:  shuttleengine.Spec{Role: "judge", ConfigRole: agentconfig.RoleTreadleJudge},
			render: func(engine string) (string, error) {
				return treadleengine.RenderCirclingJudgePrompt(judgeInputs(engine))
			},
		},
		{
			function: "internal/treadleengine#runTargeting",
			literal:  shuttleengine.Spec{Role: "targeting", ConfigRole: agentconfig.RoleTreadleTargeting},
			render: func(engine string) (string, error) {
				return treadleengine.RenderTargetingPrompt(env.StencilsDir, env.ParentName, engine, 1, "", standIn("seed.md"))
			},
		},
		{
			function: "internal/websterengine#FixPlanSpawnOrAttach",
			literal:  shuttleengine.Spec{Role: "plan-fixer", ConfigRole: agentconfig.RoleWebster},
			render: func(engine string) (string, error) {
				prompt, err := websterengine.RenderPlanFixPrompt(standIn("conflict.json"), standIn("reply.md"), worktree(), standIn("plan"), env.StencilsDir, engine, env.SpecsDir, "", env.ParentName)
				return string(prompt), err
			},
		},
		{
			function:       "internal/websterengine#recoverSpawn",
			literal:        shuttleengine.Spec{Role: string(websterengine.RoleRecovery), ConfigRole: agentconfig.RoleWebster},
			languageSkills: true,
			render: func(engine string) (string, error) {
				prompt, err := websterengine.RenderRecoveryPrompt(batch, "- card gate", "- batch gate", "", "", "", standIn("report.yaml"), worktree(), standIn("plan"), worktree(), env.StencilsDir, engine, env.SpecsDir, "", nil, nil, 1, "", env.ParentName, false)
				return string(prompt), err
			},
		},
		{
			function: "internal/websterengine#Run",
			literal: shuttleengine.Spec{
				Role: websterengine.MerriamStrandRole, ConfigRole: agentconfig.RoleWebster, ForkSubagents: true,
				Requires: []shuttleengine.Requirement{shuttleengine.RequirementBackgroundWake},
			},
			languageSkills: true,
			render: func(engine string) (string, error) {
				prompt, err := websterengine.RenderMasterPrompt([]batcher.Batch{batch}, nil, standIn("outcome.yaml"), standIn("summary.md"), standIn("verify-fix.md"), standIn("plan"), 1, worktree(), worktree(), env.StencilsDir, engine, nil, nil, "", env.ParentName)
				return string(prompt), err
			},
		},
		{
			function: "internal/samplerengine#SessionSpec",
			literal:  shuttleengine.Spec{Role: samplerengine.Role, ConfigRole: agentconfig.RoleSampler, Interactive: true, DenyPublish: true},
			render: func(engine string) (string, error) {
				return samplerengine.RenderSessionPrompt(samplerengine.SessionInputs{
					StencilsDir: env.StencilsDir, ParentName: env.ParentName, Slug: env.Slug,
					WorktreeRoot: worktree(), DecisionRecordPath: standIn("decision-record.md"),
				}, engine)
			},
		},
		{
			function:       "internal/landingshed#PRReview.sessionSpec",
			literal:        shuttleengine.Spec{Role: "pr-reviewer", ConfigRole: agentconfig.RolePRReviewer, Interactive: true, DenyPublish: true},
			languageSkills: true,
			render: func(engine string) (string, error) {
				return landingshed.RenderPRReviewPrompt(landingshed.Deps{
					StencilsDir: env.StencilsDir, ParentName: env.ParentName, Slug: env.Slug, WorktreeRoot: worktree(),
					TaskBranch: "task", ParentBranch: "parent", DescriptionPath: standIn("description.md"),
				}, "https://example.invalid/pull/1", engine)
			},
		},
		{
			function: "internal/loomcli#driverSpec",
			literal: shuttleengine.Spec{
				Role: "driver", ConfigRole: agentconfig.RoleDriver, ForkSubagents: true,
				Requires: []shuttleengine.Requirement{shuttleengine.RequirementBackgroundWake},
			},
			render: func(engine string) (string, error) {
				return loomcli.RenderDriverPrompt(env.StencilsDir, env.ParentName, "run", standIn("drive-report.md"), false, engine)
			},
		},
		{
			function: "internal/dobbyengine#TidySpec",
			literal:  shuttleengine.Spec{Role: dobbyengine.BoardTidyRole, ConfigRole: agentconfig.RoleBoardTidy},
			render: func(engine string) (string, error) {
				return dobbyengine.TidyPrompt(env.StencilsDir, engine, dobbyengine.TidyPromptInput{Landed: []string{env.Slug}, ReportPath: standIn("tidy.md"), Parent: env.ParentName})
			},
		},
		{
			function:       "internal/mergeresolve#buildConflictSpec",
			literal:        shuttleengine.Spec{Role: mergeresolve.ConflictRole, ConfigRole: agentconfig.RoleConflictResolver},
			languageSkills: true,
			render: func(engine string) (string, error) {
				return mergeresolve.RenderConflictPrompt(mergeresolve.Deps{StencilsDir: env.StencilsDir, ParentName: env.ParentName}, []string{"file.go"}, standIn("conflict.md"), engine)
			},
		},
		{
			function: "internal/shedarbiter#Spec",
			literal:  shuttleengine.Spec{Role: loomengine.ArbiterRole, ConfigRole: agentconfig.RoleArbiter},
			render: func(engine string) (string, error) {
				return shedarbiter.Prompt(env.StencilsDir, engine, shedarbiter.PromptInput{
					RunID: "run", Slug: env.Slug, Worktree: worktree(),
					EnvelopePath: standIn("envelope.json"), VerdictPath: standIn("verdict.md"),
				})
			},
		},
		{
			function:       "internal/weaverengine#attemptSpec",
			literal:        shuttleengine.Spec{Role: weaverengine.BatchStrandRole, ConfigRole: agentconfig.RoleWebster},
			languageSkills: true,
			render: func(engine string) (string, error) {
				prompt, err := weaverengine.RenderBatchPrompt(weaverengine.BatchPromptInputs{
					StencilsDir: env.StencilsDir, Engine: engine, PromptsDir: standIn("prompts"), Label: "01-alpha", Attempt: 1,
					Cards: batch.Cards, PlanDirDisplay: "_lym/plan",
					Gate: gateslot.Config{
						Tiers: map[string]gateslot.Tier{"tier0": {About: "unit"}},
						Gates: map[string]gateslot.GateKey{
							"card":  {Tiers: []int{0}, Scope: "card"},
							"batch": {Tiers: []int{0}, Scope: "card"},
						},
					},
					RepoRoot: worktree(), DecisionRecordPath: standIn("decision-record.md"),
					JournalPath: standIn("journal.md"), ReportPath: standIn("report.yaml"), ParentName: env.ParentName,
				})
				return string(prompt), err
			},
		},
		{
			function: "internal/shedadapters#Bouncer.runSeedSpawn",
			literal:  shuttleengine.Spec{Role: "bouncer-seed", ConfigRole: agentconfig.RoleBouncerSeed},
			render: func(engine string) (string, error) {
				b, err := bouncer()
				if err != nil {
					return "", err
				}
				return b.RenderSeedPrompt(engine, standIn("focus.yaml"))
			},
		},
		{
			function: "internal/shedadapters#Bouncer.judgeCall",
			literal:  shuttleengine.Spec{Role: "bouncer-judge", ConfigRole: agentconfig.RoleBouncerJudge},
			render: func(engine string) (string, error) {
				b, err := bouncer()
				if err != nil {
					return "", err
				}
				return b.RenderJudgePrompt(engine, 1)
			},
		},
		{
			function:       "internal/gatefixer#SpawnOrAttach",
			literal:        shuttleengine.Spec{Role: gatefixer.RoleGateFixer, ConfigRole: agentconfig.RoleGateFixer},
			languageSkills: true,
			render: func(engine string) (string, error) {
				prompt, err := gatefixer.RenderPrompt(gatefixer.PromptInputs{
					StencilsDir: env.StencilsDir, Engine: engine, BodyStencil: "weaver-body-verify-fix",
					BodyValues: map[string]string{
						"findings": "a failure", "journal_path": standIn("journal.md"),
						"plan_dir": standIn("plan"), "worktree_root": worktree(),
					},
					ReportPath: standIn("report.md"), ReplyPath: standIn("reply.md"),
					RepoRoot: worktree(), ParentName: env.ParentName,
				})
				return string(prompt), err
			},
		},
		{
			function: "internal/frictionengine#buildReflectionSpec",
			literal:  shuttleengine.Spec{Role: "friction", ConfigRole: agentconfig.RoleFriction, Timeout: time.Hour},
			render: func(engine string) (string, error) {
				return frictionengine.RenderReflectionPrompt(frictionengine.Deps{
					StencilsDir: env.StencilsDir, ParentName: env.ParentName, FrictionDir: standIn("friction"), TaskSlug: env.Slug,
				}, []string{"note.md"}, standIn("reflection.md"), engine)
			},
		},
	}

	var list []producer
	for _, entry := range entries {
		list = append(list, producer{Function: entry.function, Specs: func(engine string) (shuttleengine.Spec, error) {
			spec := entry.literal
			if entry.languageSkills {
				skills, err := languageSkillsOf(env)
				if err != nil {
					return shuttleengine.Spec{}, err
				}
				spec.Skills = slices.Clone(skills)
			}
			prompt, err := entry.render(engine)
			if err != nil {
				return shuttleengine.Spec{}, err
			}
			spec.Prompt, spec.PromptEngine, spec.ModelChoice = prompt, engine, shuttleengine.ModelChoice{Engine: engine}
			return spec, nil
		}})
	}
	return list
}

// TestProducers_CodexManifestHoldsClaudes pins Codex's launch against Claude's.
// It covers every producer outside the seat tables.
// Each compared item Claude's launch gets, Codex's gets too.
// It sets CLAUDE_CONFIG_DIR and HOME and installs the process passage reader, so it does not run in parallel.
//
//lym:guard
func TestProducers_CodexManifestHoldsClaudes(t *testing.T) {
	tiergate.Require(t, 0)

	fx := newParityFixture(t)
	cfg, err := shuttleengine.LoadConfig(fx.env.Dirs, "shuttle")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	list := producers(fx)
	specs := make([]map[string]shuttleengine.Spec, len(list))
	for i, p := range list {
		specs[i] = map[string]shuttleengine.Spec{}
		for _, engine := range []string{ReferenceEngine, ComparedEngine} {
			spec, err := p.Specs(engine)
			if err != nil {
				t.Fatalf("producer %s: %v", p.Function, err)
			}
			specs[i][engine] = spec
		}
	}
	fx.addSkillsOfSpecs(t, cfg, specs)

	set, err := shuttleengines.NewWithSeams(cfg, fx.env.Dirs, shuttleengines.LaunchSeams{RunHook: UnrunHooks, SandboxProbe: ConfiguredSandbox})
	if err != nil {
		t.Fatalf("NewWithSeams: %v", err)
	}
	for i, p := range list {
		manifests, err := PrepareEach(set, cfg, fx.env.StencilsDir, t.TempDir(), specs[i])
		if err != nil {
			t.Errorf("producer %s: %v", p.Function, err)
			continue
		}
		for _, item := range Compare(set, manifests) {
			t.Errorf("producer %s: Codex's launch lacks %s %q; %s", p.Function, item.Channel, item.Key, codexDeliveryWayForward)
		}
	}
}
