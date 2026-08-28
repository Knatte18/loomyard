# Batch: header-declines-the-stencil-seed-pass

```yaml
task: "reed: header pane's boot sometimes leaves shell/log noise in its scrollback"
batch: "header-declines-the-stencil-seed-pass"
number: 2
cards: 6
verify: go test ./cmd/lyx/ ./internal/clihelp/ ./internal/reedcli/ && go test -tags smoke -run TestSmokeHeaderDeclinesStencilSeedPass ./internal/reedcli/
depends-on: []
```

## Batch Scope

This batch removes noise class 3 — the `stencilstore` WARN lines that reach the header pane on the keepalive's own stderr — by giving a command a way to decline `cmd/lyx`'s root `PersistentPreRunE` stencil-seed pass, and annotating `reed header` with it.
It closes both Warn emitters at once, because the header declines the whole pass rather than any particular warning: the dev-refusal warn (`ModeDev` only) and the port-back drift warn (either mode, whenever the process runs inside a worktree carrying `contracts/stencils`) are both suppressed by construction.
It also stops a long-lived display-pane process from ever reaching `fabricengine.CommitSeededStencils` and performing a git commit in the hub — a distinct exposure with its own scenarios, never the same event as the dev-refusal warn, and a second independent reason for the opt-out.

The external interface this batch creates and batch 3 does not consume: `clihelp.SkipStencilSeedAnnotation` / `clihelp.AnnotationEnabled`, the declarative key any future quiet, long-lived command can carry without editing `cmd/lyx` again.

Card order is load-bearing.
Cards 6 and 7 land the smoke helpers and P2 while the gate does not yet exist, so P2 is observed **red on unmodified `main`** exactly as the discussion requires;
cards 8 through 10 then land the gate and P2 goes green.
Card 10 carries the red excerpt in its commit body.

Batch-local decisions beyond the overview's Shared Decisions:

- The annotation is a key/value pair (`"true"`), not bare key presence, so a future `"false"` cannot silently read as an opt-out.
- Card 6 lands **both** new smoke helpers, including `capturePaneScrollback`, which only batch 3 uses.
  Keeping every `internal/reedcli/smoke_test.go` edit in one batch avoids two batches writing the same shared harness file;
  an unused Go test helper is legal and `go vet` does not flag one.
- The `cmd/lyx` gate tests assert the predicate and the annotation's presence on `reed header`, and nothing else.
  There is deliberately **no** ordering assertion that the gate is checked before `stencilSeedTarget`: `seedStencils` returns under `testing.Testing()` before either step runs, so an in-process test cannot observe their relative order, and the one observable form ("no `git rev-parse` was spawned") is the unfalsifiable shape the discussion rejected.
  Accidental spawning stays covered by the package's existing Test Tier Purity guard.

## Cards

### Card 6: add the dev-stamped build and scrollback-capture smoke helpers

- **Context:**
  - `internal/reedcli/smoke_lifecycle_test.go`
