// entries_burler.go implements the Constructors of the "BurlerReview" and "BurlerFix" registry rows.
// A segment's review and fix rows share its Bouncer row's run_subdir value, so the three write into one run directory.

package shedrecipe

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/hardenshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// hasFanName reports whether any entry of the fan list names a fan; a nil list and a list of "" entries name none.
func hasFanName(fans []string) bool {
	for _, fan := range fans {
		if fan != "" {
			return true
		}
	}
	return false
}

// effectiveFanList returns the per-round fan list a row runs: the wiring's list when it names a fan, else the recipe profile's one fan, else the wiring's list.
func effectiveFanList(wiring []string, profileFan string) []string {
	switch {
	case hasFanName(wiring):
		return wiring
	case profileFan != "":
		return []string{profileFan}
	}
	return wiring
}

// burlerRowTimeout returns the round deadline of a row whose timeout_s Config key read timeoutS: the row's own value, else env.ReviewTimeout.
// Zero marks an absent key, because a zero timeout has no meaning.
func burlerRowTimeout(timeoutS int, env Env) time.Duration {
	if timeoutS == 0 {
		return env.ReviewTimeout
	}
	return time.Duration(timeoutS) * time.Second
}

// reviewTargetSnapshot returns the target snapshot a review segment's rows read the review target through.
// Its scope and paths are unset.
// Its overlay copies live in the anchor's `.lym`, one directory per run_subdir.
// Its source reads come from the Env tree closures.
func reviewTargetSnapshot(env Env, runSubdir string) shedadapters.TargetSnapshot {
	return shedadapters.TargetSnapshot{
		WorktreeRoot:  env.WorktreeRoot,
		CacheDir:      filepath.Join(env.AnchorPath, lymdirs.DotLymDirName, lymdirs.ReviewSnapshotDirName, runSubdir),
		SourceTreeID:  env.ReviewTreeID,
		SourceChanges: env.ReviewTreeChanges,
	}
}

