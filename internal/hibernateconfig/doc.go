// doc.go carries the package godoc for hibernateconfig.

// Package hibernateconfig owns the hub-wide hibernate.yaml:
// whether the wake from a `lym hibernate request` lifts the hub pause that request added.
//
// The module's one key, resume_on_wake, is off in the template.
// The operator often works on another machine while this one sleeps, and does not want runs to go on after the wake.
// `lym hibernate request --resume-on-wake` and `--resume-on-wake=false` override it for one request.
// The key is classified cross-pair: lifting a hub pause lets every pair's run go on.
// A worktree's local override therefore cannot set it.
//
// LoadConfig degrades to the template when the hub holds no hibernate.yaml.
// A request therefore never fails on a hub not yet reconciled.
// The module sits apart from the hibernate engine because configreg registers its template.
// The engine already reaches configreg through its own imports.
// LoadConfig never resolves cwd itself: the caller hands it the directories.
package hibernateconfig
