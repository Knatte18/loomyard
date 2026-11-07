// doc.go holds the package doc of configreg.

// Package configreg is the registry of lym's config modules and their templates.
// Callers branch on a module's registry entry rather than on its name.
//
// # Hub-wide modules
//
// A config module that describes a hub-level fact is marked HubWide here.
// Its file is read and written only at `<BoardDir>/_lym/config/`, never in a worktree's `_lym/config/`.
// Its never-tracked machine override is `<BoardDir>/.lym/config/<module>.machine.yaml`.
// The override holds only the module's machine keys and never sits in a worktree.
// `<BoardDir>/_lym/config/` also holds each per-worktree module's repo default, the base layer of that module's stack.
// Module.HubWide tells the two kinds of file apart.
// A hub-wide module also reads the reading worktree's local override, restricted to the module's LocalKeys.
//
// # Key moves and copies
//
// KeyMoves declares keys that moved from one module's file to another's.
// A reconcile relocates the value and drops the source key.
// KeyCopies declares keys copied from one module's hub file into another's, under the same or another name.
// The source keeps its key.
// A copy places the value only when the source file sets the key and the target file does not.
// So a second reconcile changes nothing.
// A copy reads no worktree layer, local override or run setting.
//
// # Run settings
//
// PrepareRunSettings migrates and validates a task's run settings against the module's local layer.
// A key a module's Migrate renames is rewritten there with a Warn.
// A key the module's Spec lists in `RetiredWayForward` or `HubOnlyWayForward` is refused.
// The refusal carries that text as its way forward.
// The hub-only keys are set only in the hub's file, such as drift's `diff` and `key_patterns`.
// They still load from the hub layers.
//
// # Reinterpretations
//
// A binary that changes what a value in a module's hub base file means declares a Reinterpretation on that module.
// It is an ID and a rewrite that keeps what the hub meant, or flags a value it cannot settle with a way forward.
// The hub-wide reconcile applies each once per hub behind a board marker carrying its ID.
// It tells the hub's orch what it rewrote or flagged.
package configreg