// burlerReviewEntry is the Constructor for the "BurlerReview" registry row.
// It takes no "gates" key: its only gate is the engine's review-parse entry.
// The optional "stage_diff" key, set true, gives each spawned reviewer the task's committed diff from Env.CodeDiff.
// burlerengine writes that diff out in per-package pieces.
// A nil Env.CodeDiff then refuses the row when it is built.
// Its producer records each round's target id, through a snapshot over the profile's fix scope and target paths.
// Its models, review key, assistant and parent seat come from its segment's settings.
// The fan lists come from the settings named by reviewer_fan_setting and fixer_fan_setting.
// Without such a setting, they come from the recipe profile's cluster-fan and fix-fan.
// A setting's list that names a fan beside the profile's own fan is a construction error.
// A row that names a setting or sets a profile fan records its round's fans;
// a row with neither runs solo and records nothing.
// The optional bool key "harden_round", set true, refuses a nil Env.Harden.
// It then wraps the producer in the campaign's round row.
func burlerReviewEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	runSubdir, err := configString(cfg, "run_subdir", true)
	if err != nil {
		return nil, err
	}
	profileCfg, err := configMap(cfg, "profile", true)
	if err != nil {
		return nil, err
	}
	timeoutS, err := configInt(cfg, "timeout_s", false)
	if err != nil {
		return nil, err
	}
	colorSegment, err := colorSegmentKey("BurlerReview", cfg)
	if err != nil {
		return nil, err
	}
	rolePrefix, err := rolePrefixKey("BurlerReview", cfg)
	if err != nil {
		return nil, err
	}
	stageDiff, err := configBool(cfg, "stage_diff", false)
	if err != nil {
		return nil, err
	}
	hardenRound, err := configBool(cfg, "harden_round", false)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "run_subdir", "profile", "timeout_s", "color_segment", "role_prefix", "reviewer_fan_setting", "fixer_fan_setting", "stage_diff", "harden_round"); err != nil {
		return nil, err
	}
	if hardenRound {
		if err := requireHarden("BurlerReview", `config key "harden_round"`, env); err != nil {
			return nil, err
		}
	}
	var diff func() (string, error)
	if stageDiff {
		if err := requireSeam("BurlerReview", "CodeDiff", env.CodeDiff); err != nil {
			return nil, err
		}
		diff = env.CodeDiff
	}
	segment, err := rowSegment("BurlerReview", row, env)
	if err != nil {
		return nil, err
	}
	clusterList, clusterRef, clusterNamed, err := fanSetting("BurlerReview", row, cfg, "reviewer_fan_setting", env)
	if err != nil {
		return nil, err
	}
	fixList, fixRef, fixNamed, err := fanSetting("BurlerReview", row, cfg, "fixer_fan_setting", env)
	if err != nil {
		return nil, err
	}

	runDir, err := burlerRunDir("BurlerReview", runSubdir, env)
	if err != nil {
		return nil, err
	}
	notes, err := reviewRubricNotes(row, env, runDir)
	if err != nil {
		return nil, err
	}
	profile, err := burlerRoundProfile(profileCfg, env.StencilsDir, env.SpecsDir, notes)
	if err != nil {
		return nil, err
	}
	rubricStencil, err := configString(profileCfg, "rubric_stencil", false)
	if err != nil {
		return nil, err
	}
	var rubric func() (string, error)
	if rubricStencil != "" {
		rubric = func() (string, error) {
			roundNotes, err := reviewRubricNotes(row, env, runDir)
			if err != nil {
				return "", err
			}
			return shedadapters.ReadRubric(env.StencilsDir, rubricStencil, env.SpecsDir, roundNotes)
		}
	}
	profile.Segment = colorSegment
	profile.RolePrefix = rolePrefix

	if hasFanName(clusterList) && profile.ClusterFan != "" {
		return nil, fmt.Errorf("shedrecipe: BurlerReview: row %q: cluster fan list %v from %s conflicts with cluster-fan %q set in the recipe's profile; way forward: remove cluster-fan from the recipe's profile for this row", row.Name, clusterList, clusterRef, profile.ClusterFan)
	}
	if hasFanName(fixList) && profile.FixFan != "" {
		return nil, fmt.Errorf("shedrecipe: BurlerReview: row %q: fix fan list %v from %s conflicts with fix-fan %q set in the recipe's profile; way forward: remove fix-fan from the recipe's profile for this row", row.Name, fixList, fixRef, profile.FixFan)
	}
	var fans *shedadapters.RoundFans
	if clusterNamed || fixNamed || profile.ClusterFan != "" || profile.FixFan != "" {
		fans = &shedadapters.RoundFans{
			Review: effectiveFanList(clusterList, profile.ClusterFan),
			Fix:    effectiveFanList(fixList, profile.FixFan),
		}
		// The producer picks each round's fans from the lists, so the profile carries none.
		profile.ClusterFan, profile.FixFan = "", ""
	}

	snapshot := reviewTargetSnapshot(env, runSubdir)
	snapshot.Scope, snapshot.Paths = profile.FixScope, profile.Target.Paths
	opts := burlerengine.RunOpts{Timeout: burlerRowTimeout(timeoutS, env), ReviewKey: segment.ReviewKey}
	halfDeps := shedadapters.BurlerHalfDeps{Runner: env.Burler, Models: segment.Models, Fans: fans, Assist: segment.Assist, Parent: segment.Parent, GateFindingsPath: env.gateFindingsPath(row.Name), Snapshot: &snapshot, DecisionRecordPath: env.DecisionRecordPath, FocusFindings: hardenRound, Diff: diff, Rubric: rubric}
	// Only the plan segment's review answers to the plan amendment record and the plan executor's halt reason.
	if row.Name == loomshed.NamePlanBurlerReview {
		halfDeps.AmendmentTitles, halfDeps.HaltConflictDir = amendmentTitlesAt(env.AmendmentPath), env.HaltConflictDir
	}
	producer, err := shedadapters.NewBurlerReviewProducer(row.Name, halfDeps, profile, opts, runDir, env.Now)
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: BurlerReview: %w", err)
	}
	if hardenRound {
		return env.Harden.WrapRow(producer, row.Name, hardenshed.RowReview, runDir), nil
	}
	return producer, nil
}

// amendmentTitlesAt returns a reader of the pending amending decision titles the plan amendment record at path lists.
// An empty path gives a nil reader, which reads none.
func amendmentTitlesAt(path string) func() ([]string, error) {
	if path == "" {
		return nil
	}
	return func() ([]string, error) {
		record, _, err := loomshed.ReadAmendment(path)
		return record.Titles, err
	}
}

