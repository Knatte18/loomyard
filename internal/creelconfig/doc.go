// doc.go carries the package godoc for creelconfig.

// Package creelconfig owns the hub-wide creel.yaml: which other hubs' orch this hub obeys.
//
// The module's one key, trusted_hubs, lists hub shortnames.
// The key is classified guard, since it grants authority over this hub's agents, so a worktree's local override cannot set it.
//
// LoadConfig degrades to the template when the hub holds no creel.yaml, so a hub with no file trusts no hub.
// The module sits apart from the creel engine because configreg registers its template, and the engine would otherwise import configreg's dependencies.
// LoadConfig never resolves cwd itself: the caller hands it the directories (see PATTERN-cwd-resolution).
package creelconfig
