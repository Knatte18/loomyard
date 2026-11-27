// seats.go holds the seat catalog: every seat table builder of production code, run on stand-ins for a run's own inputs.
// seatsOf turns the catalog into the seats the parity test and `lym shuttle manifest` prepare on each engine.

package shuttleparity

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/languageconfig"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/partsparser"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// standInModel is the model every stand-in input names where a builder needs one the registry does not resolve.
const standInModel = "stand-in"

// SeatEnv is what the seat table builders run on: the worktree's configuration and stand-ins for a run's own inputs.
type SeatEnv struct {
	// StencilsDir and SpecsDir are the told directories the stencils and deployed specs are read from.
	StencilsDir string
	SpecsDir    string
	// Dir is a directory the caller removes, where Build writes the stand-in files a builder reads.
	Dir string
	// Registry is the model-spec registry the builders resolve their models against.
	Registry modelspec.Registry
	// Location is the worktree the seats are built for.
	Location *lymcwd.Location
	// Dirs are the config layers of that worktree.
	Dirs configengine.Dirs
	// Shortname, Slug and ParentName form the seats' agent names and their parent directive.
	Shortname  string
	Slug       string
	ParentName string
}

// Seat is one seat of a production seat table.
type Seat struct {
	// Name is the seat's strand role.
	Name string
	// Builder is the glyph of the builder whose table holds the seat.
	Builder string
	// Specs returns the spec the seat starts with on engine.
	// The spec's model choice names the engine alone: the manifest compares what the session is given, not the model.
	Specs func(engine string) (shuttleengine.Spec, error)
}

// seatTable is one production seat table builder of the catalog.
type seatTable struct {
	// Builder is the builder's glyph, such as `internal/darnengine#DarnTable`.
	Builder string
	// Holders are the glyphs of every function whose body holds a seatengine.Table literal,
	// or a seatengine.Seat literal that sets Stencil, that the builder returns.
	Holders []string
	// Build calls the builder on env's stand-ins and returns the tables it builds.
	Build func(env SeatEnv) ([]seatengine.Table, error)
}

// buildWayForward ends the error of a builder that failed on its stand-ins.
const buildWayForward = "way forward: fix the stand-in inputs that this builder's seatTables entry's Build writes in internal/shuttleparity, " +
	"since the builder ran on those stand-ins and not on a run's inputs"

// seatsOf builds every table of tables on env and returns one Seat per seat of each table that built.
// It returns beside them one error per builder that failed, so a failing builder costs only its own seats.
func seatsOf(env SeatEnv, tables []seatTable) ([]Seat, []error) {
	var seats []Seat
	var failures []error
	for _, entry := range tables {
		built, err := entry.Build(env)
		if err != nil {
			failures = append(failures, fmt.Errorf("shuttleparity: builder %s: %w; %s", entry.Builder, err, buildWayForward))
			continue
		}
		for _, table := range built {
			for _, seat := range table.Seats {
				seats = append(seats, Seat{
					Name:    seatengine.SeatRole(table.RolePrefix, seat.Name),
					Builder: entry.Builder,
					Specs:   specsOf(env, table, seat.Name),
				})
			}
		}
	}
	return seats, failures
}

// specsOf returns the function that gives the spec of the seat called seatName of table on an engine.
func specsOf(env SeatEnv, table seatengine.Table, seatName string) func(engine string) (shuttleengine.Spec, error) {
	return func(engine string) (shuttleengine.Spec, error) {
		onEngine := table
		onEngine.Seats = slices.Clone(table.Seats)
		for i := range onEngine.Seats {
			onEngine.Seats[i].ModelChoice = shuttleengine.ModelChoice{Engine: engine}
		}
		specs, err := seatengine.Specs(seatengine.Geometry{
			WorktreeRoot:  env.Location.WorktreePath(),
			AnchorPath:    env.Location.AnchorPath(),
			StencilsDir:   env.StencilsDir,
			DefaultEngine: engine,
			ParentName:    env.ParentName,
			Shortname:     env.Shortname,
			Slug:          env.Slug,
		}, onEngine)
		if err != nil {
			return shuttleengine.Spec{}, fmt.Errorf("shuttleparity: the specs of table %s on %s: %w", onEngine.RolePrefix, engine, err)
		}
		return specs[seatName], nil
	}
}

