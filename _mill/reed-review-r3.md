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

Hermetic gates (all green):
- `go build ./...` — pass.
- `go vet ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/...` — pass.
- `go test -count=5 ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/... ./cmd/lyx/...` — pass (reedengine 0.79s, cmd/lyx 0.88s).

Direct tmux 3.6 grammar probes (isolated socket `r3probe1`, torn down after each):
- `new-session -d -s "a<TAB>b"` → exit 0, session created as the FOUR literal characters `a\tb` (vis-encoded), `has-session -t '=a<TAB>b'` MISSES. Same for newline (`\n` → literal `\n`), ESC (→ `\033`), DEL (→ `\177`), BEL (→ `\a`), and the invalid-UTF-8 byte 0xFF (→ literal `\377`). Exit 0 every time, exact-match target misses every time. **This is a live-confirmed R2-F1-shaped gap — see finding R3-F1.**
- Names that pass verbatim and exact-hit: a space-only name `" "`, unicode `svc-åäö-⚙`, `=`-leading, `-`-leading, `a#b%c`. So refusing only the vis-encode class is correct — no over-refusal needed.
- Socket-key side: `tmux -L "r3<TAB>sock" new-session` works (a socket key is a filename; control characters are legal there). The vis-encode gap is session-name-only.

Live CLI driving (dev binary `./deploy-dev` @ 84f3558b; real hub built with `lyx fabric clone` into the session scratchpad — `warp-HUB` with warp prime + two unicode worktrees):
- **S1 fresh-hub first boot**: `lyx reed up` in the warp prime of a hub whose `_board/.lyx/logs` did NOT yet exist → ok in 0.17s, logs dir created 0755, session `warp` up with header pane (%1 running the real `lyx reed header --blocking` keepalive) + initial bash pane. `fabricengine.HubLogsDir` ownership move holds on the very first boot.
- **S2 lifecycle + child reap**: added parent strand `bash -c "sleep 600 & sleep 600 & wait"` + child strand; `remove --recursive` removed both strands and the WHOLE process subtree (both `sleep 600` grandchildren confirmed gone immediately after return); session + header survived; status empty.
- **S3 crash/rebirth**: added strand with distinct `--resume-cmd 'sleep 401'`; `tmux kill-server` out from under; `lyx reed resume` → rebooted server, resumed:1, replayed the RESUME cmd (`sleep 401` live, not `sleep 400`), header pane relaunched with rendered hub token.
- **S4 unicode worktrees**: `lyx fabric add svc-åäö-⚙` and `svc-åäö-⚙-x` (a unicode PREFIX-colliding pair — a fixture neither round 1 nor round 2 used); `reed up` + `add` in both → sessions created verbatim, exact targets hit.
- **S5 unicode prefix-pair scope**: `reed down` in `svc-åäö-⚙` (the prefix) left `svc-åäö-⚙-x`'s session, strand, and `sleep 301` process untouched; a SECOND down (idempotence path, where a bare `-t` would prefix-match now that our session is gone) also left the sibling untouched; the downed worktree's `sleep 300` was reaped.
