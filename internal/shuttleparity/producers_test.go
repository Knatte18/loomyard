// producers_test.go pins the Codex manifest of every launch-spec producer outside the seat tables.
// Each producer's Codex manifest must hold what Claude's holds.
// It also scans production code for a launch-spec literal, seat table or seat that the parity tests do not list.

package shuttleparity

import (
	"fmt"
	"go/ast"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/burlerengine"
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
	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weaverengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// The strand roles that production launch literals spell as unexported constants of their own packages.
const (
	planFixerStrandRole = "plan-fixer"
)

// The stencil names and the plan directory spelling the stand-in prompts render from.
const (
	standInRubricStencil  = "loom-rubric-code-review"
	standInGateFixerBody  = "gatefix-body-card"
	standInPlanDisplayDir = "_lym/plan"
)

// The glyphs of the functions that hold a launch-spec, seat table or seat literal.
// No producer entry or seat table builds these on its own.
const (
	seatSpecGlyph      = "internal/seatengine#seatSpec"
	multiLLMEntryGlyph = "internal/shedrecipe#multiLLMEntry"
	multiLLMSeatGlyph  = "internal/shedrecipe#multiLLMSeat"
)

// The import path suffixes of the packages that declare the literals the scan looks for.
const (
	shuttleengineImportSuffix = "/internal/shuttleengine"
	seatengineImportSuffix    = "/internal/seatengine"
)

// The way forward each kind of unlisted literal ends its message with.
const (
	specWayForward  = "way forward: add the function to producers in internal/shuttleparity, or, for a probe, set no Prompt"
	tableWayForward = "way forward: give a new table builder its own seatTables entry with Builder, Holders and Build, so its seats reach the parity test, and add a helper that holds an existing builder's literal to that builder's Holders, with a Build that reaches the seat it adds"
	seatWayForward  = tableWayForward
)

// skippedPlaces are the directories whose literals the scan leaves out.
// The orch spec names the Claude engine, `lym shuttle run` is hand-written and `lym shuttle open` carries no role.
var skippedPlaces = []string{"internal/orchcli/", "internal/orchengine/", "internal/shuttlecli/", "internal/testkit/"}

// producer is one production function that holds a launch-spec literal outside the seat tables.
type producer struct {
	// Function is the glyph of the function that holds the literal.
	Function string
	// Specs returns the spec the producer launches with on engine, its prompt rendered for that engine.
	Specs func(engine string) (shuttleengine.Spec, error)
}

// producerBuild builds the spec of one producer on engine from the fixture's stand-ins.
type producerBuild func(fx parityFixture, engine string) (shuttleengine.Spec, error)

// producers returns one entry per function outside the seat tables that holds a launch-spec literal.
// Each entry renders the producer's prompt through the producer's own render function on the fixture's seeded stencils.
// It builds the spec with the literal's role, config role, skills, ForkSubagents and Requires.
func producers(t *testing.T, fx parityFixture) []producer {
	t.Helper()
	builds := []struct {
		function string
		build    producerBuild
	}{
		{"internal/sheddriver#tacklerSpec", buildTackler},
		{"internal/websterengine#FixPlanSpawnOrAttach", buildPlanFixer},
		{"internal/websterengine#recoverSpawn", buildRecovery},
		{"internal/websterengine#Run", buildMerriam},
		{"internal/samplerengine#SessionSpec", buildSampler},
		{"internal/landingshed#PRReview.sessionSpec", buildPRReview},
		{"internal/loomcli#driverSpec", buildDriver},
		{"internal/dobbyengine#TidySpec", buildBoardTidy},
		{"internal/dobbyengine#BeamDescribeSpec", buildBeamDescribe},
		{"internal/mergeresolve#buildConflictSpec", buildConflict},
		{"internal/shedarbiter#Spec", buildArbiter},
		{"internal/weaverengine#attemptSpec", buildWeaverAttempt},
		{"internal/shedadapters#Bouncer.runSeedSpawn", buildBouncerSeed},
		{"internal/shedadapters#Bouncer.judgeCall", buildBouncerJudge},
		{"internal/shedadapters#Bouncer.runVerifier", buildBouncerVerify},
		{"internal/gatefixer#SpawnOrAttach", buildGateFixer},
		{"internal/frictionengine#buildReflectionSpec", buildReflection},
	}
	all := make([]producer, len(builds))
	for i, entry := range builds {
		all[i] = producer{
			Function: entry.function,
			Specs: func(engine string) (shuttleengine.Spec, error) {
				spec, err := entry.build(fx, engine)
				if err != nil {
					return shuttleengine.Spec{}, fmt.Errorf("shuttleparity: the spec of producer %s on %s: %w", entry.function, engine, err)
				}
				return spec, nil
			},
		}
	}
	return all
}

// promptSpec returns spec with the prompt rendered for engine and a model choice naming the engine alone.
func promptSpec(engine, prompt string, spec shuttleengine.Spec) shuttleengine.Spec {
	spec.Prompt = prompt
	spec.PromptEngine = engine
	spec.ModelChoice = shuttleengine.ModelChoice{Engine: engine}
	return spec
}

// standInCard is the one card of the stand-in batch the webster and weaver prompts render.
var standInCard = planparser.Card{Number: 1, Slug: "stand-in", Summary: "the stand-in card", SourcePath: "_lym/plan/01-stand-in.md"}

// standInGateConfig lists the `card` and `batch` gate keys the weaver prompt's gate calls render from.
func standInGateConfig() gateslot.Config {
	return gateslot.Config{
		Tiers: map[string]gateslot.Tier{"tier0": {About: "unit"}, "tier1": {About: "integration"}},
		Gates: map[string]gateslot.GateKey{
			"card":  {Tiers: []int{0}, Scope: "card"},
			"batch": {Tiers: []int{0, 1}, Scope: "card"},
		},
	}
}

func buildTackler(fx parityFixture, engine string) (shuttleengine.Spec, error) {
	env := fx.env
	prompt, err := sheddriver.RenderTacklerPrompt(env.StencilsDir, engine, sheddriver.TacklerPromptInput{
		RunID:         "run",
		Slug:          env.Slug,
		Recipe:        "the stand-in recipe",
		GotoRows:      "stand-in-row",
		Worktree:      env.Location.WorktreePath(),
		EnvelopePath:  standInPath(env, "envelope.json"),
		ReportPath:    standInPath(env, "report.md"),
		TraceCopy:     standInPath(env, "trace-copy"),
		StderrPath:    standInPath(env, "stderr.log"),
		TraceDir:      standInPath(env, "trace"),
		DriverLog:     standInPath(env, "driver.log"),
		VerdictPath:   standInPath(env, "verdict.yaml"),
		CaseBriefPath: standInPath(env, "case-brief.md"),
	})
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	return promptSpec(engine, prompt, shuttleengine.Spec{
		ConfigRole: agentconfig.RoleTackler,
		Role:       sheddriver.TacklerRole,
	}), nil
}

func buildPlanFixer(fx parityFixture, engine string) (shuttleengine.Spec, error) {
	env := fx.env
	prompt, err := websterengine.RenderPlanFixPrompt(standInPath(env, "conflict-record.json"), standInPath(env, "plan-fix-reply.md"),
		env.Location.WorktreePath(), standInPath(env, "plan"), env.StencilsDir, engine, env.SpecsDir, "", env.ParentName)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	return promptSpec(engine, string(prompt), shuttleengine.Spec{
		ConfigRole: agentconfig.RoleWebster,
		Role:       planFixerStrandRole,
	}), nil
}

func buildRecovery(fx parityFixture, engine string) (shuttleengine.Spec, error) {
	env := fx.env
	skills, err := languageSkillsOf(env)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	worktree := env.Location.WorktreePath()
	prompt, err := websterengine.RenderRecoveryPrompt(batcher.Batch{Cards: []planparser.Card{standInCard}}, "the card gates", "the batch gate", "", "", "", "",
		standInPath(env, "report.yaml"), worktree, standInPath(env, "plan"), worktree, env.StencilsDir, engine, env.SpecsDir, "", nil, nil, 2, "", env.ParentName, false)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	return promptSpec(engine, string(prompt), shuttleengine.Spec{
		ConfigRole: agentconfig.RoleWebster,
		Skills:     skills,
		Role:       string(websterengine.RoleRecovery),
	}), nil
}

func buildMerriam(fx parityFixture, engine string) (shuttleengine.Spec, error) {
	env := fx.env
	skills, err := languageSkillsOf(env)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	worktree := env.Location.WorktreePath()
	prompt, err := websterengine.RenderMasterPrompt([]batcher.Batch{{Cards: []planparser.Card{standInCard}}}, nil,
		standInPath(env, "outcome.yaml"), standInPath(env, "summary.md"), standInPath(env, "verify-fix.md"), standInPath(env, "plan"), 2,
		worktree, worktree, env.StencilsDir, engine, nil, nil, "", env.ParentName)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	return promptSpec(engine, string(prompt), shuttleengine.Spec{
		ConfigRole:    agentconfig.RoleWebster,
		Skills:        skills,
		ForkSubagents: true,
		Requires:      []shuttleengine.Requirement{shuttleengine.RequirementBackgroundWake},
		Role:          websterengine.MerriamStrandRole,
	}), nil
}

func buildSampler(fx parityFixture, engine string) (shuttleengine.Spec, error) {
	env := fx.env
	skills, err := languageSkillsOf(env)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	prompt, err := samplerengine.RenderSessionPrompt(samplerengine.SessionInputs{
		StencilsDir:        env.StencilsDir,
		ParentName:         env.ParentName,
		Slug:               env.Slug,
		WorktreeRoot:       env.Location.WorktreePath(),
		DecisionRecordPath: standInPath(env, "decision-record.md"),
	}, engine)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	return promptSpec(engine, prompt, shuttleengine.Spec{
		Interactive: true,
		Unawaited:   true,
		DenyPublish: true,
		ConfigRole:  agentconfig.RoleSampler,
		Skills:      skills,
		Role:        samplerengine.Role,
	}), nil
}

func buildPRReview(fx parityFixture, engine string) (shuttleengine.Spec, error) {
	env := fx.env
	skills, err := languageSkillsOf(env)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	prompt, err := landingshed.RenderPRReviewPrompt(landingshed.Deps{
		WorktreeRoot:       env.Location.WorktreePath(),
		TaskBranch:         "task",
		ParentBranch:       "parent",
		DescriptionPath:    standInPath(env, "description.md"),
		StencilsDir:        env.StencilsDir,
		ParentName:         env.ParentName,
		Slug:               env.Slug,
		DecisionRecordPath: standInPath(env, "decision-record.md"),
		PlanDir:            standInPath(env, "plan"),
	}, "https://example.test/pull/1", engine, false)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	return promptSpec(engine, prompt, shuttleengine.Spec{
		Interactive: true,
		Unawaited:   true,
		DenyPublish: true,
		ConfigRole:  agentconfig.RolePRReviewer,
		Skills:      skills,
		Role:        string(agentconfig.RolePRReviewer),
	}), nil
}

func buildDriver(fx parityFixture, engine string) (shuttleengine.Spec, error) {
	env := fx.env
	prompt, err := loomcli.RenderDriverPrompt(env.StencilsDir, env.ParentName, "run", standInPath(env, "driver-report.md"), false, engine)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	return promptSpec(engine, prompt, shuttleengine.Spec{
		ConfigRole:    agentconfig.RoleDriver,
		ForkSubagents: true,
		Requires:      []shuttleengine.Requirement{shuttleengine.RequirementBackgroundWake},
		Role:          string(agentconfig.RoleDriver),
	}), nil
}

func buildBoardTidy(fx parityFixture, engine string) (shuttleengine.Spec, error) {
	env := fx.env
	prompt, err := dobbyengine.TidyPrompt(env.StencilsDir, engine, dobbyengine.TidyPromptInput{
		Landed:     []string{env.Slug},
		ReportPath: standInPath(env, "tidy-report.md"),
		Parent:     env.ParentName,
	})
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	return promptSpec(engine, prompt, shuttleengine.Spec{
		ConfigRole: agentconfig.RoleBoardTidy,
		Role:       dobbyengine.BoardTidyRole,
	}), nil
}

func buildBeamDescribe(fx parityFixture, engine string) (shuttleengine.Spec, error) {
	env := fx.env
	prompt, err := dobbyengine.BeamDescribePrompt(env.StencilsDir, engine, dobbyengine.BeamDescribePromptInput{
		Slug:      env.Slug,
		DraftPath: standInPath(env, "describe-draft.md"),
		Parent:    env.ParentName,
		Facts: dobbyengine.BeamDescribeFacts{
			DescriptionPath: standInPath(env, "summary.md"),
			Branch:          "beam-stand-in",
			ParentBranch:    "main",
			Changes:         "- stand-in change",
		},
	})
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	return promptSpec(engine, prompt, shuttleengine.Spec{
		ConfigRole: agentconfig.RoleDescriber,
		Role:       dobbyengine.BeamDescribeRole,
	}), nil
}

func buildConflict(fx parityFixture, engine string) (shuttleengine.Spec, error) {
	env := fx.env
	skills, err := languageSkillsOf(env)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	prompt, err := mergeresolve.RenderConflictPrompt(mergeresolve.Deps{StencilsDir: env.StencilsDir, ParentName: env.ParentName},
		[]string{"conflicted.txt"}, standInPath(env, "conflict-report.md"), engine)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	return promptSpec(engine, prompt, shuttleengine.Spec{
		ConfigRole: agentconfig.RoleConflictResolver,
		Skills:     skills,
		Role:       mergeresolve.ConflictRole,
	}), nil
}

func buildArbiter(fx parityFixture, engine string) (shuttleengine.Spec, error) {
	env := fx.env
	prompt, err := shedarbiter.Prompt(env.StencilsDir, engine, shedarbiter.PromptInput{
		RunID:        "run",
		Slug:         env.Slug,
		Worktree:     env.Location.WorktreePath(),
		EnvelopePath: standInPath(env, "envelope.json"),
		VerdictPath:  standInPath(env, "verdict.yaml"),
	})
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	return promptSpec(engine, prompt, shuttleengine.Spec{
		ConfigRole: agentconfig.RoleArbiter,
		Role:       loomengine.ArbiterRole,
	}), nil
}

func buildWeaverAttempt(fx parityFixture, engine string) (shuttleengine.Spec, error) {
	env := fx.env
	skills, err := languageSkillsOf(env)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	promptsDir := standInPath(env, "weaver-prompts")
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		return shuttleengine.Spec{}, err
	}
	prompt, err := weaverengine.RenderBatchPrompt(weaverengine.BatchPromptInputs{
		StencilsDir:        env.StencilsDir,
		Engine:             engine,
		PromptsDir:         promptsDir,
		Label:              "01-stand-in",
		Attempt:            1,
		Cards:              []planparser.Card{standInCard},
		PlanDirDisplay:     standInPlanDisplayDir,
		Gate:               standInGateConfig(),
		LastBatch:          true,
		RepoRoot:           env.Location.WorktreePath(),
		DecisionRecordPath: standInPath(env, "decision-record.md"),
		JournalPath:        standInPath(env, "journal.md"),
		ReportPath:         standInPath(env, "weaver-report.yaml"),
		ParentName:         env.ParentName,
	})
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	return promptSpec(engine, string(prompt), shuttleengine.Spec{
		ConfigRole: agentconfig.RoleWebster,
		Skills:     skills,
		Role:       weaverengine.BatchStrandRole,
	}), nil
}

