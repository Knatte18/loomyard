# muxpoc — single-column psmux daemon/resume proof-of-concept

**Reference only. NOT `internal/mux`.** This is a deliberately minimal spike that lives in
parallel to the future real `internal/mux` (roadmap milestone 5) so it never has to be
overwritten. It exists to prove the *hard* part of the [mux design](../../docs/modules/mux.md):

> keeping a Claude session alive across a psmux crash and recovering it with
> `claude --resume`, for **one in-place column** (no worktrees).

If that works for one column, the architecture is proven; the real module then scales it to
N worktree columns + the daemon/Slack layers. Design rationale and the full empirical trail
are in [`docs/modules/mux.md`](../../docs/modules/mux.md) and
[`docs/modules/mux-exploration.md`](../../docs/modules/mux-exploration.md).

## Usage

```
mhgo muxpoc up        # create the column (or cold-recover from state); launch claude --session-id
mhgo muxpoc review    # stack a reviewer pane below and launch a claude in it
mhgo muxpoc attach    # pop a maximized Windows Terminal attached to the session (the one visible pop)
mhgo muxpoc status    # JSON: state, live panes, server up, which env vars are stripped
mhgo muxpoc down      # kill the psmux server and clear state
mhgo muxpoc daemon    # foreground watchdog: poll the server, rebuild+resume on crash (blocks)
```

Flags (all optional): `--psmux`, `--pwsh`, `--claude` (binary paths), `--launch`/`--resume`
(in-pane command templates, `%CLAUDE%`/`%SID%`), `--width`/`--height`, `--interval` (daemon).
The launch template lets tests/demos use a cheap placeholder (e.g.
`--launch "Write-Host ready"`) instead of a token-spending claude.

State lives in `<cwd>/_mhgo/muxpoc-state.json`; the server runs on an isolated per-cwd socket
`psmux -L muxpoc-<dir>` so it never touches the operator's real psmux.

## The load-bearing finding it encodes

The pane's claude **must not** inherit the Claude Code parent-session env, or it treats itself
as a nested child and silently stops writing its transcript — which breaks `--resume`. Since
mhgo is normally launched *from inside* a Claude Code session (claude spawning reviewers/
implementers), `muxpoc` builds the psmux server's `exec.Cmd.Env` with `CLAUDECODE` and every
`CLAUDE_CODE_*` removed (`sanitizeEnv` in `psmux.go`). The server — and every pane and claude
under it — then inherits a clean top-level env.

## Verified live

- `up` assigns `--session-id`, reports the `stripped_env`, and panes come up with a **clean**
  env (`$env:CLAUDECODE` empty in-pane) — validating the server-env sanitize approach.
- `review` stacks a reviewer pane below the column (vertical split).
- `kill-server` (simulated crash) + `up` cold-recovers: rebuilds both panes and relaunches
  `claude --resume <session-id>` per pane.
- Real `claude --resume` recall after a crash was verified manually with the same env hygiene
  (see mux-exploration.md, codeword OSPREY-4).

## Known PoC limitations / next steps for the real module

- `daemon` is a foreground poller (no mutual watchdog, no detached background mode yet).
- `status.server_up` uses `list-sessions`, which can auto-start an empty server on a dead
  socket; `hasSession(name)` is the reliable liveness check (recovery already uses it).
- The visible `attach` (Windows Terminal) is best-effort (design decision 6).
- One column only; multi-column layout-string rendering (decision 3) is out of scope here.