// soloSessionHolder is the holder glyph of the one-seat table builder the landing rows' session tables share.
const soloSessionHolder = "internal/landingshed#soloSession.table"

// seatTables is the catalog: one entry per seat table builder of production code.
var seatTables = []seatTable{
	{Builder: "internal/darnengine#DarnTable", Holders: []string{"internal/darnengine#DarnTable"}, Build: buildDarn},
	{Builder: "internal/landingshed#DescribeTable", Holders: []string{soloSessionHolder, "internal/landingshed#describeReviewSeat"}, Build: buildDescribe},
	{Builder: "internal/landingshed#FixerTable", Holders: []string{soloSessionHolder}, Build: buildFixer},
	{Builder: "internal/landingshed#FixReviewSeatTable", Holders: []string{soloSessionHolder}, Build: buildFixReview},
	{
		Builder: "internal/burlerengine#Engine.SeatTables",
		Holders: []string{"internal/burlerengine#Profile.reviewTable", "internal/burlerengine#Profile.fixTable", "internal/burlerengine#Engine.assistSeat"},
		Build:   buildBurler,
	},
	{Builder: "internal/driftengine#ExpectTable", Holders: []string{"internal/driftengine#ExpectTable"}, Build: buildExpect},
	{Builder: "internal/driftengine#JudgeTable", Holders: []string{"internal/driftengine#JudgeTable"}, Build: buildJudge},
	{Builder: "internal/loomengine#PartsTable", Holders: []string{"internal/loomengine#PartsTable"}, Build: buildParts},
	{Builder: "internal/loomengine#PlanTable", Holders: []string{"internal/loomengine#PlanTable"}, Build: buildPlan},
	{Builder: "internal/loomengine#PartsPlanTable", Holders: []string{"internal/loomengine#PartsPlanTable"}, Build: buildPartsPlan},
	{Builder: "internal/loomengine#DiscussionTable", Holders: []string{"internal/loomengine#DiscussionTable"}, Build: buildDiscussion},
	{Builder: "internal/loomengine#RaddleTable", Holders: []string{"internal/loomengine#RaddleTable"}, Build: buildRaddle},
}

// standInPath returns the absolute stand-in path of the file called name under env.Dir.
func standInPath(env SeatEnv, name string) string {
	return filepath.Join(env.Dir, name)
}

// writeStandIn writes content to path, making its directories.
func writeStandIn(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("write the stand-in %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write the stand-in %s: %w", path, err)
	}
	return nil
}

// standInLocation returns a worktree under env.Dir, whose layout paths the loom builders read their input files at.
func standInLocation(env SeatEnv) (*lymcwd.Location, error) {
	return lymcwd.NamedWorktree(env.Dir, "stand-in-worktree")
}

// languageSkillsOf returns the language skills of env's hub.
func languageSkillsOf(env SeatEnv) ([]string, error) {
	language, err := languageconfig.Load(env.Dirs)
	if err != nil {
		return nil, err
	}
	return language.Skills, nil
}

