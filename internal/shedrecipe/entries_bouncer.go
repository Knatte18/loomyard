// entries_bouncer.go implements bouncerEntry, the Constructor for the "Bouncer" registry row: the
// segment's entry point, which joins and creates the run directory its opposite BurlerRound row
// writes into and reads from.

package shedrecipe

import (
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// bouncerEntry is the Constructor for the "Bouncer" registry row: it validates cfg and env, joins
// and creates the run directory a segment's Bouncer and BurlerRound rows share, resolves
// artifact_paths against env.AnchorPath, resolves the optional commit_seam key to one of
// env.CommitPlan or env.CommitDiscussion, resolves the optional approve_seam key to
// env.ApprovePlan and nothing else, and returns shedadapters.NewBouncer(cfg).
func bouncerEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
	runSubdir, err := configString(cfg, "run_subdir", true)
	if err != nil {
		return nil, err
	}
	artifactPaths, err := configStringSlice(cfg, "artifact_paths", true)
	if err != nil {
		return nil, err
	}
	rubricStencil, err := configString(cfg, "rubric_stencil", true)
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
	version, err := configString(cfg, "version", false)
	if err != nil {
		return nil, err
	}
	commitSeam, err := configString(cfg, "commit_seam", false)
	if err != nil {
		return nil, err
	}
	approveSeam, err := configString(cfg, "approve_seam", false)
	if err != nil {
		return nil, err
	}
	// A row setting model/effort/version overrides the Env value; a row omitting it takes the Env
	// value; both absent leaves the provider default. An empty Config value and an absent key are
	// the same thing here -- configString with required false returns "" for both -- and that is
	// deliberate: there is no meaningful "explicitly empty" model.
	if model == "" {
		model = env.ReviewModel
	}
	if effort == "" {
		effort = env.ReviewEffort
	}
	if version == "" {
		version = env.ReviewVersion
	}
	// There is deliberately no "report_name" key: BouncerConfig.ReportName is pinned below, not
	// recipe-authorable, so a "report_name" entry in cfg is rejected here as unrecognised rather
	// than silently ignored.
	if err := configRejectUnknown(cfg, "run_subdir", "artifact_paths", "rubric_stencil", "model", "effort", "version", "commit_seam", "approve_seam"); err != nil {
		return nil, err
	}

	// commit_seam and approve_seam are two independent optional config keys: commit_seam names
	// which of Env's two commit closures fills BouncerConfig.Commit, and approve_seam (below)
	// resolves to env.ApprovePlan and nothing else, filling BouncerConfig.Approve. Absent
	// leaves the resolved closure nil -- "no seam configured" is a legitimate configuration and
	// never an error, which is what keeps every existing Bouncer row valid unchanged. A present
	// key is guarded by requireSeam on the named Env field rather than assigned directly: without
	// that guard a nil Env closure would silently assign a nil Commit, reproducing the exact
	// no-seam condition this key exists to eliminate, with no error anywhere.
	var commit func() error
	switch commitSeam {
	case "":
		// No seam configured; commit stays nil.
	case "plan":
		if err := requireSeam("Bouncer", "CommitPlan", env.CommitPlan); err != nil {
			return nil, err
		}
		commit = env.CommitPlan
	case "discussion":
		if err := requireSeam("Bouncer", "CommitDiscussion", env.CommitDiscussion); err != nil {
			return nil, err
		}
		commit = env.CommitDiscussion
	default:
		return nil, fmt.Errorf("shedrecipe: Bouncer: config key %q must be %q or %q, got %q", "commit_seam", "plan", "discussion", commitSeam)
	}

	// approve_seam names env.ApprovePlan and nothing else -- there is no discussion-side or
	// webster-side approval flag, so a second accepted value would be a hypothetical. It mirrors
	// commit_seam's switch one-for-one: absent leaves the resolved closure nil, which keeps every
	// existing Bouncer row valid unchanged, and a present key is guarded by requireSeam for the
	// same reason commit_seam's guard exists -- without it a nil Env closure would silently assign
	// a nil Approve, reproducing the exact no-seam condition this key exists to eliminate.
	var approve func() error
	switch approveSeam {
	case "":
		// No seam configured; approve stays nil.
	case "plan":
		if err := requireSeam("Bouncer", "ApprovePlan", env.ApprovePlan); err != nil {
			return nil, err
		}
		approve = env.ApprovePlan
	default:
		return nil, fmt.Errorf("shedrecipe: Bouncer: config key %q must be %q, got %q", "approve_seam", "plan", approveSeam)
	}

	if err := requireAbsRoot("Bouncer", "RunRoot", env.RunRoot); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("Bouncer", "AnchorPath", env.AnchorPath); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("Bouncer", "StencilsDir", env.StencilsDir); err != nil {
		return nil, err
	}
	if err := requireSeam("Bouncer", "Shuttle", env.Shuttle); err != nil {
		return nil, err
	}

	runDir, err := resolveUnderRoot("Bouncer", "run_subdir", env.RunRoot, runSubdir)
	if err != nil {
		return nil, err
	}
	// The entry creates the joined run directory, not the caller or the round producer: Bouncer.Call
	// reaches shedadapters.ResolveRound first, which os.Stats RunDir and hard-errors when it is
	// absent, and the Bouncer is its segment's entry point, so it runs before BurlerProducer.Call's
	// own idempotent os.MkdirAll would have created it.
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return nil, fmt.Errorf("shedrecipe: Bouncer: create run dir %q: %w", runDir, err)
	}

	resolvedArtifactPaths := make([]string, len(artifactPaths))
	for i, p := range artifactPaths {
		resolved, err := resolveUnderRoot("Bouncer", "artifact_paths", env.AnchorPath, p)
		if err != nil {
			return nil, err
		}
		resolvedArtifactPaths[i] = resolved
	}

	bouncerCfg := shedadapters.BouncerConfig{
		Name:          name,
		RunDir:        runDir,
		ArtifactPaths: resolvedArtifactPaths,
		// ReportName is pinned rather than configurable: shedadapters.BurlerProducer writes its
		// report to a hardcoded round-<n>-review.md under RunDir, and shedadapters.ResolveRound
		// finds the current round by statting that same name, so any other value resolves the
		// round to 0 forever and the Bouncer re-seeds every call until its bounce budget is spent,
		// with no error anywhere.
		ReportName:    func(round int) string { return fmt.Sprintf("round-%d-review.md", round) },
		StencilsDir:   env.StencilsDir,
		RubricStencil: rubricStencil,
		Model:         model,
		Effort:        effort,
		Version:       version,
		Shuttle:       env.Shuttle,
		Approve:       approve,
		Commit:        commit,
		Now:           env.Now,
	}

	// NewBouncer's own eager rubric-stencil probe is what makes a mistyped rubric_stencil fail here,
	// at construction, rather than at first Call.
	producer, err := shedadapters.NewBouncer(bouncerCfg)
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: Bouncer: %w", err)
	}
	return producer, nil
}
