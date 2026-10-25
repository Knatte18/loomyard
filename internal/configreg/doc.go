// doc.go carries the package godoc for configreg: the registry's role and the hub-wide module rule.

// Package configreg is the registry of lyx's config modules and their templates, which the config CLI, fabric clone and the reconcile verbs branch on rather than on a module name.
//
// # Hub-wide modules
//
// A config module that describes a hub-level fact is marked HubWide here.
// Its file is read and written only at `<BoardDir>/_lyx/config/`, never in a worktree's `_lyx/config/`.
// Its never-tracked machine override is `<BoardDir>/.lyx/config/<module>.machine.yaml`, holding only the module's machine keys, and never sits in a worktree.
// `<BoardDir>/_lyx/config/` also holds each per-worktree module's repo default, the base layer of that module's stack; Module.HubWide tells the two kinds of file apart.
// A hub-wide module also reads the reading worktree's local override, restricted to the module's LocalKeys.
//
// # Key moves and copies
//
// KeyMoves declares keys that moved from one module's file to another's; a reconcile relocates the value and drops the source key.
// KeyCopies declares keys copied instead, under the same or another name, from a source module's hub file into a target module's hub file; the source keeps its key.
// A copy places the value only when the source file sets the key and the target file does not, so a second reconcile changes nothing, and it reads no worktree layer, local override or run setting.
package configreg
