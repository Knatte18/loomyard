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
)

// burlerRoundEntry is the Constructor for the "BurlerRound" registry row: it validates cfg and env,
// maps cfg's profile map onto a burlerengine.Profile, joins and creates the run directory this
// row's segment shares with its Bouncer row, and returns
// shedadapters.NewBurlerProducer(name, env.Burler, profile, opts, runDir, env.Now).
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

	profile, err := burlerRoundProfile(profileCfg)
	if err != nil {
		return nil, err
	}

	opts := burlerengine.RunOpts{
		Model:   model,
		Effort:  effort,
		Timeout: time.Duration(timeoutS) * time.Second,
	}

	if err := requireAbsRoot("BurlerRound", "RunRoot", env.RunRoot); err != nil {
		return nil, err
	}
	if err := requireSeam("BurlerRound", "Burler", env.Burler); err != nil {
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

	producer, err := shedadapters.NewBurlerProducer(name, env.Burler, profile, opts, runDir, env.Now)
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: BurlerRound: %w", err)
	}
	return producer, nil
}

// burlerRoundFileSet maps profile's target or fasit sub-map onto a burlerengine.FileSet, recognising
// exactly paths and instructions and rejecting any other key.
func burlerRoundFileSet(entry, field string, cfg Config) (burlerengine.FileSet, error) {
	paths, err := configStringSlice(cfg, "paths", false)
	if err != nil {
		return burlerengine.FileSet{}, err
	}
	instructions, err := configString(cfg, "instructions", false)
	if err != nil {
		return burlerengine.FileSet{}, err
	}
	if err := configRejectUnknown(cfg, "paths", "instructions"); err != nil {
		return burlerengine.FileSet{}, err
	}
	return burlerengine.FileSet{Paths: paths, Instructions: instructions}, nil
}

// burlerRoundProfile maps cfg's profile map onto a burlerengine.Profile, recognising exactly six
// keys -- target, fasit, rubric, fix-scope, tool-use, and cluster-fan -- all optional at this level.
//
// These six key names are a hand-maintained duplicate of internal/burlercli's profileYAML
// kebab-case shape, kept identical deliberately so a human who has written a burler profile file
// reads a recipe row without a second vocabulary. review-path, fixer-report-path, prior-reviews,
// prior-fixer-reports, and cluster-exclude are deliberately absent because
// shedadapters.NewBurlerProducer's own doc states those five burlerengine.Profile fields and
// burlerengine.RunOpts.Round are overwritten per round, so a recipe author setting one would be
// setting a value the producer silently discards.
//
// This entry does not check profile's inner required-ness: burlerengine.Profile.validate already
// rejects an empty Rubric and a Target/Fasit with neither Paths nor Instructions, and duplicating
// that here would drift from it.
func burlerRoundProfile(cfg Config) (burlerengine.Profile, error) {
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
	if err := configRejectUnknown(cfg, "target", "fasit", "rubric", "fix-scope", "tool-use", "cluster-fan"); err != nil {
		return burlerengine.Profile{}, err
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
