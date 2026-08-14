# Batch: burler-runtime-read

```yaml
task: "Relocate producer prompt files into a stencils/ directory"
batch: "burler-runtime-read"
number: 4
cards: 4
verify: go build ./... && go test ./stencils/... ./internal/burlerengine/... ./internal/burlercli/... ./internal/perchcli/... ./internal/lyxcwd/...
depends-on: [3]
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (package or module declaration, imports, identifier retargeting, seam splits).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

## Batch Scope

Burler's four round prompts move to `stencils/burler/` and `burlerengine` switches to a call-time `stencilstore.Read`.
The batch also carries the `.gitattributes` churn burler's move causes — four new `stencils/burler/**` rows and four now-stale `internal/burlerengine/*` rows — and rewrites the three instruction banners that cross-reference the orchestrator by its old filename.

The one behavioural subtlety is `internal/burlerengine/template_test.go`.
It enforces the Review Round Invariant (`TestTemplate_StatesRoundDiscipline`, `TestTemplate_StatesClusterForkDiscipline`, `TestTemplate_OrchestratorExcludesDownstreamBodies`) and its subject is the **shipped** prompt, which remains the right subject — so it keeps testing the embedded defaults rather than a disk copy.
But the file is `package burlerengine` and reads the four unexported vars declared in `template.go`, which this batch deletes, so those identifiers cease to exist in that package.
It therefore becomes a cross-package read of the new top-level `stencils` package's exported defaults.
That is a cross-package import change, not a rename of the variables.
The test file stays in `internal/burlerengine` so CONSTRAINTS.md's Review Round Invariant "Enforced by" pointer stays accurate.

Batch-local decision: `burlerengine.New` gains a fourth `stencilsDir string` parameter rather than deriving the directory from the `*lyxcwd.Location` it already carries, so the engine is told its geometry at both of its two construction sites.

## Cards

### Card 17: Move burler's four prompts into `stencils/burler/` and register them

- **Context:**
  - `internal/stencilstore/stencilstore.go`
  - `stencils/registry_test.go`
- **Edits:**
  - `stencils/stencils.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/burlerengine/round-orchestrator-template.md` -> `stencils/burler/burler-template-round-orchestrator.md`
  - `internal/burlerengine/instruction-1-explore-template.md` -> `stencils/burler/burler-step-1-explore.md`
  - `internal/burlerengine/instruction-2-review-template.md` -> `stencils/burler/burler-step-2-review.md`
  - `internal/burlerengine/instruction-3-fix-template.md` -> `stencils/burler/burler-step-3-fix.md`
- **Requirements:**
  Add four `//go:embed` directives and their exported typed `[]byte` vars to `stencils/stencils.go`, following the shape card 7 established:
  `BurlerTemplateRoundOrchestrator`, `BurlerStep1Explore`, `BurlerStep2Review`, `BurlerStep3Fix`.
  Add the four matching rows to the `entries` slice, keyed by the stencil names `burler-template-round-orchestrator`, `burler-step-1-explore`, `burler-step-2-review`, and `burler-step-3-fix`, placed after loom's two so `Registry().Names()` stays family-grouped in the order `lyx stencil list` prints.
  Do not restructure the `registryEntry` type, the `registry` methods, or `Registry()` — this card only adds rows.
  `stencils/registry_test.go` needs no edit: it derives its expected set by walking the family subfolders, so the four new files and four new rows must simply agree.
- **Commit:** `feat(stencils): relocate burler round prompts into stencils/burler`

### Card 18: Switch `burlerengine` to a call-time `stencilstore.Read`

- **Context:**
  - `internal/stencilstore/reconcile.go`
  - `internal/burlerengine/profile.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/fabricengine/junctionnames.go`
  - `stencils/stencils.go`
- **Edits:**
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/prompt.go`
  - `internal/burlercli/cli.go`
  - `internal/perchcli/cli.go`
- **Creates:** none
- **Deletes:**
  - `internal/burlerengine/template.go`
- **Moves:** none
- **Requirements:**
  Delete `internal/burlerengine/template.go` outright — it exists only to declare the four `//go:embed` directives and their package vars `roundOrchestratorTemplate`, `instruction1Template`, `instruction2Template`, and `instruction3Template`, all of which now live in `stencils/stencils.go`.

  In `engine.go`, add a `stencilsDir string` field to `type Engine struct` beside the existing `layout` field, and change

```go
func New(shuttle Shuttle, layout *lyxcwd.Location, cfg Config) *Engine {
```

  to take a trailing `stencilsDir string` parameter, assigning it to the new field.

  In `prompt.go`, change `composePrompt` from
  `func composePrompt(p *Profile, patternDirective, inst1Path, inst2Path, inst3Path string) (string, []instructionFile, error)`
  to take a leading `stencilsDir string` parameter, and replace each of its four template reads with a `stencilstore.Read(stencilsDir, <name>)` whose error is returned unwrapped, feeding the returned bytes into the existing `stencil.Fill`/`stencil.FillOptional` call that already sits at that line:
  - `prompt.go:44` `roundOrchestratorTemplate` becomes `stencilstore.Read(stencilsDir, "burler-template-round-orchestrator")`
  - `prompt.go:56` `instruction1Template` becomes `stencilstore.Read(stencilsDir, "burler-step-1-explore")`, keeping its `[]string{"pattern_directive"}` optional-marker list
  - `prompt.go:66` `instruction2Template` becomes `stencilstore.Read(stencilsDir, "burler-step-2-review")`
  - `prompt.go:76` `instruction3Template` becomes `stencilstore.Read(stencilsDir, "burler-step-3-fix")`

  Thread `e.stencilsDir` from the engine into every `composePrompt` call site inside `burlerengine`.

  Update both production construction sites to pass the resolved directory, computed from the `*lyxcwd.Location` each site already holds:
  - `internal/burlercli/cli.go:104` — `burlerengine.New(runner, layout, burlerCfg)` becomes `burlerengine.New(runner, layout, burlerCfg, fabricengine.StencilsDir(layout.HubPath))`
  - `internal/perchcli/cli.go:144` — the same change to that file's `burlerengine.New(runner, layout, burlerCfg)` call

  Add the `internal/fabricengine` import to whichever of those two files does not already carry it.
  Do not import `internal/fabricengine` from `internal/burlerengine`, and do not import the top-level `stencils` package from `internal/burlerengine` production code — the engine reads by name and never needs the registry.
- **Commit:** `refactor(burlerengine): read round prompts from the stencils directory at call time`

### Card 19: Repoint `burlerengine`'s prompt-content tests at the `stencils` package defaults

- **Context:**
  - `stencils/stencils.go`
  - `internal/burlerengine/prompt.go`
  - `internal/burlerengine/profile.go`
  - `internal/stencilstore/reconcile.go`
- **Edits:**
  - `internal/burlerengine/template_test.go`
  - `internal/burlerengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `template_test.go` (still `package burlerengine`, still in `internal/burlerengine`), import `github.com/Knatte18/loomyard/stencils` and replace each read of the four deleted package vars with its exported counterpart: `roundOrchestratorTemplate` becomes `stencils.BurlerTemplateRoundOrchestrator`, `instruction1Template` becomes `stencils.BurlerStep1Explore`, `instruction2Template` becomes `stencils.BurlerStep2Review`, `instruction3Template` becomes `stencils.BurlerStep3Fix`.
  Keep every assertion's subject and text unchanged — `TestTemplate_StatesRoundDiscipline`, `TestTemplate_HasClusterRulesSection`, `TestTemplate_StatesClusterForkDiscipline`, `TestTemplate_OrchestratorExcludesDownstreamBodies`, `TestTemplate_FillsWithAllMarkers`, and `TestTemplate_PatternDirectiveOptional` are all testing the shipped default, which remains the correct subject.
  Where a test calls `composePrompt`, add a `stencilsDir` argument built from a `t.TempDir()` seeded with the four burler `.md` files written out from the `stencils` package vars.
  Update the comment at `template_test.go:50` so it names `burler-step-2-review.md` rather than `instruction-2-review-template.md`.

  Add one new test asserting the runtime-read contract: seed a `t.TempDir()`, overwrite `burler/burler-step-2-review.md` with a modified body, call `composePrompt`, and assert the modified text — not the embedded default — reaches the composed instruction file.

  In `doc.go`, update the sentence at `doc.go:28` naming `round-orchestrator-template.md` so it names `burler-template-round-orchestrator.md` and states the prompts now ship from the top-level `stencils` package and are read from the hub's stencils directory at call time.
- **Commit:** `test(burlerengine): read shipped defaults from the stencils package`

### Card 20: Rewrite burler's relocated banners and re-pin `.gitattributes`

- **Context:**
  - `stencils/stencils.go`
- **Edits:**
  - `stencils/burler/burler-template-round-orchestrator.md`
  - `stencils/burler/burler-step-1-explore.md`
  - `stencils/burler/burler-step-2-review.md`
  - `stencils/burler/burler-step-3-fix.md`
  - `.gitattributes`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Each of the four relocated files opens with a leading `<!-- ... -->` banner that `stencil.StripLeadingComment` removes before parsing, so it is read by humans only and never reaches the LLM.
  The three instruction banners each name the orchestrator by its old filename at line 3 — rewrite `round-orchestrator-template.md` to `burler-template-round-orchestrator.md` in all three.
  Rewrite any banner sentence describing these files as embedded in `internal/burlerengine` to state that they ship as embedded defaults in the top-level `stencils` package and are read from `<hub>/_board/_lyx/stencils/burler/` at call time.
  A banner naming a file that no longer exists is read by a human constantly even though the stripper hides it from the producer, which is why this is not cosmetic.
  Do not change any `{{.marker}}` token, any instruction prose below the banner, or any line ending — the body's hash is what edit detection keys on.

  In `.gitattributes`, add four rows pinning the relocated files to LF and remove the four rows that name the now-moved `internal/burlerengine` paths:
  ```
  stencils/burler/burler-template-round-orchestrator.md text eol=lf
  stencils/burler/burler-step-1-explore.md text eol=lf
  stencils/burler/burler-step-2-review.md text eol=lf
  stencils/burler/burler-step-3-fix.md text eol=lf
  ```
  Leave every `internal/treadleengine/*` row in place — those files move in batch 5, and removing their pins early would unpin a still-embedded asset.
- **Commit:** `docs(stencils): retarget burler banners and re-pin LF for the new paths`

## Batch Tests

`verify: go build ./... && go test ./stencils/... ./internal/burlerengine/... ./internal/burlercli/... ./internal/perchcli/... ./internal/lyxcwd/...`

`./stencils/...` runs the registry-completeness test, which is what fails if a `.md` lands without a registry row or vice versa.
`internal/burlerengine` runs the repointed Review Round Invariant tests plus the new runtime-read assertion.
`internal/burlercli` and `internal/perchcli` are the two packages whose `burlerengine.New` calls change signature, so they must compile and pass.
`internal/lyxcwd` runs the Fabric Vocabulary enforcement walk over the four newly relocated prompts — a move that leaves coverage is exactly the silent failure this batch could introduce.
`go build ./...` guards the deletion of `template.go` against any read site outside the packages named above.
