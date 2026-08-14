# Batch: webster-runtime-read

```yaml
task: "Relocate producer prompt files into a stencils/ directory"
batch: "webster-runtime-read"
number: 6
cards: 4
verify: go build ./... && go test ./stencils/... ./internal/websterengine/... ./internal/webstercli/... ./internal/lyxcwd/...
depends-on: [5]
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (package or module declaration, imports, identifier retargeting, seam splits).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

## Batch Scope

Webster's five prompt assets move to `stencils/webster/`, completing the fifteen, and `websterengine` switches to a call-time read.
Webster is the only family that **composes** rather than reading directly: `joinTemplateAssets` concatenates a prefix and a body with a blank line before `stencil.Fill` runs, because `internal/stencil` has no include mechanism.
`composeForkTemplate` joins `fork-prefix` ahead of `implementer-body`; `composeRecoveryTemplate` joins `recovery-prefix` ahead of the same body.
Three files therefore participate in two composed prompts, and all three must read through `stencilstore` for an edit to any of them to take effect.

That composition creates a hazard this task introduces and must fix in the same batch.
Today the composed result carries only the prefix's banner, which `Fill` strips, so nothing leaks — `implementer-body.md` is the one default of the fifteen that ships without a leading banner at all, opening straight on its `# Webster implementer job` heading.
Once seeding stamps that file with `<!-- lyx-stencil: sha256=… -->`, the joined pair carries two banners and `stencil.Fill` strips only the first, so the bookkeeping hash would be delivered into both the fork prompt and the recovery prompt as if it were instruction text.
The fix is to strip **every** asset's banner inside `joinTemplateAssets` using the `stencil.StripLeadingComment` export from batch 1, which also hardens any future banner-carrying asset rather than special-casing this one.

The five no-arg accessors cannot stay no-arg once reading can fail: each takes the stencils directory and gains an `error` return.

## Cards

### Card 25: Move webster's five assets into `stencils/webster/` and register them

- **Context:**
  - `internal/stencilstore/stencilstore.go`
- **Edits:**
  - `stencils/stencils.go`
  - `.gitattributes`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/websterengine/master-template.md` -> `stencils/webster/webster-template-master.md`
  - `internal/websterengine/integration-template.md` -> `stencils/webster/webster-template-integration.md`
  - `internal/websterengine/fork-prefix.md` -> `stencils/webster/webster-prefix-fork.md`
  - `internal/websterengine/recovery-prefix.md` -> `stencils/webster/webster-prefix-recovery.md`
  - `internal/websterengine/implementer-body.md` -> `stencils/webster/webster-body-implementer.md`
- **Requirements:**
  Add five `//go:embed` directives and their exported typed `[]byte` vars to `stencils/stencils.go`, following card 7's shape:
  `WebsterTemplateMaster`, `WebsterTemplateIntegration`, `WebsterPrefixFork`, `WebsterPrefixRecovery`, `WebsterBodyImplementer`.
  Add the five matching rows to the `entries` slice, keyed by the stencil names `webster-template-master`, `webster-template-integration`, `webster-prefix-fork`, `webster-prefix-recovery`, and `webster-body-implementer`, placed after treadle's four.
  With this card the registry holds all fifteen producer prompts.

  In `.gitattributes`, add five rows pinning the relocated files to LF:
  ```
  stencils/webster/webster-template-master.md text eol=lf
  stencils/webster/webster-template-integration.md text eol=lf
  stencils/webster/webster-prefix-fork.md text eol=lf
  stencils/webster/webster-prefix-recovery.md text eol=lf
  stencils/webster/webster-body-implementer.md text eol=lf
  ```
  Webster's five are unpinned today, so these rows close a pre-existing gap rather than relocating existing ones — there is no stale `internal/websterengine` row to remove.
- **Commit:** `feat(stencils): relocate webster prompt assets into stencils/webster`

### Card 26: Strip every asset's banner when composing, and read webster's assets at call time

