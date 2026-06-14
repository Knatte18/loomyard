# Batch: muxpoc-doc

```yaml
task: Fix stale .mhgo/ config-layer docs + cross-cutting stale-docs sweep
batch: muxpoc-doc
number: 2
cards: 2
verify: go build ./... && go vet ./...
depends-on: []
```

## Batch Scope

Documents the shipped-but-undocumented `internal/muxpoc` module: creates
`docs/modules/muxpoc.md` (a full module doc on par with board.md/worktree.md) and fixes the
one factually-stale doc-comment in `internal/muxpoc/cli.go` (the `daemon` subcommand is
marked "not yet implemented" but `daemon.go` implements it). The new `muxpoc.md` is the
**external interface** the next batch (tree-sweep) cross-references from overview.md,
roadmap.md, and mux.md — which is why tree-sweep depends on this batch. Follows the
`muxpoc-coexist-framing` Shared Decision: muxpoc is a proof-of-concept that coexists with the
still-planned `internal/mux`. Doc/comment-only (Shared Decision `docs-and-doc-comments-only`);
the `.go` edit must not touch line endings (`never-modify-line-endings`).

## Cards

### Card 6: Write docs/modules/muxpoc.md

- **Context:**
  - `internal/muxpoc/cli.go`
  - `internal/muxpoc/cmd.go`
  - `internal/muxpoc/state.go`
  - `internal/muxpoc/up.go`
  - `internal/muxpoc/daemon.go`
  - `internal/muxpoc/review.go`
  - `internal/muxpoc/attach.go`
  - `internal/muxpoc/status.go`
  - `internal/muxpoc/down.go`
  - `internal/muxpoc/spawn_windows.go`
  - `internal/muxpoc/spawn_other.go`
  - `cmd/mhgo/main.go`
  - `docs/modules/worktree.md`
  - `docs/modules/board.md`
  - `docs/modules/mux.md`
  - `docs/shared-libs/state.md`
- **Edits:** none
- **Creates:**
  - `docs/modules/muxpoc.md`
- **Deletes:** none
- **Requirements:** Author a module doc modelled on the structure of `docs/modules/worktree.md`
  and `docs/modules/board.md`. Read the muxpoc source listed in Context for accuracy — do not
  rely on summary alone. Cover, at minimum: (1) **What it is** — a proof-of-concept psmux
  session orchestrator for driving `claude` TUIs across panes; dispatched as `mhgo muxpoc
  <subcommand>` (see `cmd/mhgo/main.go`, which labels it "proof-of-concept psmux mux"); one
  psmux server per repo, socket+session name = `muxpoc-<sanitised filepath.Base(cwd)>` (the
  `socketName` func in `state.go`). (2) **Subcommands** (from `cli.go`): `up` (cold-start a
  new session, or cold-recover an existing one whose server died), `review` (add a reviewer
  pane via `split-window`), `attach` (pop into a maximized Windows Terminal on Windows /
  interactive `psmux attach` elsewhere — `spawn_windows.go`/`spawn_other.go`), `status`
  (state-exists + server-running + live panes + saved metadata), `down` (stop server + delete
  state = intentional shutdown, distinct from a crash), `daemon` (foreground poller; recovers
  a dead session up to `maxRecoveries=3` within a `windowDur=60s` window via a crash-loop
  guard — `daemon.go`). (3) **State model** (`state.go`): `.mhgo/muxpoc-state.json` — the
  gitignored **runtime-state dir**, explicitly NOT the removed config layer (cross-reference
  `../shared-libs/state.md` and note the distinction); fields `session`, `socket`,
  `stripped_env`, `panes[]` where `Pane = {id, session_id, kind: main|review}`; atomic write
  via `board.AtomicWrite` under an exclusive `internal/lock` file
  (`.mhgo/muxpoc-state.lock`), reads under a shared lock; corrupt state ⇒ warn + treat as no
  session. (4) **Recovery model** (`up.go`/`daemon.go`): state survives a server crash; psmux
  reassigns fresh pane ids across a restart, so recover re-launches `claude --resume
  <session-id>` per saved pane and re-tiles; `down` deletes state to mark intentional
  shutdown so `up` won't recover. (5) **Layout** (`cmd.go`): vertical column, bottom (active)
  pane gets ~`activePaneShare=55%` via a hand-built window-layout string with a tmux-
  compatible checksum (`buildColumnLayout`/`layoutChecksum`) because presets like
  `even-vertical` cannot express "bottom dominant". (6) **Env sanitization** (`state.go`):
  `sanitizeEnv` strips `CLAUDECODE` and `CLAUDE_CODE_*` from the server's environment so child
  claude processes don't inherit them; stripped keys are recorded in state. (7) **Config:**
  flag-driven (`-psmux`/`-pwsh`/`-claude` paths, `-launch`/`-resume` templates,
  `-width`/`-height`, `-interval`) — muxpoc does **not** use `internal/config` or an `_mhgo/`
  YAML, unlike board/worktree; note this difference. (8) **Dependencies:** external
  `psmux.exe`, `claude`, `pwsh`, Windows Terminal (attach); internal `output` (JSON), `lock`
  (state lock), and `board.AtomicWrite` (a cross-module reach worth flagging — tie it to
  `state.md`'s note that `AtomicWrite`/`PathGuard` want a real home). (9) **Windowless
  spawning:** `spawn_windows.go` (detached, `CREATE_NO_WINDOW`, own process group) vs
  `spawn_other.go` (`Setsid`). (10) **Relationship to planned `internal/mux`:** coexist —
  muxpoc is the POC, `mux.md` is the forward design (per `muxpoc-coexist-framing`). (11)
  **Tests:** `cli_test.go`, `cmd_test.go`, `state_test.go` (unit; pure parsers like
  `parsePaneList`/`buildColumnLayout`/`socketName`), `muxpoc_smoke_test.go` (smoke). Use
  relative doc links consistent with the other module docs (e.g. `../shared-libs/state.md`,
  `mux.md`, `../overview.md`).
- **Commit:** `docs(muxpoc): add module doc for the psmux POC orchestrator`

### Card 7: Fix muxpoc cli.go daemon "not yet implemented" comment

- **Context:**
  - `internal/muxpoc/daemon.go`
- **Edits:**
  - `internal/muxpoc/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/muxpoc/cli.go`, the `RunCLI` doc-comment's subcommand list
  has `daemon    [not yet implemented in this batch]`. This is stale — `daemon.go` implements
  `cmdDaemon` (a foreground polling loop with a crash-loop guard: recovers a dead session up
  to `maxRecoveries=3` within `windowDur=60s`). Replace the bracketed "[not yet implemented in
  this batch]" with a short accurate description, e.g. "foreground poller; recovers a crashed
  session (crash-loop-guarded)". Comment-only; do not change the `switch` dispatch or any
  code, and do not touch line endings.
- **Commit:** `docs(muxpoc): cli.go reflects implemented daemon subcommand`

## Batch Tests

`verify: go build ./... && go vet ./...` — Card 7 is a doc-comment-only edit to
`internal/muxpoc/cli.go`; `go build`/`go vet` confirm the comment edit doesn't break
compilation or vet. Card 6 only creates a Markdown file (`docs/modules/muxpoc.md`), which has
no runnable surface — its correctness (accurate symbol names, working relative links) is
validated by plan/code review and the handoff link/grep checks, not by `go` tooling. No
behaviour changes, so muxpoc's existing tests (`cli_test.go`, `cmd_test.go`, `state_test.go`,
`muxpoc_smoke_test.go`) are unaffected and stay green under the full `go test ./...` at
handoff.
