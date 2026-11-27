// tables.go is the catalog of the production seat-table builders and the stand-in inputs each is called with.

package shuttleparity

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/languageconfig"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/partsparser"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// standInRunTimeout stands in for a run's timeout, which only bounds the sessions the tables describe.
const standInRunTimeout = time.Hour

// standInPartsFile is the parts file of one part that the parts-plan table is built over.
const standInPartsFile = `# Parts: stand-in

## Split note

The task is one part.

## Part: alpha

- Decisions: D1
- Takes in: the decision record
- Delivers: the behaviour
- Area:
  - ` + "`internal/alpha`" + `
`

// seatTables holds one entry per production seat-table builder.
var seatTables = []seatTable{
	{
		Builder: "internal/darnengine#DarnTable",
		Holders: []string{"internal/darnengine#DarnTable"},
		Build:   darnSeatTables,
	},
	{
		Builder: "internal/landingshed#DescribeTable",
		Holders: []string{"internal/landingshed#describeSessionTable"},
		Build:   describeSeatTables,
	},
	{
		Builder: "internal/landingshed#FixerTable",
		Holders: []string{"internal/landingshed#FixerTable"},
		Build:   fixerSeatTables,
	},
	{
		Builder: "internal/landingshed#FixReviewSeatTable",
		Holders: []string{"internal/landingshed#fixReviewTable"},
		Build:   fixReviewSeatTables,
	},
	{
		Builder: "internal/driftengine#ExpectTable",
		Holders: []string{"internal/driftengine#ExpectTable"},
		Build:   expectSeatTables,
	},
	{
		Builder: "internal/driftengine#JudgeTable",
		Holders: []string{"internal/driftengine#JudgeTable"},
		Build:   judgeSeatTables,
	},
	{
		Builder: "internal/loomengine#PartsTable",
		Holders: []string{"internal/loomengine#PartsTable"},
		Build:   partsSeatTables,
	},
	{
		Builder: "internal/loomengine#PlanTable",
		Holders: []string{"internal/loomengine#PlanTable"},
		Build:   planSeatTables,
	},
	{
		Builder: "internal/loomengine#PartsPlanTable",
		Holders: []string{"internal/loomengine#PartsPlanTable"},
		Build:   partsPlanSeatTables,
	},
	{
		Builder: "internal/loomengine#DiscussionTable",
		Holders: []string{"internal/loomengine#DiscussionTable"},
		Build:   discussionSeatTables,
	},
	{
		Builder: "internal/loomengine#RaddleTable",
		Holders: []string{"internal/loomengine#RaddleTable"},
		Build:   raddleSeatTables,
	},
	{
		Builder: "internal/burlerengine#Engine.SeatTables",
		Holders: []string{
			"internal/burlerengine#Profile.reviewTable",
			"internal/burlerengine#Profile.fixTable",
			"internal/burlerengine#Engine.assistSeat",
		},
		Build: burlerSeatTables,
	},
}

// oneTable wraps a builder's single table and error as the catalog's result.
func oneTable(table seatengine.Table, err error) ([]seatengine.Table, error) {
	if err != nil {
		return nil, err
	}
	return []seatengine.Table{table}, nil
}

// writeStandIn writes content at path, making its directory.
func writeStandIn(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("make the stand-in directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write the stand-in input: %w", err)
	}
	return nil
}

// languageSkillsOf returns the language skills of env's configuration.
func languageSkillsOf(env SeatEnv) ([]string, error) {
	language, err := languageconfig.Load(env.Dirs)
	if err != nil {
		return nil, err
	}
	return language.Skills, nil
}

// loomConfigOf returns env's loom configuration with the plan segment's tapestry values applied.
func loomConfigOf(env SeatEnv) (loomengine.Config, error) {
	cfg, err := loomengine.LoadConfig(env.Dirs, "loom")
	if err != nil {
		return loomengine.Config{}, err
	}
	tapestry, err := loomengine.LoadTapestryConfig(env.Dirs, "tapestry")
	if err != nil {
		return loomengine.Config{}, err
	}
	return cfg.WithTapestry(tapestry), nil
}

// gateTiersOf returns the gate-tier text the plan prompts render; a hub with no gate configuration renders none.
func gateTiersOf(env SeatEnv) (string, error) {
	cfg, err := gateslot.LoadConfig(env.Dirs)
	if errors.Is(err, gateslot.ErrConfigAbsent) {
		return "none", nil
	}
	if err != nil {
		return "", err
	}
	return cfg.TiersValue(), nil
}

