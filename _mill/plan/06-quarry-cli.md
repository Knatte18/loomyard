# Batch: quarry-cli

```yaml
task: "Adopt quarry's glyph alphabet as the plan alphabet"
batch: "quarry-cli"
number: 6
cards: 5
verify: go test ./internal/quarrycli/ ./cmd/lyx/
depends-on: [5]
```

## Batch Scope

This batch gives the planner its only source of glyphs: a new `lyx quarry` command group with four read-only repository queries, registered as an ordinary cobra subtree, plus the three registration obligations a new module trips and the stencil rewrite that replaces the planner's old guess-it-yourself instructions.
It is one batch because the package, its registration, its help-tree and sandbox coverage, and the stencil section that sends the planner to it are one deliverable — shipping the verbs without the stencil change would leave the hard rule unsatisfiable, and shipping the stencil change without the verbs would point the planner at nothing.
The external interface batch 7 consumes is nothing new; batch 7's work is inside `planglyph` and `websterengine`.

Batch-local decisions, beyond `## Shared Decisions`:

- **Four verbs, and `delta` and `name` are deliberately absent.** `glyphs` and `resolve` are what the planner needs for copied-verbatim spelling; `toc` and `expand` are what the stencil's replaced section becomes. `delta` and `name` are pipeline-internal and never agent-facing — `name` in an agent's hands is a glyph-spelling machine, the one thing the hard rule exists to prevent.
- **The verbs never re-shape the answer.** They delegate to the facade queries with their frozen presets and emit quarry's own rendering. The stencil's "copy the line verbatim" instruction only works if the answer is quarry's answer.
- **No arbitrary-repo path flag.** If the planner could point the verbs at a different tree than the validator resolves against, it could copy a spelling that is verbatim from *some* quarry answer but not from *this* repository's — breaking copied-verbatim exactly as version drift would.

## Cards

### Card 26: create internal/quarrycli and its four verbs

- **Context:**
  - `internal/stencilcli/cli.go`
  - `internal/planglyph/doc.go`
  - `internal/preflight/predicates.go`
  - `internal/output/output.go`
  - `internal/clihelp/exec.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/planglyph/repo.go`
  - `internal/planglyph/repo_test.go`
