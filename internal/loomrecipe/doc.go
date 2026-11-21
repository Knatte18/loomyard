// Package loomrecipe owns loom's recipe-backed producer-list construction.
// It takes every absolute path from its caller.
// It has no direct production import of internal/lymcwd, per the Told-Geometry Invariant.
//
// New builds a recipe's producer list and Routing projects its routing, each over any embedded loom recipe by name.
// An empty recipe name reads as the tapestry recipe, and a name that selects no embedded recipe is refused.
// Every accessor reads the expanded recipe, so a block row's sub-recipe rows are seen like any other.
//
// Consumers read a recipe's rows and keys through the accessors, never by comparing its name.
//
// The bounce budget of a review segment is not declared in the recipe.
// New takes it from shedrecipe.Env.Segments and Routing from a map of the same keys, so both hold one value per segment.
// Both set it on every row of the segment and refuse a missing entry or a negative one, naming the segment.
// A budget of 0 stamps `zero_rounds` on the segment's Bouncer row, which then settles approved at once;
// the segment keeps its route.
//
// The darn recipe's verify budget is not declared in it either.
// When a build row runs the DarnWrite engine, New stamps shedrecipe.Env.DarnVerifyAttempts onto that row's verify gates.
// It does the same for the verify gates of the Code-Review fix row, and refuses a value below 1.
// A darn budget of 0 for Code-Review also skips the `Describe` row through its entry gate `budget.Code-Review`.
//
// Every review segment of the tapestry, darn and arras recipes sets one `role_prefix`, and no two segments share one.
// Each Bouncer row's `carry_over` names the row's own segment.
//
// DiscussionApprovalRow answers the Bouncer row that approves a recipe's discussion, and none for a recipe without one.
// PastDiscussionApproval answers whether a run standing at a row is past that approval.
// Both read the expanded rows and never the recipe's name.
//
// HoldsSplitPerch and RowsHoldSplitPerch answer whether rows hold a split perch, a Bouncer row over the raddle proposal.
// CodePerchGate answers the entry gate of a recipe's Code-Review Bouncer row, nil when the perch carries none;
// a reader of whether the perch is off follows that gate as well as the budget.
//
// This package sits above internal/loomshed because internal/shedrecipe's registry imports loomshed.
// Building the recipe inside loomshed would make a loomshed -> shedbuild -> shedrecipe -> loomshed import cycle.
package loomrecipe