- **Edits:**
  - `internal/reedcli/smoke_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `buildLyxBinaryWithLDFlags(t *testing.T, ldflags string) string` to `internal/reedcli/smoke_test.go`, holding the body `buildLyxBinary` has today plus a `-ldflags <ldflags>` pair appended to the `go build` argv when `ldflags` is non-empty, and reduce `buildLyxBinary` to `return buildLyxBinaryWithLDFlags(t, "")` so its behaviour for every current caller is byte-identical.
  Document on the new helper that the dev channel stamp `-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev` is what makes `stencilstore.ModeFor(buildinfo.IsDev())` return `ModeDev`, since `buildinfo.Channel` is `""` for a plain `go build` and an unstamped binary is therefore production mode and never emits the dev-refusal warn.
  Add `capturePaneScrollback(t *testing.T, tmuxPath, socket, target string) string`, running `capture-pane -p -S - -t <target>` and returning its stdout, failing the test on error.
  Document on it that it is deliberately a new helper rather than an edit to `capturePane`: `capturePane` passes no `-S` and captures the visible viewport only, which is what its existing callers assert against, whereas the header-noise assertions need the full scrollback that `-S -` reaches.
  Change no other helper in the file.
- **Commit:** `test(reedcli): add dev-stamped build and scrollback capture smoke helpers`

### Card 7: pin that the header command's stderr is silent

- **Context:**
  - `internal/reedcli/smoke_test.go`
  - `internal/reedcli/smoke_lifecycle_test.go`
  - `cmd/lyx/stencilseed.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/stencilstore/stencilstore.go`
  - `internal/fabricengine/junctionnames.go`
  - `contracts/stencils/stencils.go`
  - `internal/hubforge/hub.go`
- **Edits:** none
- **Creates:**
  - `internal/reedcli/smoke_headerseed_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create a `//go:build smoke` file holding `TestSmokeHeaderDeclinesStencilSeedPass`, the P2 pin.
  Build a dev-stamped binary via `buildLyxBinaryWithLDFlags(t, "-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev")`.
  Forge a hub with `hubforge.NewHub` and release it with `deferHubRelease` exactly as `TestSmokeHeaderPaneDisplaysRenderedHeaderText` does — never hand-assemble a hub, per the hubforge Fabric-Fixture Invariant.
  Arrange **both** Warn emitters.
  For the dev-refusal warn, pick the first name from `stencils.Registry().Names()`, take its shipped default via `Default`, derive a deliberately different body by appending a line to it, and write that body to `stencilstore.Path(fabricengine.StencilsDir(hub.Path), name)` through `stencilstore.ApplyStamp(body, stencilstore.BodyHash(body))`, creating parent directories first — a stamp matching its own body plus a body differing from the shipped default is exactly `StateUntouched` with drift, which `reconcileOne` warns about and refuses to refresh under `ModeDev`.
  For the port-back drift warn, materialise `contracts/stencils/<stencilstore.RelPath(name)>` inside `hub.PrimeWorktree()` with a body differing from the board copy just planted;
  a `hubforge` fixture worktree carries no `contracts/` directory of its own, so `seedStencilsAt` sets `sourceDir` empty and `warnPortBackDrift` cannot fire without this step.
  Then run the dev-stamped binary as a real subprocess, `lyx reed header` with no `--blocking`, with its working directory set to `hub.PrimeWorktree()`, capturing **stderr separately from stdout**, and assert stderr is empty.
  Assert emptiness only — never a line count and never a particular message;
  either emitter alone is enough to make it non-empty pre-fix, and post-fix it is silent because the pass does not run at all.
  On failure, name the captured stderr in the failure message so a future regression is diagnosable from the output alone.
  Give the file a header comment stating that this test observes noise class 3's suppression directly, that no tmux, pane, or escape sequence is anywhere in the picture, and that it is therefore structurally incapable of being masked by the `ED 3` backstop batch 3 adds.
  Run this test now, before card 9, and keep the failure output — it is P2's required red-on-unmodified-`main` evidence, which card 10's commit body carries.
- **Commit:** `test(reedcli): pin the header command's stderr silence`

### Card 8: declare the stencil-seed skip annotation key

- **Context:**
  - `internal/clihelp/exec.go`
- **Edits:** none
- **Creates:**
  - `internal/clihelp/annotations.go`
  - `internal/clihelp/annotations_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/clihelp/annotations.go` declaring two exported constants: `SkipStencilSeedAnnotation = "lyx.skip-stencil-seed"`, the cobra-annotation key a command carries to decline the root pre-run's stencil-seed pass, and `AnnotationEnabled = "true"`, the one value that reads as "on".
  Document on the key that declining is all-or-nothing per command, that it is for commands that read no stencils and are expected to stay silent, and that a value other than `AnnotationEnabled` never opts out — so a `"false"` cannot silently read as an opt-out.
  Document on the file that `internal/clihelp` is where this belongs because it already owns the CLI-wide seams both `cmd/lyx` and every `*cli` package import, so the constant creates no new dependency edge in either direction.
  Create `internal/clihelp/annotations_test.go` asserting both constants' exact literal values, so a rename cannot silently decouple the producer from the consumer.
