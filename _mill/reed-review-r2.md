# reed — independent review, round 2 (`opus-high-r2`)

> Clean-room safety pass over `internal/reedengine` + `internal/reedcli` + `internal/hubgeom`, per `_mill/reed-review-prompt.md`.
> Written before any production or test file was touched (the prompt's BLOCKING sequencing rule).

## Scope of this round

- `internal/reedengine/**` (told-geometry seam `geometry.go`, plus `lifecycle.go`, `lock.go`, `header.go`, `strand.go`, `spawn.go`, `apply.go`, `reconcile.go`, `overlay.go`, `proctree*.go`, `server.go`, `probe.go`, `serverlog.go`, `state.go`, `io.go`, `config.go`, `name.go`, `headerpane.go`)
- `internal/reedcli/**` (all eight verbs + the `PersistentPreRunE` construction seam)
- `internal/hubgeom/**` (the one-way told-geometry adapter)
- `cmd/lyx` integration, plus the four other `hubgeom.ReedGeometry` call sites (`burlercli`, `shuttlecli`, `perchcli`, `webstercli`) as construction-seam consumers only
- Docs: `docs/overview.md` reed bullet, `CONSTRAINTS.md`, `internal/reedengine/doc.go`, `manifest/designs/producers-standalone.md`

Out of scope this round (per prompt): `internal/shuttleengine`/`internal/shuttlecli` behaviour, hubgeom's unbuilt wave-3 siblings, Windows-specific tmux/path behaviour.

## Environment

- Linux host, `tmux 3.6` at `/usr/bin/tmux` (PATH-resolved).
- Worktree `/home/knatte/Code/loomyard/wts/reed-shuttle-crucible-hardening`, branch `reed-shuttle-crucible-hardening`, clean at start (HEAD `61cead10`).
- Dev binary deployed via `./deploy-dev` → `.dev-bin/lyx`.

## What was tested

(appended as each command/scenario returned)

### Hermetic gates — baseline, before any edit

| command | result |
| --- | --- |
| `go build ./...` | PASS (rc 0) |
| `go vet ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/...` | PASS (rc 0) |
| `go test -count=5 ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/... ./cmd/lyx/...` | PASS (all 5 packages ok) |

### Import-direction check (hubgeom one-way told direction)

`go list -deps ./internal/reedengine | grep -c hubgeom` → `0`.
The full internal dependency list of `reedengine` is `envsource, proc, gitexec, lyxcwd, lyxdirs, logger, yamlengine, configengine, lock, reedengine/render, shell, fsx, state, stencil, tokenvocab` — no `hubgeom`, no `fabricengine`.
`lyxcwd` is present transitively via `logger`, exactly as `doc.go` already states honestly.
**Told direction holds.**

### Live fixture (deliberately different from round 1's)

Round 1 drove a 3-worktree hub with one subpath-anchored worktree.
This round's fixture is a **hub-wide DEEP three-segment anchor** (`apps/web/svc`, recorded at `<hub>/_board/.lyx-anchor`) with **two hubs live at once** and up to four worktrees under the first, including a **prefix-colliding pair** (`bet` / `beta`) and a **dot-named worktree** (`svc.v2`):

```
<scratch>/fx/deepcrucible-HUB/_board/.lyx-anchor   -> "apps/web/svc"
<scratch>/fx/deepcrucible-HUB/{alpha,beta,bet,svc.v2}/apps/web/svc   (each its own git repo)
<scratch>/fx/othercrucible-HUB/gamma                                 (root-anchored, second hub)
```

Every scenario below ran the DEPLOYED `.dev-bin/lyx` (re-deployed from source at `b38f58c3`) directly, foreground, from the anchored cwd.

| # | scenario | observed | verdict |
| --- | --- | --- | --- |
| L1 | `reed status` from the worktree ROOT of a deep-anchored worktree | `ErrCwdOutsideAnchor` naming both sides and `.lyx-anchor`; exit 1 | OK |
| L2 | `reed up` from the deep anchor (`alpha/apps/web/svc`), FRESH hub whose `_board/.lyx` did not exist | `ok:true`, session `alpha`, socket `lyx-deepcrucible-HUB-d8e18cc3`; `<hub>/_board/.lyx/logs` **created on this first boot**; state at `<anchor>/.lyx/reed.json`; both panes' `pane_current_path` == the deep anchor | OK — closes the HubLogsDir-ownership-move invariant |
| L3 | `reed up` in a second worktree of the same hub (`beta`) and in a second hub (`gamma`) | `beta` shares socket `lyx-deepcrucible-HUB-d8e18cc3`; `gamma` gets `lyx-othercrucible-HUB-8717841b`. One server per hub, shared across worktrees | OK |
| L4 | three strand `add`s across `alpha`/`beta`; a payload writing `pwd` | every pane's cwd == the told deep anchor, and the payload's own `pwd` confirms the SHELL cwd (not just tmux's `pane_current_path`) | OK — F2's `-c` invariant holds at all three spawn sites |
| L5 | `reed remove` a strand in `alpha` | its `sleep` child reaped; `alpha`'s OTHER strand child and `beta`'s child both untouched | OK — cross-worktree scope holds for `remove` |
| L6 | crash/rebirth: `tmux -L <socket> kill-server` with two sessions live, then `reed status`/`reed resume` in each worktree | `status` → friendly `no reed session (1 strands persisted)`; `resume` → `resumed:1` in both worktrees, fresh header + strand panes, all at the told anchor | OK |
| L7 | header keepalive under the new `Geometry` construction | header pane runs `<dev-bin>/lyx reed header --blocking`, rendering `hub: <hub>` — the told `HubPath`, resolved by the pane's own re-entrant CLI at the told anchor cwd | OK |
| L8 | `reed down` in `alpha` with a **SIGHUP-immune** pane descendant (`bash -c 'bash -c "trap \"\" HUP; exec sleep 3000" & exec sleep 3000'`) in BOTH `alpha` and `beta` | `alpha`'s immune descendants gone the moment `down` returned (15.1s — the full `reapExitTimeout` graceful window, then force-kill); `beta`'s immune descendants and all its panes untouched; `alpha`'s `reed.json` deleted | OK — F1's force-kill fallback still live, cross-worktree scope holds for `down` |
| L9 | prefix-collision: `reed status` and `reed down` from worktree `bet` while only session `beta` is live | `status` → `no reed session`; `down` → `ok:true` and `beta` fully intact (4 panes) | OK — `=<name>` exact targeting holds |
| L10 | dead-pane handling: strand whose pane shell `exit`s → `pane_dead=1`; then `up`, then `resume` | `up` reaps the corpse and clears the binding (`live:false`, `paneId:""`); `resume` relaunches it into a fresh pane; header stays physically top at height 1 | OK |
| L11 | foreign pane planted in `beta`'s session by a raw `tmux split-window` | `reed status` (read-only) leaves it alone; the next mutating `reed up` reaps it deterministically | OK |
| L12 | 5 CONCURRENT `reed add` calls at one worktree | all five succeed, five distinct guids, five distinct pane ids, no duplicate binding, layout consistent | OK — `reed.lock` op lock serialises correctly |
| L13 | mid-operation interrupt: `SIGKILL` the `lyx reed add` process mid-flight | state stayed consistent (add persists immediately after launch); no orphan pane, no stale-lock residue — the next op proceeded with no wait | OK |
| L14 | remove the SOLE strand on native tmux, then `add` again | header keeps the session alive; the second `add` lands a live pane and runs its command | OK |
| L15 | **`reed up` from a worktree whose directory name contains `.`** (`svc.v2`) | 20.1s hang, then `{"error":"tmux server is up but session \"svc.v2\" did not materialize within 20s"}`; a session named `svc_v2` left on the SHARED hub server; `reed status` says "no reed session"; **`reed down` returns `{"ok":true,"session":"svc.v2"}` while the stray session survives** | **FAIL — finding R2-F1** |
| L16 | tmux session-name grammar probe (`a.b`, `a:b`, `a b`, `a/b`, `a$b`, `a-b`, `a_b`, `a%b`, `a#b`, `a@b`) | tmux 3.6 SILENTLY rewrites `.` and `:` to `_` and accepts with exit 0; every other character passes through verbatim | evidence for R2-F1 |
| L17 | dead-pane `#{pane_pid}` probe | tmux reports a dead pane's `pane_pid` as the pid of the already-exited process (verified: pid 2036473, `pane_dead=1`, process gone) | evidence for R2-F2 |
| L18 | `tmux -L 'lyx-/-deadbeef' new-session` (the socket key `ServerName` derives when the hub resolves to `/`) | `error creating /tmp/tmux-1000/lyx-/-deadbeef (No such file or directory)` — and tmux still **exits 0** | evidence for R2-F3 |

### Hermetic + tagged gates after the live pass (still baseline, no edits yet)

| command | result |
| --- | --- |
| `go test -tags smoke ./internal/reedcli/... -run '<all 18 tmux-only Smoke tests>' -count=1` | PASS, 30.1s (the one `claude`-spawning test, `TestSmokeClaudeResumeRecallsCodeword`, deliberately excluded from the sweep per the cost declaration) |
| `go test -tags integration ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/... -count=1` | PASS |

## Findings

Five findings: 1 BLOCKING, 1 MEDIUM, 1 LOW, 2 NIT.
Round 1's ten findings (F1–F10) were re-verified where behavioural (see L4, L5, L8, L9 above, and the `producers-standalone.md` table re-read for F5) — **no regression of any of them**.

---

### R2-F1 — BLOCKING — CONFIRMED — a worktree directory name containing `.` or `:` makes `reed up` fail after a 20 s hang and strands an untearable tmux session on the shared per-hub server

**Where.** `internal/reedengine/server.go:29` (`SessionName`), consumed via `internal/hubgeom/hubgeom.go:20` into `Geometry.SessionName` (`internal/reedengine/geometry.go:18`), spent at `internal/reedengine/lifecycle.go:294` (`new-session -s <session>`) and read back through `exactSessionTarget`/`exactSessionWindowTarget` at every other site.

**Failure scenario.**
`SessionName(worktreeRoot)` is `filepath.Base(worktreeRoot)`, passed to tmux verbatim, and neither `reedengine.New` (which by documented contract validates nothing) nor `hubgeom.ReedGeometry` (which passes the `Location`'s values through untouched) checks it against tmux's own session-name grammar.
tmux 3.6 does not reject a session name containing `.` or `:` — it **silently rewrites both characters to `_` and exits 0** (L16: `a.b` → `a_b`, `a:b` → `a_b`; every other character tested passes through verbatim).

So for a worktree named `svc.v2` (L15, driven end to end):

1. `lyx reed up` spawns `new-session -d -s svc.v2`; tmux creates a session actually named `svc_v2`.
2. The boot loop then polls `has-session -t =svc.v2` — an EXACT-match target, and correctly so — which can never match. It burns the full `bootAttemptTimeout` (20 s).
3. `list-sessions` is non-empty (the rewritten session, plus any sibling worktree's), so the loop takes the "server is up but session did not materialize" branch and returns
   `{"error":"tmux server is up but session \"svc.v2\" did not materialize within 20s"}` — a message that names neither the cause nor a remedy.
4. **The `svc_v2` session survives forever.** `lyx reed status` reports `no reed session`. `lyx reed down` targets `=svc.v2`, kills nothing, and returns **`{"ok":true,"session":"svc.v2"}`** — a false success.
5. Because the server is shared per hub, that orphan session also keeps the hub's tmux server alive, so a sibling worktree's `down` will never tidy the server either (`Down` only kills the server when `list-sessions` comes back empty).

This is the normal single-instance flow — one operator, one worktree, one `lyx reed up` — so it is squarely inside the stated merge bar.
It is not a hypothetical name either: `fabricengine`'s `validateWorktreeSlug` (`internal/fabricengine/slug.go`) rejects only empty, separator-carrying, `.`/`..`, weft-suffixed, and hub-reserved slugs, so `lyx fabric add release-1.2` is accepted today, and reed additionally runs in any plain git repo (this round's whole fixture is plain git repos).

**Why wave-1 is the right round to catch this.** Before wave-1 the session name was derived inside the engine from the one `*lyxcwd.Location`; wave-1 turned it into a told string with an explicit "the caller owns populating it" contract — and that contract states a safety obligation for `SocketKey` ("a socket-safe key") while stating none at all for `SessionName`. Exactly F2's shape: an implicit derivation became an explicit contract the code does not honour.

**Suggested fix.** Refuse the geometry loudly, before any tmux round trip, at the one chokepoint every public engine op passes (`withOpLock`), with an actionable message naming the offending characters and the worktree directory — the same fail-loud-pre-flight discipline `debugLogArgs`, `mouseOption`, `ValidateHeader` and `probeCapabilityLocked` already follow. Do NOT sanitize the name into `_`: two sibling worktrees `svc.v2` and `svc_v2` would then collide on one session name and each would adopt the other's panes, which is strictly worse than a loud refusal. State the constraint on `Geometry.SessionName`'s field doc so a future non-hub teller inherits it.

---

### R2-F2 — MEDIUM — CONFIRMED — `Down` snapshots reap roots from DEAD panes too, so it can block on, and then SIGKILL, an unrelated process tree that recycled the pid

**Where.** `internal/reedengine/lifecycle.go:822-834` (`panePIDsLocked`) and `:838-840` (`paneProcessTreePIDsLocked`), against `internal/reedengine/strand.go:362-380` (`alivePanePIDs`).

**Failure scenario.**
`RemoveStrand` deliberately snapshots its reap roots through `alivePanePIDs`, whose doc comment states the rule: *"only a still-running pane's pid is a safe descendant-closure root (a dead pane's recorded pid may already have been reused by an unrelated process)."*
`Down` does not. `panePIDsLocked` filters on `p.PID > 0` alone and keeps every pane the session lists, **including `pane_dead=1` corpses** — and L17 confirms live that tmux reports a dead pane's `#{pane_pid}` as the pid of the already-exited process (pid 2036473, `pane_dead=1`, process confirmed gone).

A dead pane is a routine, long-lived reed state, not an exotic one: `remain-on-exit on` is set at every boot precisely so a strand whose process exits stays enumerable, reconcile deliberately KEEPS the last dead pane and any dead header corpse, and hours can pass between that death and the operator's `lyx reed down`. If the host recycles that pid in the interval, `Down` will:

1. feed the recycled pid into `descendantClosurePIDs`, which expands it to the unrelated process **and its entire descendant subtree** (`proctree_linux.go` walks all of `/proc`),
2. block in `reapPaneChildren` for up to `reapExitTimeout` (15 s) waiting for a process it does not own to exit, and then
3. `proc.KillPID` it — SIGKILL — plus every descendant, each with a further 5 s `forceKillExitGrace`.

That is reed force-killing processes outside its own session, on a code path whose entire purpose is "leave no stray state of OUR OWN".

**Also false, in the same neighbourhood.** `waitProcessExit`'s own doc comment (`lifecycle.go:934-935`) asserts *"the callers bound that by snapshotting only pids whose panes are alive at snapshot time (see alivePanePIDs)"*. That is true of `RemoveStrand` and false of `Down` — the comment documents an invariant one of its two callers does not keep.

**Suggested fix.** Give both reap-root snapshots one shared purity predicate (present ∧ not dead ∧ pid > 0), so `Down`'s whole-session form and `RemoveStrand`'s targeted form can never drift again, and correct `waitProcessExit`'s doc comment to what the code actually guarantees (including that `proc.IsAlive` reads a not-yet-reaped zombie as alive).

---

### R2-F3 — LOW — PLAUSIBLE (both halves confirmed separately, not driven end to end) — a hub that resolves to the filesystem root yields a socket key containing a path separator, which tmux cannot create a socket for, and reed burns the full 90 s boot deadline without ever naming the cause

**Where.** `internal/reedengine/server.go:20-26` (`ServerName`) → `Geometry.SocketKey` → `TmuxCmd.socket` (`overlay.go:52,68`).

**Failure scenario.**
`lyxcwd` derives `HubPath` as `filepath.Dir(worktreeRoot)`, so a git worktree checked out one level under the filesystem root — `/workspace`, `/app`, `/src`, the standard container shapes — resolves `HubPath` to `/`. Go's `filepath.Base("/")` returns `"/"`, so `ServerName` produces `lyx-/-<hash>`.
tmux resolves `-L <name>` as a filename under `/tmp/tmux-<uid>/`, so this becomes `/tmp/tmux-1000/lyx-/-<hash>`, whose parent does not exist. L18 confirms tmux's response: `error creating /tmp/tmux-1000/lyx-/-deadbeef (No such file or directory)` — **and tmux still exits 0.**
`spawnSession` uses `cmd.Start()` and never inspects the exit status (correctly — it is a detached spawn), so the boot loop just polls, fails every attempt, reaps nothing (no processes hold the socket), and finally reports `tmux session did not start after 8 attempts` or `... within 1m30s` — after up to 90 s, naming nothing about the real cause.

`Geometry.SocketKey`'s own field doc already asserts the caller must supply "a socket-safe key"; nothing checks that the shipped hub-mode teller actually does.

Not driven end to end because creating a git repo directly under `/` needs root on this host; both halves — the derivation and tmux's response to the derived name — were confirmed independently.

**Suggested fix.** Validate the told `SocketKey` in the same pre-flight chokepoint as R2-F1: non-empty and free of path separators, refused with a message naming the hub path.

---

### R2-F4 — NIT — CONFIRMED — `ConfigTemplate`'s doc comment advertises a `claude` key reed's template does not have

**Where.** `internal/reedengine/template.go:12-13`: *"The template uses `${env:VAR:-default}` syntax for the machine tool paths (tmux/shell/claude) and debug_log."*

`internal/reedengine/template_posix.yaml` and `template_windows.yaml` carry `tmux`, `shell`, `debug_log` and `mouse` under `${env:…}` — and **no `claude` key at all** (that one belongs to shuttle's template). The same sentence also omits `mouse`, which IS `${env:…}`-driven.
Harmless at runtime, but it is a package-doc claim about reed's own config surface that a reader can check and find false — and the Shuttle Provider-Seam Invariant makes a stray Claude reference in `reedengine` exactly the kind of provider leakage that is meant to stay out of this package.

**Suggested fix.** Name the four keys the templates actually gate on env, and drop `claude`.

---

### R2-F5 — NIT — CONFIRMED — `reedcli` still names the resolved `*lyxcwd.Location` `layout`, keeping alive the exact vocabulary wave-1 retired, at the seam where it is most confusable

**Where.** `internal/reedcli/cli.go:5` and `:30` (header/`Command` doc comments: "resolves cwd -> layout -> config -> geometry"), and the local variable at `:66`, `:79`, `:86`.

`Engine.layout` was renamed to `Engine.geom` by wave-1 precisely because the value is not a layout, and round 1 cleared the leftover `layout` vocabulary from `reedengine`'s comments. The identifier survives in `reedcli` — the one file whose whole job is converting a `Location` into a `Geometry`. It is doubly wrong here: `layout` is a live first-class domain term in this very module (the tmux `window_layout` string, `planLayout`, `applyLayoutLocked`, `select-layout`), so a local named `layout` holding a `*lyxcwd.Location` collides head-on with reed's own vocabulary.

**Suggested fix.** Rename the local to `location` and reword the two doc comments to "cwd -> location -> config -> geometry".
The same identifier also survives in `internal/burlercli`, `internal/shuttlecli`, `internal/perchcli` and `internal/webstercli` — all out of this round's scope (shuttle explicitly so), recorded here as a follow-up rather than touched.

---

## Observations — NOT findings (no fix, recorded for honesty)

- **Stale tmux socket FILES accumulate in `/tmp/tmux-<uid>/`.** After a clean `lyx reed down`, the socket file remains. This is tmux's own behaviour, not reed's: the two `probe-crucible`/`probe2-crucible` sockets this review created and tore down with a plain `tmux kill-server` are still present too. Functionally harmless (tmux replaces a stale socket on the next connect), so no fix — but `TestSmokeDownLeavesNoTmuxOnSocket` checks for stray PROCESSES, not stray socket files, and that is the correct scope.
- **`sessionlessSocketHolderPersists`'s 5 s grace is a heuristic, not a guarantee.** A sibling worktree whose boot exceeds the grace window on a saturated machine can have its just-spawned, not-yet-reachable server reaped. Self-healing (the victim's boot loop retries, and nothing of the victim's had materialised yet to lose) and already documented as a grace rather than a proof, so it is left as is.
- **Editing `<hub>/_board/.lyx-anchor` while a session is live** moves `stateDir` out from under the running session, producing a second header pane and orphaning the old strands' panes. Out of contract — the anchor is by design "a fixed per-repo anchor recorded once at clone/create" (Cwd Resolution Invariant) — so not treated as a defect.
- **Windows verification gap**, as the prompt anticipates: `proctree_windows.go`, the psmux version floor, and the psmux-only silent-split shape are compile-checked and unit-tested here but not driven on this Linux host.

## Scope assessment

reed's scope is unchanged and correct post wave-1. The told-geometry refactor dropped no observable behaviour: every pre-wave-1 derivation now arrives as a told field, and this round drove each one to its real substrate effect —
`SocketKey` (L3: one server per hub, distinct across hubs), `SessionName` (L9: exact targeting, no prefix bleed), `AnchorPath` (L2/L4: state dir, and all three tmux spawn sites' pane cwd, confirmed by the pane payload's own `pwd`), `WorktreeRoot` (strand `Worktree` stamp and `<WORKTREE>` name token), `LogsDir` (L2: created on a FRESH hub's very first boot, closing the `HubLogsDir` ownership move), and `RepoName`/`HubPath` (L7: the live header pane's rendered tokens).

The seven fields cannot disagree about which hub or worktree they mean, because `hubgeom.ReedGeometry` builds all seven from one already-resolved `*lyxcwd.Location` in a single expression and is the only production populator; the four other call sites (`burlercli`, `shuttlecli`, `perchcli`, `webstercli`) each resolve exactly one `Location` in their own `PersistentPreRunE` and pass it straight in, anchoring config at `AnchorPath()` identically to `reedcli`.
What the refactor DID introduce is a validation vacuum — the fields are now independently settable and nothing checks any of them — and R2-F1/R2-F3 are the two places where the shipped teller can hand reed a field the substrate cannot honour.

Nothing is shipped beyond scope; nothing plan-promised is deferred that should be v1.

## Merge-readiness verdict

**Not merge-ready as reviewed** — R2-F1 is a BLOCKING defect in the normal single-instance flow (hard failure plus permanent stray state on a shared server, plus a `down` that falsely reports success).
Merge-ready once R2-F1–R2-F5 are fixed, all gates stay green, and the live scenarios are re-driven against a re-deployed binary; see `_mill/reed-review-r2-fixer-report.md`.
