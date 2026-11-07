// Package shedbuild owns the recipe file format.
// It decodes a recipe document into a Recipe.
// It assembles a Recipe and a shedrecipe.Env into the []shedengine.ProducerDef that shedengine.Shed consumes.
//
// A recipe may set an optional top-level amendment_target naming the row a plan amendment routes to.
// Parse decodes it into Recipe.AmendmentTarget without checking that it names a row.
// Routing belongs to shedengine and shedcheck.
//
// A recipe may set a top-level contract, empty or `loom`, and a top-level verify_source, empty, `plan` or `tree_scope`.
// Parse refuses any other value of either.
// Build checks a recipe declaring `contract: loom` against the loom contract before any registry lookup:
// exactly one record row; exactly one build row after it on the on_done path from the entry;
// a Finalize row followed only by terminals; and a set verify_source.
// A recipe holding neither a build row nor a Finalize row is the contract's record-only form.
// It lands nothing and needs the record row alone.
// The engine sets come from shedrecipe, so a new loom recipe needs no change to the check.
//
// A recipe may set an optional top-level retired map.
// Each key is the name of a retired row, and each value is the row a run on it moves to.
// Parse decodes it into Recipe.Retired.
// It refuses a key that is also a row of the recipe and a target that names no row.
// Each refusal names the key and the way forward.
// NewShedFrom copies the map onto the shed, which moves a run off a retired row at its next step.
//
// # Block rows
//
// A row with block or block_gate set is a block row: it names a recipe to run inline and the check that gates it.
// A block row carries only name, block, block_gate and on_done, and no engine.
// Parse refuses a block row that also sets engine, config, on_stuck, on_reroute, segment, max_bounces or entry_gate;
// it refuses one missing block or block_gate, and a hand-written row whose engine is shedrecipe.EngineBlock.
// Each refusal names the row and the way forward.
// A block row's on_done may be empty at Parse; Expand judges it.
// Row.SkipTo and Row.BlockPath sit outside the format and are set only by expansion,
// so a file that writes either is refused as an unknown key.
//
// # Blocks
//
// Expand inlines each block row's sub-recipe into its parent at build time.
// It takes a parsed Recipe and a told resolver from a recipe name to its bytes.
// It parses each sub-recipe through Parse and returns a new Recipe, never mutating its argument.
// A recipe with no block row is returned as it is, whatever the resolver.
//
// A sub-recipe is resolved, parsed and expanded first, its own blocks included.
// The block row is emitted with engine shedrecipe.EngineBlock;
// its SkipTo is the block's exit and its on_done the sub-recipe's entry.
// The sub-recipe's rows follow under their own names.
// A block's exit is its row's on_done as the recipe file sets it.
// When that is empty, the exit is the exit of the block enclosing it.
// Every inlined row's empty on_done becomes the exit of the innermost block enclosing the row.
// An inlined block row's empty skip target does too.
// Each inlined row's BlockPath is the names of the block rows enclosing it, outermost first, joined by "/";
// the outermost block row and every row outside a block carry none.
// A sub-recipe's terminals and its top-level keys, contract, verify_source and amendment_target, are not read;
// the parent's stay the run's, so a recipe that runs alone may also be a block.
//
// Expand refuses, naming the block row and a way forward:
// a resolver that refuses the recipe; a nil resolver meeting a block row;
// a recipe that includes itself directly or through its own blocks; an inlined row named like a row outside its block;
// an inlined segment used outside the block; an inlined on_done, on_stuck or on_reroute that names no row of its recipe;
// an outermost block row with an empty on_done; and a block row whose on_done lies inside its own block.
// The same recipe in two blocks of one parent is refused by the row-name check.
//
// Build and RoutingFrom take the expanded recipe; Build refuses a block row that was never expanded.
// A block row's entry gate is the check its block_gate names, one of the closed table shedrecipe ships.
// It is not an entry_gate.
//
// # Entry gate
//
// A row may carry an optional entry_gate, one check or a list of checks that must all hold.
// Build turns it into the row's shedengine.EntryGate.
// Parse refuses a malformed check, naming the row and the way forward.
// A check is one of three forms, with no or, no arithmetic and no other expression:
//
//	<operand>
//	<operand> == <literal>
//	<operand> != <literal>
//
// An operand is `module.key`, a top-level key of a registered config module.
// It may also be `budget.<Segment>`, a review segment's resolved soft cap.
// A literal is a YAML scalar; an unquoted one holds no whitespace.
//
// Build refuses a `module.key` operand that Env.SettingKnown rejects.
// It also refuses one that meets a nil Env.SettingKnown or Env.Setting.
// It also refuses a segment with no Env.Segments entry.
// The gate reads at each evaluation:
// a setting through Env.Setting, over the pair's config stack, and a segment budget from Env.Segments.
// A bare `module.key` operand must hold a bool and holds when it is true,
// and any other type is an evaluation error naming the way forward and Env.Resume.
// A bare `budget.<Segment>` operand holds when it is non-zero.
// A comparison compares the value with the literal, numbers by value.
// A gate that does not hold returns the check's text with the value it read.
// A gate skips only its own row.
// RowEntryGate builds one row's gate the same way for a caller that reads a row's gate without building the recipe.
//
// This package holds PATTERN-cwd-resolution:
// Load reads exactly the absolute path it is told, and the package derives no path of its own.
//
// Parse is filesystem-free, but Build is not: some registry constructors reach disk at construction time.
// The bouncer constructor creates its resolved run directory and eagerly probes its rubric stencil.
// The burler-round constructor only creates its run directory.
// This package neither suppresses nor wraps those effects.
//
// This package defines no on-disk location for recipe files:
// no directory constant, no filename convention, no embedded default.
package shedbuild
