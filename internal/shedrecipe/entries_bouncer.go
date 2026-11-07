// entries_bouncer.go implements bouncerEntry, the Constructor for the "Bouncer" registry row.
// It is the segment's entry point: it joins and creates the run directory the segment's review and fix rows use.

package shedrecipe

import (
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// rolePrefixKey reads the optional role_prefix key of cfg as the role prefix of a Bouncer or Burler row's sessions.
// An absent key gives "".
// A present value that agentname.ValidateRole rejects is refused.
// The error names the entry, the key, the value and the way forward.
func rolePrefixKey(entry string, cfg Config) (string, error) {
	prefix, err := configString(cfg, "role_prefix", false)
	if err != nil || prefix == "" {
		return "", err
	}
	if err := agentname.ValidateRole(prefix); err != nil {
		return "", fmt.Errorf("shedrecipe: %s: config key %q value %q is not a valid role prefix: %w; way forward: set a lowercase [a-z][a-z0-9-]* prefix such as %q", entry, "role_prefix", prefix, err, "plan")
	}
	return prefix, nil
}

// bouncerEntry is the Constructor for the "Bouncer" registry row.
// It validates cfg and env, and it creates the run directory the segment's rows share.
// It resolves each seam key to the Env closure it names, and an absent key leaves that closure unset.
// It returns shedadapters.NewBouncer(cfg).
func bouncerEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
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
	subjectKind, err := configString(cfg, "subject", false)
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
	escalateDecisions, err := configBool(cfg, "escalate_decisions", false)
	if err != nil {
		return nil, err
	}
	zeroRounds, err := configBool(cfg, "zero_rounds", false)
	if err != nil {
		return nil, err
	}
	colorSegment, err := colorSegmentKey("Bouncer", cfg)
	if err != nil {
		return nil, err
	}
	// A recipe cannot set "report_name": BouncerConfig.ReportName is pinned in the literal below.
	// A "report_name" entry in cfg is therefore rejected as unrecognised, not silently ignored.
	// Likewise "model", "effort" and "version" are rejected:
	// a Bouncer pass's model comes from the segment's Bouncer pair alone.
	rolePrefix, err := rolePrefixKey("Bouncer", cfg)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "run_subdir", "artifact_paths", "rubric_stencil", "commit_seam", "approve_seam", "skip_seam", "carry_over", "subject", "cluster_excludes", "blocking_escalates", "escalate_decisions", "zero_rounds", "color_segment", "role_prefix", "reviewer_fan_setting", "auto_accept_setting"); err != nil {
		return nil, err
	}
	segment, err := rowSegment("Bouncer", row, env)
	if err != nil {
		return nil, err
	}
	// A setting naming a fan makes the judge decide per round from the usage record;
	// a row without one keeps its cluster_excludes key.
	fans, _, _, err := fanSetting("Bouncer", row, cfg, "reviewer_fan_setting", env)
	if err != nil {
		return nil, err
	}
	var clusterFans []string
	if hasFanName(fans) {
		clusterFans = fans
	}
	autoAccept, err := boolSetting("Bouncer", row, cfg, "auto_accept_setting", env)
	if err != nil {
		return nil, err
	}

	// commit_seam and approve_seam are independent optional keys.
	// commit_seam picks which of Env's two commit closures fills BouncerConfig.Commit.
	// approve_seam picks env.ApprovePlan or env.ApproveDiscussion for BouncerConfig.Approve.
	// An absent key leaves the closure nil: having no seam is legitimate, not an error.
	// A present key is guarded by requireSeam rather than assigned directly.
	// Without that guard, a nil Env closure would silently leave the closure nil with no error.
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

	// approve_seam accepts only the two approval closures on Env, env.ApprovePlan and env.ApproveDiscussion.
	// A present key is guarded by requireSeam, as commit_seam's is.
	var approve func() error
	switch approveSeam {
	case "":
		// No seam configured; approve stays nil.
	case "plan":
		if err := requireSeam("Bouncer", "ApprovePlan", env.ApprovePlan); err != nil {
			return nil, err
		}
		approve = env.ApprovePlan
	case "discussion":
		if err := requireSeam("Bouncer", "ApproveDiscussion", env.ApproveDiscussion); err != nil {
			return nil, err
		}
		approve = env.ApproveDiscussion
	default:
		return nil, fmt.Errorf("shedrecipe: Bouncer: config key %q must be %q or %q, got %q", "approve_seam", "plan", "discussion", approveSeam)
	}

	// skip_seam accepts "rework-exempt", which names env.SkipPlanReview.
	// An absent key leaves BouncerConfig.Skip nil.
	// A present key is guarded by requireSeam, so a nil Env closure never silently disables the skip.
	var skip func() (bool, error)
	switch skipSeam {
	case "":
		// No seam configured; skip stays nil.
	case "rework-exempt":
		if err := requireSeam("Bouncer", "SkipPlanReview", env.SkipPlanReview); err != nil {
			return nil, err
		}
		skip = env.SkipPlanReview
	default:
		return nil, fmt.Errorf("shedrecipe: Bouncer: config key %q must be %q, got %q", "skip_seam", "rework-exempt", skipSeam)
	}
	// A zero-round Bouncer skips over any named skip seam,
	// so it settles approved through its approve and commit seams with no review.
	if zeroRounds {
		skip = func() (bool, error) { return true, nil }
	}

	// carry_over names the segment the Bouncer files its carry-over entry under.
	// A present key selects env.CarryOver, guarded by requireSeam so a nil Env closure never silently drops the entry.
	// An absent key leaves BouncerConfig.CarryOver nil.
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
	// The entry creates the joined run directory, not the caller or the round producer.
	// Bouncer.Call reaches shedadapters.ResolveRound first, which stats RunDir and fails when it is absent.
	// The Bouncer is the segment's entry point, so it runs before the review and fix producers.
	// Their idempotent os.MkdirAll would otherwise be what creates the directory.
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

	// subject names the Env subject seams that tell the Bouncer what its segment judges.
	// An absent key tells the Bouncer no subject, which keeps it on the clear-and-reseed path.
	// A present key is guarded by requireSeam on every Env seam it selects.
	var (
		subject     func() (shedadapters.Subject, bool, error)
		mergeInOnly func(since string) (bool, error)
		settledBase func() (string, error)
	)
	switch subjectKind {
	case "":
		// No subject configured; the Bouncer keeps no record.
	case shedadapters.SubjectDocument:
		if err := requireSeam("Bouncer", "DocumentSubject", env.DocumentSubject); err != nil {
			return nil, err
		}
		if err := requireSeam("Bouncer", "RecordHead", env.RecordHead); err != nil {
			return nil, err
		}
		subject = func() (shedadapters.Subject, bool, error) { return env.DocumentSubject(resolvedArtifactPaths) }
		settledBase = env.RecordHead
	case shedadapters.SubjectCode:
		if err := requireSeam("Bouncer", "CodeSubject", env.CodeSubject); err != nil {
			return nil, err
		}
		if err := requireSeam("Bouncer", "MergeInOnly", env.MergeInOnly); err != nil {
			return nil, err
		}
		subject = env.CodeSubject
		mergeInOnly = env.MergeInOnly
	default:
		return nil, fmt.Errorf("shedrecipe: Bouncer: config key %q must be %q or %q, got %q", "subject", shedadapters.SubjectDocument, shedadapters.SubjectCode, subjectKind)
	}
	var reviewInput func() (string, time.Time, bool, error)
	if subject != nil {
		reviewInput = env.ReviewInput
	}

	// Slug and SegmentBounces are both optional.
	// A caller that fills neither still gets a working Bouncer.
	// Its escalation Reason omits the slug, its brief render degrades, and it never escalates on budget.
	var bounces func() (count, budget int, ok bool, err error)
	if env.SegmentBounces != nil {
		bounces = func() (int, int, bool, error) { return env.SegmentBounces(row.Name) }
	}

	bouncerCfg := shedadapters.BouncerConfig{
		Name:          row.Name,
		RunDir:        runDir,
		ArtifactPaths: resolvedArtifactPaths,
		// ReportName is pinned, not configurable.
		// shedadapters.BurlerReviewProducer writes its report to round-<n>-review.md under RunDir.
		// shedadapters.ResolveRound finds the current round by statting that name.
		// Any other name resolves every round to 0:
		// the Bouncer re-seeds on each call until its bounce budget is spent, with no error.
		ReportName:        func(round int) string { return fmt.Sprintf("round-%d-review.md", round) },
		StencilsDir:       env.StencilsDir,
		WorktreeRoot:      env.WorktreeRoot,
		PatternRecordPath: env.PatternRecordPath,
		SpecsDir:          env.SpecsDir,
		WebsterRecord:     env.WebsterRecord,
		ReviewBase:        env.ReviewBase,
		GateTiers:         env.GateTiers,
		RubricStencil:     rubricStencil,
		Models:            segment.Bouncer,
		ClusterExcludes:   clusterExcludes,
		ClusterFans:       clusterFans,
		Shuttle:           env.Shuttle,
		Approve:           approve,
		Commit:            commit,
		Skip:              skip,
		Segment:           carryOverSegment,
		AnchorPath:        env.AnchorPath,
		CarryOver:         carryOver,
		Now:               env.Now,
		Slug:              env.Slug,
		ParentName:        env.ParentName,
		Bounces:           bounces,
		AutoAccept:        autoAccept,
		HardCap:           segment.HardMaxBounces,
		Subject:           subject,
		MergeInOnly:       mergeInOnly,
		SettledBase:       settledBase,
		AmendAllowance:    segment.AmendMaxBounces,
		ReviewInput:       reviewInput,

		BlockingEscalates: blockingEscalates,
		EscalateDecisions: escalateDecisions,
		ColorSegment:      colorSegment,
		RolePrefix:        rolePrefix,

		DecisionRecordPath: env.DecisionRecordPath,

		CirclingCheckpoint: env.ReviewCirclingCheckpoint,
	}

	// NewBouncer probes the rubric stencil eagerly, at construction.
	// A mistyped rubric_stencil therefore fails here, not at the first Call.
	producer, err := shedadapters.NewBouncer(bouncerCfg)
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: Bouncer: %w", err)
	}
	return producer, nil
}
