// Package shedbuild owns the recipe file format: decoding a recipe document into a Recipe and
// assembling a Recipe plus a caller-supplied shedrecipe.Env into the []shedengine.ProducerDef that
// shedengine.Shed already consumes unchanged.
//
// A recipe may set an optional top-level amendment_target naming the row a plan amendment routes to.
// Parse decodes it into Recipe.AmendmentTarget without checking that it names a row, since routing is shedengine's and shedcheck's.
//
// A recipe may set a top-level contract, empty or `loom`, and a top-level verify_source, empty, `plan` or `tree_scope`.
// Parse refuses any other value of either.
// Build checks a recipe declaring `contract: loom` against the loom contract before any registry lookup:
// exactly one record row, exactly one build row after it on the on_done path from the entry, a Finalize row followed only by terminals, and a set verify_source.
// The engine sets come from shedrecipe, so a new loom recipe needs no change to the check.
//
// # Block rows and profile_recipe
//
// A row with block or block_gate set is a block row: it names a recipe to run inline and the check that gates it.
// A block row carries only name, block, block_gate and on_done, and no engine.
// Parse refuses a block row that also sets engine, config, on_stuck, on_reroute, segment, max_bounces or entry_gate, one missing block or block_gate, and a hand-written row whose engine is shedrecipe.EngineBlock, each naming the row and the way forward.
// A block row's on_done may be empty at Parse; Expand judges it.
// Row.SkipTo and Row.BlockPath sit outside the format and are set only by expansion, so a file that writes either is refused as an unknown key.
//
// A recipe may set a top-level profile_recipe naming the recipe whose run-profile block a task takes; empty means the recipe's own name.
// Parse decodes it into Recipe.ProfileRecipe without checking that it names a recipe.
//
// # Blocks
//
// Expand inlines each block row's sub-recipe into its parent at build time.
// It takes a parsed Recipe and a told resolver from a recipe name to its bytes, parses each sub-recipe through Parse and returns a new Recipe, never mutating its argument.
// A recipe with no block row is returned as it is, whatever the resolver.
//
// A sub-recipe is resolved, parsed and expanded first, its own blocks included.
// The block row is emitted with engine shedrecipe.EngineBlock, SkipTo the block's exit and on_done the sub-recipe's entry, followed by the sub-recipe's rows under their own names.
// A block's exit is its row's on_done as the recipe file sets it, or, when that is empty, the exit of the block enclosing it.
// Every inlined row's empty on_done, and an inlined block row's empty skip target, become the exit of the innermost block enclosing the row.
// Each inlined row's BlockPath is the names of the block rows enclosing it, outermost first, joined by "/"; the outermost block row and every row outside a block carry none.
// A sub-recipe's terminals are not read; the parent's stay the run's.
//
// Expand refuses, naming the block row and a way forward: a resolver that refuses the recipe, a nil resolver meeting a block row, a recipe that includes itself directly or through its own blocks, an inlined row named like a row outside its block, an inlined segment used outside the block, an inlined on_done, on_stuck or on_reroute that names no row of its recipe, an outermost block row with an empty on_done, a block row whose on_done lies inside its own block, and a sub-recipe that sets contract, verify_source, amendment_target or profile_recipe.
// The same recipe in two blocks of one parent is refused by the row-name check.
//
// Build and RoutingFrom take the expanded recipe; Build refuses a block row that was never expanded.
// A block row's entry gate is the check its block_gate names, one of the closed table shedrecipe ships, and not an entry_gate.
//
// # Entry gate
//
// A row may carry an optional entry_gate, one check or a list of checks that must all hold, which Build turns into the row's shedengine.EntryGate.
// Parse refuses a malformed check, naming the row and the way forward.
// A check is one of three forms, with no or, no arithmetic and no other expression:
//
//	<operand>
//	<operand> == <literal>
//	<operand> != <literal>
//
// An operand is `module.key`, a top-level key of a registered config module, or `budget.<Segment>`, a review segment's resolved soft cap.
// A literal is a YAML scalar; an unquoted one holds no whitespace.
//
// Build refuses a `module.key` operand that Env.SettingKnown rejects, or that meets a nil Env.SettingKnown or Env.Setting, and a segment with no Env.Segments entry.
// The gate reads at each evaluation: a setting through Env.Setting, over the pair's config stack, and a segment budget from Env.Segments.
// A bare `module.key` operand must hold a bool and holds when it is true, and any other type is an evaluation error naming the way forward and Env.Resume.
// A bare `budget.<Segment>` operand holds when it is non-zero.
// A comparison compares the value with the literal, numbers by value.
// A gate that does not hold returns the check's text with the value it read.
// A gate skips only its own row.
// RowEntryGate builds one row's gate the same way for a caller that reads a row's gate without building the recipe.
//
// This package holds PATTERN-cwd-resolution: Load reads
// exactly the absolute path it is told, and the package derives no path of its own.
//
// Parse is filesystem-free but Build is not, because two registry constructors reach disk at construction time between them, producing three distinct effects.
// The bouncer constructor does both, creating its resolved run directory and eagerly probing its rubric stencil, while the burler-round constructor only creates its run directory.
// This package neither suppresses nor wraps those effects.
//
// This package defines no on-disk location for recipe files -- no directory constant, no filename
// convention, no embedded default.
package shedbuild
