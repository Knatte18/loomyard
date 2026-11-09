// loomshed.go declares loom's durable row names and nothing else.

package loomshed

// The producer names, verbatim per the rows of contracts/recipes/tapestry-recipe.yaml.
// Each name is the durable on-disk identity in current_producer.
// A later rename breaks resume for any in-flight task.
// So every row is built from these constants, never a repeated string literal.
//
// NameParentAlign and NameFinalize are generic ShedProducers shared with the landing shed, not loom's own producers.
// They are declared here like every other row, because the tapestry recipe names them too.
// contracts/recipes/tapestry-recipe.yaml spells the same names as YAML strings; these constants remain the authority.
// internal/loomrecipe's coverage guard pins the two declarations together.
// It keys its row table off these constants rather than off string literals.
//
// The constants stay here rather than in internal/loomrecipe, because interruptpolicy.go reads them.
// Moving them would make loomshed import loomrecipe, which imports shedbuild -> shedrecipe -> loomshed:
// the production cycle the consumer-package split exists to avoid.
const (
	NamePreflight         = "Preflight"
	NameDiscussionWrite   = "Discussion-Write"
	NameDiscussionBouncer = "Discussion-Bouncer"
	NamePlanWrite         = "Plan-Write"
	NamePartsWrite        = "Parts-Write"
	NamePartsPlanWrite    = "Parts-Plan-Write"
	NamePlanBouncer       = "Plan-Bouncer"
	NameWebster           = "Webster"
	NameDescribe          = "Describe"
	NameParentAlign       = "Parent-Align"
	NameDarnWrite         = "Darn-Write"
	NameSamplerSession    = "Sampler-Session"
	NameRecordWrite       = "Record-Write"
	NameFinalize          = "Finalize"
	NameFrictionReflect   = "Friction-Reflect"

	NameDiscussionBurlerReview = "Discussion-Burler-Review"
	NameDiscussionBurlerFix    = "Discussion-Burler-Fix"
	NamePlanBurlerReview       = "Plan-Burler-Review"
	NamePlanBurlerFix          = "Plan-Burler-Fix"

	NameCodeBouncer      = "Code-Bouncer"
	NameCodeBurlerReview = "Code-Burler-Review"
	NameCodeBurlerFix    = "Code-Burler-Fix"
	NamePRReview         = "PR-Review"

	// NameMotif is the block row of the tapestry recipe that runs the motif recipe.
	NameMotif = "Motif"

	// NameRaddleWrite is the raddle recipe's row of the agent session that proposes the split of a decision record.
	NameRaddleWrite = "Raddle-Write"
	// NameRaddleFile is the raddle recipe's Go row that files the approved proposal on the board.
	NameRaddleFile = "Raddle-File"

	// NameRaddleBouncer, NameRaddleBurlerReview and NameRaddleBurlerFix are the arras recipe's split perch.
	// They are the Raddle-Review ring over the raddle proposal.
	NameRaddleBouncer      = "Raddle-Bouncer"
	NameRaddleBurlerReview = "Raddle-Burler-Review"
	NameRaddleBurlerFix    = "Raddle-Burler-Fix"

	NameDriftBaseline = "Drift-Baseline"
	NameDriftAwait    = "Drift-Await"
	NameDriftExpect   = "Drift-Expect"
	NameDriftVerify   = "Drift-Verify"
	NameDriftRecheck  = "Drift-Recheck"
)
