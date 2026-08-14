# Batch: treadle-runtime-read

```yaml
task: "Relocate producer prompt files into a stencils/ directory"
batch: "treadle-runtime-read"
number: 5
cards: 4
verify: go build ./... && go test ./stencils/... ./internal/treadleengine/... ./internal/perchengine/... ./internal/perchcli/... ./internal/lyxcwd/...
depends-on: [4]
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (package or module declaration, imports, identifier retargeting, seam splits).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

## Batch Scope

Treadle's four ephemeral-utility prompts move to `stencils/treadle/`, and `treadleengine` switches to a call-time `stencilstore.Read`.
This is the batch the whole "told, never derives" design exists for: `internal/treadleengine` is barred from `internal/lyxcwd` and is told only `runDir` and `Profile.GateDir`, neither of which is the hub, so the stencils directory must arrive as caller-supplied data and travel down to four package-level read sites that are not methods on `Engine`.

The four read sites are `runCircling` (`judge.go:58`), `runMilestone` (`judge.go:73`), `runTriage` (`judge.go:147`), and `runTargeting` (`targeting.go:31`).
`runJudgeCall` (`judge.go:93`) takes the already-selected template as a `template []byte` parameter and reads nothing itself, so it is not a read site and its signature does not change.
Two of the four — `runCircling` and `runMilestone` — take a single `judgeInputs` struct built as a composite literal by their callers at `run.go:322` and `run.go:357`, so one new `judgeInputs` field reaches both; the other two take loose scalars and need one new parameter each.
A `judgeInputs` field rather than a fifth loose parameter is the existing bundling idiom for exactly those callers, touches two call sites instead of four, and keeps "treadle is told its geometry" visibly true because the value is set in a caller-owned literal.

The value enters through `treadleengine.Options`, set at the package's single production construction site, `internal/perchengine/engine.go:113`, which in turn gains it from `internal/perchcli`.
Batch-local decision: `perchengine.Engine.Run` gains a trailing `stencilsDir string` parameter rather than resolving the directory itself, so the resolution stays in the `*cli` layer that already holds the `*lyxcwd.Location`.

This batch also amends the machine-enforced Treadle Runner-Seam import allowlist and the CONSTRAINTS bullet it mirrors, in the same commit as the import that needs it.

## Cards

### Card 21: Move treadle's four prompts into `stencils/treadle/` and register them

- **Context:**
  - `internal/stencilstore/stencilstore.go`
- **Edits:**
  - `stencils/stencils.go`
  - `.gitattributes`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/treadleengine/judge-circling-template.md` -> `stencils/treadle/treadle-template-judge-circling.md`
  - `internal/treadleengine/judge-milestone-template.md` -> `stencils/treadle/treadle-template-judge-milestone.md`
  - `internal/treadleengine/triage-template.md` -> `stencils/treadle/treadle-template-triage.md`
  - `internal/treadleengine/targeting-template.md` -> `stencils/treadle/treadle-template-targeting.md`
- **Requirements:**
  Add four `//go:embed` directives and their exported typed `[]byte` vars to `stencils/stencils.go`, following card 7's shape:
  `TreadleTemplateJudgeCircling`, `TreadleTemplateJudgeMilestone`, `TreadleTemplateTriage`, `TreadleTemplateTargeting`.
  Add the four matching rows to the `entries` slice, keyed by the stencil names `treadle-template-judge-circling`, `treadle-template-judge-milestone`, `treadle-template-triage`, and `treadle-template-targeting`, placed after burler's four.

  In `.gitattributes`, add four rows pinning the relocated files to LF and remove the four rows naming the now-moved `internal/treadleengine` paths:
  ```
  stencils/treadle/treadle-template-judge-circling.md text eol=lf
  stencils/treadle/treadle-template-judge-milestone.md text eol=lf
  stencils/treadle/treadle-template-triage.md text eol=lf
  stencils/treadle/treadle-template-targeting.md text eol=lf
  ```
  These four files' banners name no other stencil by filename, so unlike burler's they need no cross-reference rewrite; check each banner and rewrite only a sentence that names the file's own old path or describes it as embedded in `internal/treadleengine`.
  Do not change any `{{.marker}}` token or any body text below the banner.
- **Commit:** `feat(stencils): relocate treadle judge and utility prompts into stencils/treadle`

### Card 22: Thread the stencils directory into treadle's four read sites

- **Context:**
  - `internal/stencilstore/reconcile.go`
  - `internal/treadleengine/profile.go`
  - `internal/treadleengine/doc.go`
  - `stencils/stencils.go`