- **Commit:** `feat(clihelp): declare the stencil-seed skip annotation key`

### Card 9: let a command decline the stencil-seed pre-run pass

- **Context:**
  - `internal/clihelp/annotations.go`
  - `internal/stencilstore/reconcile.go`
- **Edits:**
  - `cmd/lyx/stencilseed.go`
  - `cmd/lyx/main.go`
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `cmd/lyx/stencilseed.go`, add `skipStencilSeed(cmd *cobra.Command) bool`, returning true when `cmd` is non-nil and its `Annotations` map holds `clihelp.SkipStencilSeedAnnotation` with the value `clihelp.AnnotationEnabled`.
  Extract it as its own directly-assertable function rather than inlining it, for the reason `stencilSeedTarget`'s own comment already records: `seedStencils` returns immediately under `testing.Testing()`, so a test can never observe the gate through it.
  Change `seedStencils(ctx context.Context)` to `seedStencils(cmd *cobra.Command)`, deriving the context it needs from `cmd.Context()` at the `stencilSeedTarget` call.
  The signature change is not optional: today the function never sees the `*cobra.Command` and cannot read its annotations.
  Place the `skipStencilSeed(cmd)` early return **before** the `stencilSeedTarget` call, so an opted-out command resolves no geometry and spawns no `git rev-parse`;
  keep the existing `testing.Testing()` early return where it is, ahead of both.
  Document on the new early return why declining is safe: the annotated command reads no stencils, so the pass is pure waste for it, and skipping also keeps a long-lived pane process from reaching `fabricengine.CommitSeededStencils` and committing in the hub.
  Update the call site in `cmd/lyx/main.go`'s root `PersistentPreRunE` to `seedStencils(cmd)`, keeping the existing comment that seeding must never block a command from running.
  In `CONSTRAINTS.md`, amend the Stencil Ownership Invariant's third bullet, preserving its existing sentence verbatim and appending one: the bullet becomes "Seed/refresh runs once per process pre-run, never lazily inside `Read`. A command that reads no stencils may decline the pass entirely by carrying the skip annotation; declining is all-or-nothing per command and never defers seeding to a later or lazier point." — written as two lines under the repo's semantic-line-break markdown convention, one sentence per line.
  Change no other bullet of that invariant and no other invariant.
- **Commit:** `feat(lyx): let a command decline the stencil-seed pre-run pass`

### Card 10: opt reed header out of the stencil-seed pass

- **Context:**
  - `internal/clihelp/annotations.go`
  - `cmd/lyx/stencilseed.go`