// standInBouncer returns a bouncer whose seed and judge prompts render from the fixture's stand-ins.
// It runs no shuttle, so its seam is a nil runner that no render reaches.
func standInBouncer(env SeatEnv) (*shedadapters.Bouncer, error) {
	runDir := standInPath(env, "bouncer-run")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return nil, err
	}
	return shedadapters.NewBouncer(shedadapters.BouncerConfig{
		Name:               "stand-in",
		RunDir:             runDir,
		ArtifactPaths:      []string{standInPath(env, "artifact.md")},
		ReportName:         func(round int) string { return fmt.Sprintf("round-%d-review.md", round) },
		StencilsDir:        env.StencilsDir,
		WorktreeRoot:       env.Location.WorktreePath(),
		SpecsDir:           env.SpecsDir,
		RubricStencil:      standInRubricStencil,
		Shuttle:            (*shuttleengine.Runner)(nil),
		CirclingCheckpoint: 3,
		HardCap:            5,
		Slug:               env.Slug,
		ParentName:         env.ParentName,
	})
}

func buildBouncerSeed(fx parityFixture, engine string) (shuttleengine.Spec, error) {
	env := fx.env
	bouncer, err := standInBouncer(env)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	prompt, err := bouncer.RenderSeedPrompt(engine, standInPath(env, "focus.md"))
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	return promptSpec(engine, prompt, shuttleengine.Spec{
		ConfigRole: agentconfig.RoleBouncerSeed,
		Role:       string(agentconfig.RoleBouncerSeed),
	}), nil
}