// darnSeatTables builds the darn writer's table.
func darnSeatTables(env SeatEnv) ([]seatengine.Table, error) {
	cfg, err := darnengine.LoadConfig(env.Dirs, "darn")
	if err != nil {
		return nil, err
	}
	loom, err := loomConfigOf(env)
	if err != nil {
		return nil, err
	}
	skills, err := languageSkillsOf(env)
	if err != nil {
		return nil, err
	}
	return oneTable(darnengine.DarnTable(darnengine.DarnInputs{
		StencilsDir:             env.StencilsDir,
		SpecsDir:                env.SpecsDir,
		WorktreeRoot:            env.Location.WorktreePath(),
		Slug:                    env.Slug,
		DescriptionPath:         filepath.Join(env.Dir, "description.md"),
		DecisionRecordPath:      filepath.Join(env.Dir, "decision-record.md"),
		StopMarkerPath:          filepath.Join(env.Dir, "stop"),
		ConditionStopMarkerPath: filepath.Join(env.Dir, "condition-stop"),
		LanguageSkills:          skills,
		SubagentModel:           loom.SubagentModel,
	}, cfg, env.Registry))
}

// describeSeatTables builds the describe session's table.
func describeSeatTables(env SeatEnv) ([]seatengine.Table, error) {
	cfg, err := landingshed.LoadConfig(env.Dirs, "landing")
	if err != nil {
		return nil, err
	}
	record := filepath.Join(env.Dir, "describe", "decision-record.md")
	if err := writeStandIn(record, "# Decision record\n"); err != nil {
		return nil, err
	}
	return oneTable(landingshed.DescribeTable(landingshed.DescribeInputs{
		StencilsDir:        env.StencilsDir,
		DecisionRecordPath: record,
		DescriptionPath:    filepath.Join(env.Dir, "describe", "description.md"),
		TaskBranch:         "task",
		ParentBranch:       "parent",
		MergeBase:          "merge-base",
		Slug:               env.Slug,
	}, cfg, env.Registry))
}

// fixerSeatTables builds the merge fixer's table.
func fixerSeatTables(env SeatEnv) ([]seatengine.Table, error) {
	cfg, err := landingshed.LoadConfig(env.Dirs, "landing")
	if err != nil {
		return nil, err
	}
	skills, err := languageSkillsOf(env)
	if err != nil {
		return nil, err
	}
	return oneTable(landingshed.FixerTable(landingshed.FixerInputs{
		StencilsDir:        env.StencilsDir,
		Mode:               landingshed.FixerModeFinalize,
		DecisionRecordPath: filepath.Join(env.Dir, "decision-record.md"),
		MergeRecordPath:    filepath.Join(env.Dir, "merge-record.json"),
		MergedTip:          "merged-tip",
		ReportPath:         filepath.Join(env.Dir, "fixer-report.md"),
		TaskBranch:         "task",
		ParentBranch:       "parent",
		Slug:               env.Slug,
		LanguageSkills:     skills,
	}, cfg, env.Registry))
}

// fixReviewSeatTables builds the fix review's table.
func fixReviewSeatTables(env SeatEnv) ([]seatengine.Table, error) {
	cfg, err := landingshed.LoadConfig(env.Dirs, "landing")
	if err != nil {
		return nil, err
	}
	skills, err := languageSkillsOf(env)
	if err != nil {
		return nil, err
	}
	return oneTable(landingshed.FixReviewSeatTable(env.StencilsDir, skills, cfg, env.Registry))
}

// expectSeatTables builds the drift expect agent's table.
func expectSeatTables(env SeatEnv) ([]seatengine.Table, error) {
	cfg, err := driftengine.LoadConfig(env.Dirs)
	if err != nil {
		return nil, err
	}
	return oneTable(driftengine.ExpectTable(driftengine.ExpectInputs{
		Subject:            driftengine.SubjectDecisionRecord,
		Slug:               env.Slug,
		DecisionRecordPath: filepath.Join(env.Dir, "decision-record.md"),
		BaselineDir:        filepath.Join(env.Dir, "baseline"),
		ListingPath:        filepath.Join(env.Dir, "listing"),
		ExpectedPath:       filepath.Join(env.Dir, "expected.yaml"),
		DiffMode:           driftengine.DiffLines,
	}, cfg, env.Registry))
}

// judgeSeatTables builds the drift judge's table.
func judgeSeatTables(env SeatEnv) ([]seatengine.Table, error) {
	cfg, err := driftengine.LoadConfig(env.Dirs)
	if err != nil {
		return nil, err
	}
	return oneTable(driftengine.JudgeTable(driftengine.JudgeInputs{
		Subject:            driftengine.SubjectDecisionRecord,
		Slug:               env.Slug,
		DecisionRecordPath: filepath.Join(env.Dir, "decision-record.md"),
		RequestPath:        filepath.Join(env.Dir, "judge-request.yaml"),
		JudgementPath:      filepath.Join(env.Dir, "judgement.yaml"),
	}, cfg, env.Registry))
}