// reviewRubricNotes gathers the notes that fill a review row's rubric stencil from env and the run directory's records.
// The row calls it at build and again at each round.
// The amendment and review input notes follow the generation record.
// A failed review base read is an error naming row.
func reviewRubricNotes(row RowIdentity, env Env, runDir string) (shedadapters.RubricNotes, error) {
	notes := shedadapters.RubricNotes{
		GateTiers:        env.GateTiers,
		Amendment:        shedadapters.AmendmentNote(runDir),
		ReviewInput:      shedadapters.ReviewInputNote(runDir),
		RedundancyReport: env.RedundancyReport,
		ReachReport:      env.ReachReport,
		MutationReport:   env.MutationReport,
	}
	if env.WebsterRecord != nil {
		notes.WebsterRecord = env.WebsterRecord()
	}
	if env.ReviewBase != nil {
		base, err := env.ReviewBase()
		if err != nil {
			return shedadapters.RubricNotes{}, fmt.Errorf("shedrecipe: BurlerReview: row %q: read the review base: %w", row.Name, err)
		}
		notes.ReviewBase = base
	}
	return notes, nil
}

// burlerFixEntry is the Constructor for the "BurlerFix" registry row.
// The "gates" key selects its validator, since every fix row shares this constructor.
// The Webster and Darn fixers name the "verify" gate, so a round that changed code ends with a passing verify;
// the review segments name their own validators.
// The optional "gate_key" string names the hub's gate.yaml key that the "verify" gate runs.
// Without it, the key is the row name in lower case.
// Its models and fix key come from its segment's settings.
// Setting "decisions_escalated" to true marks a segment whose Bouncer escalates the round's decision findings itself.
// The fixer is then given none, and the row posts no notice of its own.
// The row has no fan list of its own.
// A fixer fan reaches it through the review row's recorded round;
// its fixer_fan_setting key, when it names a setting, is cited as the fan's source and requires that recorded round.
func burlerFixEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	return newBurlerFixProducer(row, cfg, env, func() (shuttleengine.GateSpec, error) {
		return resolveRowGateSpec("BurlerFix", row.Name, cfg, env)
	}, burlerFixKeys)
}

// burlerFixKeys are the keys a BurlerFix row's config may carry.
var burlerFixKeys = []string{"run_subdir", "review_row", "profile", "timeout_s", "gates", "gate_key", "color_segment", "role_prefix", "fixer_fan_setting", "decisions_escalated"}

// newBurlerFixProducer builds a fix row from cfg: the producer of a BurlerFix row, whose gates resolveGate returns.
// resolveGate is read once the row's own keys are, and keys are the only keys cfg may carry.
func newBurlerFixProducer(row RowIdentity, cfg Config, env Env, resolveGate func() (shuttleengine.GateSpec, error), keys []string) (shedengine.ShedProducer, error) {
	runSubdir, err := configString(cfg, "run_subdir", true)
	if err != nil {
		return nil, err
	}
	reviewRow, err := configString(cfg, "review_row", true)
	if err != nil {
		return nil, err
	}
	profileCfg, err := configMap(cfg, "profile", true)
	if err != nil {
		return nil, err
	}
	timeoutS, err := configInt(cfg, "timeout_s", false)
	if err != nil {
		return nil, err
	}
	gate, err := resolveGate()
	if err != nil {
		return nil, err
	}
	colorSegment, err := colorSegmentKey("BurlerFix", cfg)
	if err != nil {
		return nil, err
	}
	rolePrefix, err := rolePrefixKey("BurlerFix", cfg)
	if err != nil {
		return nil, err
	}
	decisionsEscalated, err := configBool(cfg, "decisions_escalated", false)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, keys...); err != nil {
		return nil, err
	}
	segment, err := rowSegment("BurlerFix", row, env)
	if err != nil {
		return nil, err
	}
	_, fixFanRef, fixFanNamed, err := fanSetting("BurlerFix", row, cfg, "fixer_fan_setting", env)
	if err != nil {
		return nil, err
	}

	profile, err := burlerFixProfile(profileCfg)
	if err != nil {
		return nil, err
	}
	profile.Segment = colorSegment
	profile.RolePrefix = rolePrefix
	runDir, err := burlerRunDir("BurlerFix", runSubdir, env)
	if err != nil {
		return nil, err
	}
	opts := burlerengine.RunOpts{Timeout: burlerRowTimeout(timeoutS, env), Gate: gate, FixKey: segment.FixKey}
	if fixFanNamed {
		opts.FixFanKey = fixFanRef.Key
	}
	deps := shedadapters.BurlerHalfDeps{Runner: env.Burler, Models: segment.Models, RequiresFans: fixFanNamed, GateFindingsPath: env.gateFindingsPath(row.Name), NotifyParent: env.NotifyParent, DecisionsEscalated: decisionsEscalated}
	producer, err := shedadapters.NewBurlerFixProducer(row.Name, reviewRow, deps, profile, opts, runDir, env.Now)
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: BurlerFix: %w", err)
	}
	return producer, nil
}

