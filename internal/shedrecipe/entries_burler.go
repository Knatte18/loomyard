// entries_burler.go implements burlerReviewEntry and burlerFixEntry, the Constructors for the "BurlerReview" and "BurlerFix" registry rows:
// the review row and the fix row opposite a segment's Bouncer row, sharing that row's run_subdir value so all three write into one joined run directory.

package shedrecipe

import (
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// burlerRowModels returns the review and fix model lists of the row called name: its env.RowReviewModels entry, else env.ReviewModels.
// Each row of a segment takes its own half of the pick.
func burlerRowModels(name string, env Env) burlerengine.RoundModels {
	if rowModels, ok := env.RowReviewModels[name]; ok {
		return rowModels
	}
	return env.ReviewModels
}

// burlerRowTimeout returns the round deadline of a row whose timeout_s Config key read timeoutS: the row's own value, else env.ReviewTimeout.
// configInt with required false returns 0 for an absent key, so 0 is the absent sentinel here.
// The reasoning is configString's: an empty string is the absent sentinel because there is no meaningful explicit zero.
func burlerRowTimeout(timeoutS int, env Env) time.Duration {
	if timeoutS == 0 {
		return env.ReviewTimeout
	}
	return time.Duration(timeoutS) * time.Second
}

// burlerReviewEntry is the Constructor for the "BurlerReview" registry row: it validates cfg and env, maps cfg's profile map onto a burlerengine.Profile, joins and creates the run directory this row's segment shares with its Bouncer and fix rows, and returns shedadapters.NewBurlerReviewProducer(name, shedadapters.BurlerHalfDeps{Runner: env.Burler, Models: burlerRowModels(name, env)}, profile, opts, runDir, env.Now).
//
// The row takes no "gates" key: its only gate is the engine's review-parse entry.
// A cluster fan from env.RowClusterFans reaches the profile, and one also set in the recipe's profile is a construction error.
func burlerReviewEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
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
	if err := configRejectUnknown(cfg, "run_subdir", "profile", "timeout_s", "color_segment", "role_prefix"); err != nil {
		return nil, err
	}

	var notes shedadapters.RubricNotes
	if env.PublishFailure != nil {
		notes.PublishFailure = env.PublishFailure()
	}
	if env.WebsterRecord != nil {
		notes.WebsterRecord = env.WebsterRecord()
	}
	if env.MergeReview != nil {
		notes.MergeReview = env.MergeReview()
	}
	profile, err := burlerRoundProfile(profileCfg, env.StencilsDir, env.SpecsDir, notes)
	if err != nil {
		return nil, err
	}
	profile.Segment = colorSegment
	profile.RolePrefix = rolePrefix

	if fan := env.RowClusterFans[name]; fan != "" {
		if profile.ClusterFan != "" {
			return nil, fmt.Errorf("shedrecipe: BurlerReview: row %q: cluster fan %q from loom.yaml through the wiring conflicts with cluster-fan %q set in the recipe's profile; way forward: remove cluster-fan from the recipe's profile for this row", name, fan, profile.ClusterFan)
		}
		profile.ClusterFan = fan
	}

	runDir, err := burlerRunDir("BurlerReview", runSubdir, env)
	if err != nil {
		return nil, err
	}
	opts := burlerengine.RunOpts{Timeout: burlerRowTimeout(timeoutS, env)}
	producer, err := shedadapters.NewBurlerReviewProducer(name, shedadapters.BurlerHalfDeps{Runner: env.Burler, Models: burlerRowModels(name, env)}, profile, opts, runDir, env.Now)
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: BurlerReview: %w", err)
	}
	return producer, nil
}

// burlerFixEntry is the Constructor for the "BurlerFix" registry row: it validates cfg and env, maps cfg's profile map onto a burlerengine.Profile through burlerFixProfile, resolves the row's "gates" Config key through resolveRowGateSpec into the RunOpts.Gate it builds, joins and creates the run directory this row's segment shares with its Bouncer and review rows, and returns shedadapters.NewBurlerFixProducer(name, review_row, shedadapters.BurlerHalfDeps{Runner: env.Burler, Models: burlerRowModels(name, env)}, profile, opts, runDir, env.Now).
//
// All four fix rows share this one constructor, which is why the validator is selected by the "gates" key rather than implied by the constructor:
// the Webster and Darn fixers name the "verify" gate, so a round that changed code ends with a passing verify, and the two review segments name their own validators.
// The row takes no cluster fan, since the fixer runs no fan.
func burlerFixEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
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
	gate, err := resolveRowGateSpec("BurlerFix", name, cfg, env)
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
	if err := configRejectUnknown(cfg, "run_subdir", "review_row", "profile", "timeout_s", "gates", "color_segment", "role_prefix"); err != nil {
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
	opts := burlerengine.RunOpts{Timeout: burlerRowTimeout(timeoutS, env), Gate: gate}
	producer, err := shedadapters.NewBurlerFixProducer(name, reviewRow, shedadapters.BurlerHalfDeps{Runner: env.Burler, Models: burlerRowModels(name, env)}, profile, opts, runDir, env.Now)
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: BurlerFix: %w", err)
	}
	return producer, nil
}

