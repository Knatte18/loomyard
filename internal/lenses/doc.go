// Package lenses is the `lenses` config module: the lens and fan library, its file lenses.yaml, and the fan key grammar loom and darn share.
//
// A lens maps a name to review prose, and a fan maps a name to an ordered list of lens names.
// Burler's reviewers and fixers, loom's twill writers and fan keys and darn's fan keys all read the one file.
//
// # Reconcile
//
// Both keys are per-name maps, so lenses.yaml holds only the lenses and fans the repo tuned or added, and a later template edit to any other name reaches the repo.
// Reconcile never copies a template lens or fan into the file,
// and it drops, reporting each, an entry whose prose or ordered fan list equals the effective template's current value, which leaves the loaded value unchanged.
// Every other entry is kept as written, and a lens or fan the template lacks is kept and reported, never pruned.
// A base-layer `--set` leaves template-equal entries out the same way.
// A template lens or fan cannot be removed: a deleted one resolves to the template's value at the next load.
// A fan is turned off at the loom or darn fan key entry that names it, by pointing the entry at another fan or setting it to "".
//
// A file is judged against, and a missing name filled from, the effective template: the base with the picked language layer laid over it.
//
// # Language layers
//
// The embedded files under `sets/` are language layers, each a document holding a `lenses` and a `fans` mapping, named by its file's base name.
// The key `lens_set` in the base file picks one by that name, and `""` picks none.
// A layer lens or fan replaces the base entry of the same name whole or adds one the base lacks, and a name the repo's own file defines still beats the layer.
// A layer fan holds the base fan's entries first, in base order, so a layer only appends.
// Only the base file may hold `lens_set`: the hub file in a hub, set with `lyx config lenses --layer default --set lens_set=<name>` from the hub's prime worktree.
// A worktree file holding another value than the base's, and a name no layer carries, are refused naming that command.
// lyx picks no layer itself.
//
// # Guard keys
//
// `lenses`, `fans` and `lens_set` are guard keys, since a lens's prose, a fan's lens set and the layer that supplies them are what every review fork is given.
// No local override layer and no run setting holds them, and configengine reads `lens_set` from the base layer only.
// The module is per-worktree, not hub-wide.
//
// # Load
//
// Load reads the layer stack through `configengine.LoadOrTemplate` and decodes the resolved document strictly, each error prefixed `lenses: `.
// An absent base file resolves the template, and a lens or fan the stack lacks resolves to its effective template value through the load's fill.
// A lens prose holding `${env:NAME}` expands at load, and the load fails when `NAME` is unset and has no default.
//
// # Fan resolution
//
// ResolveFan reads a loaded Config alone and returns a fan's lenses in order, repeats kept.
// It refuses an unknown fan, an empty fan, a fan longer than MaxFanLenses and a fan naming an undefined lens.
// CheckLoad runs Load and then ResolveFan over every fan in name order, and joins every failure in that order.
// A lenses.yaml with several failing fans therefore takes every fix in one `--set` or one `--edit`, since a partial fix changes the joined failure and is rolled back.
//
// # Fan key grammar
//
// A fan key holds a FanList: one fan name for every round, or a list of fan names, one per round, whose last entry serves every later round.
// An empty entry runs its round solo, or with the single fixer.
// ValidateFanKeys refuses an empty list and resolves each non-empty entry through Load and ResolveFan.
//
// # Imports
//
// The package imports only the standard library, `internal/configengine` and `gopkg.in/yaml.v3`.
package lenses
