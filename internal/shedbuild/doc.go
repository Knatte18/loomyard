// Package shedbuild owns the recipe file format.
// It decodes a recipe document into a Recipe.
// It assembles a Recipe and a shedrecipe.Env into the []shedengine.ProducerDef that shedengine.Shed consumes.
//
// A recipe may set a top-level amendment_target, which Parse decodes without checking that it names a row.
// Routing belongs to shedengine.
// A top-level contract is empty or `loom`, and a top-level verify_source is empty, `plan` or `tree_scope`.
// Parse refuses any other value of either.
// Build checks a recipe declaring `contract: loom` against the loom contract before any registry lookup.
// The engine sets come from shedrecipe, so a new loom recipe needs no change to the check.
//
// # Block rows
//
// A block row (block or block_gate set) names a recipe to run inline and the check that gates it.
// It carries only name, block, block_gate and on_done, and no engine; Parse refuses anything else.
// Row.SkipTo and Row.BlockPath sit outside the format and are set only by expansion,
// so a file that writes either is refused as an unknown key.
// Expand inlines each block row's sub-recipe at build time, and Build and RoutingFrom take the expanded recipe.
// A block row's entry gate is the check its block_gate names, never an entry_gate.
//
// # Entry gate
//
// A row may carry an optional entry_gate, one check or a list of checks that must all hold.
// A check is one of three forms, with no or, no arithmetic and no other expression:
//
//	<operand>
//	<operand> == <literal>
//	<operand> != <literal>
//
// An operand is `module.key`, a key of a registered config module named by its dotted path.
// It may instead be `budget.<Segment>`, a review segment's resolved soft cap.
// A literal is a YAML scalar; an unquoted one holds no whitespace.
// A bare `module.key` operand must hold a bool and holds when it is true.
// A bare `budget.<Segment>` operand holds when it is non-zero.
// A gate skips only its own row.
//
// This package holds PATTERN-cwd-resolution:
// the package derives no path of its own.
// It defines no on-disk location for recipe files.
//
// Parse is filesystem-free, but Build is not: some registry constructors reach disk at construction time.
// The bouncer constructor creates its resolved run directory and eagerly probes its rubric stencil.
// The burler-round constructor only creates its run directory.
// This package neither suppresses nor wraps those effects.
package shedbuild