func buildBouncerJudge(fx parityFixture, engine string) (shuttleengine.Spec, error) {
	bouncer, err := standInBouncer(fx.env)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	prompt, err := bouncer.RenderJudgePrompt(engine, 1)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	return promptSpec(engine, prompt, shuttleengine.Spec{
		ConfigRole: agentconfig.RoleBouncerJudge,
		Role:       string(agentconfig.RoleBouncerJudge),
	}), nil
}

func buildBouncerVerify(fx parityFixture, engine string) (shuttleengine.Spec, error) {
	bouncer, err := standInBouncer(fx.env)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	fixed := []burlerengine.Finding{{ID: "F1", Severity: burlerengine.SeverityBlocking, Summary: "stand-in finding", Fix: "stand-in fix"}}
	prompt, err := bouncer.RenderVerifyPrompt(engine, 1, []string{"stand-in-key"}, fixed)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	return promptSpec(engine, prompt, shuttleengine.Spec{
		ConfigRole: agentconfig.RoleBouncerJudge,
		Role:       "bouncer-verify",
	}), nil
}

func buildGateFixer(fx parityFixture, engine string) (shuttleengine.Spec, error) {
	env := fx.env
	skills, err := languageSkillsOf(env)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	worktree := env.Location.WorktreePath()
	prompt, err := gatefixer.RenderPrompt(gatefixer.PromptInputs{
		StencilsDir: env.StencilsDir,
		BodyStencil: standInGateFixerBody,
		BodyValues: map[string]string{
			"gate_call":     "lym gate test --gate card --card 1-stand-in",
			"card_ids":      "1-stand-in",
			"worktree_root": worktree,
		},
		ReportPath: standInPath(env, "gate-report.md"),
		ReplyPath:  standInPath(env, "gate-fixer-reply.md"),
		RepoRoot:   worktree,
		ParentName: env.ParentName,
		Engine:     engine,
	})
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	return promptSpec(engine, string(prompt), shuttleengine.Spec{
		ConfigRole: agentconfig.RoleGateFixer,
		Skills:     skills,
		Role:       gatefixer.RoleGateFixer,
	}), nil
}

