// Package lenses is the `lenses` config module:
// the lens and fan library, its file lenses.yaml, and the fan key grammar loom and darn share.
//
// A lens maps a name to review prose, and a fan maps a name to an ordered list of lens names.
//
// # Reconcile
//
// lenses.yaml holds only the lenses and fans the repo tuned or added,
// so a later template edit to any other name reaches the repo.
// Reconcile never copies a template entry into the file and drops, reporting each, an entry equal to the template's.
// A lens or fan the template lacks is kept and reported, never pruned.
//
// # Language layers
//
// The embedded files under `sets/` are language layers, picked by the base file's `lenses.lens_set` key; `""` picks none.
// A layer entry replaces the base entry of the same name, and a name the repo's own file defines still beats the layer.
// A layer fan holds the base fan's entries first, so a layer only appends.
// Only the base file may hold `lenses.lens_set`; set it with `lym config lenses --layer default --set lens_set=<name>`.
//
// # Remove
//
// The key `lenses.remove` lists lens names.
// Load deletes each from the lenses and every fan; a fan it empties is recorded in `Config.EmptiedFans`.
// A name matching no lens is skipped with one Warn, so a template edit or layer switch is harmless.
// A worktree file's `lenses.remove` list replaces the hub's whole.
//
// # Guard keys
//
// `lenses.lenses`, `lenses.fans`, `lenses.lens_set` and `lenses.remove` are guard keys.
// They decide what every review fork is given.
// No local override layer and no run setting holds them.
// The module is per-worktree, not hub-wide.
//
// # Load
//
// Load reads the layer stack through `configengine.LoadOrTemplate` and decodes the resolved document strictly.
// A lens or fan the stack lacks resolves to its effective template value.
// A lens prose holding `${env:NAME}` expands at load, and the load fails when `NAME` is unset and has no default.
//
// # Fan resolution
//
// ResolveFan returns a fan's lenses in order, repeats kept.
// It refuses a fan the removal emptied and an unknown or empty fan.
// It also refuses one longer than MaxFanLenses and one naming an undefined lens.
// CheckLoad joins every failure in fan-name order, so one `--set` or `--edit` can take every fix.
//
// # Fan key grammar
//
// A fan key holds a FanList: one fan name for every round, or one per round with the last serving every later round.
// An empty entry runs its round solo, or with the single fixer.
//
// The package imports only the standard library, `internal/configengine` and `gopkg.in/yaml.v3`.
package lenses
