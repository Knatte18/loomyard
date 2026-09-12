# Batch: compose-webster

```yaml
task: 'self-report Tier 2: per-agent friction notes for unsupervised runs'
batch: 'compose-webster'
number: 6
cards: 3
verify: go test ./internal/websterengine/... ./contracts/stencils/...
depends-on: [1]
```

## Batch Scope

This batch injects the friction directive into all four `internal/websterengine/render.go` composers — `RenderForkPrompt` (`:145`), `RenderRecoveryPrompt` (`:177`), `RenderIntegrationPrompt` (`:210`), and `RenderMasterPrompt` (`:264`) — adds the matching marker to their four stencils, and threads a told `FrictionDir` through the three Deps structs the four call sites receive.

It depends on batch 1 only.
Every new field defaults to the empty string, so `internal/webstercli` and `internal/loomcli` compile unchanged;
filling those fields is batch 7's job.

The external interface batch 7 consumes is `websterengine.RunDeps.FrictionDir`, `websterengine.BeginDeps.FrictionDir`, and `websterengine.RecoverDeps.FrictionDir`.

Batch-local decision, already recorded in the overview's Shared Decisions and restated here because it is this batch's central structural choice: the field lands on **three** Deps structs rather than on `RunDeps` alone, because `beginbatch.go` takes `BeginDeps` and `recoverbatch.go` takes `RecoverDeps`, neither of which has a `RunDeps` in scope.
It does not land on `websterengine.Geometry`, because `internal/hubgeom` and `internal/standalonegeom` are the Told-Geometry Invariant's only `Geometry`-struct constructors and this value needs no geometry derivation.

## Cards

### Card 19: add `{{.friction_directive}}` to webster's four prompt assets

- **Context:**
  - `contracts/stencils/webster/webster-body-implementer.md`
  - `contracts/stencils/friction/friction-directive-implementer.md`
  - `contracts/stencils/friction/friction-directive-orchestrator.md`
  - `internal/websterengine/render.go`
  - `internal/stencil/stencil.go`
- **Edits:**
  - `contracts/stencils/webster/webster-prefix-fork.md`
  - `contracts/stencils/webster/webster-prefix-recovery.md`
  - `contracts/stencils/webster/webster-template-integration.md`
  - `contracts/stencils/webster/webster-template-master.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add the literal marker `{{.friction_directive}}` on its own line to each of the four files, placed as its own block before the file's first work instruction — in `webster-prefix-recovery.md` and `webster-template-master.md` immediately after the existing `{{.pattern_directive}}` line, and in the other two at the equivalent position.

  The marker goes in the two **prefix** assets, never in `contracts/stencils/webster/webster-body-implementer.md`.
  The body is shared: `joinTemplateAssets` concatenates `webster-prefix-fork` + `webster-body-implementer` for the fork prompt and `webster-prefix-recovery` + `webster-body-implementer` for the recovery prompt (`internal/websterengine/render.go:80-104`), so a marker in the body would appear once in each composed template while each prefix already carries its own role-specific framing.
  Keeping it in the prefixes means each of the two composed templates carries the marker exactly once and each file's banner can describe its own marker set honestly.

  Update each file's opening HTML banner comment to name `friction_directive` as an optional marker filled via `stencil.FillOptional`, rendering as nothing when Tier 2 is off, in the same wording `webster-template-master.md:5` and `webster-prefix-recovery.md:5` already use for `pattern_directive`.
  `webster-prefix-recovery.md`'s banner currently states that `{{.pattern_directive}}` is its ONLY marker — correct that statement rather than leaving it contradicted.
  Keep every banner's existing no-conditionals statement, and do not introduce a `{{if}}` or `{{range}}`.
- **Commit:** `feat(stencils): add the friction_directive marker to webster's four prompt assets`

### Card 20: told `FrictionDir` on the three Deps structs