func buildReflection(fx parityFixture, engine string) (shuttleengine.Spec, error) {
	env := fx.env
	prompt, err := frictionengine.RenderReflectionPrompt(frictionengine.Deps{
		StencilsDir: env.StencilsDir,
		ParentName:  env.ParentName,
		FrictionDir: standInPath(env, "friction"),
		TaskSlug:    env.Slug,
	}, []string{"stand-in-note.md"}, standInPath(env, "reflection-report.md"), engine)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	return promptSpec(engine, prompt, shuttleengine.Spec{
		ConfigRole: agentconfig.RoleFriction,
		Role:       string(agentconfig.RoleFriction),
	}), nil
}

// TestProducers_CodexManifestHoldsClaudes prepares every launch-spec producer outside the seat tables on both engines.
// It fails for each producer that cannot render on its stand-ins.
// It fails for each compared item of Claude's manifest that Codex's lacks.
// It covers a producer Codex cannot start too, since Prepare refuses no requirement.
// It sets CLAUDE_CONFIG_DIR and HOME and installs the process passage reader, so it does not run in parallel.
//
//lym:guard
func TestProducers_CodexManifestHoldsClaudes(t *testing.T) {
	tiergate.Require(t, 0)

	fx := newParityFixture(t)
	all := producers(t, fx)
	engines := []string{ReferenceEngine, ComparedEngine}
	specsOf := make([]map[string]shuttleengine.Spec, len(all))
	var skills []string
	for i, entry := range all {
		specsOf[i] = map[string]shuttleengine.Spec{}
		for _, engine := range engines {
			spec, err := entry.Specs(engine)
			if err != nil {
				t.Fatalf("producer %s: %v", entry.Function, err)
			}
			specsOf[i][engine] = spec
			if spec.ConfigRole != "" {
				roleSkills, err := fx.cfg.Agents.Skills(spec.ConfigRole, spec.Skills)
				if err != nil {
					t.Fatalf("producer %s: expand the skills of role %q: %v", entry.Function, spec.ConfigRole, err)
				}
				skills = append(skills, roleSkills...)
			} else {
				skills = append(skills, spec.Skills...)
			}
		}
	}
	slices.Sort(skills)
	fx.addSkills(t, slices.Compact(skills))

	set, err := shuttleengines.NewWithSeams(fx.cfg, fx.env.Dirs, shuttleengines.LaunchSeams{RunHook: UnrunHooks, SandboxProbe: ConfiguredSandbox})
	if err != nil {
		t.Fatalf("NewWithSeams() error: %v", err)
	}
	for i, entry := range all {
		t.Run(entry.Function, func(t *testing.T) {
			manifests, err := PrepareEach(set, specsOf[i], fx.stencilsDir, t.TempDir(), fx.cfg)
			if err != nil {
				t.Fatalf("producer %s: %v", entry.Function, err)
			}
			for _, missing := range Compare(set, manifests) {
				t.Errorf("producer %s: the Codex manifest lacks %s %q that Claude's holds; way forward: give the Codex engine a delivery for it in internal/shuttleengine/codexengine",
					entry.Function, missing.Channel, missing.Key)
			}
		})
	}
}