- **Edits:**
  - `internal/reedcli/header.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `headerCmd`, add `Annotations: map[string]string{clihelp.SkipStencilSeedAnnotation: clihelp.AnnotationEnabled}` to the `&cobra.Command{...}` literal.
  Leave `Use`, `Short`, `Long`, the `RunE` body, and the `--blocking` flag registration otherwise untouched — the CLI/Cobra Invariant's non-empty `Short`, JSON-envelope error rule, and `clihelp.ShouldAbort`-first rule all still hold, and `reed header --blocking` stays one of the named interactive-handoff exceptions.
  Update the file's header comment to record that both header modes decline the root pre-run's stencil-seed pass, that this is deliberate rather than a `--blocking`-only gate because a cobra annotation is per-command and neither mode reads a stencil, and that it is what keeps the keepalive's stderr — and therefore the header pane's scrollback — free of `stencilstore` warnings and the hub free of a preview command's git commits.
  Re-run card 7's `TestSmokeHeaderDeclinesStencilSeedPass` to confirm it is now green, and paste a condensed excerpt of the pre-fix failure output recorded in card 7 into this commit's message body, labelled as P2's red-on-unmodified-`main` observation.
- **Commit:** `fix(reedcli): opt reed header out of the stencil-seed pass`

### Card 11: pin the gate predicate and the annotation's registration

- **Context:**
  - `cmd/lyx/stencilseed.go`
  - `cmd/lyx/registration_test.go`
  - `cmd/lyx/stencilseed_integration_test.go`
  - `internal/clihelp/annotations.go`
  - `internal/reedcli/header.go`
  - `internal/reedcli/cli.go`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/stencilseedgate_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create an untagged test file in `package main` holding two tests.
  `TestSkipStencilSeed_HonoursTheAnnotation` drives `skipStencilSeed` directly against synthetic `*cobra.Command` values built in-test — one carrying `clihelp.SkipStencilSeedAnnotation` set to `clihelp.AnnotationEnabled` (expect true), one with no `Annotations` map at all (expect false), one with an unrelated annotation key (expect false), one carrying the key with the value `"false"` (expect false), and a nil command (expect false).
  `TestReedHeaderCarriesTheStencilSeedSkipAnnotation` walks `reedcli.Command()`'s subcommands for the one whose `Name()` is `"header"` and asserts it carries the key with the value `clihelp.AnnotationEnabled`, failing with a message naming the gate as worthless if the annotation is silently dropped in a later refactor.
  Building the command tree only constructs cobra values and runs no hook, so nothing spawns a process and the Test Tier Purity Invariant holds.
  Do not add an ordering assertion about `skipStencilSeed` running before `stencilSeedTarget`, and do not add a "no `git rev-parse` was spawned" assertion — neither is observable in-process and the batch scope records why.
  Give the file a header comment stating what the two tests pin and, explicitly, what is deliberately not pinned here and why.
- **Commit:** `test(lyx): pin the stencil-seed skip gate and its registration`

## Batch Tests

`verify:` runs two commands.
The untagged half, `go test ./cmd/lyx/ ./internal/clihelp/ ./internal/reedcli/`, is scoped to exactly the three packages this batch edits.
`./cmd/lyx/` is run whole rather than by file because card 9 changes `seedStencils`' signature and the root `PersistentPreRunE` call site, both reached by the package's existing guards — `helptree_test.go`, `registration_test.go`, `tierpurity_test.go`, `seamsignature_test.go`, `main_test.go` — every one of which must pass unchanged.
The tagged half, `go test -tags smoke -run TestSmokeHeaderDeclinesStencilSeedPass ./internal/reedcli/`, runs this batch's own smoke assertion and nothing else;
the unfiltered `-tags smoke` package additionally drives real `claude` sessions and transcript persistence and is far too slow for a per-round verify loop.

New coverage this batch adds:

- `internal/reedcli/smoke_headerseed_test.go` — `TestSmokeHeaderDeclinesStencilSeedPass`, the P2 pin: a dev-stamped real binary, a hubforge hub with both a stale-but-untouched board stencil and a drifted `contracts/stencils` worktree copy, `lyx reed header` run as a subprocess, stderr asserted empty.
  Red on unmodified `main`, green after card 10.
- `cmd/lyx/stencilseedgate_test.go` — the predicate's skip/proceed behaviour and `reed header`'s annotation registration, both untagged and both hermetic.
- `internal/clihelp/annotations_test.go` — the two constants' literal values.

Existing coverage that must pass unchanged: `cmd/lyx/helptree_test.go` (adding an annotation must not disturb help output), `cmd/lyx/registration_test.go`, `cmd/lyx/tierpurity_test.go`, `cmd/lyx/stencilenvelope_integration_test.go` and `cmd/lyx/stencilseed_integration_test.go` (both integration-tagged and both untouched — `stencilSeedTarget` and `seedStencilsAt` keep their signatures), and `internal/reedcli/header_test.go`.

The composite end-to-end scrollback outcome is not asserted in this batch and is not its pin — see batch 3's B.
