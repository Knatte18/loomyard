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
  registration AST guard matches `muxcli.Command()`): define a receiver `type muxCLI struct {
  eng *muxengine.Engine }` so every verb (defined in cards 22-27 as methods on `*muxCLI`) can
  read the same PreRunE-populated engine. `func Command() *cobra.Command`: build the parent
  (`Use: "mux"`, non-empty `Short`, `RunE: clihelp.GroupRunE`), create `c := &muxCLI{}`, set a
  `PersistentPreRunE` that returns `nil` early when `cmd.Name() == "mux"`, else resolves
  `hubgeometry.Getwd()` -> `layout, _ := hubgeometry.Resolve(cwd)` -> `muxengine.LoadConfig(
  baseDir, "mux")` with **`baseDir = layout.Cwd`** (the `_lyx/config/` root is anchored at
  `layout.Cwd`; `configengine.FindBaseDir` walks up from there) -> `muxengine.New(cfg, layout)`
  into `c.eng` (on failure: `output.Err(
  cmd.OutOrStdout(), err.Error())` + `clihelp.Abort(cmd.Context(), 1)` + `return nil`), and
  return the parent. **This card registers NO subcommands** — each verb card (22-27) creates
  its `(c *muxCLI) xCmd()` method AND edits this `Command()` to `parent.AddCommand(c.xCmd())`,
  so every card compiles at its own boundary (no forward reference to a not-yet-created
  builder). `func RunCLI(out io.Writer, args []string) int { return clihelp.Execute(Command(),
  out, args) }`. By the end of batch 6 all seven verbs (`up, down, add, remove, status,
  resume, attach`) are registered (helptree's `wantSubs` in batch 7 is a set check, so
  AddCommand order is irrelevant); every subcommand carries a non-empty `Short`.
- **Commit:** `feat(muxcli): cobra scaffold with muxCLI receiver, Command/RunCLI seam, PreRunE wiring`

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
- **Requirements:** In `internal/muxcli/up.go`, add the `up` and `down` verbs as methods
  `(c *muxCLI) upCmd()` and `(c *muxCLI) downCmd()` (each returning `*cobra.Command`), each
  RunE following Idiom B: `if clihelp.ShouldAbort(cmd.Context()) { return nil }`; call
  `c.eng.Up()` / `c.eng.Down()`; on error `clihelp.SetExit(ctx, output.Err(out,
  err.Error()))`; on success `clihelp.SetExit(ctx, output.Ok(out, <resultMap>))`; always
  `return nil`. `up` help must state it is substrate-only (boots server/session, applies
  layout, reconciles — runs **no** strand command); `down` kills the server + clears state.
  Non-empty `Short` + a `Long` with an example on each. Edit `Command()` in `cli.go` to
  `parent.AddCommand(c.upCmd(), c.downCmd())`.
- **Commit:** `feat(muxcli): up and down verbs`

### Card 23: `add` verb (full flag spec)

- **Context:**
  - `internal/muxcli/cli.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/muxengine/strand.go`
  - `internal/muxengine/render/types.go`
- **Edits:**
  - `internal/muxcli/cli.go`
