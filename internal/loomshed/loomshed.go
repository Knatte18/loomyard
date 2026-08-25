// loomshed.go declares loom's seventeen durable row names and nothing else.

package loomshed

// The seventeen producer names, verbatim per manifest/designs/loom.md's producer table. The name is
// the durable on-disk identity in current_producer; a later rename breaks resume for any in-flight
// task, so every row below is built from these constants, never a repeated string literal.
//
// NamePublish and NameFinalize are not loom's own producers -- both are generic ShedProducers
// shared by reference with Hardener's own list (see internal/landingshed's own package
// documentation) -- but the name constants live here regardless, same as every other row, because
// loom's own producer table names them, and the table is now the recipe's:
// contracts/recipes/loom-recipe.yaml spells the same seventeen names as yaml strings, these
// constants remain the authority, and internal/loomrecipe's coverage guard is what pins the two
// declarations together by keying its row table off these symbols rather than off string literals.
//
// The constants stay here rather than moving to internal/loomrecipe: seed.go and loompreflight.go
// read two of them, so loomshed would have to import loomrecipe, and loomrecipe imports shedbuild
// -> shedrecipe -> loomshed -- the production cycle the consumer-package split exists to avoid.
const (
	NamePreflight          = "Preflight"
	NameLoomPreflight      = "Loom-Preflight"
	NameDiscussionWrite    = "Discussion-Write"
	NameDiscussionValidate = "Discussion-Validate"
	NameDiscussionBouncer  = "Discussion-Bouncer"
	NameDiscussionBurler   = "Discussion-Burler"
	NamePlanWrite          = "Plan-Write"
	NamePlanValidate       = "Plan-Validate"
	NamePlanBouncer        = "Plan-Bouncer"
	NamePlanBurler         = "Plan-Burler"
	NamePlanRevalidate     = "Plan-Revalidate"
	NameBatchifier         = "Batchifier"
	NameWebster            = "Webster"
	NameWebsterBouncer     = "Webster-Bouncer"
	NameWebsterBurler      = "Webster-Burler"
	NamePublish            = "Publish"
	NameFinalize           = "Finalize"
)
