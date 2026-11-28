// doc.go carries the package godoc for creelconfig.

// Package creelconfig owns the hub-wide creel.yaml: which other hubs' orch this hub obeys.
//
// The module's one key, trusted_hubs, lists the trusted hubs by repository name, owner/repo.
// Each name is read from the hub's origin remote.
// The key is classified guard, since it grants authority over this hub's agents.
// A worktree's local override therefore cannot set it.
//
// The key once listed hub shortnames.
// RewriteShortnamesToRepositories carries a hub's file across that change once, as a config reinterpretation.
// It rewrites each shortname to the repository name it resolves to on the machine that runs it.
// It drops a shortname the resolver reports as naming no repository, and returns any other resolver error unhandled.
// The caller tells it the resolver, so the module imports neither the machine record nor git.
//
// LoadConfig degrades to the template when the hub holds no creel.yaml, so a hub with no file trusts no hub.
// The module sits apart from the creel engine because configreg registers its template,
// and the engine would otherwise import configreg's dependencies.
// LoadConfig never resolves cwd itself: the caller hands it the directories.
package creelconfig