- **Context:**
  - `internal/stencil/stencil.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `stencils/stencils.go`
- **Edits:**
  - `internal/websterengine/render.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Delete the five `//go:embed` directives and their package vars `masterTemplate`, `forkPrefix`, `recoveryPrefix`, `implementerBody`, and `integrationTemplate` from `render.go`; all five directives now live in `stencils/stencils.go`.
  Remove the now-unused `embed` import.

  Change `joinTemplateAssets` from

```go
// joinTemplateAssets concatenates prefix and body's raw template bytes
// separated by a blank line. stencil.stripLeadingComment removes the leading banner.
func joinTemplateAssets(prefix, body []byte) []byte {
```

  so that it applies `stencil.StripLeadingComment` to **each** asset before concatenating, rather than relying on `stencil.Fill` to strip only the joined result's first banner.
  Rewrite its doc comment to state the new rule and why it exists: `Fill` strips one leading banner from the joined text, so a banner on the second asset would reach the LLM verbatim, and every seeded stencil now carries a `lyx-stencil:` stamp in its banner.
  Keep the blank-line separator between the two stripped bodies.

  Change the five accessors to take the stencils directory and return an error, each delegating to `stencilstore.Read` and returning its error unwrapped:
  - `func MasterTemplate(stencilsDir string) ([]byte, error)` reading `webster-template-master`
  - `func IntegrationTemplate(stencilsDir string) ([]byte, error)` reading `webster-template-integration`
  - `func ImplementerBodyTemplate(stencilsDir string) ([]byte, error)` reading `webster-body-implementer`
  - `func ForkTemplate(stencilsDir string) ([]byte, error)` delegating to `composeForkTemplate(stencilsDir)`
  - `func RecoveryTemplate(stencilsDir string) ([]byte, error)` delegating to `composeRecoveryTemplate(stencilsDir)`

  Change `composeForkTemplate` and `composeRecoveryTemplate` to `func composeForkTemplate(stencilsDir string) ([]byte, error)` and `func composeRecoveryTemplate(stencilsDir string) ([]byte, error)`, each reading its own prefix (`webster-prefix-fork` / `webster-prefix-recovery`) and the shared `webster-body-implementer` through `stencilstore.Read` and joining them with `joinTemplateAssets`.

  Update the four `Render*Prompt` functions to supply the directory and propagate the new errors:
  - `RenderForkPrompt(batch batcher.Batch, prevDigest, reportPath string, l *lyxcwd.Location, selfFixCap int)` and `RenderRecoveryPrompt(...)` compute `fabricengine.StencilsDir(l.HubPath)` from the `*lyxcwd.Location` they already take — no signature change.
  - `RenderMasterPrompt(plan *planparser.Plan, st *State, outcomePath, summaryPath, integrationPromptPath string, selfFixCap, pollWaitS int, l *lyxcwd.Location)` does the same — no signature change. Keep its `stencil.FillOptional(..., []string{"pattern_directive"})` optional-marker list unchanged.
  - `RenderIntegrationPrompt(plan *planparser.Plan, reportPath, worktreeRoot string)` has no `*lyxcwd.Location`, so it gains a trailing `stencilsDir string` parameter.

  Add the `internal/fabricengine` import to `render.go` — it does not carry one today, and the three wrappers above now call `fabricengine.StencilsDir(l.HubPath)`.

  Update the file's header doc comment (`render.go:1-2,18-19`) and the `RenderMasterPrompt` doc comment at `render.go:193`, all of which name assets by their old filenames, to name the new ones and to state that the assets ship as embedded defaults in the top-level `stencils` package and are read from the hub's stencils directory at call time.
- **Commit:** `refactor(websterengine): read composed prompts at call time and strip every banner`

### Card 27: Update webster's callers for the new signatures

