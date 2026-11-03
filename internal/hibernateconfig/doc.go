// doc.go carries the package godoc for hibernateconfig.

// Package hibernateconfig owns the hub-wide hibernate.yaml: whether the wake from a `lym hibernate request` lifts the hub pause that request added.
//
// The module's one key, resume_on_wake, is off in the template, since the operator often works on another machine while this one sleeps and does not want runs to go on by themselves after the wake.
// `lym hibernate request --resume-on-wake` and `--resume-on-wake=false` override it for one request.
// The key is classified cross-pair, since lifting a hub pause lets every pair's run go on, so a worktree's local override cannot set it.
//
// LoadConfig degrades to the template when the hub holds no hibernate.yaml, so a request never fails on a hub not yet reconciled.
// The module sits apart from the hibernate engine because configreg registers its template, and the engine already reaches configreg through its own imports.
// LoadConfig never resolves cwd itself: the caller hands it the directories (see PATTERN-cwd-resolution).
package hibernateconfig
