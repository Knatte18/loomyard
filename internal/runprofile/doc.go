// Package runprofile holds the hub's named run profiles.
// They live in the `profiles` config module, whose one file is `profiles.yaml` on Bolt.
//
// A profile maps a recipe name, such as `tapestry` or `darn`, to that recipe's settings.
// The settings take the shape of a board task's `run_settings`: module name to that module's config keys.
// One profile name serves every recipe, and each recipe's block may set the same key to its own value:
// darn's and pawl's Code-Review perch read loom's `code` area,
// so each recipe's block sets `loom.code.max_bounces` for its own runs.
// The reserved top-level `defaults` entry maps a recipe name to the profile a task of that recipe takes.
// A task takes it when it names no profile.
// A board task names a profile in its `profile` field.
// Batten's Seed-Child writes the block of the recipe the task runs into the pair's local config layers.
// The block comes from the task's profile or else the recipe's default;
// the task's own `run_settings` are layered on top.
// A later change of the profile never reaches a run already seeded.
// A task naming no profile, whose recipe has no default, runs on the hub's own config.
//
// The template's starting profiles are a ladder of review rounds per perch, `speed`, `light`, `solid` and `wide`.
// Each perch's hard cap is its rounds plus one.
// `docs` is a darn-only profile for prose and config value edits.
// A round count of 0 turns a perch off, since its Bouncer then settles approved with no review round.
// A darn run has no Discussion or Plan review,
// so its Code-Review reviewer also judges whether the run did what the entry asks.
// The tapestry recipe's plan perch keys are set under the `tapestry` module, not under `loom`.
//
// The module is hub-wide and seed-only.
// The hub reconcile seeds an absent file with the template's entries.
// Beside a present file it keeps `profiles.template-base` on Bolt, the template the file last carried in.
// On each reconcile a key the file holds as that base does takes the template's new value, gained, changed or dropped.
// A key the operator set, added or removed is kept, and reported under `differs` when the template holds it otherwise.
// A file with no base yet gains each template key it lacks and keeps every value it holds.
// It reports under `differs` a value the template holds otherwise.
// Seed-Child refuses a profile or `defaults` that holds a recipe only under its former name,
// and the refusal names the hand rename with `lym config profiles` as the way forward.
// Each profile, like `defaults`, is one whole entry,
// and `lym config profiles --set <name>='<json object>'` adds or replaces one whole entry.
// A dotted key sets one value inside a profile and keeps the rest, as in `--set docs.darn.loom.code.max_bounces=0`.
// No key is open to the local layer.
//
// `Load` decodes the stacked file without filling it from the template,
// so a removed starting profile stays removed.
// It reads the template when no file is present.
// It checks the shape, and that each default names a profile holding its recipe's block;
// `configreg` checks each recipe block's keys against the modules' config, as it does a task's `run_settings`.
// These checks run on every write of the file, and at the board write and Seed-Child that read it.
package runprofile
