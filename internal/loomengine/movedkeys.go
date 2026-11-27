// movedkeys.go declares the move tables of loom.yaml and tapestry.yaml: each flat key name and the nested path it took.
// Each table is written out key by key, so a later template key never joins it.

package loomengine

import "github.com/Knatte18/loomyard/internal/reviewbudget"

// loomMovedKeys returns the loom.yaml move table, each old key mapped to its new dotted path.
func loomMovedKeys() map[string]string {
	discussion, code, split := string(PerchDiscussion), string(PerchCode), string(PerchSplit)
	return map[string]string{
		"discussion":                           discussionModelKey,
		"discussion_timeout_min":               discussionTimeoutMinKey,
		"discussion_interactive":               discussionInteractiveKey,
		"discussion_advisors":                  discussionAdvisorsKey,
		"discussion_lenses":                    discussionLensesKey,
		"discussion_gate_fixer":                discussionGateFixerKey,
		"discussion_reviewer_model":            segmentKey(areaDiscussion, leafReviewerModel),
		"discussion_fixer_model":               segmentKey(areaDiscussion, leafFixerModel),
		"discussion_reviewer_fan":              segmentKey(areaDiscussion, leafReviewerFan),
		"discussion_fixer_fan":                 segmentKey(areaDiscussion, leafFixerFan),
		"discussion_assist_reviewer_model":     segmentKey(areaDiscussion, leafAssistReviewerModel),
		"discussion_assist_reviewer_fan":       segmentKey(areaDiscussion, leafAssistReviewerFan),
		"discussion_judge":                     segmentKey(areaDiscussion, leafJudge),
		"discussion_seed":                      segmentKey(areaDiscussion, leafSeed),
		"discussion_max_bounces":               reviewbudget.SoftKey(discussion),
		"discussion_hard_max_bounces":          reviewbudget.HardKey(discussion),
		"discussion_auto_accept":               reviewbudget.AutoAcceptKey(discussion),
		"discussion_review_ask_parent":         segmentKey(areaDiscussion, leafReviewAskParent),
		"discussion_review_parent_timeout_min": discussionReviewParentTimeoutKey,

		"code_reviewer_model":            segmentKey(areaCode, leafReviewerModel),
		"code_fixer_model":               segmentKey(areaCode, leafFixerModel),
		"code_reviewer_fan":              segmentKey(areaCode, leafReviewerFan),
		"code_fixer_fan":                 segmentKey(areaCode, leafFixerFan),
		"code_assist_reviewer_model":     segmentKey(areaCode, leafAssistReviewerModel),
		"code_assist_reviewer_fan":       segmentKey(areaCode, leafAssistReviewerFan),
		"code_judge":                     segmentKey(areaCode, leafJudge),
		"code_seed":                      segmentKey(areaCode, leafSeed),
		"code_max_bounces":               reviewbudget.SoftKey(code),
		"code_hard_max_bounces":          reviewbudget.HardKey(code),
		"code_auto_accept":               codeAutoAcceptKey,
		"code_review_ask_parent":         segmentKey(areaCode, leafReviewAskParent),
		"code_review_parent_timeout_min": codeReviewParentTimeoutKey,

		"split_reviewer_model":   segmentKey(split, leafReviewerModel),
		"split_fixer_model":      segmentKey(split, leafFixerModel),
		"split_reviewer_fan":     segmentKey(split, leafReviewerFan),
		"split_fixer_fan":        segmentKey(split, leafFixerFan),
		"split_judge":            segmentKey(split, leafJudge),
		"split_max_bounces":      reviewbudget.SoftKey(split),
		"split_hard_max_bounces": reviewbudget.HardKey(split),
		"split_auto_accept":      reviewbudget.AutoAcceptKey(split),

		"raddle":               raddleModelKey,
		"raddle_auto_approve":  raddleAutoApproveKey,
		"friction":             frictionModelKey,
		"friction_timeout_min": frictionTimeoutMinKey,

		"review_timeout_min":         reviewTimeoutMinKey,
		"review_circling_checkpoint": reviewCirclingCheckpointKey,
		"review_stalemate_rounds":    reviewStalemateRoundsKey,
		"review_max_bounces":         reviewbudget.RunSoftKey,
		"review_hard_max_bounces":    reviewbudget.RunHardKey,
		"review_amend_max_bounces":   reviewbudget.RunAmendKey,

		"reviewer_model":              reviewReviewerModelKey,
		"fixer_model":                 reviewFixerModelKey,
		"judge":                       reviewJudgeKey,
		"seed":                        reviewSeedKey,
		"assist_reviewer_timeout_min": reviewAssistTimeoutMinKey,
	}
}

// tapestryMovedKeys returns the tapestry.yaml move table, each old key mapped to its new dotted path.
func tapestryMovedKeys() map[string]string {
	plan := string(PerchPlan)
	return map[string]string{
		"plan":                       planModelKey,
		"plan_timeout_min":           planTimeoutMinKey,
		"plan_lenses":                planLensesKey,
		"plan_gate_fixer":            planGateFixerKey,
		"plan_split":                 planSplitKey,
		"plan_reviewer_model":        segmentKey(areaPlan, leafReviewerModel),
		"plan_fixer_model":           segmentKey(areaPlan, leafFixerModel),
		"plan_reviewer_fan":          segmentKey(areaPlan, leafReviewerFan),
		"plan_fixer_fan":             segmentKey(areaPlan, leafFixerFan),
		"plan_assist_reviewer_model": segmentKey(areaPlan, leafAssistReviewerModel),
		"plan_assist_reviewer_fan":   segmentKey(areaPlan, leafAssistReviewerFan),
		"plan_judge":                 segmentKey(areaPlan, leafJudge),
		"plan_seed":                  segmentKey(areaPlan, leafSeed),
		"plan_max_bounces":           reviewbudget.SoftKey(plan),
		"plan_hard_max_bounces":      reviewbudget.HardKey(plan),
		"plan_auto_accept":           reviewbudget.AutoAcceptKey(plan),

		"parts":                   partsModelKey,
		"parts_depth":             partsDepthKey,
		"parts_max":               partsMaxKey,
		"parts_chair":             partsChairKey,
		"parts_chair_timeout_min": partsChairTimeoutMinKey,
		"part_writer":             partsWriterKey,
	}
}
