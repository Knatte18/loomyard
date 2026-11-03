// Package hibernatecli builds the `lym hibernate` group with `lym hibernate request [--after <slug>] [--after-all] [--resume-on-wake]`, `lym hibernate cancel` and `lym hibernate status` over internal/hibernateengine, and wires the Carrier the prime's dobby carries the request with.
// The engine's sequence, the request file and the callers it refuses are documented there; this package wires it to the hub.
//
// Every verb runs from any worktree of the hub and resolves it with lymcwd.Resolve.
// The module is hub-only and not standalone-capable, so internal/cliwire does not serve it.
// `request`, `cancel` and `status` carry the operator audience, so the operator command index lists them.
// The envelope carries the engine's result under `request`, `cancel` or `status`, and a refusal through output.Err with the engine's message, which names its way forward.
// `request`'s result is the written request with whether the prime's dobby runs, `dobby_live`, and when it does not, the way forward that starts it.
//
// # Wiring
//
// The request file and the file a notice too long for one creel line is written to live in the hub's board-level `.lym` directory under `lymdirs.HibernateDirName`, beside the pause record.
// The prime is resolved the way a creel notice to the orch resolves it, and the batten status and land lock readers read the prime's run directories through shedrun's own path declarations.
// The prime dobby's liveness is `dobbyengine.Live` over `hubgeom.DobbyGeometry` of the prime.
// The pause, tender status and lift seams each build tendercli.HubDeps at the call, so every reading sees the hub's current pairs, and the request's pause entry carries the caller label the engine names.
// `status` builds no tender wiring, since those seams run only when called.
// The request's notice is creel mail to the orch signed as `HIBERNATE`, never keys typed into the orch's pane.
// `request` hands the engine whether the wake lifts the pause: `--resume-on-wake` when the flag is given, either way, and otherwise the hub's hibernate.yaml read through internal/hibernateconfig, off when the hub holds no such file.
//
// # The carrier
//
// `NewDobbyCarrier` builds the engine's Carrier over the same Deps for the prime, the running process's pid and start time being the carrier's identity, and `RequestPath` names the request file the prime's dobby watches.
// `NewDobbyCarrier` also creates the hibernate directory, since the dobby's file watch opens only a directory that exists.
// internal/beamcli runs the Carrier in the prime's dobby; the sequence's log lines go to that dobby's own output.
//
// # Spawns
//
// The machine's hibernate is `systemctl hibernate`, which returns once the job is queued; a non-zero exit returns the exit and the last line of its output.
//
// # Bounds
//
// The land lock probe takes the lock once and releases it at once, so a landing that tries it in that instant polls again a second later, as a contended lock already makes it.
package hibernatecli