- **Creates:**
  - `internal/quarrycli/cli.go`
  - `internal/quarrycli/toc.go`
  - `internal/quarrycli/glyphs.go`
  - `internal/quarrycli/resolve.go`
  - `internal/quarrycli/expand.go`
  - `internal/quarrycli/cli_test.go`
  - `internal/quarrycli/verbs_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create the module as an ordinary cobra subtree following the CLI/Cobra Invariant's seam, mirroring `internal/stencilcli/cli.go`'s shape.
  In `cli.go`, expose `Command() *cobra.Command` returning the `quarry` subtree, `RunCLI(out io.Writer, args []string) int`, and `RunCLIIn(cwd string, out io.Writer, args []string) int` delegating to `clihelp.Execute`/`clihelp.ExecuteIn` exactly as `stencilcli` does.
  `RunCLIIn` is required here, not optional: the group's root resolution is cwd-dependent.
  Resolve the repository root once in the parent's `PersistentPreRunE`, skipping it when `cmd.Name()` is `quarry` so a bare `lyx quarry` listing never requires a git repository, via `preflight.ResolveMode(cwd)` — the tier-1 probe the Told-Geometry Invariant names for a standalone CLI — and use the returned location's worktree path as the root.
  Add no repository-path flag to any verb.
  These are repository queries rather than plan queries and must answer before any plan exists, which is precisely when the planner needs them, so none of the four requires a plan to be in scope.
  Add the four subcommands, each with a non-empty `Short` and a `Long` carrying an example, each checking `clihelp.ShouldAbort` first, and each reporting errors as JSON via `internal/output`: `toc <path>`, `glyphs <dir>`, `resolve <glyph>...` accepting one or more targets in one call, and `expand <glyph>`.
  Route every quarry call through `internal/planglyph` rather than importing the facade here, so the package-ownership seam stays intact: add four exported query wrappers — `TOC`, `Glyphs`, `Resolve` and `Expand`, each taking `worktreeRoot string` plus that verb's own argument — beside `openRepo` in `internal/planglyph/repo.go`, each opening the repository, delegating to the matching `quarry.Repo` method, and returning quarry's own answer and error unchanged, then call those from these verbs.
  Cover the four wrappers in `internal/planglyph/repo_test.go` against the same fixture-repository pattern that file already uses.
  Emit quarry's own renderers verbatim — `quarry.RenderJSON`, `quarry.RenderGlyphsJSON`, `quarry.RenderResolveJSON`, `quarry.RenderExpandJSON` — and never re-shape, filter, sort or re-key an answer.
  Use `quarry.GlyphsOptions()`, the frozen preset, for `glyphs` rather than assembling a `TOCOptions` value locally.
  Cover in tests: each verb's `Short` being non-empty; `Command()`'s child set being exactly the four verbs; `RunCLIIn` against a fixture repository returning exit 0 and emitting parseable JSON for each verb; a bad glyph argument producing a JSON error envelope and a non-zero exit; and a golden assertion that `glyphs`' output is byte-identical to the facade's own rendering of the same answer.
- **Commit:** `26: feat(quarrycli): add the lyx quarry verb group over toc, glyphs, resolve and expand`

### Card 27: register the module under the cobra root

- **Context:**
  - `internal/quarrycli/cli.go`
  - `cmd/lyx/registration_test.go`
  - `cmd/lyx/longlist_test.go`
- **Edits:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/helptree_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Register the new subtree the way every other module is registered.
  In `cmd/lyx/main.go`, add the `internal/quarrycli` import and add `quarrycli.Command()` to the `root.AddCommand(...)` call, placed in the same alphabetical-by-module neighbourhood the existing entries sit in.
  In `cmd/lyx/helptree_test.go`, add a case to the module table with `module: "quarry"` and `wantSubs: []string{"toc", "glyphs", "resolve", "expand"}`, matching the shape of the `stencil` and `loom` cases already there.
  Run the package's own guards after the edit and fix what they report rather than adjusting them: `registration_test.go` enforces that an existing module is registered, `longlist_test.go` skips cobra's own `help` and `completion` subtrees the same way the sandbox guard does, and `jsonhelp_test.go` and `seamsignature_test.go` check the envelope and seam shapes every module must satisfy.
  Do not add an entry to `cmd/lyx/sandbox_coverage_test.go`'s `excludedModules` map to silence that guard — card 28 satisfies it with a real scenario, which is the correct discharge for four read-only, trivially exercisable queries.
- **Commit:** `27: feat(lyx): register the quarry module under the cobra root`

### Card 28: cover the module with a real sandbox scenario

- **Context:**
  - `cmd/lyx/sandbox_coverage_test.go`
  - `internal/quarrycli/cli.go`
- **Edits:**
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Discharge the Sandbox Suite Coverage invariant with a scenario rather than an allowlist entry.
  The invariant requires every registered module to be exercised by a `**Covers:**`-tagged scenario in one of the `tools/sandbox/*SUITE.md` suites, or to be named on `excludedModules` with a documented reason; an exclusion would be unjustified here, because the four verbs are read-only repository queries against the sandbox's own tree and are trivially exercisable.
  Add a scenario to `tools/sandbox/SANDBOX-CORE-SUITE.md` carrying a `**Covers:** quarry` line — matching the exact `^\*\*Covers:\*\*\s*(.+)$` shape `coversLinePattern` matches, so the token is discovered — and walking the four verbs against the sandbox repository: `lyx quarry toc` on a directory, `lyx quarry glyphs` on a package directory, `lyx quarry resolve` on a glyph copied from that `glyphs` output, and `lyx quarry expand` on the same glyph.
  Write the scenario in the same voice and structure as the suite's existing scenarios, and make the `resolve` step consume a glyph taken from the preceding `glyphs` step's own output rather than a hardcoded one, so the scenario exercises the copied-verbatim workflow the whole task exists to enable.
  Confirm the guard passes by running the `cmd/lyx` package's tests; a `**Covers:**` token naming a module that is not registered is also a failure, so the token must read exactly `quarry`.
- **Commit:** `28: test(sandbox): cover the quarry module with a core-suite scenario`

### Card 29: record the module's naming deviation and its module-table entry

- **Context:**
  - `internal/quarrycli/cli.go`
  - `internal/planglyph/doc.go`
  - `cmd/lyx/main.go`
- **Edits:**
  - `CONSTRAINTS.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Record both `CONSTRAINTS.md` obligations the new subtree creates, in two distinct places within the same section.
  The CLI/Cobra Invariant's package-naming rule is `<module>cli` imports `<module>engine`; `quarrycli` imports `internal/planglyph`, which deviates.
  The invariant already records one such deviation, `stencilcli` → `internal/stencilstore`, so add `quarrycli` → `internal/planglyph` to that same line rather than starting a second convention.
  Then recount the invariant's module-count line, which today reads that each module exposes `Command()` and `RunCLI` and "eleven of twelve also carry `RunCLIIn`": the subtree count rises by one and `quarrycli` does carry `RunCLIIn`, because its root resolution is cwd-dependent, so both halves of that sentence change.
  Derive the new numbers by counting the entries in `cmd/lyx/main.go`'s `root.AddCommand(...)` call rather than by arithmetic on the old sentence, and note that `loomcli.RunAliasCommand()` is a second registration of an existing subtree's verb rather than a thirteenth module.
  In `docs/overview.md`, add a `quarrycli` entry to the module list describing the four verbs, their read-only posture, and the deliberate absence of `delta` and `name`, and note that it imports `internal/planglyph` rather than a `quarryengine`.
- **Commit:** `29: docs(constraints): record the quarrycli naming deviation and recount the RunCLIIn line`

### Card 30: replace the planner's lookup instructions with the quarry verbs

- **Context:**
  - `internal/quarrycli/cli.go`
  - `contracts/specs/loom-plan-spec.md`
- **Edits:**
  - `contracts/stencils/loom/loom-template-plan.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace the stencil's `### No quarry inventory exists — do the lookups yourself` section, which today tells the planner that no quarry inventory is handed to it and points it at `go doc <pkg> <Symbol>` for a symbol's existence and a package-scoped recursive grep for its call sites.
  Those instructions make the planner's symbol names its own guesses, checked by nothing, which is exactly the state this task ends.
  Write the replacement section around the four verbs: `lyx quarry glyphs <dir>` as the flat index the planner reads spellings out of, `lyx quarry resolve <glyph>...` as the check that a spelling names something real, and `lyx quarry toc <path>` plus `lyx quarry expand <glyph>` as the structure and detail queries that replace the old lookups.
  State the hard rule in the section itself, in the terms the decision requires: the planner never spells a glyph — it copies a line verbatim out of a quarry answer, and a bare package-qualified symbol is a hard finding because it is the one spelling that cannot have come verbatim from one.
  State that `lyx quarry` answers against the current worktree only and takes no repository-path flag, so a copied spelling is always from the tree the validator resolves against.
  Do not mention `lyx quarry delta` or `lyx quarry name` — neither exists, and naming them would invite the planner to ask for them.
  Keep the surrounding sections' structure and voice, and keep the glyph spelling rules card 8 already wrote into this file rather than restating them here, per the Producer Pointer-Rule Invariant's spirit that an instruction file points rather than duplicates.
- **Commit:** `30: docs(stencil): replace the planner's do-the-lookups-yourself section with the lyx quarry verbs`

## Batch Tests

`verify: go test ./internal/quarrycli/ ./cmd/lyx/` covers the new package and the registration guards that police it.
`./internal/quarrycli/` runs `cli_test.go` and `verbs_test.go`: the seam assertions and the per-verb runs against a fixture repository built under `t.TempDir()`, which reach `quarry.TOC`, `Glyphs`, `Resolve` and `Expand` — all file readers, none of which spawns a process, so the tests stay untagged and tier1-pure.
`./cmd/lyx/` is where the batch's real gates live and is why it is in scope: `registration_test.go`, `longlist_test.go`, `helptree_test.go`, `jsonhelp_test.go`, `seamsignature_test.go` and `sandbox_coverage_test.go` each independently fail if the module is registered incorrectly, documented incorrectly, or left uncovered, and card 28's scenario is validated by that last one parsing `tools/sandbox/*SUITE.md` rather than by any assertion this batch writes itself.
The two-package scope excludes `internal/planglyph` even though card 26 may add query wrappers there: batch 4's verify covers that package's own behaviour, and any wrapper added here is exercised through the verbs in this batch's own run, with the overview's module-wide `go build ./...` catching a compile break at this batch's boundary.
</content>
