// Package hibernatecli builds the `lym hibernate` group over internal/hibernateengine.
// The verbs are `lym hibernate request`, `lym hibernate cancel` and `lym hibernate status`.
// `request` takes `[--after <slug>] [--after-all] [--resume-on-wake]`.
// The package also wires the Carrier the prime's dobby carries the request with.
// The engine documents its sequence, the request file and the callers it refuses;
// this package wires the engine to the hub.
//
// Every verb runs from any worktree of the hub and resolves it with lymcwd.Resolve.
// The module is hub-only and not standalone-capable, so internal/cliwire does not serve it.
// `request`, `cancel` and `status` carry the operator audience, so the operator command index lists them.
// The envelope carries the engine's result under `request`, `cancel` or `status`.
// A refusal goes through output.Err with the engine's message, which names its way forward.
// `request`'s result is the written request with whether the prime's dobby runs, `dobby_live`.
// When it does not run, the result also gives the way forward that starts it.
//
// # Wiring
//
// The request file lives in the hub's board-level `.lym` directory under `lymdirs.HibernateDirName`.
// It sits beside the pause record.
// So does the file that holds a notice too long for one creel line.
// The prime is resolved the way a creel notice to the orch resolves it.
// The batten status and land lock readers read the prime's run directories through shedrun's own path declarations.
// The prime dobby's liveness is `dobbyengine.Live` over `hubgeom.DobbyGeometry` of the prime.
// The pause, tender status and lift seams each build tendercli.HubDeps at the call,
// so every reading sees the hub's current pairs.
// The request's pause entry carries the caller label the engine names.
// `status` builds no tender wiring, since those seams run only when called.
// The request's notice is creel mail to the orch signed as `HIBERNATE`, never keys typed into the orch's pane.
// `request` hands the engine whether the wake lifts the pause.
// The value of `--resume-on-wake` answers it when the flag is given, true or false.
// Otherwise the answer is read from the hub's hibernate.yaml through internal/hibernateconfig.
// It is off when the hub holds no such file.
//
// # The carrier
//
// `NewDobbyCarrier` builds the engine's Carrier over the same Deps for the prime.
// The running process's pid and start time are the carrier's identity.
// `RequestPath` names the request file the prime's dobby watches.
// `NewDobbyCarrier` also creates the hibernate directory.
// The dobby's file watch opens only a directory that exists.
// internal/beamcli runs the Carrier in the prime's dobby; the sequence's log lines go to that dobby's own output.
//
// # Spawns
//
// The machine's hibernate is `systemctl hibernate`, which returns once the job is queued;
// a non-zero exit returns the exit and the last line of its output.
//
// # Bounds
//
// The land lock probe takes the lock once and releases it at once,
// so a landing that tries it in that instant polls again a second later, as a contended lock already makes it.
package hibernatecli