- **Creates:**
  - `internal/muxcli/add.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxcli/add.go`, add the `add` verb as `(c *muxCLI) addCmd()`
  with flags `--cmd` (required), `--role`, `--round`, `--name`, `--resume-cmd`, `--parent`,
  `--anchor` (default `below-parent`), `--focus`. In RunE (Idiom B): reject `--anchor
  own-window` (deferred) and any value outside `top|below-parent|hidden` with `output.Err`;
  build `muxengine.AddSpec{ Role: <--role>, Round: <--round>, NameOverride: <--name>, Cmd:
  <--cmd>, ResumeCmd: <--resume-cmd>, Parent: <--parent>, Display: render.Display{Anchor:
  <--anchor>, Focus: <--focus>, ShrinkWhenWaitingOnChild: true} }` — the CLI is a thin
  flag-to-spec mapper; the engine (`AddStrand`, card 19) owns guid generation, worktree
  stamping, and name resolution (`NameOverride` else `FormatStrandName` else `<SHORT_GUID>`),
  so the CLI needs neither `cfg` nor `layout`. Call `c.eng.AddStrand(spec)`; emit `output.Ok`
  with the returned strand's `guid` + resolved `name` (so a later `--parent`/`remove` can
  reference it). Non-empty `Short` + `Long` with an example. Edit `Command()` in `cli.go` to
  `parent.AddCommand(c.addCmd())`.
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
- **Requirements:** In `internal/muxcli/remove.go`, add the `remove <guid>` verb as `(c
  *muxCLI) removeCmd()` with a `--recursive` bool flag. RunE (Idiom B): require exactly one
  positional `guid`; call `c.eng.RemoveStrand(guid, recursive)` — the engine (card 19)
  returns the `strand has children, use --recursive` error for a non-leaf when `recursive` is
  false, which the CLI surfaces via `output.Err` + non-zero exit; on success emit `output.Ok`
  listing every removed strand (`guid` + `name`) from the `Removed` result. Non-empty `Short`
  + `Long`. Edit `Command()` in `cli.go` to `parent.AddCommand(c.removeCmd())`.
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
- **Requirements:** In `internal/muxcli/status.go`, add the `status` verb as `(c *muxCLI)
  statusCmd()`. RunE (Idiom B): call `c.eng.Status()` and emit `output.Ok` with the tracked
  strands + live/dead reconcile for **this session only** (no stray-server enumeration —
  deferred). Non-empty `Short` + `Long` noting v1 reports only the current worktree's session.
  Edit `Command()` in `cli.go` to `parent.AddCommand(c.statusCmd())`.
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
- **Requirements:** In `internal/muxcli/resume.go`, add the `resume` verb as `(c *muxCLI)
  resumeCmd()`. RunE (Idiom B): call `c.eng.Resume()` and emit `output.Ok` with the per-strand
  replay result. Help must state `resume` is the only replayer (recreates not-live, non-hidden
  panes and runs each stored `resumeCmd`/`cmd`), skips `hidden` strands, and leaves
  already-live strands untouched. Non-empty `Short` + `Long` with an example. Edit `Command()`
  in `cli.go` to `parent.AddCommand(c.resumeCmd())`.
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
- **Requirements:** In `internal/muxcli/attach.go`, add the `attach` verb as `(c *muxCLI)
  attachCmd()` (session-level, no strand arg). RunE: run all fail-able work **pre-flight on
  the envelope** (session existence via `c.eng`, lock/reconcile) — emit `output.Err` +
  non-zero on any failure; only the **terminal-handover tail** (`psmux -L <socket>
  attach-session -t <session>` inheriting the operator's stdio, in-place — no `wt.exe` new
  window) is exempt and emits **no** JSON on success. Build the invocation with the exported
  engine accessors `c.eng.Socket()` (the psmux `-L` socket) + `c.eng.SessionName()` (the
  `attach-session -t` target) — never the unexported `muxengine.socketName`. In `cli_test.go`
  (default, no psmux): assert via `muxcli.RunCLI(&out, args)` that bare `lyx mux` lists the
  seven subcommands (exit 0), an unknown subcommand yields `ok=false` exit 1, and the built
  `attach` invocation targets the worktree session (assert the argv, not a JSON round-trip —
  the documented exception). Use `lyxtest.SeedConfig`/`CopyPaired` for a fixture hub. In
  `smoke_test.go` (`//go:build smoke`): a real `up` -> `add` -> `status` -> `down` round-trip
  against a live psmux server. Edit `Command()` in `cli.go` to `parent.AddCommand(c.attachCmd())`.
- **Commit:** `feat(muxcli): attach verb (in-place, envelope exception) with integration tests`

## Batch Tests

`verify: go test ./internal/muxcli/...` runs the seam integration tests: bare-group listing,
unknown-subcommand JSON envelope, and the built-`attach`-invocation assertion — all through
`RunCLI(&out, args)` with a `lyxtest` fixture hub and **no** live psmux. The real
`up/add/status/down` overlay round-trip lives in `smoke_test.go` behind `//go:build smoke`,
so the default `verify:` stays hermetic. Every command's non-empty `Short` is what
`cmd/lyx/drift_test.go` will enforce in batch 7.
