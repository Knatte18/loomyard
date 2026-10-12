// entries_bouncer.go implements bouncerEntry, the Constructor for the "Bouncer" registry row:
// the segment's entry point, which joins and creates the run directory its partner BurlerReview and BurlerFix rows write into and read from.

package shedrecipe

import (
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// bouncerEntry is the Constructor for the "Bouncer" registry row: it validates cfg and env, joins and creates the run directory a segment's Bouncer, BurlerReview and BurlerFix rows share, resolves artifact_paths against env.AnchorPath, takes the row's seed and judge models from env.RowBouncerModels, resolves the optional commit_seam key to one of env.CommitPlan or env.CommitDiscussion, resolves the optional approve_seam key to env.ApprovePlan or env.ApproveMergeReview, resolves the optional skip_seam key to env.SkipPlanReview or env.SkipMergeReview, reads the optional boolean blocking_escalates key (default false) and the optional color_segment key, reads the optional carry_over key, the segment name that selects env.CarryOver, reads the optional boolean cluster_excludes key (default false) telling the judge whether its partner BurlerReview row has a cluster fan, and returns shedadapters.NewBouncer(cfg).
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
	commitSeam, err := configString(cfg, "commit_seam", false)
	if err != nil {
		return nil, err
	}
	approveSeam, err := configString(cfg, "approve_seam", false)
	if err != nil {
		return nil, err
	}
	skipSeam, err := configString(cfg, "skip_seam", false)
	if err != nil {
		return nil, err
	}
	carryOverSegment, err := configString(cfg, "carry_over", false)
	if err != nil {
		return nil, err
	}
	clusterExcludes, err := configBool(cfg, "cluster_excludes", false)
	if err != nil {
		return nil, err
	}
	blockingEscalates, err := configBool(cfg, "blocking_escalates", false)
	if err != nil {
		return nil, err
	}
	colorSegment, err := colorSegmentKey("Bouncer", cfg)
	if err != nil {
		return nil, err
	}
	// A fan the wiring names for this row asks the judge for exclude_lenses exactly as the row's own cluster_excludes key does.
	clusterExcludes = clusterExcludes || env.RowClusterFans[name] != ""
	// There is deliberately no "report_name" key: BouncerConfig.ReportName is pinned below, not
	// recipe-authorable, so a "report_name" entry in cfg is rejected here as unrecognised rather
	// than silently ignored.
	// The same holds for "model", "effort" and "version": a Bouncer pass's model comes from env.RowBouncerModels alone.
	if err := configRejectUnknown(cfg, "run_subdir", "artifact_paths", "rubric_stencil", "commit_seam", "approve_seam", "skip_seam", "carry_over", "cluster_excludes", "blocking_escalates", "color_segment"); err != nil {
		return nil, err
	}
	models, ok := env.RowBouncerModels[name]
	if !ok {
		return nil, fmt.Errorf("shedrecipe: Bouncer: row %q has no entry in Env.RowBouncerModels; the wiring must fill one per Bouncer row", name)
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
	case "merge":
		if err := requireSeam("Bouncer", "ApproveMergeReview", env.ApproveMergeReview); err != nil {
			return nil, err
		}
		approve = env.ApproveMergeReview
	default:
		return nil, fmt.Errorf("shedrecipe: Bouncer: config key %q must be %q or %q, got %q", "approve_seam", "plan", "merge", approveSeam)
	}

	// skip_seam names env.SkipPlanReview (rework-exempt) or env.SkipMergeReview (merge-overlap), and mirrors approve_seam: absent leaves BouncerConfig.Skip nil,
	// and a present key is guarded by requireSeam so a nil Env closure never silently disables the skip.
	var skip func() (bool, error)
	switch skipSeam {
	case "":
		// No seam configured; skip stays nil.
	case "rework-exempt":
		if err := requireSeam("Bouncer", "SkipPlanReview", env.SkipPlanReview); err != nil {
			return nil, err
		}
		skip = env.SkipPlanReview
	case "merge-overlap":
		if err := requireSeam("Bouncer", "SkipMergeReview", env.SkipMergeReview); err != nil {
			return nil, err
		}
		skip = env.SkipMergeReview
	default:
		return nil, fmt.Errorf("shedrecipe: Bouncer: config key %q must be %q or %q, got %q", "skip_seam", "rework-exempt", "merge-overlap", skipSeam)
	}

	// carry_over carries the segment name the Bouncer files its carry-over entry under and selects env.CarryOver, mirroring skip_seam:
	// absent leaves BouncerConfig.CarryOver nil, and a present key is guarded by requireSeam so a nil Env closure never silently drops the entry.
	var carryOver func(discussionparser.CarryOver) error
	if carryOverSegment != "" {
		if err := requireSeam("Bouncer", "CarryOver", env.CarryOver); err != nil {
			return nil, err
		}
		carryOver = env.CarryOver
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
	if err := requireAbsRoot("Bouncer", "SpecsDir", env.SpecsDir); err != nil {
		return nil, err
	}
	if err := requireSeam("Bouncer", "Shuttle", env.Shuttle); err != nil {
		return nil, err
	}
	if env.ReviewCirclingCheckpoint < 1 {
		return nil, fmt.Errorf("shedrecipe: Bouncer: Env.ReviewCirclingCheckpoint must be positive, got %d", env.ReviewCirclingCheckpoint)
	}

	runDir, err := resolveUnderRoot("Bouncer", "run_subdir", env.RunRoot, runSubdir)
	if err != nil {
		return nil, err
	}
	// The entry creates the joined run directory, not the caller or the round producer:
	// Bouncer.Call reaches shedadapters.ResolveRound first, which os.Stats RunDir and hard-errors when it is absent,
	// and the Bouncer is its segment's entry point, so it runs before the review and fix producers' own idempotent os.MkdirAll would have created it.
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

	// Slug and SegmentBounces are both optional:
	// a caller filling neither keeps a working Bouncer whose escalation Reason omits the slug, whose brief render degrades, and which never escalates on budget.
	var bounces func() (count, budget int, ok bool, err error)
	if env.SegmentBounces != nil {
		bounces = func() (int, int, bool, error) { return env.SegmentBounces(name) }
	}

	bouncerCfg := shedadapters.BouncerConfig{
		Name:          name,
		RunDir:        runDir,
		ArtifactPaths: resolvedArtifactPaths,
		// ReportName is pinned rather than configurable: shedadapters.BurlerReviewProducer writes its
		// report to a hardcoded round-<n>-review.md under RunDir, and shedadapters.ResolveRound
		// finds the current round by statting that same name, so any other value resolves the
		// round to 0 forever and the Bouncer re-seeds every call until its bounce budget is spent,
		// with no error anywhere.
		ReportName:      func(round int) string { return fmt.Sprintf("round-%d-review.md", round) },
		StencilsDir:     env.StencilsDir,
		WorktreeRoot:    env.WorktreeRoot,
		SpecsDir:        env.SpecsDir,
		PublishFailure:  env.PublishFailure,
		WebsterRecord:   env.WebsterRecord,
		MergeReview:     env.MergeReview,
		RubricStencil:   rubricStencil,
		Models:          models,
		ClusterExcludes: clusterExcludes,
		Shuttle:         env.Shuttle,
		Approve:         approve,
		Commit:          commit,
		Skip:            skip,
		Segment:         carryOverSegment,
		AnchorPath:      env.AnchorPath,
		CarryOver:       carryOver,
		Now:             env.Now,
		Slug:            env.Slug,
		ParentName:      env.ParentName,
		Bounces:         bounces,
		AutoAccept:      env.RowAutoAccept[name],

		BlockingEscalates: blockingEscalates,
		ColorSegment:      colorSegment,

		DecisionRecordPath: env.DecisionRecordPath,

		CirclingCheckpoint: env.ReviewCirclingCheckpoint,
	}

	// NewBouncer's own eager rubric-stencil probe is what makes a mistyped rubric_stencil fail here,
	// at construction, rather than at first Call.
	producer, err := shedadapters.NewBouncer(bouncerCfg)
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: Bouncer: %w", err)
	}
	return producer, nil
}
