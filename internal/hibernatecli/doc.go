// Package hibernatecli builds the `lyx hibernate` group with `lyx hibernate request [--after <slug>] [--after-all]`, `lyx hibernate cancel`, `lyx hibernate status` and the hidden `lyx hibernate watch` over internal/hibernateengine.
// The engine's sequence, the request file and the callers it refuses are documented there; this package wires it to the hub.
//
// Every verb runs from any worktree of the hub and resolves it with lyxcwd.Resolve.
// The module is hub-only and not standalone-capable, so internal/cliwire does not serve it.
// `request`, `cancel` and `status` carry the operator audience, so the operator command index lists them; `watch` carries the internal audience.
// The envelope carries the engine's result under `request`, `cancel`, `status` or `watch`, and a refusal through output.Err with the engine's message, which names its way forward.
//
// # Wiring
//
// The request file and the daemon's log live in the hub's board-level `.lyx` directory under `lyxdirs.HibernateDirName`, beside the pause record.
// The prime is resolved the way a creel notice to the orch resolves it, and the batten status and land lock readers read the prime's run directories through shedrun's own path declarations.
// The pause, tender status and lift seams each build tendercli.HubDeps at the call, so every reading sees the hub's current pairs, and the daemon's pause entry carries the caller label the engine names.
// `status` builds no tender wiring, since those seams run only when called.
// The daemon's notice is creel mail to the orch signed as `HIBERNATE`, never keys typed into the orch's pane.
//
// # Spawns
//
// `request` spawns `<this binary> hibernate watch` detached from the prime's anchor, with its output appended to the watch log, logs the spawn with its pid and log, and reaps the child while the spawner lives.
// Under go test the spawn returns an error and starts nothing, so no test re-executes the test binary or records a pid nobody runs.
// The machine's hibernate is `systemctl hibernate`, which returns once the job is queued; a non-zero exit returns the exit and the last line of its output.
//
// # Bounds
//
// The land lock probe takes the lock once and releases it at once, so a landing that tries it in that instant polls again a second later, as a contended lock already makes it.
package hibernatecli
