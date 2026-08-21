# Discussion: loom: convert to a Shed recipe

```yaml
task: 'loom: convert to a Shed recipe'
slug: loom-convert-to-shed-recipe
status: discussing
parent: main
```

## Problem

`internal/loomshed.New()` builds loom's thirteen-row `[]shedengine.ProducerDef` as a hand-written Go literal (`internal/loomshed/loomshed.go:137-151`).
The "Shed recipe" initiative exists to replace that literal with a declarative recipe file, and three of its four pieces have already shipped:
the engine registry (`internal/shedrecipe`), the recipe loader/builder (`internal/shedbuild`), and the Shed-setup validity checker (`internal/shedcheck`).
This task is piece 4 — the conversion itself, and the mechanism's first real consumer.

Why now: five further roadmap items (`loom: real LLM producers`) each add or replace rows in this exact list, and they are deliberately sequenced *after* this task so they author their rows directly in recipe form rather than as Go-literal rows that then need converting twice.
`manifest/designs/shed-recipe.md` also carries an explicit banner that piece 4 is "an early concept sketch, not a settled design … do not implement piece 4 from this doc as written", so the location, consumer, and test-ownership decisions below are what settle it.

## Scope

**In:**

- A new production recipe file, `contracts/recipes/loom-recipe.yaml`, expressing loom's current thirteen rows in `internal/shedbuild`'s format.
- A new embed package, `contracts/recipes` (one Go file, `recipes.go`), exporting the recipe's shipped-default bytes — the same shape `contracts/stencils` uses for prompts.
- A new package, `internal/loomrecipe`, which parses the embedded recipe, builds it through `shedbuild.Build` against a caller-supplied `shedrecipe.Env`, and returns the assembled `*shedengine.Shed`.
- Rewiring `internal/loomcli` (`wiring.go`, `drive.go`) to fill a `shedrecipe.Env` instead of a `loomshed.Deps`, and to call `loomrecipe.New` instead of `loomshed.New`.
- Deleting `loomshed.New` and `loomshed.Deps`, and the `NamePreflight`…`NameFinalize` constants if they lose every consumer (see Decisions).
- Moving the tests that drive `loomshed.New` (`loomshed_test.go`'s New-driving half, `sequence_test.go`, `resume_test.go`) into `internal/loomrecipe`, repointed at the recipe-built list.
- Moving `internal/shedrecipe/coverage_guard_test.go` into `internal/loomrecipe`, repointed at the recipe file.
- Deleting `internal/shedbuild/equivalence_test.go` and `internal/shedbuild/testdata/loom-recipe.yaml`.
- Doc updates in the same commit: `manifest/designs/shed-recipe.md`, `manifest/designs/loom.md`, `docs/overview.md`, `CONSTRAINTS.md`, `manifest/roadmap.md`.

**Out:**

- Any change to `internal/shedengine`, `internal/shedcheck`, `internal/shedbuild`, or `internal/shedrecipe` production code.
  This task is those packages' first consumer, not their reviser.
  A genuine defect found in one of them is a finding to report, not a licence to widen scope.
- Any change to which producers loom runs, in what order, with what routing.
  The five stubbed rows (`Discussion-Write`, `Discussion-Review`, `Plan-Write`, `Plan-Review`, `Webster-Review`) stay stubs, verbatim.
  No row gains a `segment`, and no `Bouncer`/`BurlerRound` row appears — those belong to the five `loom: real LLM producers` items.
- Filling `Env.Landing`.
  `internal/loomcli` does not fill `loomshed.Deps.Landing` today either — see the `landing-parity` Decision.
- Operator-editable recipes: no seeding, no hash-stamping, no edit detection, no `_lyx/recipes/` directory, no override path.
- Any second recipe (a `Hardener` recipe, a standalone-mode recipe).
  The format is proven by loom's own; a second product's recipe arrives with that product.
- Moving loom's six producer constructors out of `internal/loomshed`.
  The import cycle is avoided by putting the consumer above `loomshed`, not by emptying it.

## Decisions

### recipe-location — embedded shipped default under `contracts/`

- Decision: the recipe file is `contracts/recipes/loom-recipe.yaml`, embedded via `//go:embed` in a new one-file package `contracts/recipes` (`package recipes`, exporting `var LoomRecipe []byte`).
  It is read through `shedbuild.Parse(recipes.LoomRecipe)`, never `shedbuild.Load`.
  There is no on-disk runtime location, no seeding pass, and no operator override.
- Rationale: a producer graph is a structural definition of what loom *is*, not per-hub configuration.
  A hand-edited graph does not degrade gracefully — it produces a run-time validation failure or, worse, a silently mis-routed run — so the editability that stencils deliberately buy is a liability here rather than a feature.
  Embedding also keeps the file inside the compiled binary, so a recipe and the registry it names can never version-skew across a deploy.
  `contracts/` is already this repo's home for declarative product contracts (`contracts/specs/`, `contracts/stencils/`), and `contracts/stencils/stencils.go` is the exact precedent for a production Go embed package outside `internal/`.
- Rejected: **seeded on disk under `<hub>/_board/_lyx/recipes/`, stencil-style.**
  This is the Stencil Ownership Invariant's shape, and it does not transfer: that invariant exists so a human can tune a *prompt* at run time, and `internal/stencilstore` owns seeding, hash-stamping, edit detection, and the `board.lock`-holding positive-pathspec commit for that purpose.
  Reusing it here means either extending `stencilstore` to a non-stencil file type or writing a second copy of that machinery, for a file nobody has asked to edit.
- Rejected: **a repo file read at run time by an absolute path the caller derives.**
  This makes `lyx` depend on its own source tree being present at run time, which no other producer input does, and adds a geometry question (which root does the path resolve against?) with no benefit.
- Consequence, accepted: `shedbuild.Load` then has no production caller.
  That is fine — it is already exported, documented, and covered by `load_test.go`, and it is the entry a future non-embedded consumer needs.
  Do not delete it, and do not add a contrived production call to justify it.

### consumer-package — a new `internal/loomrecipe` above `loomshed`

- Decision: a new package `internal/loomrecipe` owns the conversion.
  It imports `contracts/recipes`, `internal/shedbuild`, `internal/shedrecipe`, and `internal/shedengine`, and exports one constructor that is the drop-in replacement for `loomshed.New`.
  `internal/loomcli` is its only production caller.
- Rationale: `internal/loomshed` cannot be the consumer — `internal/shedrecipe`'s registry already imports `loomshed` for six of its constructors (`entries_simple.go`), so a `loomshed` → `shedbuild` → `shedrecipe` → `loomshed` production import cycle would not compile.
  The roadmap item states this constraint and leaves the choice between "consumer sits above" and "loomshed sheds its constructors" to this task; "sits above" is chosen.
  Putting it in its own package rather than inline in `loomcli` keeps it drivable from a tier-1 test with a hand-built `Env` — `loomcli`'s own wiring is cwd- and cobra-shaped and its tests carry that weight — and gives the moved sequence/resume/coverage/`Check` tests a natural home.
- Rejected: **inline in `internal/loomcli`.**
  `wiring.go` is already the widest function in the CLI, and the assembled-graph tests would then have to live beside cobra plumbing.
- Rejected: **move the six producer constructors out of `loomshed` so `loomshed` can consume the loader.**
  A much larger refactor (six types plus their tests relocated) whose only gain is keeping one package name in one place, and which would leave `loomshed` a near-empty shell anyway.

### constructor-shape — `New(env, paths)` returning `*shedengine.Shed`

- Decision: `loomrecipe.New` takes the caller's `shedrecipe.Env` plus a small told struct carrying Shed's own four fields (`StatusPath`, `LockPath`, `StatusLockPath`, `MaxBounces`), and returns `(*shedengine.Shed, error)` — exactly `loomshed.New`'s return shape.
  It parses the embedded recipe, calls `shedbuild.Build`, and assembles the `Shed`.
- Rationale: a drop-in return type means `drive.go` changes only which constructor it names, and one package remains the single answer to "how is loom's Shed built".
  Shed's four told fields cannot travel in `shedrecipe.Env` — `Env` is defined as roots and run-wide values the *registry entries* read, and no entry reads `LockPath` — so they need their own told carrier.
- Rejected: **returning `[]shedengine.ProducerDef` and letting `loomcli` assemble the `Shed`.**
  Splits the assembly across two packages and pushes the `MaxBounces`-is-a-per-producer-default subtlety (documented in `wiring.go` today) into cobra code.
- Naming is the implementer's call within the package (`New` plus a `ShedPaths`-style struct is the expected shape); the contract above is what matters.
- The told struct is stored on the `loomCLI` value, not built inside `drive.go`: `status`, `pause`, and `run` all read `StatusPath`/`StatusLockPath`/`LockPath` off `c.deps` today (see Technical context), so the replacement carrier must be reachable from every verb, exactly as `Deps` is.
  The `shedrecipe.Env` is stored beside it and read only by `drive`.

### delete-loomshed-new — one authoritative definition of the list

- Decision: `loomshed.New` and `loomshed.Deps` are deleted, not deprecated.
  `internal/loomshed` keeps its six producer constructors, `Seed`/`ErrSeedExists`, and its ctx helpers.
  The thirteen `Name*` constants: keep only those that retain a consumer after the conversion (see Technical context — `loomcli` currently references `NamePreflight`, and `internal/shedrecipe`'s coverage guard references the row names as strings); delete any that go unreferenced rather than leaving orphans.
- Rationale: the whole point of the conversion is that the recipe file becomes *the* definition.
  Keeping a Go literal beside it — even deprecated — guarantees the two drift, and the drift is silent because nothing would compare them once `equivalence_test.go` is gone.
- Rejected: **keep `New` as a deprecated fallback.**
  There is no caller to fall back for; `loomcli` is the only one.
- Note: `internal/loomshed/seam_enforcement_test.go`'s import allowlist may shrink once `New` goes (`landingshed`, `websterengine`, and `shedadapters` were pulled in largely by `Deps`).
  Tighten the allowlist to what production code still imports rather than leaving it wide — a membership allowlist that over-permits is the failure mode that test exists to prevent.

### env-webster-run — fill `Env.WebsterRun` explicitly

- Decision: `internal/loomcli` sets `Env.WebsterRun = websterengine.Run` explicitly.
- Rationale: this is a real behavioural difference the conversion must absorb, not a detail.
  Today `wiring.go` deliberately leaves `Deps.WebsterRun` nil, with a comment saying `shedadapters.NewWebsterProducer` defaults nil to the production entry point (`internal/shedadapters/webster.go:50-55`).
  But `shedrecipe`'s `websterEntry` calls `requireSeam("Webster", "WebsterRun", env.WebsterRun)` and **errors on nil** — so a straight port leaving it nil fails at build time with `shedrecipe: Webster: Env.WebsterRun must not be nil`.
  `shedadapters.WebsterRunner` is a function type and `var _ WebsterRunner = websterengine.Run` already compiles, so naming it is a one-line change.
- Rejected: **relax `websterEntry` to accept nil.**
  That is a production change to a package this task consumes rather than revises, and it inverts the registry's told-not-derived posture to preserve an implicit default that only existed because the Go literal could rely on the adapter's own fallback.

### preflight-row — constructed by the registry, not injected

- Decision: the `Preflight` row is built by the registry's `preflightEntry` from `Env.Cwd`, and `loomcli` stops pre-constructing it.
  `Env.Cwd` is filled from `c.cwd`, the same value `wiring.go` passes to `preflightshed.NewPreflight` today.
- Rationale: `preflightEntry` already does exactly `preflightshed.NewPreflight(name, env.Cwd)`, so the injection field that `Deps.Preflight` existed for has no purpose once the recipe drives construction.
  Deleting it also removes `loomshed.New`'s nil-`Preflight` guard, whose job the registry's `requireAbsRoot("Preflight", "Cwd", …)` now does.
- Note for the implementer: this changes *when* the row name is told — from `loomshed.NamePreflight` in Go to the `name:` key in the recipe file.
  The recipe's `name: Preflight` must match, and the coverage guard is what pins it.

### landing-parity — leave `Env.Landing` unfilled

- Decision: `Env.Landing` is left unfilled, exactly as `Deps.Landing` is today.
  No behaviour change, no new failure, no fix.
- Rationale: `internal/loomcli/wiring.go` never sets `Deps.Landing`, and `landingshed.NewPublish` rejects a nil `OpenFabric`/`PushBranch` (`internal/landingshed/publish.go:60-66`), so `loomshed.New` **already fails in production today** with `loomshed: build Publish row: …`.
  `landingshed/deps.go:73-76` names the gap and says the resolution chain belongs to a later item.
  The recipe path reproduces this identically: `publishEntry`/`finalizeEntry` call the same constructors with the same zero value.
  Preserving parity keeps this task a conversion.
- Rejected: **build the parent-fabric resolution chain here.**
  A substantial feature (list worktrees, match the parent branch, resolve and open the pair) that would dominate the task and obscure whether the conversion itself is correct.
- Implementer obligation: every test in `internal/loomrecipe` that builds the real thirteen-row list must therefore fill `Env.Landing` with test doubles, the way `internal/shedbuild/equivalence_test.go` and `internal/shedrecipe/coverage_guard_test.go` already do (`testLandingDeps`, `coverageGuardFakeMergeShuttle`).
  Reuse those fixture shapes rather than inventing new ones.

### test-ownership — the assembled-graph tests move to `internal/loomrecipe`

- Decision: `internal/loomrecipe` becomes the home for every test whose subject is loom's assembled graph:
  the row-sequence test (`internal/loomshed/sequence_test.go`), the resume test (`internal/loomshed/resume_test.go`), the `shedcheck`-over-loom's-list invariant currently in `internal/loomshed/loomshed_test.go`, and the registry coverage guard (`internal/shedrecipe/coverage_guard_test.go`).
  Each is repointed at the recipe-built list.
  Tests in `internal/loomshed` whose subject is one producer's own behaviour (`batchifier_test.go`, `discussionvalidate_test.go`, `planvalidate_test.go`, `loompreflight_test.go`, `stub_test.go`, `webster_test.go`, `ctx_test.go`, `seed_test.go`) stay where they are.
- Rationale: those four test the graph, and the graph is now the recipe's output; leaving them in `loomshed` would require hand-building a literal list there, recreating the very duplication being deleted.
- Coverage guard specifics: it must move rather than stay, because an in-package `shedrecipe` test cannot import `shedbuild` (`shedbuild` imports `shedrecipe` — an import cycle).
  In `internal/loomrecipe` it keeps both directions of its assertion: every row in the recipe resolves through `shedrecipe.Lookup`, and `shedrecipe.Names()` has exactly the twelve entries with no unreachable extras beyond those the recipe does not yet use.
  Note the registry ships twelve engines while the recipe uses eight distinct ones — `SingleLLM`, `Bouncer`, and `BurlerRound` are unused until the `loom: real LLM producers` items land — so the guard's "no orphan registry entries" half must allow those three by name, with a comment pointing at the items that will consume them.
  This is a genuine weakening versus today's guard and must be written down where a reader sees it, not silently dropped.
- `CONSTRAINTS.md`'s Shed Recipe Registry Invariant names `internal/shedrecipe/coverage_guard_test.go` as its enforcement point; that line is repointed in the same commit.

### retire-the-fixture — delete `equivalence_test.go` and its testdata

- Decision: `internal/shedbuild/equivalence_test.go` and `internal/shedbuild/testdata/loom-recipe.yaml` are deleted.
  Their replacement is a test in `internal/loomrecipe` asserting that the real embedded recipe parses, builds thirteen rows in the expected order with the expected names/routing/concrete producer types, and that `shedbuild.Check` reports no findings.
- Rationale: the fixture's own header says it is a stand-in and that "the conversion item sequenced after this task depends on this fixture staying in sync".
  Once the real recipe exists, keeping the fixture means two copies of loom's thirteen rows in the tree with nothing comparing them to each other — the exact drift this fixture was built to prevent, relocated.
  Deleting it also keeps `internal/shedbuild` free of loom-specific data, which matters for the "second product's recipe" case.
- Rejected: **repoint `equivalence_test.go` at the real recipe.**
  Makes `internal/shedbuild`'s test suite import `contracts/recipes` and encode loom's row list, inverting the layering — the generic loader would then depend on one product's shape.
- Rejected: **keep the fixture and compare real-vs-fixture.**
  Two files that must be edited together, forever, to express one fact.
- Check first: confirm no other test in `internal/shedbuild` reads `testdata/loom-recipe.yaml` before deleting the directory (`equivalence_test.go` claims to be its only consumer; verify with a grep rather than trusting the comment).

### docs — five files, same commit

- Decision: update, in the implementation commit:
  `manifest/designs/shed-recipe.md` (drop the "do not implement piece 4 from this doc as written" banner, mark piece 4 shipped, record the on-disk-location and consumer decisions the doc explicitly deferred);
  `manifest/designs/loom.md` (loom's producer list is recipe-backed, with a pointer to the recipe file);
  `docs/overview.md` (module table gains `internal/loomrecipe` and `contracts/recipes`, the `internal/loomshed` line loses "13-row producer list", and the Shed-recipe narrative paragraph at lines 322-324 stops saying "leaving only the conversion");
  `CONSTRAINTS.md` (repoint the Shed Recipe Registry Invariant's enforcement line off `internal/shedrecipe/coverage_guard_test.go`);
  `manifest/roadmap.md` (move the item from Planned to Done and close out the "Shed recipe" group's remaining-work framing).
- Rationale: project rule — a task adding a module or introducing cross-cutting infrastructure updates docs in the same commit; four of these five carry statements this task falsifies.
- No new invariant is introduced by this task, so `CONSTRAINTS.md` gains no section — only the enforcement-pointer correction.
  If the implementer concludes a new invariant *is* warranted (e.g. "loom's producer list is defined only in the recipe"), it is added to `CONSTRAINTS.md` in the same commit with a named enforcing test, per the project rule.

## Technical context

**The thirteen rows and their engines** (from `internal/shedrecipe/coverage_guard_test.go`'s `loomRowEngines`, which the recipe must match):
`Preflight`→`Preflight`, `Loom-Preflight`→`LoomPreflight`, `Discussion-Write`→`Stub`, `Discussion-Validate`→`DiscussionValidate`, `Discussion-Review`→`Stub`, `Plan-Write`→`Stub`, `Plan-Validate`→`PlanValidate`, `Plan-Review`→`Stub`, `Batchifier`→`Batchifier`, `Webster`→`Webster`, `Webster-Review`→`Stub`, `Publish`→`Publish`, `Finalize`→`Finalize`.

**The recipe's content is already written.**
`internal/shedbuild/testdata/loom-recipe.yaml` is a correct, `Parse`-passing, `Build`-passing expression of exactly these thirteen rows, proven equivalent to `loomshed.New`'s output field-by-field and type-by-type by `equivalence_test.go`.
The production recipe is that file with its four-line "test fixture only" header comment replaced by a production header.
Carry over verbatim: `version: 1`, `entry: Preflight`, `terminals: [Finalize]`, the row order, every `on_done`/`on_stuck`, no `segment` on any row, no `max_bounces` on any row, no `config` block on any row, and the explicit `on_done: ""` on `Finalize` with its load-bearing comment.

Do not copy the fixture blind, though.
Its equivalence to `loomshed.New`'s literal was last proven when `equivalence_test.go` was written, and that test is being deleted in this same task — so re-verify, at implementation time, that the fixture's thirteen rows still match `loomshed.go`'s live literal row-for-row before the literal is deleted.
The cheapest way to do that is ordering: run the existing `internal/shedbuild` equivalence test unchanged as the first step, confirm it is green against today's tree, and only then copy the fixture and start deleting.
A green run of that test is the proof; copying first and deleting the test first destroys it.

**`shedrecipe.Env` fields loom's thirteen rows actually read** — fill exactly these in `loomcli`, and leave the rest zero (each entry validates only what it reads, so a partially-filled `Env` is legal by design):

| Env field | Source in `wiring.go` today | Read by |
|---|---|---|
| `Cwd` | `c.cwd` (the `wire` parameter) | `Preflight` |
| `AnchorPath` | `location.AnchorPath()` | `Batchifier`, `PlanValidate`, `Webster` |
| `WorktreeRoot` | `location.WorktreePath()` | `PlanValidate` |
| `StatusPath` | `loomengine.LoomStatusFile(location)` | `LoomPreflight` |
| `StatusLockPath` | `loomengine.LoomStatusLock(location)` | `LoomPreflight` |
| `DecisionRecordPath` | `loomengine.DiscussionDecisionRecord(location)` | `DiscussionValidate` |
| `SupportLogPath` | `loomengine.DiscussionSupportLog(location)` | `DiscussionValidate` |
| `WebsterRun` | **new** — `websterengine.Run`, see the `env-webster-run` Decision | `Webster` |
| `WebsterDeps` | the assembled `runDeps` | `Webster` |
| `Landing` | unfilled today, stays unfilled — see `landing-parity` | `Publish`, `Finalize` |

`StencilsDir` and `RunRoot` stay zero: only `SingleLLM`, `Bouncer`, and `BurlerRound` read them, and no row uses those engines yet.
`Shuttle` and `Burler` stay zero for the same reason.
`Now` stays nil (the underlying constructors default it to `time.Now`).

Shed's own four told values move out of `Deps` into `loomrecipe.New`'s second argument: `StatusPath` = `loomengine.LoomStatusFile(location)`, `LockPath` = `loomengine.LoomRunLock(location)`, `StatusLockPath` = `loomengine.LoomStatusLock(location)`, `MaxBounces` = 0.
Note `StatusPath` and `StatusLockPath` are told twice — once in `Env` (for `LoomPreflight`) and once here (for `Shed`).
That duplication is inherent to the split and is what `equivalence_test.go`'s paired-fixture helper already does; do not try to collapse it.

**`loomcli` call sites that change.**
Two are structural:
`internal/loomcli/wiring.go:88-109` (the `c.deps = loomshed.Deps{…}` literal becomes a `shedrecipe.Env` plus the Shed-paths value, both stored on `c`) and `internal/loomcli/cli.go:41-43` (the `deps` field's type and its doc comment, which names `loomshed.Deps` and `loomshed.New` explicitly).
One is the constructor swap: `internal/loomcli/drive.go:45` (`loomshed.New(c.deps)` → `loomrecipe.New(…)`).

The rest are plain reads of `c.deps.StatusPath` / `c.deps.StatusLockPath` / `c.deps.LockPath` that must be repointed at whichever field replaces them — enumerate them with a `c\.deps\.` grep rather than working from this list alone, but as of exploration they are:
`drive.go:40-41` (the no-status-file refusal),
`status.go:65,67,71,81,95` (`status` and `status --watch`),
`pause.go:33,35,46`,
`run.go:100` (the `loomshed.Seed` call — `Seed` itself is unchanged) and `run.go:181` (`c.deps.LockPath`).
This is the widest mechanical edit in the task: four of loom's five verbs read the status pair off `Deps` today, so the replacement carrier has to be reachable from every command, not just `drive`.

**`Build` is not filesystem-free, and construction can fail.**
Three registry constructors reach disk at construction time (see `internal/shedbuild/doc.go` for the enumeration), and `publishEntry`/`finalizeEntry` call `landingshed.NewPublish`/`NewFinalize`, which open a fabric pair.
This matches today's behaviour — `loomshed.New` calls the same two constructors — so `loomrecipe.New` must surface the error rather than swallow it, exactly as `loomshed.New` does.

**Errors carry position.** `shedbuild` names the offending row's zero-based index and `name` in every error it raises after decode, and the decoder keeps yaml line numbers.
No error-wrapping work is needed in `loomrecipe` beyond a package prefix.

**Do not call `Check` from production.** `shedbuild.Check` is authoring-time only; `internal/shedcheck/doc.go` and `internal/shedbuild/check.go` both state why (a resumed run legitimately starts mid-graph, so reachability-from-entry is the wrong production question).
It belongs in `internal/loomrecipe`'s test suite, which is also how `internal/loomshed/loomshed_test.go` uses it today.

**Reference files worth reading before writing code:**
`internal/loomshed/loomshed.go` (the literal being replaced, and a long doc comment on routing rationale worth preserving somewhere — the recipe file's own header is the natural home for the parts that describe the graph);
`internal/shedbuild/equivalence_test.go` (the paired `Env`/`Deps` fixture, and the assertion shape the new loomrecipe test inherits);
`internal/shedrecipe/coverage_guard_test.go` (the guard being moved, and its `landingshed` test doubles);
`contracts/stencils/stencils.go` (the embed-package precedent).

## Constraints

From `CONSTRAINTS.md`:

- **Shed Recipe Registry Invariant** — the registry stays one central `map[string]Constructor` literal reached only via `Lookup`/`Names`; no `init()` self-registration, no runtime `Register`.
  This task adds no registration mechanism.
  Its enforcement pointer to `internal/shedrecipe/coverage_guard_test.go` must be repointed when that file moves.
- **Told-Geometry Invariant** — `internal/loomrecipe` takes every absolute path from its caller (in `Env` and in the Shed-paths value) and must have no direct production import of `internal/lyxcwd`.
  Add a `seam_enforcement_test.go` to the new package with a membership allowlist, modelled on `internal/loomshed/seam_enforcement_test.go` and `internal/shedrecipe/seam_enforcement_test.go`.
- **Shed Producer-Seam Invariant** — untouched; `internal/shedengine` gains no import.
- **Stencil Ownership Invariant** — not engaged by the recipe (a recipe is not a producer prompt), but it is the reason the seeded-on-disk alternative was rejected; do not extend `internal/stencilstore` to cover recipes.
- **Cwd Resolution Invariant** — `internal/lyxcwd` alone resolves cwd; `loomcli` passes `c.cwd` down, and neither `loomrecipe` nor the recipe file names a path.
- **Test Tier Purity Invariant** — the new package's tests must be tier 1: hand-built `Env` over `t.TempDir()`, test doubles for every seam, no process spawn.
  `internal/shedbuild/equivalence_test.go` is the working example.
- **CLI / Cobra Invariant** — `drive.go`'s command shape, `Short`, and help tree are unchanged; only the constructor it calls changes.
- **Documentation Lifecycle** — the five doc files in the `docs` Decision land in the implementation commit.
- **Config Strictness Invariant** — not engaged: the recipe is not a `_lyx/` module config and goes through neither `Load` nor `LoadOrTemplate`.
- **Fabric Vocabulary Invariant** — its `.md` half covers `contracts/stencils/**/*.md`; the new `.yaml` under `contracts/recipes/` is outside both halves today.
  Do not widen the walk as a side effect of this task; if the implementer judges the recipe should be in scope for the vocabulary walk, that is a separate proposal.

Discovered during exploration:

- `shedrecipe`'s `websterEntry` rejects a nil `Env.WebsterRun` — see the `env-webster-run` Decision.
  This is the one place a naive port silently breaks.
- `internal/shedbuild` imports `internal/shedrecipe`, so no in-package `shedrecipe` test may import `shedbuild`.
  This forces the coverage guard's relocation.
- `Publish`/`Finalize` construction already fails in production for want of `Landing` — see `landing-parity`.
  Do not "fix" this while converting, and do not let it read as a regression introduced here.

## Testing

**`internal/loomrecipe` (new, the bulk of the test work — TDD candidate).** Write the recipe file and the package's tests together; the recipe is data, so the tests are what make it verifiable.

- *Build equivalence, restated as a shape assertion.* Parse and build the embedded recipe from a hand-built `Env` over `t.TempDir()`, and assert thirteen rows in the expected order, each with the expected `Name`, `OnDone`, `OnStuck`, empty `Segment`, zero `MaxBounces`, and expected concrete `Producer` type (`reflect.TypeOf`).
  This is `equivalence_test.go`'s assertion loop with its `loomshed.New` side replaced by an expected-value table.
- *Structural check.* `shedbuild.Check(recipe, built)` reports no findings.
- *Registry coverage guard (moved).* Every recipe row's `engine` resolves through `shedrecipe.Lookup`; `shedrecipe.Names()` returns exactly twelve; the only registered-but-unused engines are `SingleLLM`, `Bouncer`, `BurlerRound`.
- *Row sequence (moved from `loomshed/sequence_test.go`).* A clean `Run` over the built list visits the expected row-name sequence, asserted against a literal expected list.
- *Resume (moved from `loomshed/resume_test.go`).* A run resuming from a persisted `current_producer` starts mid-graph as before.
- *Construction failure surfaces.* An `Env` missing a field a row reads (e.g. empty `Cwd`) returns an error naming the row, not a panic and not a silently-degraded list.
- *Seam enforcement.* Membership allowlist over the package's production imports.

**`contracts/recipes`.** A test asserting the embedded bytes are non-empty and `shedbuild.Parse`-able is redundant with the loomrecipe tests and should not be duplicated there; the package needs no test of its own beyond what `contracts/stencils` has (its `registry_test.go` exists for registry-vs-disk completeness, which a single-file embed does not need).
If a second recipe is ever added, revisit.

**`internal/loomshed`.** After deletion, run the remaining suite and confirm the per-producer tests still pass untouched.
Tighten `seam_enforcement_test.go`'s allowlist to what production still imports.
`fixture_test.go` may carry helpers used only by the moved tests — move or delete those with them rather than leaving dead fixtures.

**`internal/loomcli`.** Existing `wire` tests must be updated to assert the new `Env`/Shed-paths values instead of `loomshed.Deps` fields, with a specific assertion that `Env.WebsterRun` is non-nil — that is the regression this conversion is most likely to reintroduce.
`drive.go`'s no-status-file refusal test must keep passing unchanged.

**`internal/shedbuild`.** Run `equivalence_test.go` unchanged and green *before* copying its fixture or deleting anything — that green run is the only remaining proof the fixture matches `loomshed.New`'s literal, and both are deleted by the end of this task (see Technical context).
After deleting `equivalence_test.go` and `testdata/`, the remaining suite (`parse_test`, `build_test`, `build_engines_test`, `load_test`, `check_test`, `seam_enforcement_test`) must pass with no edits.
If any of them turns out to read the deleted testdata, keep the minimum that they need rather than resurrecting the loom fixture.

**Whole-repo gate.** `go build ./...` and `go test ./...` green, and the `docs`/`CONSTRAINTS.md` link-integrity checks pass (Markdown Link Integrity is an enforced invariant and this task edits five docs).

## Q&A log

- **Q:** Where does the recipe file live and how is it delivered? **A:** [auto-pick] Embedded shipped default at `contracts/recipes/loom-recipe.yaml` via a `contracts/recipes` embed package, parsed with `shedbuild.Parse`. **Why:** a producer graph is a structural product definition, not per-hub configuration; the seeded-on-disk alternative demands stencilstore-class seed/hash/commit machinery for a file whose hand-edit yields a broken graph at run time.
- **Q:** Who consumes the loader, given `internal/loomshed` cannot? **A:** [auto-pick] A new package `internal/loomrecipe`, called by `internal/loomcli`. **Why:** the registry already imports `loomshed`, so a back-import is a compile-time cycle; a package above it keeps the graph tests tier-1 and avoids relocating six producer constructors.
- **Q:** Do `loomshed.New`/`Deps` survive? **A:** [auto-pick] Deleted, not deprecated. **Why:** two authoritative copies of the thirteen-row list is exactly the drift the recipe exists to eliminate, and nothing would compare them once the equivalence fixture is gone.
- **Q:** Where do the sequence, resume, and `shedcheck` invariant tests live afterwards? **A:** [auto-pick] Moved into `internal/loomrecipe`, driving the recipe-built list. **Why:** their subject is loom's assembled graph, which is now the recipe's output; leaving them in `loomshed` would require hand-building a literal list there.
- **Q:** What happens to `shedbuild/equivalence_test.go` and its `testdata/loom-recipe.yaml`? **A:** [auto-pick] Both deleted, replaced by a shape-assertion test in `internal/loomrecipe`. **Why:** the fixture's own header calls itself a stand-in until this conversion; keeping it leaves two uncompared copies of the same thirteen rows and drags loom-specific data into the generic loader's suite.
- **Q:** What happens to `shedrecipe/coverage_guard_test.go`, which drives the deleted `loomshed.New`? **A:** [auto-pick] Moved to `internal/loomrecipe` and repointed at the recipe file, with `CONSTRAINTS.md`'s enforcement line updated in the same commit. **Why:** an in-package `shedrecipe` test cannot import `shedbuild` (import cycle), and a table-only guard would pass forever regardless of what loom's list grew.
- **Q:** `websterEntry` rejects a nil `Env.WebsterRun`, but `wiring.go` deliberately leaves the field nil today. Which side gives? **A:** [auto-pick] `loomcli` fills `Env.WebsterRun = websterengine.Run` explicitly. **Why:** relaxing the entry would be a production change to a package this task only consumes, and would invert the registry's told-not-derived posture to preserve an implicit adapter default.
- **Q:** `Env.Landing` is unfilled by `loomcli`, so `Publish`/`Finalize` construction already fails in production. Fix it here? **A:** [auto-pick] No — preserve parity, leave it unfilled. **Why:** the parent-fabric resolution chain is a substantial feature owned by a later item; building it here would dominate the task and obscure whether the conversion itself is correct.
- **Q:** How does `loomrecipe.New` receive Shed's own four told fields, which no registry entry reads? **A:** [auto-pick] As a second told argument beside `shedrecipe.Env`, with `New` returning `*shedengine.Shed` — a drop-in for `loomshed.New`. **Why:** `Env` is defined as values registry entries read, and a drop-in return type keeps `drive.go`'s shape and one single answer to "how is loom's Shed built".
- **Q:** The recipe's content already exists as `internal/shedbuild/testdata/loom-recipe.yaml` — is copying it a shortcut around verification? **A:** [orchestrator review] No, but the ordering is load-bearing: run the existing `equivalence_test.go` green against today's tree *first*, then copy the fixture and delete. **Why:** the fixture's equivalence to `loomshed.New`'s literal was proven when the test was written, and this task deletes both the test and the literal — copying first would leave the claim unverified at exactly the moment its only prover is removed.
- **Q:** Which docs land in the same commit? **A:** [auto-pick] `manifest/designs/shed-recipe.md`, `manifest/designs/loom.md`, `docs/overview.md`, `CONSTRAINTS.md`, `manifest/roadmap.md`. **Why:** the project's documentation rule, and four of the five carry statements this task falsifies — including shed-recipe.md's "do not implement piece 4 from this doc as written" banner.