// burlerRunDir checks the Env fields a burler half row reads.
// It then joins and creates the run directory runSubdir names under env.RunRoot.
// A segment's Bouncer, review and fix rows share one run_subdir value, which puts all three in one directory.
// That directory is where the Bouncer looks for the review, and where shedadapters' roundComplete finds it.
func burlerRunDir(entry, runSubdir string, env Env) (string, error) {
	if err := requireAbsRoot(entry, "RunRoot", env.RunRoot); err != nil {
		return "", err
	}
	if err := requireSeam(entry, "Burler", env.Burler); err != nil {
		return "", err
	}
	runDir, err := resolveUnderRoot(entry, "run_subdir", env.RunRoot, runSubdir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return "", fmt.Errorf("shedrecipe: %s: create run dir %q: %w", entry, runDir, err)
	}
	return runDir, nil
}

// burlerRoundFileSet maps the target or fasit sub-map of a profile onto a burlerengine.FileSet.
// It recognises exactly paths and instructions and rejects any other key.
//
// entry and field qualify every error it returns.
// Without them the target and fasit maps report identical errors,
// and a stray key never says which map it is in.
func burlerRoundFileSet(entry, field string, cfg Config) (burlerengine.FileSet, error) {
	qualify := func(err error) error {
		return fmt.Errorf("shedrecipe: %s: config key %q: %w", entry, field, err)
	}

	paths, err := configStringSlice(cfg, "paths", false)
	if err != nil {
		return burlerengine.FileSet{}, qualify(err)
	}
	instructions, err := configString(cfg, "instructions", false)
	if err != nil {
		return burlerengine.FileSet{}, qualify(err)
	}
	if err := configRejectUnknown(cfg, "paths", "instructions"); err != nil {
		return burlerengine.FileSet{}, qualify(err)
	}
	return burlerengine.FileSet{Paths: paths, Instructions: instructions}, nil
}