// loomConfigOf returns loom's configuration with the tapestry keys a plan-review row adds.
// It reconciles the hub's config after a binary change before the loads.
func loomConfigOf(env SeatEnv) (loomengine.Config, error) {
	hubgeom.ReconcileBeforeLoad(env.Location)
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

// gateTiersStandIn is the gate_tiers value the plan builders are given, the one a hub with no tier gives.
const gateTiersStandIn = "none"

func buildDarn(env SeatEnv) ([]seatengine.Table, error) {
	cfg, err := darnengine.LoadConfig(env.Dirs, "darn")
	if err != nil {
		return nil, err
	}
	skills, err := languageSkillsOf(env)
	if err != nil {
		return nil, err
	}
	table, err := darnengine.DarnTable(darnengine.DarnInputs{
		StencilsDir:             env.StencilsDir,
		SpecsDir:                env.SpecsDir,
		WorktreeRoot:            env.Location.WorktreePath(),
		Slug:                    env.Slug,
		DescriptionPath:         standInPath(env, "description.md"),
		DecisionRecordPath:      standInPath(env, "decision-record.md"),
		StopMarkerPath:          standInPath(env, "stop-marker"),
		ConditionStopMarkerPath: standInPath(env, "condition-stop-marker"),
		LanguageSkills:          skills,
		SubagentModel:           standInModel,
		MeasureCall:             measureCallStandIn,
	}, cfg, env.Registry)
	return []seatengine.Table{table}, err
}

// measureCallStandIn is the measuring call the darn builder is given.
// It is the call a hub with no tier gives its darn-write key.
const measureCallStandIn = "lym gate test --gate darn-write -- <args>"

func buildDescribe(env SeatEnv) ([]seatengine.Table, error) {
	cfg, err := landingshed.LoadConfig(env.Dirs, "landing")
	if err != nil {
		return nil, err
	}
	table, err := landingshed.DescribeTable(landingshed.DescribeInputs{
		StencilsDir:        env.StencilsDir,
		DecisionRecordPath: standInPath(env, "decision-record.md"),
		DescriptionPath:    standInPath(env, "description.md"),
		ReviewDir:          standInPath(env, "describe-review"),
		TaskBranch:         "task",
		ParentBranch:       "parent",
		MergeBase:          "merge-base",
		Slug:               env.Slug,
	}, cfg, env.Registry)
	return []seatengine.Table{table}, err
}

func buildFixer(env SeatEnv) ([]seatengine.Table, error) {
	cfg, err := landingshed.LoadConfig(env.Dirs, "landing")
	if err != nil {
		return nil, err
	}
	skills, err := languageSkillsOf(env)
	if err != nil {
		return nil, err
	}
	table, err := landingshed.FixerTable(landingshed.FixerInputs{
		StencilsDir:        env.StencilsDir,
		Mode:               landingshed.FixerModeParentAlign,
		DecisionRecordPath: standInPath(env, "decision-record.md"),
		MergeRecordPath:    standInPath(env, "merge-record.json"),
		AlignedTip:         "aligned-tip",
		MergedTip:          "merged-tip",
		ReportPath:         standInPath(env, "fixer-report.md"),
		TaskBranch:         "task",
		ParentBranch:       "parent",
		Slug:               env.Slug,
		LanguageSkills:     skills,
	}, cfg, env.Registry)
	return []seatengine.Table{table}, err
}

func buildFixReview(env SeatEnv) ([]seatengine.Table, error) {
	cfg, err := landingshed.LoadConfig(env.Dirs, "landing")
	if err != nil {
		return nil, err
	}
	skills, err := languageSkillsOf(env)
	if err != nil {
		return nil, err
	}
	table, err := landingshed.FixReviewSeatTable(env.StencilsDir, skills, cfg, env.Registry)
	return []seatengine.Table{table}, err
}

// burlerFan names the lens fan the fanned burler profile and its assistant run.
const burlerFan = "standard"

// buildBurler builds the review and fix tables of a solo round and of a fanned round, each with an assistant seat.
// The two profiles carry different role prefixes, so no two seats of the catalog share a name.
func buildBurler(env SeatEnv) ([]seatengine.Table, error) {
	lensesCfg, err := lenses.Load(env.Dirs)
	if err != nil {
		return nil, err
	}
	skills, err := languageSkillsOf(env)
	if err != nil {
		return nil, err
	}
	engine := burlerengine.New(nil, burlerengine.Geometry{
		WorktreeRoot: env.Location.WorktreePath(),
		AnchorPath:   env.Location.AnchorPath(),
		RepoRoot:     env.Location.WorktreePath(),
		ParentName:   env.ParentName,
		Shortname:    env.Shortname,
		Slug:         env.Slug,
		SpecsDir:     env.SpecsDir,
	}, lensesCfg, env.StencilsDir, "", skills)

	assist := shuttleengine.ModelChoice{Engine: ReferenceEngine, Model: standInModel}
	solo := burlerengine.Profile{
		ReviewPath:      standInPath(env, "solo-review.md"),
		FixerReportPath: standInPath(env, "solo-fixer-report.md"),
		LanguageSkills:  true,
	}
	fanned := burlerengine.Profile{
		RolePrefix:      "fanned",
		ClusterFan:      burlerFan,
		FixFan:          burlerFan,
		ReviewPath:      standInPath(env, "fanned-review.md"),
		FixerReportPath: standInPath(env, "fanned-fixer-report.md"),
		LanguageSkills:  true,
	}
	tables := engine.SeatTables(&solo, burlerengine.RunOpts{Assist: assist})
	return append(tables, engine.SeatTables(&fanned, burlerengine.RunOpts{Assist: assist, AssistFan: burlerFan})...), nil
}

// driftSubject is the subject the drift builders read the task from: the run's decision record.
const driftSubject = driftengine.SubjectDecisionRecord

func buildExpect(env SeatEnv) ([]seatengine.Table, error) {
	cfg, err := driftengine.LoadConfig(env.Dirs)
	if err != nil {
		return nil, err
	}
	table, err := driftengine.ExpectTable(driftengine.ExpectInputs{
		Subject:            driftSubject,
		Slug:               env.Slug,
		DecisionRecordPath: standInPath(env, "decision-record.md"),
		BaselineDir:        standInPath(env, "baseline"),
		ListingPath:        standInPath(env, "listing.md"),
		ExpectedPath:       standInPath(env, "expected.yaml"),
		DiffMode:           driftengine.DiffLines,
	}, cfg, env.Registry)
	return []seatengine.Table{table}, err
}

func buildJudge(env SeatEnv) ([]seatengine.Table, error) {
	cfg, err := driftengine.LoadConfig(env.Dirs)
	if err != nil {
		return nil, err
	}
	table, err := driftengine.JudgeTable(driftengine.JudgeInputs{
		Subject:            driftSubject,
		Slug:               env.Slug,
		DecisionRecordPath: standInPath(env, "decision-record.md"),
		RequestPath:        standInPath(env, "judge-request.json"),
		JudgementPath:      standInPath(env, "judgement.yaml"),
	}, cfg, env.Registry)
	return []seatengine.Table{table}, err
}

func buildParts(env SeatEnv) ([]seatengine.Table, error) {
	cfg, err := loomConfigOf(env)
	if err != nil {
		return nil, err
	}
	location, err := standInLocation(env)
	if err != nil {
		return nil, err
	}
	table, err := loomengine.PartsTable(location, env.StencilsDir, env.SpecsDir, cfg, env.Registry)
	return []seatengine.Table{table}, err
}

func buildPlan(env SeatEnv) ([]seatengine.Table, error) {
	cfg, err := loomConfigOf(env)
	if err != nil {
		return nil, err
	}
	location, err := standInLocation(env)
	if err != nil {
		return nil, err
	}
	table, err := loomengine.PlanTable(location, env.Dirs, env.StencilsDir, env.SpecsDir, cfg, env.Registry, gateTiersStandIn, false)
	return []seatengine.Table{table}, err
}

// standInPartsFile is a parts file of one part, the least the parts parser accepts.
const standInPartsFile = `# Parts: Stand-in

## Split note

The task is one part.

## Part: core

- Decisions: D1
- Takes in: the stand-in request
- Delivers: the stand-in change
- Area:
  - ` + "`internal/stand-in`" + `
`

func buildPartsPlan(env SeatEnv) ([]seatengine.Table, error) {
	cfg, err := loomConfigOf(env)
	if err != nil {
		return nil, err
	}
	location, err := standInLocation(env)
	if err != nil {
		return nil, err
	}
	if err := writeStandIn(partsparser.File(location.AnchorPath()), standInPartsFile); err != nil {
		return nil, err
	}
	table, err := loomengine.PartsPlanTable(location, env.StencilsDir, env.SpecsDir, cfg, env.Registry, gateTiersStandIn, 0,
		func(string) (shuttleengine.GateSpec, error) { return nil, nil },
		func(string) func(shuttleengine.Result) error { return nil },
		func(part string) string { return standInPath(env, part+"-gate-findings.md") },
	)
	return []seatengine.Table{table}, err
}

func buildDiscussion(env SeatEnv) ([]seatengine.Table, error) {
	cfg, err := loomConfigOf(env)
	if err != nil {
		return nil, err
	}
	location, err := standInLocation(env)
	if err != nil {
		return nil, err
	}
	table, err := loomengine.DiscussionTable(location, env.Dirs, env.StencilsDir, env.SpecsDir, cfg, env.Registry, env.Slug, false)
	return []seatengine.Table{table}, err
}

func buildRaddle(env SeatEnv) ([]seatengine.Table, error) {
	cfg, err := loomConfigOf(env)
	if err != nil {
		return nil, err
	}
	location, err := standInLocation(env)
	if err != nil {
		return nil, err
	}
	table, err := loomengine.RaddleTable(location, env.StencilsDir, env.SpecsDir, cfg, env.Registry, env.Slug, loomengine.RaddleModeAuto, "")
	return []seatengine.Table{table}, err
}