- **Context:**
  - `internal/friction/friction.go`
  - `internal/websterengine/geometry.go`
  - `internal/websterengine/doc.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/beginbatch.go`
  - `internal/websterengine/recoverbatch.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a `FrictionDir string` field to `websterengine.RunDeps` (`internal/websterengine/runlevel.go:107`), `websterengine.BeginDeps` (`internal/websterengine/beginbatch.go:74`), and `websterengine.RecoverDeps` (`internal/websterengine/recoverbatch.go:52`).
  Each carries the same doc comment: the absolute friction directory, told by the caller, empty when Tier 2 is off;
  it lives here rather than on `Geometry` because `internal/hubgeom`/`internal/standalonegeom` are the only `Geometry`-struct constructors and this value needs no geometry derivation.

  This card adds the three fields and **nothing else** — no call site reads them yet, and the four `render.go` composers still take their current parameter lists.
  A struct field no code reads compiles cleanly in Go, so the tree builds on this card's own commit.
  Card 21 is what widens the four composers and updates the four call sites, and it must do both together in one commit: a Go call site passing an argument the function does not accept does not compile, so the signature change and its call sites can never be split across two cards.
  That is the same reason batch 5's card 17 lands `burlerengine.New`'s fifth parameter and all three of its call sites in one card.
- **Commit:** `feat(websterengine): add the told FrictionDir field to webster's three Deps structs`

### Card 21: widen the four `render.go` composers, inject the directive, and update their call sites

- **Context:**
  - `internal/friction/friction.go`
  - `internal/pattern/pattern.go`
  - `internal/stencil/stencil.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/logger/logger.go`
