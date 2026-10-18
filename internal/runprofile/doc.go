// Package runprofile holds the hub's named run profiles, the `profiles` config module whose one file is `profiles.yaml` on Bolt.
//
// A profile maps a name to settings in the shape of a board task's `run_settings`, module name to that module's config keys, so one profile can cover several recipes.
// A board task names a profile in its `profile` field;
// batten's Seed-Child writes the profile's settings into the pair's local config layers with the task's own `run_settings` layered on top, so a later change of the profile never reaches a run already seeded.
// A task without a profile runs on the hub's own config.
//
// The module is hub-wide and seed-only: the hub reconcile seeds an absent file with the template's starting profiles, `light`, `budget` and `docs`, and never rewrites a present one.
// Each profile is one whole entry, and `lyx config profiles --set <name>='<json object>'` adds or replaces one whole profile;
// renaming one is adding it under the new name and removing the old block in the editor, `lyx config profiles`.
// No key is open to the local layer.
//
// `Load` decodes the stacked file without filling it from the template, so a removed starting profile stays removed, and reads the template when no file is present.
// It checks only the shape;
// `configreg` checks each profile's keys against the modules' config as it checks a task's `run_settings`, on every write of the file and at the board write and Seed-Child that read it.
package runprofile