// unlistedLiterals returns "file:line: <glyph>: <way forward>" for each launch-spec, seat table or seat literal.
// It scans the files under base.
// It reports a literal only when its enclosing function's glyph is not in listed.
// It also returns the count of files it scanned.
// A glyph is `<dir>#<Function>`, or `<dir>#<Receiver>.<Method>` for a method.
// A literal in a function literal takes its enclosing named function's glyph.
// A Spec literal that sets no Prompt, an empty seatengine.Table and a seatengine.Seat that sets no Stencil are skipped.
// So are the files of skippedPlaces.
// The scan is syntactic: it sees a literal of the package-qualified type, not a value filled field by field.
func unlistedLiterals(t *testing.T, base string, listed map[string]bool) ([]string, int) {
	t.Helper()
	var unlisted []string
	scanned := scankit.Walk(t, scankit.Options{Base: base}, func(f *scankit.File) {
		if slices.ContainsFunc(skippedPlaces, func(place string) bool { return strings.HasPrefix(f.Rel, place) }) {
			return
		}
		file := f.AST(t, 0)
		specQualifier := importQualifier(file, shuttleengineImportSuffix)
		seatQualifier := importQualifier(file, seatengineImportSuffix)
		if specQualifier == "" && seatQualifier == "" {
			return
		}
		dir := path.Dir(f.Rel)
		for _, decl := range file.Decls {
			glyph := dir + "#" + declName(decl)
			ast.Inspect(decl, func(node ast.Node) bool {
				literal, ok := node.(*ast.CompositeLit)
				if !ok {
					return true
				}
				wayForward := wayForwardOf(literal, specQualifier, seatQualifier)
				if wayForward == "" || listed[glyph] {
					return true
				}
				line := f.FileSet().Position(literal.Pos()).Line
				unlisted = append(unlisted, fmt.Sprintf("%s:%d: %s: %s", f.Rel, line, glyph, wayForward))
				return true
			})
		}
	})
	return unlisted, scanned
}