- **Edits:**
  - `internal/treadleengine/engine.go`
  - `internal/treadleengine/judge.go`
  - `internal/treadleengine/judge_test.go`
  - `internal/treadleengine/targeting.go`
  - `internal/treadleengine/run.go`
  - `internal/treadleengine/engine_test.go`
- **Creates:** none
- **Deletes:**
  - `internal/treadleengine/template.go`
- **Moves:** none
- **Requirements:**
  Delete `internal/treadleengine/template.go` outright — it exists only to declare the four `//go:embed` directives and the package vars `judgeCirclingTemplate`, `judgeMilestoneTemplate`, `triageTemplate`, and `targetingTemplate`, all of which now live in `stencils/stencils.go`.

  In `engine.go`, add a `StencilsDir string` field to `type Options struct` and a matching `stencilsDir string` field to `type Engine struct`, assigned from the option inside

```go
func New(name string, runner RoundRunner, shuttle Shuttle, opts Options) *Engine {
```

  This is the caller-supplied field the discussion's design calls for: treadle is told its geometry and never derives it, exactly as it is already told `runDir` and `Profile.GateDir`.

  In `judge.go`, add a `StencilsDir string` field to

```go
// judgeInputs bundles values for composing a judge call's prompt and shuttle spec.
type judgeInputs struct {
```

  and replace the two template reads with `stencilstore.Read`, returning the read error through each function's existing failure path rather than inventing a new one:
  - `runCircling` (`judge.go:50`) reads `stencilstore.Read(in.StencilsDir, "treadle-template-judge-circling")` at `judge.go:58`
  - `runMilestone` (`judge.go:64`) reads `stencilstore.Read(in.StencilsDir, "treadle-template-judge-milestone")` at `judge.go:73`

  Add a `stencilsDir string` parameter to the two loose-scalar functions and use it for their reads:
  - `func runTriage(sh Shuttle, name string, round int, question, verdictPath, model, effort string) (TriageVerdict, string)` gains the parameter and reads `stencilstore.Read(stencilsDir, "treadle-template-triage")` at `judge.go:147`
  - `func runTargeting(sh Shuttle, name string, round int, previousHandoffPath, seedPath, model, effort string) (string, bool)` gains the parameter and reads `stencilstore.Read(stencilsDir, "treadle-template-targeting")` at `targeting.go:31`

  Do not change `runJudgeCall`'s signature — it takes the already-selected `template []byte` and reads nothing.

  In `run.go`, inside `func (e *Engine) Run(p Profile, runDir string) (result Result, err error)`, set `StencilsDir: e.stencilsDir` in both `judgeInputs` composite literals (the milestone literal at `run.go:322` and the circling literal at `run.go:357`), and pass `e.stencilsDir` as the new argument at every `runTriage` and `runTargeting` call site.

  Both new parameters break existing untagged test files in this package, which must be threaded through in this same card or the batch's own `verify:` fails to compile:
  - `internal/treadleengine/judge_test.go` — every `runTriage(...)` call site and every `judgeInputs{...}` composite literal. Add the new `stencilsDir` argument to each call and the new `StencilsDir` field to each literal, pointing at a `t.TempDir()` seeded from the `stencils` package vars.
  - `internal/treadleengine/engine_test.go` — every `runTargeting(...)` call site (e.g. line 1347) needing the new `stencilsDir` argument.

  Do not work from a call-site count: `go build` and the batch's own `verify:` enumerate every real site.

  `treadleengine.New`'s own arity is unchanged — `StencilsDir` arrives as an `Options` struct field — so the existing `New(...)` calls in these test files need no edit.

  `internal/treadleengine` must import `internal/stencilstore` and nothing else new.
  Do not import `internal/fabricengine`, `internal/lyxcwd`, or the top-level `stencils` package from this package — that is precisely the import direction the seeding pass running at the composition root exists to prevent.
- **Commit:** `refactor(treadleengine): read judge and utility prompts from the stencils directory`

### Card 23: Amend the Treadle Runner-Seam allowlist and its CONSTRAINTS bullet

- **Context:**
  - `internal/treadleengine/judge.go`
  - `internal/stencilstore/doc.go`