- **Context:**
  - `internal/websterengine/render.go`
  - `internal/websterengine/beginbatch.go`
  - `internal/websterengine/recoverbatch.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/websterengine/runlevel.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `RenderIntegrationPrompt` is the only wrapper whose signature changed, and it has exactly one production call site: `internal/websterengine/runlevel.go:469`.
  Pass `fabricengine.StencilsDir(deps.Layout.HubPath)` as its new trailing argument, using the `Layout *lyxcwd.Location` field that `RunDeps` already carries.
  Add the `internal/fabricengine` import to `runlevel.go` if it is not already present.

  The other three wrappers keep their signatures, so `beginbatch.go:198`, `recoverbatch.go:156`, and `runlevel.go:485` need no argument change — verify each still compiles and leave them otherwise untouched.
  Do not change any `Deps` struct: `BeginDeps`, `RecordDeps`, `RunDeps`, and `RecoverDeps` all already carry `Layout`, which is everything the resolution needs.
- **Commit:** `refactor(websterengine): supply the stencils directory to the integration prompt`

### Card 28: Repoint webster's template tests and add the banner-leak guard

- **Context:**
  - `internal/websterengine/render.go`
  - `stencils/stencils.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/stencil/stencil.go`
- **Edits:**
  - `internal/websterengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `template_test.go` is `package websterengine_test` and calls the five exported accessors at many sites.
  Add a package-local helper that returns a seeded stencils directory: create a `t.TempDir()`, write `webster/webster-template-master.md`, `webster/webster-template-integration.md`, `webster/webster-prefix-fork.md`, `webster/webster-prefix-recovery.md`, and `webster/webster-body-implementer.md` into it from the corresponding `stencils` package vars, and return the directory.
  Pass that directory to every `MasterTemplate`, `IntegrationTemplate`, `ImplementerBodyTemplate`, `ForkTemplate`, and `RecoveryTemplate` call, and handle each new `error` return with a `t.Fatalf` rather than ignoring it.
  `RenderIntegrationPrompt` also gained a trailing `stencilsDir` parameter in card 26, and this file calls it at two sites — `template_test.go:662` inside `TestRenderIntegrationPrompt_InjectsVerifyText` and `template_test.go:677` inside `TestRenderIntegrationPrompt_EmptyVerifyErrors`, both spelled `websterengine.RenderIntegrationPrompt(plan, "/reports/integration.yaml", "/worktree")`.
  Thread the seeded directory into both, or the batch's own `verify: go test ./internal/websterengine/...` fails to compile.
  The other three `Render*Prompt` wrappers keep their signatures, so their call sites in this file need no argument change.

  Keep every existing assertion's subject and text unchanged, including `TestTemplates_ForkAndRecoveryShareImplementerBody`, `TestMasterTemplate_QuotesDigestFieldsAndNoOthers`, and the rest.
  Update the file's header comment (`template_test.go:1-2`), which names the assets by their old filenames, and the comment at `template_test.go:89`.

  Add these new tests:
  - **Banner-leak guard**: seed the directory through `stencilstore.Reconcile` so every file carries a real stamp, then assert that `ForkTemplate` and `RecoveryTemplate` output contains no `lyx-stencil:` substring and no `<!--` at all. This is the regression guard for the stamp reaching a live prompt, and it is the assertion that fails if `joinTemplateAssets` ever stops stripping the second asset.
  - **Composed runtime read**: overwrite `webster/webster-prefix-fork.md` in the seeded directory with a modified body and assert `ForkTemplate` output changes; overwrite `webster/webster-body-implementer.md` and assert **both** `ForkTemplate` and `RecoveryTemplate` output change, since three files participate in two composed prompts.
  - **Missing board**: call `MasterTemplate` with a directory that does not exist and assert it returns an error naming the missing stencil, rather than falling back to the embedded default.
- **Commit:** `test(websterengine): pin composed runtime reads and the stamp-leak guard`

## Batch Tests

`verify: go build ./... && go test ./stencils/... ./internal/websterengine/... ./internal/webstercli/... ./internal/lyxcwd/...`

`internal/websterengine` carries the batch's real risk — the accessor signature change, the composed-read wiring, and the banner-leak guard — so its full package test run is the gate.
`internal/webstercli` is included because it constructs all four `Deps` types and must still compile against the unchanged wrapper signatures.
`./stencils/...` runs registry completeness, which now covers all fifteen stencils for the first time, and `internal/lyxcwd` runs the Fabric Vocabulary walk over the five newly relocated files.
`go build ./...` guards the five deleted `//go:embed` vars against any remaining read site.