// burlerRunDir checks the Env fields a burler half row reads, then joins and creates the run directory runSubdir names under env.RunRoot.
// The same run_subdir value in a segment's Bouncer, review and fix rows is what makes all three write into one directory.
// That is what lets shedadapters' roundComplete find the review where the Bouncer looks for it.
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

// burlerRoundFileSet maps profile's target or fasit sub-map onto a burlerengine.FileSet, recognising
// exactly paths and instructions and rejecting any other key.
//
// entry and field qualify every error it returns. Without them the two call sites are
// indistinguishable: the accessors below render only the leaf key, so a bad profile.target.paths and
// a bad profile.fasit.paths both read `config key "paths" must be a string list, got ...`, and a
// stray key under either reads `unrecognized config key "X"` with no path at all. A recipe author
// with a typo in one of two sibling maps was told which key and never which map. This is the same
// entry/field qualification resolveUnderRoot already applies one file over.
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

// burlerRoundProfile maps cfg's profile map onto a burlerengine.Profile, recognising exactly eight keys:
// target, fasit, rubric, rubric_stencil, fix-scope, tool-use, cluster-fan and language-skills.
// All are optional at this level individually, subject to the rubric/rubric_stencil mutual-exclusivity rule below.
//
// Seven of these eight key names are a hand-maintained duplicate of internal/burlercli's profileYAML
// kebab-case shape, kept identical deliberately so a human who has written a burler profile file
// reads a recipe row without a second vocabulary. review-path, fixer-report-path, prior-reviews,
// prior-fixer-reports, and cluster-exclude are deliberately absent.
// shedadapters.NewBurlerReviewProducer's own doc states that those burlerengine.Profile fields and burlerengine.RunOpts.Round are overwritten per round.
// A recipe author setting one would therefore set a value the producer silently discards.
//
// rubric_stencil is the one key with no profileYAML counterpart: it names a stencilstore rubric
// stencil, read and filled via shedadapters.ReadRubric, to set Profile.Rubric in place of a literal
// rubric string. Exactly one of rubric and rubric_stencil must be non-empty -- both set, or both
// empty, is a construction error naming both keys, because neither of Profile.validate's own checks
// can say which of the two the author meant.
//
// The stencil route fills specs_dir and the optional notes at read time; the literal route passes rubric through
// unfilled, exactly as before. The asymmetry is deliberate: a literal rubric is author-written
// prose reaching this entry through the rubric config key rather than through the stencil store,
// and running it through Fill would turn any bare {{ in it into a parse-template error and impose
// specs-directory semantics on text that never had them.
//
// This entry does not check profile's inner required-ness beyond the mutual-exclusivity rule:
// burlerengine.Profile.validate already rejects an empty Rubric and a Target/Fasit with neither
// Paths nor Instructions, and duplicating that here would drift from it.
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
	languageSkills, err := configBool(cfg, "language-skills", false)
	if err != nil {
		return burlerengine.Profile{}, err
	}
	if err := configRejectUnknown(cfg, "target", "fasit", "rubric", "rubric_stencil", "fix-scope", "tool-use", "cluster-fan", "language-skills"); err != nil {
		return burlerengine.Profile{}, err
	}

	// Exactly one of rubric and rubric_stencil must be non-empty. This is checked here, ahead of
	// burlerengine.Profile.validate's own empty-Rubric rejection, because that error names neither
	// key and cannot say which of the two the author meant.
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
		// Guarded inside this branch, not at function entry, so a row using a literal rubric: key
		// still constructs with no specs directory wired, exactly as it does today.
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

	// profile.target.paths and profile.fasit.paths are the single documented exception to the
	// general relative-Config-path rule: they are passed through relative and unjoined, with no
	// resolveUnderRoot call and no absolute-rejection check. burlerengine.Profile.validate already
	// resolves them against its own told worktree root and stats every resolved entry for
	// existence, so joining here would either double-resolve or hand validate an absolute path it
	// did not resolve itself, and an author who writes an absolute path there gets validate's
	// behaviour rather than this package's.

	return burlerengine.Profile{
		Target:         target,
		Fasit:          fasit,
		Rubric:         rubric,
		FixScope:       burlerengine.FixScope(fixScope),
		ToolUse:        toolUse,
		ClusterFan:     clusterFan,
		LanguageSkills: languageSkills,
	}, nil
}

// burlerFixProfile maps cfg's profile map onto the burlerengine.Profile of a fix row, recognising exactly three keys -- target, fix-scope and language-skills -- all optional at this level.
// The fixer holds no fasit, rubric or cluster fan, so those keys are rejected as unrecognised; profile.target.paths follows burlerRoundProfile's relative-path exception.
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
