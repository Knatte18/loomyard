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

## Findings

(recorded provisionally as spotted; severities finalised at the end)

## Merge-readiness verdict

(filled at the end of Job 1)