// importQualifier returns the name file imports the package whose path ends in suffix under, or "" when it does not.
func importQualifier(file *ast.File, suffix string) string {
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		if !strings.HasSuffix(importPath, suffix) {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return path.Base(importPath)
	}
	return ""
}

// declName returns the name a top-level declaration's glyph ends in.
// That is the function name, `<Receiver>.<Method>` for a method, or "" for a non-function.
func declName(decl ast.Decl) string {
	function, ok := decl.(*ast.FuncDecl)
	if !ok {
		return ""
	}
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return function.Name.Name
	}
	receiver := function.Recv.List[0].Type
	if star, ok := receiver.(*ast.StarExpr); ok {
		receiver = star.X
	}
	if generic, ok := receiver.(*ast.IndexExpr); ok {
		receiver = generic.X
	}
	if name, ok := receiver.(*ast.Ident); ok {
		return name.Name + "." + function.Name.Name
	}
	return function.Name.Name
}

// wayForwardOf returns the way forward for a literal the parity tests must list, or "" for any other literal.
func wayForwardOf(literal *ast.CompositeLit, specQualifier, seatQualifier string) string {
	keys := map[string]bool{}
	for _, element := range literal.Elts {
		if pair, ok := element.(*ast.KeyValueExpr); ok {
			if key, ok := pair.Key.(*ast.Ident); ok {
				keys[key.Name] = true
			}
		}
	}
	switch {
	case isQualified(literal.Type, specQualifier, "Spec") && keys["Prompt"]:
		return specWayForward
	case isQualified(literal.Type, seatQualifier, "Table") && len(literal.Elts) > 0:
		return tableWayForward
	case isQualified(literal.Type, seatQualifier, "Seat") && keys["Stencil"]:
		return seatWayForward
	}
	return ""
}

