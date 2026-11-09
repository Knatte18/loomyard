// Package runprofile holds the hub's named run profiles.
// They live in the `profiles` config module, whose one file is `profiles.yaml` on Bolt.
//
// A profile maps a recipe name, such as `tapestry` or `darn`, to that recipe's settings.
// The settings take the shape of a board task's `run_settings`: module name to that module's config keys.
// One profile name serves every recipe, and each recipe's block may set the same key to its own value:
// darn's and pawl's Code-Review perch read loom's `code_*` keys,
// so each recipe's block sets `loom.code_max_bounces` for its own runs.
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
// The template's defaults are `solid` for tapestry, motif and arras and `light` for darn, pawl and raddle.
// Sampler has no default.
// Each ladder profile's motif block carries its tapestry block's Discussion perch values, a motif run's only perch.
// Each ladder profile's arras block carries the same Discussion perch values, and the same rounds for the split perch.
// Each ladder profile's pawl block carries the same perch values as its darn block.
// Its sampler block carries them too, and also turns the review on with `sampler.code_review` from `light` up.
// A `speed` sampler run and a sampler run naming no profile skip the Code-Review perch.
// `docs` and `speed`'s tapestry block run the Code-Review round solo through `loom.code_reviewer_fan: ""`.
// `light`, `solid`, `wide` and the rest take the hub's fan.
// A round count of 0 turns a perch off, since its Bouncer then settles approved with no review round.
//
// The module is hub-wide and seed-only.
// The hub reconcile seeds an absent file with the template's entries.
// Beside a present file it keeps `profiles.template-base` on Bolt, the template the file last carried in.
// On each reconcile a key the file holds as that base does takes the template's new value, gained, changed or dropped.
// A key the operator set, added or removed is kept, and reported under `differs` when the template holds it otherwise.
// A file with no base yet gains each template key it lacks and keeps every value it holds.
// It reports under `differs` a value the template holds otherwise.
// It also renames a recipe block or `defaults` entry held under a recipe's former name, keeping its value.
// An example is `loom` for `tapestry`.
// A former block beside a current one is dropped only when it equals the current block.
// A differing one stays, and reconcile reports it under `unsettled` as `<entry>.<from>: differs from <entry>.<to>`.
// The operator settles it by hand.
// Seed-Child refuses a profile or `defaults` that holds a recipe only under its former name.
// The refusal names that reconcile as the way forward.
// Each profile, like `defaults`, is one whole entry,
// and `lym config profiles --set <name>='<json object>'` adds or replaces one whole entry.
// A dotted key sets one value inside a profile and keeps the rest, as in `--set docs.darn.loom.code_max_bounces=0`.
// Renaming a profile is adding it under the new name and removing the old block in the editor, `lym config profiles`.
// No key is open to the local layer.
//
// `Load` decodes the stacked file without filling it from the template,
// so a removed starting profile stays removed.
// It reads the template when no file is present.
// It checks the shape, and that each default names a profile holding its recipe's block;
// `configreg` checks each recipe block's keys against the modules' config, as it does a task's `run_settings`.
// These checks run on every write of the file, and at the board write and Seed-Child that read it.
package runprofile
