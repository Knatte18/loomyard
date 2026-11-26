// keys.go declares the dotted loom.yaml and tapestry.yaml key paths that readers and messages cite.
// Each path is built from its area and leaf constants, and a message names it in full.

package loomengine

import (
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/reviewbudget"
)

// PerchBudgetKeyName returns the full name of the key holding perch's own bounce budget.
func PerchBudgetKeyName(perch Perch) string {
	return FullKeyName(reviewbudget.SoftKey(string(perch)))
}

// RunBudgetKeyName returns the full name of the key holding the run-wide bounce budget.
func RunBudgetKeyName() string {
	return FullKeyName(reviewbudget.RunSoftKey)
}

// shuttleRunTimeoutName returns the full name of the shuttle.yaml key bounding a run that sets no timeout of its own.
func shuttleRunTimeoutName() string {
	return configengine.KeyName("shuttle", "run_timeout_min")
}

// DarnVerifyAttemptsName returns the full name of the darn.yaml key holding the darn recipe's verify budget.
func DarnVerifyAttemptsName() string {
	return configengine.KeyName("darn", "verify_attempts")
}

// FullKeyName returns the full name, `<module>.<path>`, of the loom.yaml or tapestry.yaml key at path.
// A path under the plan or parts area belongs to tapestry, and every other path to loom.
func FullKeyName(path string) string {
	module := loomModule
	if area, _, _ := strings.Cut(path, "."); area == areaPlan || area == areaParts {
		module = tapestryModule
	}
	return configengine.KeyName(module, path)
}

// The areas, each one mapping of loom.yaml or tapestry.yaml.
const (
	areaDiscussion = "discussion"
	areaCode       = "code"
	areaRaddle     = "raddle"
	areaFriction   = "friction"
	areaPR         = "pr"
	areaReview     = "review"
	areaPlan       = "plan"
	areaParts      = "parts"
)

// The leaf names the areas hold.
const (
	leafModel                  = "model"
	leafTimeoutMin             = "timeout_min"
	leafInteractive            = "interactive"
	leafAdvisors               = "advisors"
	leafLenses                 = "lenses"
	leafGateFixer              = "gate_fixer"
	leafReviewerModel          = "reviewer_model"
	leafFixerModel             = "fixer_model"
	leafReviewerFan            = "reviewer_fan"
	leafFixerFan               = "fixer_fan"
	leafAssistReviewerModel    = "assist_reviewer_model"
	leafAssistReviewerFan      = "assist_reviewer_fan"
	leafAssistTimeoutMin       = "assist_reviewer_timeout_min"
	leafJudge                  = "judge"
	leafSeed                   = "seed"
	leafAutoAccept             = "auto_accept"
	leafAutoApprove            = "auto_approve"
	leafReviewAskParent        = "review_ask_parent"
	leafReviewParentTimeoutMin = "review_parent_timeout_min"
	leafCirclingCheckpoint     = "circling_checkpoint"
	leafStalemateRounds        = "stalemate_rounds"
	leafDepth                  = "depth"
	leafMax                    = "max"
	leafWriter                 = "writer"
	leafChair                  = "chair"
	leafChairTimeoutMin        = "chair_timeout_min"
	leafSplit                  = "split"
)

// The loom.yaml paths.
const (
	discussionModelKey               = areaDiscussion + "." + leafModel
	discussionTimeoutMinKey          = areaDiscussion + "." + leafTimeoutMin
	discussionInteractiveKey         = areaDiscussion + "." + leafInteractive
	discussionAdvisorsKey            = areaDiscussion + "." + leafAdvisors
	discussionLensesKey              = areaDiscussion + "." + leafLenses
	discussionGateFixerKey           = areaDiscussion + "." + leafGateFixer
	discussionReviewParentTimeoutKey = areaDiscussion + "." + leafReviewParentTimeoutMin
	codeAutoAcceptKey                = areaCode + "." + leafAutoAccept
	codeReviewParentTimeoutKey       = areaCode + "." + leafReviewParentTimeoutMin
	raddleModelKey                   = areaRaddle + "." + leafModel
	raddleAutoApproveKey             = areaRaddle + "." + leafAutoApprove
	frictionModelKey                 = areaFriction + "." + leafModel
	frictionTimeoutMinKey            = areaFriction + "." + leafTimeoutMin
	prReviewAskParentKey             = areaPR + "." + leafReviewAskParent
	prReviewParentTimeoutKey         = areaPR + "." + leafReviewParentTimeoutMin
	reviewReviewerModelKey           = areaReview + "." + leafReviewerModel
	reviewFixerModelKey              = areaReview + "." + leafFixerModel
	reviewJudgeKey                   = areaReview + "." + leafJudge
	reviewSeedKey                    = areaReview + "." + leafSeed
	reviewTimeoutMinKey              = areaReview + "." + leafTimeoutMin
	reviewAssistTimeoutMinKey        = areaReview + "." + leafAssistTimeoutMin
	reviewCirclingCheckpointKey      = areaReview + "." + leafCirclingCheckpoint
	reviewStalemateRoundsKey         = areaReview + "." + leafStalemateRounds
	driverKey                        = "driver"
	stepIdleTimeoutMinKey            = "step_idle_timeout_min"
	// subagentModelKey names the writers' lookup subagent model; its value fills the stencil marker of the same name.
	subagentModelKey = "subagent_model"
)

// The tapestry.yaml paths.
const (
	planModelKey            = areaPlan + "." + leafModel
	planTimeoutMinKey       = areaPlan + "." + leafTimeoutMin
	planLensesKey           = areaPlan + "." + leafLenses
	planGateFixerKey        = areaPlan + "." + leafGateFixer
	planSplitKey            = areaPlan + "." + leafSplit
	partsModelKey           = areaParts + "." + leafModel
	partsDepthKey           = areaParts + "." + leafDepth
	partsMaxKey             = areaParts + "." + leafMax
	partsWriterKey          = areaParts + "." + leafWriter
	partsChairKey           = areaParts + "." + leafChair
	partsChairTimeoutMinKey = areaParts + "." + leafChairTimeoutMin
)

// segmentKey returns the path of leaf in the area of the review segment named segment.
func segmentKey(segment, leaf string) string {
	return segment + "." + leaf
}
