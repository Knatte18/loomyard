# Batch: muxcli

```yaml
task: 'Build internal/mux: the window to the world (overlay + strands + render)'
batch: 'muxcli'
number: 6
cards: 7
verify: go test ./internal/muxcli/...
depends-on: [5]
```

## Batch Scope

Creates `internal/muxcli`, the cobra CLI over `muxengine`, following the CLI/Cobra
Invariant's `Command()`/`RunCLI` seam and the weftcli Idiom-B subcommand shape
(`ShouldAbort` -> engine call -> `SetExit(ctx, output.Ok/Err(...))`, always `return nil`).
Verbs: `up`, `add`, `remove`, `status`, `attach`, `resume`, `down` (`UpdateStrand` is
engine-API-only — no CLI verb in v1). The parent group uses `RunE: clihelp.GroupRunE` +
`PersistentPreRunE` that resolves layout+config into a closure `*muxengine.Engine` (skipping
work when `cmd.Name() == "mux"` so bare listing / unknown-subcommand needs no git repo). CLI
verbs **never take the mux op lock** (the engine does). External interface batch 7 consumes:
`muxcli.Command()` and `muxcli.RunCLI(out, args)`.

Batch-local decisions:
- **`add` flag spec:** `--cmd` (required), `--role`, `--round`, `--name`, `--resume-cmd`,
  `--parent <guid>`, `--anchor top|below-parent|hidden` (default `below-parent`;
  `own-window` rejected), `--focus`. Name resolved via `FormatStrandName` (`--role`/`--round`
  fill the template; `--name` overrides; `<SHORT_GUID>` fallback). The `add` JSON prints the
  generated `guid` + resolved name so a later `--parent`/`remove` can reference it.
- **`remove` guard:** `lyx mux remove <guid>` on a **non-leaf** requires `--recursive` (else
  error `strand has children, use --recursive`); the result JSON lists every removed strand.
- **`attach` is the registered JSON-envelope exception:** everything that can fail runs
  pre-flight **on the envelope** (`output.Err`, non-zero); only the terminal-handover tail
  (after stdio is inherited) is exempt and emits no JSON on success.

## Cards

### Card 21: muxcli scaffold — Command(), RunCLI, group + PersistentPreRunE

- **Context:**
  - `internal/weftcli/cli.go`
  - `internal/muxpoccli/cli.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/muxengine/lock.go`
  - `internal/muxengine/config.go`
- **Edits:** none
- **Creates:**
  - `internal/muxcli/cli.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxcli/cli.go` (package `muxcli` — no import alias, so the
  registration AST guard matches `muxcli.Command()`): `func Command() *cobra.Command` with
  parent `Use: "mux"`, non-empty `Short`, `RunE: clihelp.GroupRunE`, and a
  `PersistentPreRunE` that returns `nil` early when `cmd.Name() == "mux"`, else resolves
  `hubgeometry.Getwd()` -> `hubgeometry.Resolve(cwd)` -> `muxengine.LoadConfig(baseDir,
  "mux")` -> `muxengine.New(cfg, layout)` into a closure `var eng *muxengine.Engine` (on
  failure: `output.Err(cmd.OutOrStdout(), err.Error())` + `clihelp.Abort(cmd.Context(), 1)`
  + `return nil`). `func RunCLI(out io.Writer, args []string) int { return
  clihelp.Execute(Command(), out, args) }`. Register the seven subcommands (cards 22-27) in
  the order `up, add, remove, status, attach, resume, down` (matching batch 7's helptree
  `wantSubs`). Every subcommand carries a non-empty `Short`. This card wires the scaffold and
  empty subcommand stubs (or references the funcs the later cards fill).
- **Commit:** `feat(muxcli): cobra scaffold with Command/RunCLI seam and PreRunE engine wiring`

### Card 22: `up` and `down` verbs

- **Context:**
  - `internal/muxcli/cli.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/muxengine/lifecycle.go`
- **Edits:**
  - `internal/muxcli/cli.go`
- **Creates:**
  - `internal/muxcli/up.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxcli/up.go`, add the `up` and `down` subcommand builders
  (returning `*cobra.Command`), each RunE following Idiom B: `if
  clihelp.ShouldAbort(cmd.Context()) { return nil }`; call `eng.Up()` / `eng.Down()`; on
  error `clihelp.SetExit(ctx, output.Err(out, err.Error()))`; on success
  `clihelp.SetExit(ctx, output.Ok(out, <resultMap>))`; always `return nil`. `up` help must
  state it is substrate-only (boots server/session, applies layout, reconciles — runs **no**
  strand command); `down` kills the server + clears state. Non-empty `Short` + a `Long` with
  an example on each. Wire both into `Command()` (edit `cli.go`).
- **Commit:** `feat(muxcli): up and down verbs`

### Card 23: `add` verb (full flag spec)

- **Context:**
  - `internal/muxcli/cli.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/muxengine/strand.go`
  - `internal/muxengine/name.go`
  - `internal/muxengine/config.go`
  - `internal/muxengine/render/types.go`
- **Edits:**
  - `internal/muxcli/cli.go`
- **Creates:**
  - `internal/muxcli/add.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxcli/add.go`, build the `add` command with flags `--cmd`
  (required), `--role`, `--round`, `--name`, `--resume-cmd`, `--parent`, `--anchor` (default
  `below-parent`), `--focus`. In RunE (Idiom B): reject `--anchor own-window` (deferred) and
  any value outside `top|below-parent|hidden` with `output.Err`; resolve the name via
  `muxengine.FormatStrandName(cfg.StrandName, parts)` filling `<ROLE>/<ROUND>/<WORKTREE>` and
  computing `<SHORT_GUID>` from the guid the engine returns (if `--name` given, override
  verbatim; if neither `--name` nor `--role`, fall back to `<SHORT_GUID>`); build the
  `muxengine.AddSpec` (opaque `Cmd`/`ResumeCmd`, `Worktree` from layout, `Display{Anchor,
  Focus, ShrinkWhenWaitingOnChild:true}`); call `eng.AddStrand(spec)`; emit `output.Ok` with
  the generated `guid` + resolved `name` (so a later `--parent`/`remove` can reference it).
  Non-empty `Short` + `Long` with an example. Wire into `Command()`.
