// Package runprofile holds the hub's named run profiles, the `profiles` config module whose one file is `profiles.yaml` on Bolt.
//
// A profile maps a recipe name, `loom`, `darn` or `pawl`, to that recipe's settings in the shape of a board task's `run_settings`, module name to that module's config keys.
// One profile name serves every recipe, and each recipe's block may set the same key to its own value:
// darn's and pawl's Code-Review perch read loom's `code_*` keys, so a profile's darn and pawl blocks set `loom.code_max_bounces` for darn and pawl runs while its loom block sets it for loom runs.
// The reserved top-level `defaults` entry maps a recipe name to the profile a task of that recipe takes when it names none.
// A board task names a profile in its `profile` field;
// batten's Seed-Child writes the block of the recipe the task runs, of the task's profile or else its recipe's default, into the pair's local config layers with the task's own `run_settings` layered on top, so a later change of the profile never reaches a run already seeded.
// A task naming no profile, whose recipe has no default, runs on the hub's own config.
//
// The template's starting profiles are one ladder of review rounds per perch, `speed`, `light`, `solid` and `wide`, each perch's hard cap its rounds plus one, and `docs`, a darn-only profile for prose and config value edits;
// its defaults are `solid` for loom and `light` for darn and pawl.
// Each ladder profile's pawl block carries the same perch values as its darn block.
// A round count of 0 turns a perch off, since its Bouncer then settles approved with no review round.
//
// The module is hub-wide and seed-only: the hub reconcile seeds an absent file with the template's entries and never rewrites a present one.
// Each profile, like `defaults`, is one whole entry, and `lyx config profiles --set <name>='<json object>'` adds or replaces one whole entry;
// renaming one is adding it under the new name and removing the old block in the editor, `lyx config profiles`.
// No key is open to the local layer.
//
// `Load` decodes the stacked file without filling it from the template, so a removed starting profile stays removed, and reads the template when no file is present.
// It checks the shape, and that each default names a profile holding its recipe's block;
// `configreg` checks each recipe block's keys against the modules' config as it checks a task's `run_settings`, on every write of the file and at the board write and Seed-Child that read it.
package runprofile
