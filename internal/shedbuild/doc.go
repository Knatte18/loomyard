// Package shedbuild owns the recipe file format: decoding a recipe document into a Recipe and
// assembling a Recipe plus a caller-supplied shedrecipe.Env into the []shedengine.ProducerDef that
// shedengine.Shed already consumes unchanged.
//
// A recipe may set an optional top-level amendment_target naming the row a plan amendment routes to.
// Parse decodes it into Recipe.AmendmentTarget without checking that it names a row, since routing is shedengine's and shedcheck's.
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
