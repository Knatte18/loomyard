# Batch: stencil-cli

```yaml
task: "Relocate producer prompt files into a stencils/ directory"
batch: "stencil-cli"
number: 8
cards: 5
verify: go build ./... && go test ./internal/stencilcli/... ./cmd/lyx/...
depends-on: [6, 7]
```

## Batch Scope

The mechanism is unoperatable without a CLI: `list` makes the stencil set discoverable, `validate` catches an unfillable edit before it breaks a producer mid-run, `diff` is the entire migration story, and `promote` plus `diff --exit-code` are what make the port-back mechanical rather than remembered.
This batch adds `internal/stencilcli` with all five subcommands and registers it as the twelfth seam module.

Behaviour outside loomyard is decided and asymmetric, and the asymmetry is deliberate.
`promote` and `diff --all` are defined against a `stencils/` source tree that exists only in this repo, while the module is registered globally — in a consumer repo, or for a board copy whose name matches no source file, both exit with an error naming the missing tree or file.
Neither ever creates a `stencils/` directory and neither silently no-ops: a stray source tree in a consumer repo would be read by nothing, and a silent success would misreport the guard as having run.
`list`, `validate`, `sync`, and `diff <name>` work everywhere, since they need only the board copy.
The run-time drift warning is the one member of the trio that stays quiet in a consumer repo rather than erroring, because it is unsolicited and would otherwise fire on every run forever — that behaviour already shipped in batch 3 and is not re-implemented here.

The two `diff` modes have different base texts and conflating them would make the port-back guard unusable.
`diff <name>` compares the default this file was forked from — recovered by stamp hash from board history — against the currently shipped default, showing upstream changes the operator has not yet taken.
`diff --all --exit-code` compares the worktree's own `stencils/<family>/<name>.md` source body against the live board copy's body, catching an edit made in the board copy that was never ported back.
Comparing the port-back guard against the shipped default instead would leave the warning firing right through the fix, because the shipped default stays the old embedded one until the next deploy.

Named deviation from the CLI/Cobra Invariant's package-naming rule, recorded in CONSTRAINTS.md by batch 9: `stencilcli`'s kernel is `internal/stencilstore`, not `stencilengine`.

## Cards

### Card 32: Create the `lyx stencil` cobra module with `list`, `validate`, and `sync`