- **Edits:**
  - `internal/websterengine/render.go`
  - `internal/websterengine/beginbatch.go`
  - `internal/websterengine/recoverbatch.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/render_test.go`
  - `internal/websterengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  The signature widening and every call site it breaks land in this one card, because a Go call site passing an argument the function does not accept does not compile and the plan-card contract requires the project to build immediately after each card's own commit.

  **Call sites.**
  Update the four composer call sites to compose their own site's note path as `friction.NotePath(deps.FrictionDir, <stem>)` — reading the field card 20 added — and pass it as the new trailing argument:

  - `internal/websterengine/beginbatch.go:311` (`RenderForkPrompt`) uses the stem `batchName`, the `"%02d-%s"` batch identity the surrounding code already computes.
  - `internal/websterengine/recoverbatch.go:154` (`RenderRecoveryPrompt`) uses `batchName + "-recovery"`, so a recovery strand never shares a stem with that batch's own fork note.
  - `internal/websterengine/runlevel.go:548` (`RenderIntegrationPrompt`) uses the fixed literal `"webster-integration"` — a plan has exactly one integration fork, so a literal is unique by construction.
  - `internal/websterengine/runlevel.go:564` (`RenderMasterPrompt`) uses the fixed literal `"webster-master"` — one Master per run.

  The empty-directory case propagates through `friction.NotePath` as an empty path, so no call site carries a boolean and no call site creates the friction directory.

  **Composers.**
  Give each of the four composers a new trailing `notePath string` parameter and, inside each, resolve `friction.Directive(notePath, stencilsDir, <role>)` with the role from the discussion's own table: `friction.RoleImplementer` for `RenderForkPrompt`, `RenderRecoveryPrompt`, and `RenderIntegrationPrompt`, and `friction.RoleOrchestrator` for `RenderMasterPrompt`, which never edits code and only forks.

  A non-nil `friction.Directive` error is **swallowed, never returned**: log it at `Warn` via `internal/logger` naming the role and the stencil, and continue with an empty directive string, exactly as if Tier 2 were off.
  Do **not** wrap it in each composer's own `fmt.Errorf("webster: ...: %w", err)` house style — a returned error fails the prompt render and therefore the whole run, over optional bookkeeping.
  The existing `pattern.Directive` error handling in `RenderRecoveryPrompt` (`:184-186`) and `RenderMasterPrompt` (`:271-273`) is left exactly as it is;
  see the overview's "a composer swallows a `friction.Directive` error" Shared Decision for why the two adjacent calls are deliberately not symmetrical.
  `internal/websterengine` already uses `internal/logger` in `strand.go`, `integration.go`, and `runlevel.go`, but `render.go` itself does not import it today — add the import to that file.

  Each composer adds `friction.MarkerName` to its `values` map and calls `friction.WarnIfMarkerAbsent` after obtaining its template bytes and before filling, passing the stencil name whose file actually carries the marker: `"webster-prefix-fork"` for `RenderForkPrompt`, `"webster-prefix-recovery"` for `RenderRecoveryPrompt`, `"webster-template-integration"` for `RenderIntegrationPrompt`, and `"webster-template-master"` for `RenderMasterPrompt`.
  For the two joined templates the bytes passed to the helper are the **composed** bytes `composeForkTemplate` / `composeRecoveryTemplate` returned, since those are what will actually be filled.
  The helper's third argument is the composed directive, never the note path — see `friction.WarnIfMarkerAbsent`'s own parameter naming in batch 1 card 1.

  Fill calls:

  - `RenderForkPrompt` converts `stencil.Fill` at `internal/websterengine/render.go:162` to `stencil.FillOptional(template, values, []string{friction.MarkerName})`.
  - `RenderIntegrationPrompt` converts `stencil.Fill` at `internal/websterengine/render.go:225` the same way.
  - `RenderRecoveryPrompt` widens its existing optional-names slice at `:200` from `[]string{"pattern_directive"}` to `[]string{"pattern_directive", friction.MarkerName}`.
  - `RenderMasterPrompt` widens its slice at `:291` the same way.

  Update `render.go`'s file header comment and each changed composer's own doc comment to state the new parameter and that `friction_directive` is injected when Tier 2 is on.

  **Tests.**
  `RenderForkPrompt`, `RenderRecoveryPrompt`, `RenderIntegrationPrompt`, and `RenderMasterPrompt` are all exported functions whose signatures this card changes, and the existing tests call them directly, so those tests must be updated in this same card or the package does not compile.
  Extend `internal/websterengine/render_test.go` and `internal/websterengine/template_test.go` — the two files that exercise them — with, for **each of the four** composers:

  1. **Enabled.** A non-empty note path appears in the composed prompt verbatim.
  2. **Disabled.** An empty note path composes successfully, injects no directive text, and reads no friction stencil.
  3. **Marker-free template.** A seeded stencil carrying no `{{.friction_directive}}` literal composes successfully while Tier 2 is enabled — never an error.

  Extend `internal/websterengine/template_test.go`'s existing required-marker deletion sweeps for the master and recovery templates to exclude `friction_directive` alongside `pattern_directive`, and add the equivalent optional-marker handling for the fork and integration templates, which had no optional marker before this card.

  Every added test stays untagged Tier 1: no `exec.Command`, no `gitexec`, no real spawn, no `time.Sleep` at or above one second.
- **Commit:** `feat(websterengine): inject the friction directive into all four prompt composers`

## Batch Tests

`verify: go test ./internal/websterengine/... ./contracts/stencils/...` covers both halves.

`./internal/websterengine/...` runs the extended `render_test.go` and `template_test.go` (the enabled, disabled, and marker-free cases for each of the four composers, plus the widened optional-marker sweeps) alongside the package's existing `beginbatch_test.go`, `recoverbatch_test.go`, and `runlevel_test.go`, which are what catch the three Deps structs' new field or the four composers' new parameter breaking an existing construction or call.
That is deliberately the whole package rather than a narrower `-run` pattern: the four exported composers this batch re-signatures are called from three other files in the same package, so a narrower scope would miss exactly the regression this change can cause.

`./contracts/stencils/...` covers the four edited webster stencil files through `registry_test.go` and `rubric_test.go`.

The scope is per-batch, not whole-repo: this batch adds only defaulted struct fields and package-internal call updates, so `internal/webstercli` and `internal/loomcli` compile unchanged from it.