// isQualified reports whether expr is the type qualifier.name.
func isQualified(expr ast.Expr, qualifier, name string) bool {
	if qualifier == "" {
		return false
	}
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != name {
		return false
	}
	receiver, ok := selector.X.(*ast.Ident)
	return ok && receiver.Name == qualifier
}

// realModuleListed returns the glyphs of the functions the parity tests cover.
func realModuleListed(t *testing.T) map[string]bool {
	t.Helper()
	listed := map[string]bool{seatSpecGlyph: true, multiLLMEntryGlyph: true, multiLLMSeatGlyph: true}
	for _, entry := range producers(t, parityFixture{}) {
		listed[entry.Function] = true
	}
	for _, table := range seatTables {
		listed[table.Builder] = true
		for _, holder := range table.Holders {
			listed[holder] = true
		}
	}
	return listed
}

// TestLaunchSpecLiterals_AreListed pins that the parity tests list the function of each production literal of a kind:
// a launch-spec that sets a prompt, a non-empty seat table, and a seat that sets a stencil.
// Its rows run the scan over temp-dir sources, and the RealModule row runs it over the module.
//
//lym:guard
func TestLaunchSpecLiterals_AreListed(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const header = "package x\n\nimport (\n\t\"github.com/Knatte18/loomyard/internal/shuttleengine\"\n\t\"github.com/Knatte18/loomyard/internal/seatengine\"\n)\n\n"
	// bodyLine is the line a one-line body of a row sits on.
	const bodyLine = 8
	specBody := `func build() shuttleengine.Spec { return shuttleengine.Spec{Prompt: "p"} }`
	finding := func(rel, glyph, wayForward string) string {
		return fmt.Sprintf("%s:%d: %s: %s", rel, bodyLine, glyph, wayForward)
	}
	tests := []struct {
		name   string
		rel    string
		body   string
		listed []string
		want   []string
	}{
		{name: "UnlistedSpecSettingPrompt", rel: "internal/x/x.go", body: specBody, want: []string{finding("internal/x/x.go", "internal/x#build", specWayForward)}},
		{name: "ListedSpecIsClear", rel: "internal/x/x.go", body: specBody, listed: []string{"internal/x#build"}},
		{name: "SpecSettingNoPromptIsSkipped", rel: "internal/x/x.go", body: `func build() shuttleengine.Spec { return shuttleengine.Spec{OutputFiles: nil} }`},
		{name: "ShuttleCLIIsSkipped", rel: "internal/shuttlecli/x.go", body: specBody},
		{name: "TestkitIsSkipped", rel: "internal/testkit/x/x.go", body: specBody},
		{name: "OrchCLIIsSkipped", rel: "internal/orchcli/x.go", body: specBody},
		{name: "OrchEngineIsSkipped", rel: "internal/orchengine/x.go", body: specBody},
		{
			name: "UnlistedNonEmptyTable", rel: "internal/x/x.go",
			body: `func build() seatengine.Table { return seatengine.Table{RolePrefix: "p"} }`,
			want: []string{finding("internal/x/x.go", "internal/x#build", tableWayForward)},
		},
		{name: "EmptyTableIsSkipped", rel: "internal/x/x.go", body: `func build() seatengine.Table { return seatengine.Table{} }`},
		{
			name: "UnlistedPointerSeatSettingStencil", rel: "internal/x/x.go",
			body: `func build() *seatengine.Seat { return &seatengine.Seat{Stencil: "s"} }`,
			want: []string{finding("internal/x/x.go", "internal/x#build", seatWayForward)},
		},
		{name: "SeatSettingNoStencilIsSkipped", rel: "internal/x/x.go", body: `func build() seatengine.Seat { return seatengine.Seat{} }`},
		{
			name: "MethodGlyphNamesItsReceiver", rel: "internal/x/x.go",
			body: `func (b *Builder) build() shuttleengine.Spec { return shuttleengine.Spec{Prompt: "p"} }`,
			want: []string{finding("internal/x/x.go", "internal/x#Builder.build", specWayForward)},
		},
		{
			name: "FunctionLiteralTakesTheEnclosingGlyph", rel: "internal/x/x.go",
			body: `func build() { _ = func() shuttleengine.Spec { return shuttleengine.Spec{Prompt: "p"} } }`,
			want: []string{finding("internal/x/x.go", "internal/x#build", specWayForward)},
		},
		{
			name: "ListingTheEnclosingGlyphClearsAFunctionLiteral", rel: "internal/x/x.go",
			body:   `func build() { _ = func() shuttleengine.Spec { return shuttleengine.Spec{Prompt: "p"} } }`,
			listed: []string{"internal/x#build"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			base := t.TempDir()
			abs := filepath.Join(base, filepath.FromSlash(tt.rel))
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(abs, []byte(header+tt.body+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			listed := map[string]bool{}
			for _, glyph := range tt.listed {
				listed[glyph] = true
			}
			got, _ := unlistedLiterals(t, base, listed)
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("unlistedLiterals() = %q; want %q", got, tt.want)
			}
		})
	}

	t.Run("RealModule", func(t *testing.T) {
		t.Parallel()
		got, scanned := unlistedLiterals(t, scankit.Root(t), realModuleListed(t))
		scankit.RequireFloor(t, scanned, 1, "module-wide launch-spec literal scan")
		for _, literal := range got {
			t.Errorf("a launch-spec literal, seat table or seat is not listed for the parity tests: %s", literal)
		}
	})
}