// burlerRoundProfile maps cfg's profile map onto a burlerengine.Profile.
// It recognises target, fasit, rubric, rubric_stencil, fix-scope, tool-use, cluster-fan, fix-fan and language-skills.
// Each is optional here.
// Exactly one of rubric and rubric_stencil must be non-empty:
// both set, or both empty, is a construction error that names both keys.
//
// Every key except rubric_stencil duplicates the kebab-case shape of internal/burlercli's profileYAML on purpose,
// so a human who has written a burler profile file reads a recipe row without a second vocabulary.
// review-path, fixer-report-path, prior-reviews, prior-fixer-reports and cluster-exclude are left out on purpose:
// shedadapters.NewBurlerReviewProducer overwrites those profile fields on every round,
// so a recipe author who set one would set a value the producer discards.
//
// rubric_stencil names a stencilstore rubric stencil, read and filled by shedadapters.ReadRubric.
// Its filled text sets Profile.Rubric in place of a literal rubric string.
// That text is the template's rubric.
// A review row's producer sets the rubric among the round's own fields.
// It takes the rubric from the answer of its Rubric seam, which refills the stencil at each round.
// The stencil route fills specs_dir and the optional notes at read time;
// the literal route passes rubric through unfilled.
// That asymmetry is deliberate: a literal rubric is author-written prose.
// Filling it would turn any bare {{ into a parse-template error, and impose specs-directory semantics on it.
//
// Beyond that exclusivity rule, burlerengine.Profile.validate checks the profile's inner required fields.
// Repeating those checks here would drift from it.
func burlerRoundProfile(cfg Config, stencilsDir, specsDir string, notes shedadapters.RubricNotes) (burlerengine.Profile, error) {
	targetCfg, err := configMap(cfg, "target", false)
	if err != nil {
		return burlerengine.Profile{}, err
	}
	fasitCfg, err := configMap(cfg, "fasit", false)
	if err != nil {
		return burlerengine.Profile{}, err
	}
	rubric, err := configString(cfg, "rubric", false)
	if err != nil {
		return burlerengine.Profile{}, err
	}
	rubricStencil, err := configString(cfg, "rubric_stencil", false)
	if err != nil {
		return burlerengine.Profile{}, err
	}
	fixScope, err := configString(cfg, "fix-scope", false)
	if err != nil {
		return burlerengine.Profile{}, err
	}
	toolUse, err := configBool(cfg, "tool-use", false)
	if err != nil {
		return burlerengine.Profile{}, err
	}
	clusterFan, err := configString(cfg, "cluster-fan", false)
	if err != nil {
		return burlerengine.Profile{}, err
	}
	fixFan, err := configString(cfg, "fix-fan", false)
	if err != nil {
		return burlerengine.Profile{}, err
	}
	languageSkills, err := configBool(cfg, "language-skills", false)
	if err != nil {
		return burlerengine.Profile{}, err
	}
	if err := configRejectUnknown(cfg, "target", "fasit", "rubric", "rubric_stencil", "fix-scope", "tool-use", "cluster-fan", "fix-fan", "language-skills"); err != nil {
		return burlerengine.Profile{}, err
	}

	// Check the exclusivity here: burlerengine.Profile.validate rejects an empty Rubric,
	// but its error names neither key.
	switch {
	case rubric != "" && rubricStencil != "":
		return burlerengine.Profile{}, fmt.Errorf("shedrecipe: BurlerReview: config keys \"rubric\" and \"rubric_stencil\" are mutually exclusive")
	case rubric == "" && rubricStencil == "":
		return burlerengine.Profile{}, fmt.Errorf("shedrecipe: BurlerReview: exactly one of config keys \"rubric\" and \"rubric_stencil\" is required")
	}

	if rubricStencil != "" {
		if err := requireAbsRoot("BurlerReview", "StencilsDir", stencilsDir); err != nil {
			return burlerengine.Profile{}, err
		}
		// Guarded here, not at function entry: a row with a literal rubric builds with no specs directory wired.
		if err := requireAbsRoot("BurlerReview", "SpecsDir", specsDir); err != nil {
			return burlerengine.Profile{}, err
		}
		filled, err := shedadapters.ReadRubric(stencilsDir, rubricStencil, specsDir, notes)
		if err != nil {
			return burlerengine.Profile{}, fmt.Errorf("shedrecipe: BurlerReview: rubric_stencil %q: %w", rubricStencil, err)
		}
		rubric = filled
	}

	target, err := burlerRoundFileSet("BurlerReview", "target", targetCfg)
	if err != nil {
		return burlerengine.Profile{}, err
	}
	fasit, err := burlerRoundFileSet("BurlerReview", "fasit", fasitCfg)
	if err != nil {
		return burlerengine.Profile{}, err
	}

	// profile.target.paths and profile.fasit.paths are the one exception to the relative-Config-path rule:
	// they pass through relative and unjoined, with no resolveUnderRoot call and no absolute-path check.
	// burlerengine.Profile.validate resolves them against the told worktree root and stats each entry.
	// Joining here would resolve them twice, or hand validate an absolute path it did not resolve itself.
	// An absolute path there therefore gets validate's behaviour, not this package's.

	return burlerengine.Profile{
		Target:         target,
		Fasit:          fasit,
		Rubric:         rubric,
		FixScope:       burlerengine.FixScope(fixScope),
		ToolUse:        toolUse,
		ClusterFan:     clusterFan,
		FixFan:         fixFan,
		LanguageSkills: languageSkills,
	}, nil
}

// burlerFixProfile maps cfg's profile map onto the burlerengine.Profile of a fix row.
// It recognises target, fix-scope and language-skills, each optional here.
// The fixer holds no fasit, rubric, cluster fan or fix fan, so those keys are rejected as unrecognised.
// Its target.paths stay relative and unjoined:
// burlerengine.Profile.validate resolves them against the told worktree root.
func burlerFixProfile(cfg Config) (burlerengine.Profile, error) {
	targetCfg, err := configMap(cfg, "target", false)
	if err != nil {
		return burlerengine.Profile{}, err
	}
	fixScope, err := configString(cfg, "fix-scope", false)
	if err != nil {
		return burlerengine.Profile{}, err
	}
	languageSkills, err := configBool(cfg, "language-skills", false)
	if err != nil {
		return burlerengine.Profile{}, err
	}
	if err := configRejectUnknown(cfg, "target", "fix-scope", "language-skills"); err != nil {
		return burlerengine.Profile{}, err
	}
	target, err := burlerRoundFileSet("BurlerFix", "target", targetCfg)
	if err != nil {
		return burlerengine.Profile{}, err
	}
	return burlerengine.Profile{Target: target, FixScope: burlerengine.FixScope(fixScope), LanguageSkills: languageSkills}, nil
}