// partsSeatTables builds the parts writer's table.
func partsSeatTables(env SeatEnv) ([]seatengine.Table, error) {
	cfg, err := loomConfigOf(env)
	if err != nil {
		return nil, err
	}
	return oneTable(loomengine.PartsTable(env.Location, env.StencilsDir, env.SpecsDir, cfg, env.Registry))
}

// planSeatTables builds the plan writer's table.
func planSeatTables(env SeatEnv) ([]seatengine.Table, error) {
	cfg, err := loomConfigOf(env)
	if err != nil {
		return nil, err
	}
	tiers, err := gateTiersOf(env)
	if err != nil {
		return nil, err
	}
	return oneTable(loomengine.PlanTable(env.Location, env.Dirs, env.StencilsDir, env.SpecsDir, cfg, env.Registry, tiers, false))
}

// partsPlanSeatTables builds the parts-plan chair's and writers' table.
// The table reads the parts file of its worktree, so it is built over a stand-in worktree under env.Dir.
func partsPlanSeatTables(env SeatEnv) ([]seatengine.Table, error) {
	cfg, err := loomConfigOf(env)
	if err != nil {
		return nil, err
	}
	tiers, err := gateTiersOf(env)
	if err != nil {
		return nil, err
	}
	standIn, err := lymcwd.NamedWorktree(env.Dir, "parts-plan")
	if err != nil {
		return nil, err
	}
	if err := writeStandIn(partsparser.File(standIn.AnchorPath()), standInPartsFile); err != nil {
		return nil, err
	}
	return oneTable(loomengine.PartsPlanTable(standIn, env.StencilsDir, env.SpecsDir, cfg, env.Registry, tiers, standInRunTimeout,
		func(string) (shuttleengine.GateSpec, error) { return nil, nil },
		func(string) func(shuttleengine.Result) error { return func(shuttleengine.Result) error { return nil } },
		func(part string) string { return filepath.Join(env.Dir, "findings-"+part+".md") },
	))
}

// discussionSeatTables builds the discussion chair's and advisors' table.
func discussionSeatTables(env SeatEnv) ([]seatengine.Table, error) {
	cfg, err := loomConfigOf(env)
	if err != nil {
		return nil, err
	}
	return oneTable(loomengine.DiscussionTable(env.Location, env.Dirs, env.StencilsDir, env.SpecsDir, cfg, env.Registry, env.Slug, false))
}

// raddleSeatTables builds the raddle agent's table.
func raddleSeatTables(env SeatEnv) ([]seatengine.Table, error) {
	cfg, err := loomConfigOf(env)
	if err != nil {
		return nil, err
	}
	return oneTable(loomengine.RaddleTable(env.Location, env.StencilsDir, env.SpecsDir, cfg, env.Registry, env.Slug, loomengine.RaddleModeAuto, ""))
}

// burlerSeatTables builds the review and fix tables of a solo round and of a fanned one, each with an assistant,
// so the forking reviewer and fixer and both assistant stencils are covered.
func burlerSeatTables(env SeatEnv) ([]seatengine.Table, error) {
	library, err := lenses.Load(env.Dirs)
	if err != nil {
		return nil, err
	}
	if len(library.Fans) == 0 {
		return nil, errors.New("lenses.yaml defines no fan")
	}
	fan := slices.Sorted(maps.Keys(library.Fans))[0]
	skills, err := languageSkillsOf(env)
	if err != nil {
		return nil, err
	}
	engine := burlerengine.New(nil, burlerengine.Geometry{
		WorktreeRoot: env.Location.AnchorPath(),
		AnchorPath:   env.Location.AnchorPath(),
		RepoRoot:     env.Location.WorktreePath(),
		ParentName:   env.ParentName,
		Shortname:    env.Shortname,
		Slug:         env.Slug,
		SpecsDir:     env.SpecsDir,
	}, library, env.StencilsDir, "", skills)
	profile := func(prefix string) burlerengine.Profile {
		return burlerengine.Profile{
			ReviewPath:      filepath.Join(env.Dir, prefix, "review.md"),
			FixerReportPath: filepath.Join(env.Dir, prefix, "fixer-report.md"),
			RolePrefix:      prefix,
			LanguageSkills:  true,
		}
	}
	opts := burlerengine.RunOpts{Round: "1", Assist: shuttleengine.ModelChoice{Model: "stand-in"}}
	tables := engine.SeatTables(profile("solo"), opts)

	fanned := profile("fanned")
	fanned.ClusterFan, fanned.FixFan = fan, fan
	opts.AssistFan = fan
	return append(tables, engine.SeatTables(fanned, opts)...), nil
}