- **Context:**
  - `internal/idecli/cli.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/stencilstore/validate.go`
  - `internal/stencilstore/stencilstore.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/stencilcommit.go`
  - `internal/fabricengine/mutation.go`
  - `stencils/stencils.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `cmd/lyx/stencilseed.go`
- **Edits:** none
- **Creates:**
  - `internal/stencilcli/cli.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  New package `stencilcli` modelled structurally on `internal/idecli/cli.go`, which is the smallest seam module carrying both `RunCLIIn` and geometry resolution.

  Declare the seam exactly as the CLI/Cobra Invariant requires:
  - `func Command() *cobra.Command` returning the parent `stencil` command with `Use: "stencil"`, a non-empty `Short`, a `Long` carrying concrete examples, and `RunE: clihelp.GroupRunE`.
  - `func RunCLI(out io.Writer, args []string) int { return RunCLIIn("", out, args) }`
  - `func RunCLIIn(cwd string, out io.Writer, args []string) int` branching on `cwd == ""` to `clihelp.Execute` and otherwise to `clihelp.ExecuteIn`, with `idecli`'s doc comment explaining why the branch exists (`lyxcwd.WithCwd` panics on an empty directory).
  `stencil` carries `RunCLIIn` because it reads geometry.

  Resolve geometry once in the parent's `PersistentPreRunE`, guarded by `if cmd.Name() == "stencil" { return nil }` so the bare group listing never requires a git repo, using `lyxcwd.CwdFrom(ctx)` then `lyxcwd.Resolve(cwd)`, emitting `output.Err` and calling `clihelp.Abort(ctx, 1)` on either failure — the exact shape `idecli` uses.
  Store the resolved `*lyxcwd.Location` in a closure variable and derive `fabricengine.StencilsDir(l.HubPath)` from it where needed.

  Every subcommand carries a non-empty `Short`, returns its result through the `internal/output` envelope via `clihelp.SetExit(cmd.Context(), output.Ok(...))` or `output.Err(...)`, opens with an `if clihelp.ShouldAbort(cmd.Context()) { return nil }` guard, and returns `nil` rather than a bare error — no plain-text error paths.

  Three subcommands in this card:

  - `list` — prints every registry stencil with its name, its board-copy path, and its state (`absent`, `untouched`, `edited`) derived from `stencilstore.Classify`. Emits the list under a fixed key in the `output.Ok` fields map. Works in any repo.
  - `validate` — calls `stencilstore.Validate(stencilsDir, stencils.Registry())` and emits the findings. Exits non-zero when any finding carries `SeverityError` (a marker present in the body but absent from the shipped default, which will break `stencil.Fill` at the point of use) and zero when the findings are warnings only (a default marker deleted from the body, which fills cleanly but silently drops that content). Works in any repo.
  - `sync` — calls `stencilstore.ForceRefresh(stencilsDir, stencils.Registry(), sourceDir)`, then hands the returned written-paths slice to `fabricengine.CommitSeededStencils`, and emits the verb's `MutationRecord` in the envelope. `sync` uses `ForceRefresh` rather than `Reconcile`, so an explicit `sync` performs the refresh row even from a `-dev`-stamped build: the dev skip exists to stop incidental thrash, not to refuse an explicit request, and the dev binary is the one used in the prescribed test-live loop. Works in any repo.

  `sync` is a mutating verb outcome, so its envelope carries the fixed `mutations` array and `partial` bool per the Mutation Record Invariant; a pre-flight failure (cwd or location resolution) emits a bare `output.Err` with neither key, per that invariant's pre-flight carve-out.
  Compute `sourceDir` the same way `cmd/lyx/stencilseed.go` does: `filepath.Join(l.WorktreePath(), "stencils")` when it exists, the empty string otherwise.
- **Commit:** `feat(stencilcli): add lyx stencil with list, validate, and sync`

### Card 33: Add `diff` with `--all` and `--exit-code`, and `promote`

- **Context:**
  - `internal/stencilcli/cli.go`
  - `internal/fabricengine/stencilhistory.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/stencilstore/stencilstore.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/stencil/stencil.go`
  - `internal/output/output.go`
  - `internal/clihelp/exec.go`
  - `stencils/stencils.go`
