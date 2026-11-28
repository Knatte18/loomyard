// Package lenses is the `lenses` config module:
// the lens and fan library, its file lenses.yaml, and the fan key grammar loom and darn share.
//
// A lens maps a name to review prose, and a fan maps a name to an ordered list of lens names.
// Burler's reviewers and fixers, loom's twill writers and fan keys and darn's fan keys all read the one file.
//
// # Reconcile
//
// Both keys are per-name maps, so lenses.yaml holds only the lenses and fans the repo tuned or added,
// and a later template edit to any other name reaches the repo.
// Reconcile never copies a template lens or fan into the file.
// It drops, reporting each, an entry whose prose or ordered fan list equals the effective template's current value.
// Dropping it leaves the loaded value unchanged.
// Every other entry is kept as written, and a lens or fan the template lacks is kept and reported, never pruned.
// A base-layer `--set` leaves template-equal entries out the same way.
// A fan is turned off at the loom or darn fan key entry that names it:
// point the entry at another fan or set it to "".
//
// A file is judged against, and a missing name filled from, the effective template:
// the base with the picked language layer laid over it.
//
// # Language layers
//
// The embedded files under `sets/` are language layers, each named by its file's base name.
// Each holds a `lenses` and a `fans` mapping.
// The key `lens_set` in the base file picks one by that name, and `""` picks none.
// A layer lens or fan replaces the base entry of the same name whole or adds one the base lacks,
// and a name the repo's own file defines still beats the layer.
// A layer fan holds the base fan's entries first, in base order, so a layer only appends.
// Only the base file may hold `lens_set`:
// the hub file in a hub.
// Set it with `lym config lenses --layer default --set lens_set=<name>` from the hub's prime worktree.
// A worktree file holding another value than the base's, and a name no layer carries, are refused naming that command.
// lym picks no layer itself.
//
// # Remove
//
// The key `remove` lists lens names.
// After the strict decode, Load deletes each listed name from the lenses and every occurrence of it from every fan.
// The hub's own fans are included.
// A fan that had entries and is left empty leaves the fans.
// It is recorded in `Config.EmptiedFans` with the removed names it held.
// A name matching no lens is skipped, so a lens a later template edit or a layer switch takes away is harmless.
// The skip logs one Warn naming the highest layer file holding `remove`, once per file and list in a process.
// A worktree file's `remove` list replaces the hub's whole, as any list does when layers stack.
//
// # Guard keys
//
// `lenses`, `fans`, `lens_set` and `remove` are guard keys, since they decide what every review fork is given:
// a lens's prose, a fan's lens set, the layer that supplies them and the lenses taken out of every fan.
// No local override layer and no run setting holds them, and configengine reads `lens_set` from the base layer only.
// Only a tracked base or worktree file can hold `remove`, and a shortened fan still runs its remaining lenses.
// The module is per-worktree, not hub-wide.
//
// # Load
//
// Load reads the layer stack through `configengine.LoadOrTemplate` and decodes the resolved document strictly.
// Each error is prefixed `lenses: `.
// An absent base file resolves the template,
// and a lens or fan the stack lacks resolves to its effective template value through the load's fill.
// A lens prose holding `${env:NAME}` expands at load, and the load fails when `NAME` is unset and has no default.
//
// # Fan resolution
//
// ResolveFan reads a loaded Config alone and returns a fan's lenses in order, repeats kept.
// It refuses a fan the removal emptied, naming the removed lenses and the way forward.
// It also refuses an unknown fan, an empty fan, a fan longer than MaxFanLenses and a fan naming an undefined lens.
// CheckLoad resolves only the fans a load still holds, so a `remove` that empties a fan passes it,
// and the fan is refused when a fan key names it.
// CheckLoad runs Load and then ResolveFan over every fan in name order, and joins every failure in that order.
// A lenses.yaml with several failing fans therefore takes every fix in one `--set` or one `--edit`.
// A partial fix changes the joined failure and is rolled back.
//
// # Fan key grammar
//
// A fan key holds a FanList:
// one fan name for every round, or a list of fan names, one per round, whose last entry serves every later round.
// An empty entry runs its round solo, or with the single fixer.
// ValidateFanKeys refuses an empty list and resolves each non-empty entry through Load and ResolveFan.
//
// # Imports
//
// The package imports only the standard library, `internal/configengine` and `gopkg.in/yaml.v3`.
package lenses