- **Edits:**
  - `internal/treadleengine/seam_enforcement_test.go`
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `seam_enforcement_test.go`, add one entry to the `allowedImports` map:
  `"github.com/Knatte18/loomyard/internal/stencilstore": true`.
  Add no other entry — treadle calls only `stencilstore.Read(baseDir, name)`, which needs no registry, so the top-level `stencils` package never reaches treadle's import graph.

  In `CONSTRAINTS.md`'s **Treadle Runner-Seam Invariant** section, add `internal/stencilstore` to the import-allowlist sentence, and extend the paragraph that follows it with the justification: `stencilstore` takes a fully resolved absolute stencils directory from its caller and derives no geometry of its own, so treadle is still *told* its stencils directory exactly as it is told `runDir` and `Profile.GateDir` — the exclusion of `internal/lyxcwd` still means what it meant.
  Record that the seeding pass runs once at `cmd/lyx`'s root pre-run rather than lazily inside `stencilstore.Read`, which is what keeps `internal/fabricengine` off treadle's stack.
  Both edits land in this commit, together with the import that needs them — the allowlist test and the CONSTRAINTS bullet must never disagree across a commit boundary.
- **Commit:** `docs(constraints): admit stencilstore to treadle's import allowlist`

### Card 24: Wire the stencils directory from `perchcli` through `perchengine` and repoint treadle's tests

- **Context:**
  - `internal/treadleengine/engine.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `stencils/stencils.go`
- **Edits:**
  - `internal/perchengine/engine.go`
  - `internal/perchengine/run_test.go`
  - `internal/perchcli/run.go`
  - `internal/treadleengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/perchengine/engine.go`, change

```go
func (e *Engine) Run(p Profile, runDir, scratchDir string) (Result, error) {
```

  to take a trailing `stencilsDir string` parameter, and set `StencilsDir: stencilsDir` in the `treadleengine.Options` composite literal at `engine.go:113`, beside the existing `PauseRequested`, `RunCommand`, and `ScratchDir` fields.

  In `internal/perchcli/run.go:301`, change `engine.Run(profile, runDir, scratchDir)` to pass `fabricengine.StencilsDir(layout.HubPath)` as the new fourth argument, using the `*lyxcwd.Location` that `perchcli` already resolves.
  Add the `internal/fabricengine` import to `run.go` if it is not already present.
  This is the only production path that reaches `treadleengine.New`, so it is the only wiring needed.

  `perchengine.Engine.Run`'s new parameter breaks `internal/perchengine/run_test.go`, which is untagged and holds many `e.Run(p, runDir, runDir)`-shaped call sites (e.g. line 276) — every one needs the new trailing argument, pointing at a `t.TempDir()` seeded from the `stencils` package vars.
  Do not work from a call-site count: `go build` and the batch's own `verify:` enumerate every real site.
  Without this the batch's own `verify:` fails to compile at `./internal/perchengine/...`.
  No other `*_test.go` file in `internal/perchengine` calls `Engine.Run`.

  In `internal/treadleengine/template_test.go` (still `package treadleengine`), import `github.com/Knatte18/loomyard/stencils` and replace each read of the four deleted package vars with its exported counterpart: `judgeCirclingTemplate` becomes `stencils.TreadleTemplateJudgeCircling`, `judgeMilestoneTemplate` becomes `stencils.TreadleTemplateJudgeMilestone`, `triageTemplate` becomes `stencils.TreadleTemplateTriage`, `targetingTemplate` becomes `stencils.TreadleTemplateTargeting`.
  Keep the subject and text of `TestJudgeCirclingTemplate_StatesLoadBearingRules`, `TestJudgeMilestoneTemplate_StatesLoadBearingRules`, `TestTriageTemplate_StatesLoadBearingRules`, `TestTargetingTemplate_StatesLoadBearingRules`, and the four `*_FillsWithAllMarkers` tests unchanged — they test the shipped default, which remains the right subject.
  Add one new test seeding a `t.TempDir()` with the four treadle `.md` files, overwriting `treadle/treadle-template-triage.md` with a modified body, and asserting `runTriage` composes its prompt from the modified on-disk text rather than the embedded default.
- **Commit:** `refactor(perch): pass the resolved stencils directory into treadle`

## Batch Tests

`verify: go build ./... && go test ./stencils/... ./internal/treadleengine/... ./internal/perchengine/... ./internal/perchcli/... ./internal/lyxcwd/...`

`internal/treadleengine` runs both the repointed content tests and — the load-bearing one — `TestRunnerSeamInvariant_AllowlistOnly` against the amended allowlist, which is what proves the new import is the single sanctioned one and that neither `fabricengine` nor `lyxcwd` crept in behind it.
`internal/perchengine` and `internal/perchcli` are the two packages whose signatures change.
`./stencils/...` runs registry completeness over the four new files, and `internal/lyxcwd` runs the Fabric Vocabulary walk over them.
`go build ./...` guards the `template.go` deletion and the two changed function signatures across the whole module.