- **Edits:** none
- **Creates:**
  - `internal/stencilcli/diff.go`
  - `internal/stencilcli/promote.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `diff.go` declares the `diff` subcommand with a `--all` bool flag and an `--exit-code` bool flag, and a unified-diff renderer over two `[]byte` bodies.

  Mode `diff <name>` (no `--all`): read the board copy, take its stamp via `stencilstore.ParseStamp`, recover the forked-from default via `fabricengine.StencilBaseByStamp(l.HubPath, name, stamp)`, and render base-versus-currently-shipped-default (the registry's `Default(name)`).
  When `found` is false, say so explicitly in the envelope and fall back to showing the shipped default against the on-disk body.
  It must never silently report an empty diff, which would read as "no upstream changes" when the truth is "base not found".
  When the file carries no parseable stamp, report that as the reason rather than treating it as a match.

  Mode `diff --all`: compare the worktree's own `stencils/<family>/<name>.md` source body against the live board copy's body, for every registry stencil, after `stencilstore.NormalizeLF` and `stencil.StripLeadingComment` on both sides.
  This is the port-back guard, and its base is the **source tree**, never the shipped default — comparing against the shipped default would leave the warning firing right through the fix, because `promote` does not change the embedded default until the next deploy.
  With no `stencils/` source tree present, exit with an error naming the missing tree; never create it and never silently no-op.
  `--exit-code` gives git-diff semantics: exit non-zero when any compared pair differs, zero when they agree.
  `--exit-code` is accepted with `--all` and with a single name alike.

  `promote.go` declares `promote <name>`: copy the live board copy into the current worktree's `stencils/<family>/` tree, stripping the stamp on the way in, because the source tree is the seed and carries no stamp.
  Strip it by removing only the `lyx-stencil:` line from the file's leading banner, leaving the rest of the banner and the entire body byte-identical — a `stencil.StripLeadingComment` of the whole banner would discard the human-facing documentation the source file is supposed to keep.
  With no `stencils/` source tree, or with a board-copy name matching no source file, exit with an error naming what is missing.
  `promote` writes only into the worktree source tree; it never writes to the board copy, never commits, and never pushes.
- **Commit:** `feat(stencilcli): add diff with --all/--exit-code and promote`

### Card 34: Register `stencil` as the twelfth seam module

- **Context:**
  - `internal/stencilcli/cli.go`
  - `cmd/lyx/registration_test.go`
  - `cmd/lyx/longlist_test.go`
  - `cmd/lyx/drift_test.go`
- **Edits:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/helptree_test.go`
  - `cmd/lyx/seamsignature_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `cmd/lyx/main.go`: add the `github.com/Knatte18/loomyard/internal/stencilcli` import, add `stencilcli.Command()` to the `root.AddCommand(...)` call in `newRoot()`, and append `stencil` to the root `Long`'s `Available modules:` sentence.
  An unregistered module is invisible to `--help`, and `longlist_test.go` derives its expectation from the live tree, so it fails automatically until the `Long` string is updated — the fix belongs in `main.go`, not in that test.

  In `helptree_test.go`, add `"stencil"` to the `requiredModules` slice and add a table entry for the module with `wantSubs` listing `list`, `validate`, `diff`, `sync`, and `promote`.
  Both are hardcoded lists that do not derive from `newRoot()`.

  In `seamsignature_test.go`, add `stencilcli.RunCLI` to the `RunCLI` compile-time pin slice and `stencilcli.RunCLIIn` to the `RunCLIIn` pin slice.
  Both counts rise by one: the `RunCLI` set goes from eleven to twelve and the `RunCLIIn` set from ten to eleven.

  `registration_test.go` and `drift_test.go` need no edit — the first derives registered modules from `main.go`'s AST and the second walks the live command tree for empty `Short` fields, so both are satisfied by the code changes above.
  Do not add `stencil` to `registration_test.go`'s `allowlist` map: that map is for a discovered `Command()` deliberately left unregistered, which is the opposite of this change.
- **Commit:** `feat(cmd/lyx): register the stencil module`

### Card 35: Add the sandbox scenario covering `stencil`

- **Context:**
  - `cmd/lyx/sandbox_coverage_test.go`
  - `internal/stencilcli/cli.go`
- **Edits:**
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add one scenario to `SANDBOX-CORE-SUITE.md` modelled on the existing `S3 -- Board and task interaction` shape, carrying a `**Covers:** stencil` line so `TestSandboxCoverage_AllModulesCoveredOrExcluded` finds it — that test derives registered modules from `newRoot().Commands()` and parses `**Covers:**` lines from `tools/sandbox/*SUITE.md`, so a registered module in neither set fails it.
  Follow the file's existing section conventions: a `### S<N> -- <title>` heading continuing the file's numbering, a `**Goal:**` line, the `**Covers:**` line, a `**Watch:**` line, a `**Verdict:** \`OK\` / \`WARN\` / \`FAIL\`` line, and a trailing `---`.
  Exercise the read-only half only: `lyx stencil list` naming all fifteen stencils and `lyx stencil validate` reporting a clean tree, both with sane JSON output.
  Do not script `promote` or `sync` in the scenario — both mutate the operator's tree, and the coverage requirement is satisfied at module granularity.
  Add a durability note stating the board's stencils tree is seeded on first run and persists across sessions, mirroring `S3`'s own durability note.

  Resolve the Sandbox Suite Coverage requirement with this scenario rather than an `excludedModules` row: `list` and `validate` are read-only and trivially black-box exercisable, so none of the three existing exclusion reasons — interactive stdin, real GitHub writes, an external binary on `$PATH` — applies here.
  Do not edit `cmd/lyx/sandbox_coverage_test.go`.
- **Commit:** `docs(sandbox): add a stencil coverage scenario to the core suite`

### Card 36: Test the CLI surface

- **Context:**
  - `internal/stencilcli/cli.go`
  - `internal/stencilcli/diff.go`
  - `internal/stencilcli/promote.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/stencilstore/stencilstore.go`
  - `internal/fabricengine/stencilhistory.go`
  - `internal/gitkit/hermetic.go`
  - `internal/hubforge/hub.go`
  - `internal/idecli/testmain_test.go`
  - `cmd/lyx/hermeticenv_test.go`
  - `stencils/stencils.go`
- **Edits:** none
- **Creates:**
  - `internal/stencilcli/testmain_test.go`
  - `internal/stencilcli/cli_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `testmain_test.go` declares a `TestMain` calling `gitkit.HermeticGitEnv()` before `m.Run()`, modelled on `internal/idecli/testmain_test.go`.
  The Hermetic Git Test Environment Invariant requires it of every package whose tests spawn git directly or through a `gitkit`/`hubforge` fixture helper, and `cmd/lyx/hermeticenv_test.go` fails without it.

  `cli_integration_test.go` carries a `//go:build integration` constraint as its first non-empty line — it builds a real hub through `hubforge` and spawns git, so it is Tier 2.
  Drive the module through `RunCLIIn` against that hub and assert:
  - `list` names all fifteen stencils and reports each one's state
  - `validate` reports a deliberately broken stencil: a board copy edited to add a top-level marker unknown to its shipped default is reported as an error naming the offending marker and exits non-zero, while one that deletes a default marker is reported as a warning and exits zero
  - `diff <name>` produces non-empty output against a seeded-then-changed default, and reports an unrecoverable base explicitly rather than rendering an empty diff
  - `sync` is idempotent: a second consecutive run writes nothing, creates no commit, and returns an empty `mutations` array
  - **the promote round trip in full**, which is what closes the loop: edit the board copy, run `promote`, assert the source tree received the edit with the stamp line stripped, then re-seed from the new default and assert the board copy ends up restamped and back in the untouched state via the reconciliation row. A test that stops at `promote` would pass while leaving the file permanently classified edited.
  - `diff --all --exit-code` exits non-zero when a board copy differs from the worktree source and **zero immediately after a `promote`** — both directions, because a drift check that never fires is a guard that silently passes forever
  - `promote` and `diff --all` each error, rather than no-op or create a directory, when no `stencils/` source tree is present
  - the drift warning is emitted via `logger.Warn` and never affects an exit code
- **Commit:** `test(stencilcli): cover list, validate, diff, sync, and the promote round trip`

## Batch Tests

`verify: go build ./... && go test ./internal/stencilcli/... ./cmd/lyx/...`

`cmd/lyx` is the substantive gate here, not a formality: `helptree_test.go`, `registration_test.go`, `longlist_test.go`, `drift_test.go`, `seamsignature_test.go`, `sandbox_coverage_test.go`, and `hermeticenv_test.go` all react to a new seam module, and five of them fail on a partial registration.
`internal/stencilcli`'s untagged run compiles the new package and exercises whatever unit-level assertions it carries; its integration-tagged suite runs under `pipeline.done_gate`'s `go test -tags integration ./...`.
`go build ./...` guards the new `cmd/lyx` import against the rest of the module.