- **Commit:** `feat(muxcli): add verb with anchor/role/parent flag spec`

### Card 24: `remove` verb (--recursive guard, removed-list JSON)

- **Context:**
  - `internal/muxcli/cli.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/muxengine/strand.go`
- **Edits:**
  - `internal/muxcli/cli.go`
- **Creates:**
  - `internal/muxcli/remove.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxcli/remove.go`, build `remove <guid>` with a
  `--recursive` bool flag. RunE (Idiom B): require exactly one positional `guid`; if the
  target strand is a **non-leaf** and `--recursive` is not set, emit `output.Err` with
  `strand has children, use --recursive` and non-zero exit (do this via a pre-check helper on
  the engine, e.g. `eng.HasChildren(guid)`, or by having `RemoveStrand` accept a
  `recursive bool` and refuse a non-leaf without it — pick the engine API that keeps the CLI
  a thin caller; if adding `HasChildren`, note it edits `strand.go`, but prefer threading
  `recursive` into the existing `RemoveStrand` signature). With `--recursive` (or a leaf),
  call the cascade and emit `output.Ok` listing every removed strand (`guid` + `name`) from
  the `Removed` result. Non-empty `Short` + `Long`. Wire into `Command()`.
- **Commit:** `feat(muxcli): remove verb with --recursive guard and removed-list output`

### Card 25: `status` verb

- **Context:**
  - `internal/muxcli/cli.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/muxengine/lifecycle.go`
- **Edits:**
  - `internal/muxcli/cli.go`
- **Creates:**
  - `internal/muxcli/status.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxcli/status.go`, build `status`. RunE (Idiom B): call
  `eng.Status()` and emit `output.Ok` with the tracked strands + live/dead reconcile for
  **this session only** (no stray-server enumeration — deferred). Non-empty `Short` + `Long`
  noting v1 reports only the current worktree's session. Wire into `Command()`.
- **Commit:** `feat(muxcli): status verb (this session only)`

### Card 26: `resume` verb

- **Context:**
  - `internal/muxcli/cli.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/muxengine/lifecycle.go`
- **Edits:**
  - `internal/muxcli/cli.go`
- **Creates:**
  - `internal/muxcli/resume.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxcli/resume.go`, build `resume`. RunE (Idiom B): call
  `eng.Resume()` and emit `output.Ok` with the per-strand replay result. Help must state
  `resume` is the only replayer (recreates not-live, non-hidden panes and runs each stored
  `resumeCmd`/`cmd`), skips `hidden` strands, and leaves already-live strands untouched.
  Non-empty `Short` + `Long` with an example. Wire into `Command()`.
- **Commit:** `feat(muxcli): resume verb`

### Card 27: `attach` verb (in-place; JSON-envelope exception) + integration/smoke tests

- **Context:**
  - `internal/muxcli/cli.go`
  - `internal/muxpoccli/attach.go`
  - `internal/muxpoccli/spawnattach_windows.go`
  - `internal/muxpoccli/spawnattach_other.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/muxengine/lock.go`
  - `internal/muxengine/lifecycle.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `internal/muxcli/cli.go`
- **Creates:**
  - `internal/muxcli/attach.go`
  - `internal/muxcli/cli_test.go`
  - `internal/muxcli/smoke_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxcli/attach.go`, build `attach` (session-level, no strand
  arg). RunE: run all fail-able work **pre-flight on the envelope** (session existence via
  `eng`/`hasSession`, lock/reconcile) — emit `output.Err` + non-zero on any failure; only the
  **terminal-handover tail** (`psmux -L <socket> attach-session -t <session>` inheriting the
  operator's stdio, in-place — no `wt.exe` new window) is exempt and emits **no** JSON on
  success. Build the invocation with the exported engine accessors `eng.Socket()` (the psmux
  `-L` socket) + `eng.SessionName()` (the `attach-session -t` target) — never the unexported
  `muxengine.socketName`. In
  `cli_test.go` (default, no psmux): assert via `muxcli.RunCLI(&out, args)` that bare `lyx
  mux` lists the seven subcommands (exit 0), an unknown subcommand yields `ok=false` exit 1,
  and the built `attach` invocation targets the worktree session (assert the argv, not a JSON
  round-trip — the documented exception). Use `lyxtest.SeedConfig`/`CopyPaired` for a fixture
  hub. In `smoke_test.go` (`//go:build smoke`): a real `up` -> `add` -> `status` -> `down`
  round-trip against a live psmux server. Wire `attach` into `Command()`.
- **Commit:** `feat(muxcli): attach verb (in-place, envelope exception) with integration tests`

## Batch Tests

`verify: go test ./internal/muxcli/...` runs the seam integration tests: bare-group listing,
unknown-subcommand JSON envelope, and the built-`attach`-invocation assertion — all through
`RunCLI(&out, args)` with a `lyxtest` fixture hub and **no** live psmux. The real
`up/add/status/down` overlay round-trip lives in `smoke_test.go` behind `//go:build smoke`,
so the default `verify:` stays hermetic. Every command's non-empty `Short` is what
`cmd/lyx/drift_test.go` will enforce in batch 7.
