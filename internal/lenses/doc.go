// Package lenses is the `lenses` config module: the lens and fan library, its file lenses.yaml, and the fan key grammar loom and darn share.
//
// A lens maps a name to review prose, and a fan maps a name to an ordered list of lens names.
// Burler's reviewers and fixers, loom's twill writers and fan keys and darn's fan keys all read the one file.
//
// # Reconcile
//
// Both keys are per-name maps.
// Reconcile adds each template lens or fan the file lacks and keeps every lens and fan the file holds, prose and fan lists unchanged,
// so a later template edit reaches the file only once its entry is deleted from the file or edited in.
// A lens or fan the template lacks is kept and reported, never pruned.
// A template lens or fan cannot be removed: a deleted one resolves to the template's value at the next load and is written back at the next reconcile.
// A fan is turned off at the loom or darn fan key entry that names it, by pointing the entry at another fan or setting it to "".
//
// # Guard keys
//
// `lenses` and `fans` are guard keys, since a lens's prose and a fan's lens set are what every review fork is given.
// No local override layer and no run setting holds them.
// The module is per-worktree, not hub-wide.
//
// # Load
//
// Load reads the layer stack through `configengine.LoadOrTemplate` and decodes the resolved document strictly, each error prefixed `lenses: `.
// An absent base file resolves the template, and a lens or fan the stack lacks resolves to its template value through the load's fill.
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
