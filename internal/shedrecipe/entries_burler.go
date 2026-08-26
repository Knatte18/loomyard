// entries_burler.go implements burlerRoundEntry, the Constructor for the "BurlerRound" registry
// row: the round producer opposite a segment's Bouncer row, sharing that row's run_subdir value so
// both write into one joined run directory.

package shedrecipe

import (
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// burlerRoundEntry is the Constructor for the "BurlerRound" registry row: it validates cfg and env,
// maps cfg's profile map onto a burlerengine.Profile, joins and creates the run directory this
// row's segment shares with its Bouncer row, and returns
// shedadapters.NewBurlerProducer(name, env.Burler, env.Shuttle, profile, opts, runDir, env.Now).
func burlerRoundEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
	runSubdir, err := configString(cfg, "run_subdir", true)
	if err != nil {
		return nil, err
	}
	profileCfg, err := configMap(cfg, "profile", true)
	if err != nil {
		return nil, err
	}
	model, err := configString(cfg, "model", false)
	if err != nil {
		return nil, err
	}
	effort, err := configString(cfg, "effort", false)
	if err != nil {
		return nil, err
	}
	timeoutS, err := configInt(cfg, "timeout_s", false)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "run_subdir", "profile", "model", "effort", "timeout_s"); err != nil {
		return nil, err
	}

	profile, err := burlerRoundProfile(profileCfg, env.StencilsDir)
	if err != nil {
		return nil, err
	}

	// A row setting model/effort/timeout_s overrides the Env value; a row omitting it takes the Env
	// value; both absent leaves the zero value. configInt with required false returns 0 for an
	// absent key, so 0 is the absent sentinel here -- the same "no meaningful explicit zero"
	// reasoning as configString's empty-string sentinel.
	if model == "" {
		model = env.ReviewModel
	}
	if effort == "" {
		effort = env.ReviewEffort
	}
	timeout := time.Duration(timeoutS) * time.Second
	if timeoutS == 0 {
		timeout = env.ReviewTimeout
	}

	opts := burlerengine.RunOpts{
		Model:   model,
		Effort:  effort,
		Timeout: timeout,
	}

	if err := requireAbsRoot("BurlerRound", "RunRoot", env.RunRoot); err != nil {
		return nil, err
	}
	if err := requireSeam("BurlerRound", "Burler", env.Burler); err != nil {
		return nil, err
	}
	// The same Shuttle seam the segment's Bouncer row already reads, threaded here as the round's
	// live-agent probe. Required rather than optional: without it a resumed run respawns over a
	// still-live round, producing two agents writing one review -- and on a fix-scope: source row,
	// two agents committing to one branch.
	if err := requireSeam("BurlerRound", "Shuttle", env.Shuttle); err != nil {
		return nil, err
	}

	runDir, err := resolveUnderRoot("BurlerRound", "run_subdir", env.RunRoot, runSubdir)
	if err != nil {
		return nil, err
	}
	// The same run_subdir value in this row and its segment's Bouncer row is what makes both write
	// into one directory, which is what lets shedadapters' roundComplete find this producer's report
	// where the Bouncer looks for it.
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return nil, fmt.Errorf("shedrecipe: BurlerRound: create run dir %q: %w", runDir, err)
	}

	producer, err := shedadapters.NewBurlerProducer(name, env.Burler, env.Shuttle, profile, opts, runDir, env.Now)
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: BurlerRound: %w", err)
	}
	return producer, nil
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

// burlerRoundProfile maps cfg's profile map onto a burlerengine.Profile, recognising exactly seven
// keys -- target, fasit, rubric, rubric_stencil, fix-scope, tool-use, and cluster-fan -- all
// optional at this level individually, subject to the rubric/rubric_stencil mutual-exclusivity rule
// below.
//
// Six of these seven key names are a hand-maintained duplicate of internal/burlercli's profileYAML
// kebab-case shape, kept identical deliberately so a human who has written a burler profile file
// reads a recipe row without a second vocabulary. review-path, fixer-report-path, prior-reviews,
// prior-fixer-reports, and cluster-exclude are deliberately absent because
// shedadapters.NewBurlerProducer's own doc states those five burlerengine.Profile fields and
// burlerengine.RunOpts.Round are overwritten per round, so a recipe author setting one would be
// setting a value the producer silently discards.
//
// rubric_stencil is the one key with no profileYAML counterpart: it names a stencilstore rubric
// stencil, read via stencilstore and stripped of its leading stencil-store comment banner, to set
// Profile.Rubric in place of a literal rubric string. Exactly one of rubric and rubric_stencil must
// be non-empty -- both set, or both empty, is a construction error naming both keys, because neither
// of Profile.validate's own checks can say which of the two the author meant.
//
// This entry does not check profile's inner required-ness beyond that one rule:
// burlerengine.Profile.validate already rejects an empty Rubric and a Target/Fasit with neither
// Paths nor Instructions, and duplicating that here would drift from it.
func burlerRoundProfile(cfg Config, stencilsDir string) (burlerengine.Profile, error) {
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
	if err := configRejectUnknown(cfg, "target", "fasit", "rubric", "rubric_stencil", "fix-scope", "tool-use", "cluster-fan"); err != nil {
		return burlerengine.Profile{}, err
	}

	// Exactly one of rubric and rubric_stencil must be non-empty. This is checked here, ahead of
	// burlerengine.Profile.validate's own empty-Rubric rejection, because that error names neither
	// key and cannot say which of the two the author meant.
	switch {
	case rubric != "" && rubricStencil != "":
		return burlerengine.Profile{}, fmt.Errorf("shedrecipe: BurlerRound: config keys \"rubric\" and \"rubric_stencil\" are mutually exclusive")
	case rubric == "" && rubricStencil == "":
		return burlerengine.Profile{}, fmt.Errorf("shedrecipe: BurlerRound: exactly one of config keys \"rubric\" and \"rubric_stencil\" is required")
	}

	if rubricStencil != "" {
		if err := requireAbsRoot("BurlerRound", "StencilsDir", stencilsDir); err != nil {
			return burlerengine.Profile{}, err
		}
		raw, err := stencilstore.Read(stencilsDir, rubricStencil)
		if err != nil {
			return burlerengine.Profile{}, fmt.Errorf("shedrecipe: BurlerRound: rubric_stencil %q: %w", rubricStencil, err)
		}
		// stencilstore stamps a "<!-- lyx-stencil: sha256=... -->" banner onto every seeded file, and
		// stencil.Fill strips a banner from the template it parses but never from a marker value, so
		// unstripped bytes would inject the banner into the middle of the round prompt. This mirrors
		// what shedadapters' own Bouncer already does with its rubric.
		rubric = stencil.StripLeadingComment(string(raw))
	}

	target, err := burlerRoundFileSet("BurlerRound", "target", targetCfg)
	if err != nil {
		return burlerengine.Profile{}, err
	}
	fasit, err := burlerRoundFileSet("BurlerRound", "fasit", fasitCfg)
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
		Target:     target,
		Fasit:      fasit,
		Rubric:     rubric,
		FixScope:   burlerengine.FixScope(fixScope),
		ToolUse:    toolUse,
		ClusterFan: clusterFan,
	}, nil
}
