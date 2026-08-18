# reed — independent review, round 3 (`fable-high-r3`)

Reviewer: crucible round agent, clean-room (no prior-round `_mill/reed-review-*` material read before this findings list was complete).
Scope: `internal/reedengine`, `internal/reedcli`, `internal/hubgeom`, `cmd/lyx` integration, docs — per `_mill/reed-review-prompt.md` (round 3 instance).

## What was read

- `CONSTRAINTS.md` in full (Cwd Resolution Invariant, CLI/Cobra Invariant, Live-Substrate Spawn Observability, Sandbox Suite Coverage, Test Tier Purity).
- Every production file in `internal/reedengine` (geometry, server, lock, lifecycle, strand, spawn, apply, reconcile, state, overlay, parse, probe, version, header, headerpane, headertemplate, env, io, mouse, serverlog, proctree{,_linux,_windows}, config, template{,_posix}, name, doc.go).
- Every production file in `internal/reedcli` (cli, up, add, remove, status, resume, attach, header).
- `internal/hubgeom` (doc.go, hubgeom.go).
- Wave-1 commit `b98ee2ba` (stat + the `fabricengine.HubLogsDir` addition + reed-touching hunks).
- `docs/overview.md` reed bullet (~line 280) and execution-stack section; `tools/sandbox/SANDBOX-REED-SUITE.md` (scenario ideas only).

## Static-read observations (pre-live)

- **Told-direction one-way confirmed by import graph**: `go list -deps ./internal/reedengine` contains neither `internal/hubgeom` nor `internal/fabricengine`; `internal/hubgeom` imports `{fabricengine, lyxcwd, reedengine}`. The doc.go honesty note (lyxcwd still transitively present via logger) matches reality.
- **TmuxCmd discipline sweep**: raw `exec.Command` sites in reed production code are (a) `overlay.go` run/output — the chokepoint itself; (b) `lifecycle.go` spawnSession — carries `-L e.Socket()` explicitly in argv (needs Dir/Env/Detach, cannot route through TmuxCmd); (c) `reedcli/attach.go` — carries `-L socket` explicitly via `attachArgv` (terminal handover, cannot route through TmuxCmd); (d) `proctree_windows.go` — pwsh process-table probes, not tmux invocations. No socket-scoping bypass found.
- **Reap-root snapshot sweep**: exactly two descendant-closure snapshot sites exist — `alivePanePIDs` (RemoveStrand) and `sessionReapRoots`/`sessionReapRootsLocked` (Down) — both routed through the single `safeReapRoot` predicate. `serverPIDLocked`'s `#{pid}` snapshot is taken while the server demonstrably serves the query, so it is un-recycled at snapshot time, same discipline. No third pre-predicate site.
- `planReconcile`'s untracked-alive-pane kill has no child reap, but a straggler there is caught by Down's whole-session reap (sessionReapRoots covers every alive pane, tracked or not); noted, judged not a defect on the merge bar.

## What was tested

(appended live as commands return)
