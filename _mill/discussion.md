# Discussion: Shed recipe: engine registry

```yaml
task: 'Shed recipe: engine registry'
slug: shed-recipe-engine-registry
status: discussing
parent: main
```

## Problem

`internal/loomshed.New` builds loom's thirteen-row `[]shedengine.ProducerDef` as a hand-written Go literal (`internal/loomshed/loomshed.go:137-151`).
Row order, wiring, and every producer's construction are compiled in;
changing the pipeline means editing Go and rebuilding.
`manifest/designs/shed-recipe.md` proposes replacing that literal with a declarative recipe file whose rows name an `Engine` by string, and splits the work into four separable pieces.
This task is piece 1 of 4: the **engine registry** — the name → constructor mapping the later recipe loader resolves each row's `Engine` field against.

Why now: the Shed recipe group is sequenced immediately ahead of the `loom: real LLM producers` group in `manifest/roadmap.md`, deliberately, so those five tasks author their rows directly in recipe form rather than as a Go literal that then needs converting.
The registry is the foundational piece — the loader (piece 2) and the loom conversion (piece 4) both depend on it.
Nothing here changes `shedengine`, and loom's existing hardcoded list keeps working untouched.

## Scope

**In:**

- A new package `internal/shedrecipe` holding the engine registry.
- A fixed constructor signature every registry entry satisfies, plus the two told inputs it takes: a per-row static `Config` and a caller-filled `Env`.
- Twelve registry entries covering every `ShedProducer` type reachable from loom's current list plus the two shipped review adapters: `SingleLLM`, `Bouncer`, `BurlerRound`, `Webster`, `Preflight`, `Publish`, `Finalize`, `LoomPreflight`, `Batchifier`, `DiscussionValidate`, `PlanValidate`, `Stub`.
- Exporting `internal/loomshed`'s five currently-unexported producer constructors so the registry can reach them.
- A `SpecSource` builder for the `SingleLLM` entry — stencil read plus token fill — since no production caller of `shedadapters.NewSingleLLMProducer` exists yet.
- A coverage-guard test asserting every engine backing a row in `loomshed.New`'s current list has a registry entry.
- A `seam_enforcement_test.go` import allowlist for the new package, and a new **Shed Recipe Registry Invariant** section in `CONSTRAINTS.md`.
- Doc updates in the same commit: `manifest/designs/shed-recipe.md` (narrow the DRAFT banner), `docs/overview.md` (module table), `manifest/roadmap.md` (move the item to Done).

**Out:**

- The recipe **file format** — no YAML schema, no struct tags, no file parsing. That is piece 2 (`Shed recipe: loader/builder`).
- The **loader/builder** itself — nothing in this task assembles a `[]shedengine.ProducerDef`.
- The **validity checker** (piece 3) — no `OnDone`/`OnStuck` graph inspection.
- **Converting `loomshed`'s list** (piece 4) — `loomshed.New` keeps its Go literal, and `loomshed.Deps.Preflight` keeps its pre-injected field.
- Any change to `internal/shedengine` — the `ShedProducer` seam, `ProducerDef`, and `validate()` are untouched.
- Any change to `shedengine.ProducerDef.Segment` behaviour. The design doc drops `Segment` from recipe rows, but that is the loader's concern; the registry never sees routing fields at all.
- Writing any rubric or prompt stencil content. The `loom: real LLM producers` group owns that.
- Wiring the registry into any CLI entry point.

## Decisions

### Registry lives in a new `internal/shedrecipe` package

- Decision: create `internal/shedrecipe`, importing the producer-hosting packages (`shedadapters`, `loomshed`, `landingshed`, `preflightshed`) and never imported by them.
- Rationale: the registry must reach types from four different packages at once, so it has to sit above all of them.
  `internal/shedengine` is barred outright — the **Shed Producer-Seam Invariant** restricts its production imports to stdlib, `internal/state`, and `internal/lock`, machine-enforced by `internal/shedengine/seam_enforcement_test.go`.
  `internal/shedadapters` cannot reach `loomshed`'s types without an import cycle, since `loomshed` already imports `shedadapters`.
  The name `shedrecipe` deliberately avoids the `*engine` suffix used by feature engines in this tree — this is a wiring table, not an engine.
- Rejected: putting it in `shedengine` (breaks a machine-enforced invariant);
  putting it in `shedadapters` (import cycle);
  a `cmd/`-level table (unreachable from a future standalone entry point, and untestable as a unit).

### Central table literal, not `init()` self-registration

- Decision: one file in `internal/shedrecipe` declares an explicit `map[string]Constructor` literal naming all twelve entries.
  Lookup is a pure function over that map.
- Rationale: the whole registry is greppable in one place, there is no mutable package global to corrupt across tests, and registration order is irrelevant.
  `internal/batcher`'s `init()` self-registration (`internal/batcher/registry.go`) is the in-repo precedent, but it works there because every batcher lives in the same package as the registry;
  here the entries would live in four packages, so `init()` registration would require blank imports at every consumer and would make "which engines exist" depend on which packages happened to be linked in.
- Rejected: `init()` self-registration per producer package (blank-import fragility, mutable global, non-deterministic membership);
  a `Register` function callable at runtime (same mutable-global problem, plus it makes the coverage-guard test order-dependent).

### `loomshed`'s five constructors become exported

- Decision: rename `newLoomPreflight`, `newBatchifier`, `newDiscussionValidate`, `newPlanValidate`, and `newStub` to their exported forms in `internal/loomshed`, keeping their current signatures and behaviour unchanged.
  The returned concrete types stay unexported;
  each exported constructor's declared return type becomes `shedengine.ShedProducer`.
- Rationale: the central-table decision requires the registry to call these from outside the package.
  Returning the seam interface rather than the unexported concrete type keeps the concrete types package-private, which is what they were for.
  This is the only edit this task makes to `loomshed`, and it changes no behaviour — `loomshed.New`'s call sites are updated to the new names in the same commit.
- Rejected: duplicating the constructors in `shedrecipe` (two definitions of the same producer, guaranteed to diverge);
  moving the loom-specific producers into `shedrecipe` (they are loom's, not shared — the design doc is explicit that a bespoke single-consumer engine is a valid registry entry precisely *because* it can stay where it lives).

### Fixed constructor signature

- Decision: every registry value has the signature

  ```go
  type Constructor func(name string, cfg Config, env Env) (shedengine.ShedProducer, error)
  ```

  `name` is the recipe row's `Name`, threaded straight into each producer's own name parameter.
- Rationale: `manifest/designs/shed-recipe.md` is explicit that the restriction to `ShedProducer`-implementing names exists so the builder is "registry lookup + fixed-signature constructor call, no reflection, no handling of arbitrary shapes."
  A uniform signature is what makes that true.
  The `error` return is required because several underlying constructors already fail (`NewBouncer`, `NewBurlerProducer`, `NewPublish`, `NewFinalize`), and because config validation happens inside the constructor.
- Rejected: `func(name string, cfg any) (shedengine.ShedProducer, error)` with per-engine typed config (forces a type switch or reflection in the loader — exactly what the design doc rules out);
  a signature without `error` (would force the four fallible constructors to panic).

### `Config` is a decoded `map[string]any`, validated per entry

- Decision: `Config` is a named type over `map[string]any`.
  The caller (the future loader) decodes the recipe file into it;
  each registry entry extracts and validates the keys it recognises, using small shared typed accessors (`configString`, `configStringSlice`, `configBool`, `configInt`) that report a clear error on a missing required key or a wrong type.
- Rationale: it keeps `internal/shedrecipe` free of the recipe file format — this task must not decide YAML shape, that is piece 2's scope, and an untyped map is the seam that lets piece 2 pick any format it wants.
  It also avoids a union struct carrying every engine's fields, which would grow a field set per new engine and leave eleven-twelfths of it nil at every call.
- Rejected: a union `Config` struct (rots on every added engine, no compile-time help anyway since the loader fills it dynamically);
  raw `[]byte` decoded per engine (couples the registry to the file format the loader owns);
  `map[string]string` (cannot express `Bouncer`'s `ArtifactPaths` list or an integer).

### `Config` is strict about unknown keys

- Decision: an entry returns an error when `cfg` contains a key that entry does not recognise.
  A shared `configReject Unknown(cfg, known...)` helper does the check, called at the end of every entry's extraction.
- Rationale: a mistyped recipe key must fail loud at build time, not silently produce a producer running on defaults.
  This matches the posture `CONSTRAINTS.md`'s **Config Strictness Invariant** takes for `configengine.Load`, and matches `stencil.Fill`, which already errors rather than substituting empty for a missing token.
- Rejected: ignoring unknown keys (a typo becomes a silent behaviour change in a file whose whole point is that humans edit it by hand).

### Live seams travel in a told `Env`, never in `Config`

- Decision: a single `Env` struct, filled once by whichever caller invokes the registry, carries both the resolved absolute paths and the injected non-serializable dependencies.
  Its shape:

  ```go
  type Env struct {
      // Geometry -- absolute paths, resolved by the caller.
      Cwd                string // Preflight
      AnchorPath         string // Batchifier, PlanValidate, Webster
      WorktreeRoot       string // PlanValidate
      StatusPath         string // LoomPreflight
      StatusLockPath     string // LoomPreflight
      StencilsDir        string // SingleLLM, Bouncer
      RunDir             string // Bouncer, BurlerRound
      DecisionRecordPath string // DiscussionValidate
      SupportLogPath     string // DiscussionValidate

      // Injected seams and already-resolved values.
      Shuttle     shedadapters.Shuttle
      Burler      shedadapters.BurlerRunner
      WebsterRun  shedadapters.WebsterRunner
      WebsterDeps websterengine.RunDeps
      Landing     landingshed.Deps
      Now         func() time.Time
  }
  ```

- Rationale: this is the gap `manifest/designs/shed-recipe.md` leaves open.
  The doc covers geometry ("never in a recipe, resolved once, centrally, by whichever caller invokes the recipe-builder") but says nothing about the *other* things the constructors need, which are neither paths nor static config:
  `shedadapters.Shuttle`, `shedadapters.BurlerRunner`, `shedadapters.WebsterRunner`, `websterengine.RunDeps`, `func() time.Time`, and `landingshed.Deps`'s three closures (`PushBranch`, `OpenFabric`, `OpenParentFabric`) plus its `modelspec.Registry`.
  None of those can be written in a recipe file at all.
  Putting them in the same told bundle as geometry extends the existing discipline rather than inventing a second one, and keeps the recipe portable exactly as the doc requires — the recipe still names no path and no mode.
- Rejected: extending `Config` to carry them (puts non-serializable values in the field the recipe file owns, destroying portability);
  package-level injection via a setter (mutable global, untestable in parallel);
  a variadic option list per entry (defeats the fixed signature).

### `landingshed.Deps` rides `Env` as a whole-struct passthrough

- Decision: `Env.Landing` is a `landingshed.Deps` value passed through unchanged to `landingshed.NewPublish` / `NewFinalize`.
  The `Publish` and `Finalize` registry entries therefore accept an empty `Config`.
- Rationale: `landingshed.Deps` already carries eleven fields, four of which are closures or seams, and it is already told wholesale by `loomshed.Deps.Landing` today (`internal/loomshed/loomshed.go`).
  Flattening it into `Env` would duplicate that field set and invite divergence — the exact reasoning `loomshed.Deps`'s own field doc gives for keeping it a single passthrough.
- Rejected: flattening `landingshed.Deps`'s fields into `Env` (duplication, divergence);
  making the landing fields recipe `Config` (they include closures).

### The `SingleLLM` entry builds its `SpecSource` from a stencil

- Decision: the `SingleLLM` entry constructs the `shedadapters.SpecSource` closure itself.
  Its `Config` keys are `stencil` (a `stencilstore` name), `output_files` (a list of worktree-relative paths), `model`, `effort`, `version`, `interactive`, `role`, and `tokens` (a `map[string]string` of static token values).
  The closure reads the stencil via `stencilstore.Read(env.StencilsDir, cfg.stencil)`, fills it via `stencil.Fill` with the static `tokens` merged over a fixed set of geometry-derived tokens supplied from `Env`, and returns a `shuttleengine.Spec`.
- Rationale: `shedadapters.NewSingleLLMProducer` has **no production caller today** — only `internal/shedadapters/singlellm_test.go` — so someone has to build the spec-composition step, and the registry is where the roadmap places `SingleLLMProducer`.
  `shuttleengine.Spec`'s own doc states shuttle is dumb transport and "the caller composes" the prompt, so composition belongs on this side of the seam.
  `shedadapters.Bouncer` already does exactly this pair of calls (`internal/shedadapters/bouncer.go:330-346`, `413-433`), so the pattern is established rather than invented.
- Rejected: leaving `SingleLLM` out of the registry until `loom: Discussion-Write producer` needs it (the roadmap names it explicitly as a registry entry, and piece 4 cannot convert a `Discussion-Write` row without it);
  taking a pre-built `SpecSource` from `Env` (there is one `Env` per run but many `SingleLLM` rows, each needing a different spec — the difference is precisely what `Config` is for).

### `Bouncer`'s `ReportName` closure is derived from a `Config` pattern string

- Decision: the `Bouncer` entry takes a `report_name` config key holding a format pattern with a single round placeholder, and builds the `func(round int) string` closure from it.
- Rationale: `shedadapters.BouncerConfig.ReportName` is a closure, which no recipe file can express;
  a pattern string is the serializable form of the same information.
- Rejected: a hardcoded report-name convention (different segments legitimately name their reports differently);
  putting the closure in `Env` (it is per-row, not per-run).

### `Stub` is a registry entry, despite not being in the roadmap's list

- Decision: register `Stub` alongside the eleven engines `manifest/roadmap.md` names.
- Rationale: five of loom's thirteen rows are still stubs — `Discussion-Write`, `Discussion-Review`, `Plan-Write`, `Plan-Review`, and `Webster-Review`.
  The roadmap's piece 4 says it "converts the list as it stands (including the still-stubbed `*-Write`/`*-Review` rows)", which is impossible without a `Stub` engine name.
  The omission from the roadmap's enumeration reads as an oversight in the enumeration, not a deliberate exclusion.
- Rejected: shipping only the eleven named engines (leaves piece 4 blocked on a follow-up).

### A coverage-guard test pins the registry against loom's current list

- Decision: a test in `internal/shedrecipe` asserts that every engine backing a row in `loomshed.New`'s list has a registry entry, keyed by an explicit engine-name-per-row table in the test.
- Rationale: the registry's whole reason to exist is that piece 4 can resolve every one of loom's rows.
  Without a guard, a row added to `loomshed` between now and piece 4 silently has no engine, and the gap surfaces only when piece 4 fails.
- Rejected: no guard (defers a known failure);
  reflecting over `loomshed.New`'s output at test time (the `ProducerDef` carries no engine name, so there is nothing to reflect on — the mapping genuinely has to be written down).

### `Preflight` is registered, but `loomshed.Deps.Preflight` stays

- Decision: add a `Preflight` registry entry wrapping `preflightshed.NewPreflight(name, env.Cwd)`, and leave `loomshed.Deps.Preflight`'s pre-injected field exactly as it is.
- Rationale: the injected field exists because `Preflight` is the only row that spawns git, and `loomshed` is not the layer that resolves cwd.
  Once a recipe drives the list, the registry entry supersedes the field — but that removal is piece 4's edit, made when there is a loader to replace it.
  Removing it here would break `loomshed.New` with nothing to put in its place.
- Rejected: removing `Deps.Preflight` in this task (breaks the only working consumer for no gain).

### New invariant recorded in `CONSTRAINTS.md`

- Decision: add a **Shed Recipe Registry Invariant** section stating that (a) every registry value constructs a `shedengine.ShedProducer` and nothing else — never an arbitrary Go module, and (b) `internal/shedrecipe` takes every absolute path from its caller and has no direct production import of `internal/lyxcwd`.
  Machine-enforce (b) with a `seam_enforcement_test.go` allowlist test in the new package, modelled on `internal/loomshed/seam_enforcement_test.go`.
- Rationale: `CLAUDE.md` requires a new cross-cutting invariant to be recorded in the same commit.
  The `ShedProducer`-only restriction is the design doc's central constraint and currently lives only in prose.
  The told-geometry property makes `shedrecipe` a new member of the **Told-Geometry Invariant**'s machine-enforced list, which that section's list must also be updated to name.
- Rejected: review obligation only (the repo's pattern for exactly this property is a machine guard, and eight packages already carry one).

### `manifest/designs/shed-recipe.md`'s DRAFT banner is narrowed, not removed

- Decision: rewrite the banner so it no longer says "do not implement from this doc yet", and instead marks pieces 2-4 as still-unsettled while recording piece 1 as built.
  Add the `Env` decision above to the doc's "What's never in a recipe" section, since the doc is currently silent on live seams.
- Rationale: the banner as written forbids this task outright, so it cannot survive the commit unchanged.
  Removing it entirely would over-claim — the recipe *file format* genuinely is still unsettled, and this task deliberately does not settle it.
- Rejected: deleting the banner (over-claims settledness for pieces 2-4);
  leaving it untouched (the doc would contradict the shipped code).

## Technical context

**The seam being registered.** `shedengine.ShedProducer` is a one-method interface — `Call(ctx) (Outcome, OutputPointer, error)` (`internal/shedengine/producer.go`).
`shedengine.ProducerDef` is `{Name, Producer, OnStuck, OnDone, Segment, MaxBounces}`.
The registry produces only the `Producer` field's value;
every other field is the loader's concern.

**The constructors to be wrapped**, with their current signatures:

| Engine name | Backing constructor | Notes |
| --- | --- | --- |
| `SingleLLM` | `shedadapters.NewSingleLLMProducer(name, SpecSource, Shuttle, now)` | no production caller today; `SpecSource` must be built here |
| `Bouncer` | `shedadapters.NewBouncer(BouncerConfig)` | fallible; `ReportName` is a closure; constructor does deliberate I/O (probes the rubric stencil) |
| `BurlerRound` | `shedadapters.NewBurlerProducer(name, BurlerRunner, burlerengine.Profile, burlerengine.RunOpts, runDir, now)` | fallible; `Profile` is buildable as a literal outside `burlerengine` — see `internal/burlercli/run.go:53` — despite its unexported `clusterLenses` field, which `Profile.validate` fills |
| `Webster` | `shedadapters.NewWebsterProducer(name, WebsterRunner, websterengine.RunDeps)` | `loomshed` also has its own lazy wrapper, `newWebsterProducer(name, anchorPath, run, deps)` (`internal/loomshed/webster.go:41`) — the registry entry wraps loom's lazy wrapper, since that is what row 10 actually uses |
| `Preflight` | `preflightshed.NewPreflight(name, cwd)` | already returns the interface |
| `Publish` | `landingshed.NewPublish(Deps)` | fallible; `Deps` passthrough |
| `Finalize` | `landingshed.NewFinalize(Deps)` | fallible; `Deps` passthrough |
| `LoomPreflight` | `loomshed.newLoomPreflight(name, statusPath, statusLockPath)` | to be exported |
| `Batchifier` | `loomshed.newBatchifier(name, anchorPath)` | to be exported |
| `DiscussionValidate` | `loomshed.newDiscussionValidate(name, decisionRecordPath, supportLogPath)` | to be exported |
| `PlanValidate` | `loomshed.newPlanValidate(name, anchorPath, worktreeRoot)` | to be exported |
| `Stub` | `loomshed.newStub(name)` | to be exported |

**In-repo registry precedent.** `internal/batcher/registry.go` — a package-global `map[string]Batcher`, `init()` self-registration from `identity.go`, and `Select(name)` returning `fmt.Errorf("batcher: unknown batcher %q", name)` for a miss.
This task copies the *lookup and error* shape and deliberately departs from the *registration* mechanism (see the decision above).
`CONSTRAINTS.md`'s **Batcher Registry+Config Invariant** documents that registry.

**Prompt-composition precedent.** `internal/shedadapters/bouncer.go:330-346` — `stencilstore.Read(stencilsDir, name)` for the template and the rubric, `stencil.StripLeadingComment` on the rubric, then `stencil.Fill(template, map[string]string{...})`.
`stencil.Fill` errors on a missing token rather than substituting empty, which is why `bouncer.go:394` passes the literal `"(none)"` for an empty value.
The `SingleLLM` entry follows the same three steps.

**Import-allowlist tests.** Every producer-hosting package carries one: `internal/loomshed/seam_enforcement_test.go` (`TestToldGeometryInvariant_AllowlistOnly`) is the closest model — it walks non-test `.go` files with `go/parser` in `parser.ImportsOnly` mode and fails any import outside an explicit allowlist map.
`internal/shedrecipe`'s allowlist will be the largest in the repo (it imports `shedengine`, `shedadapters`, `loomshed`, `landingshed`, `preflightshed`, `websterengine`, `burlerengine`, `shuttleengine`, `stencilstore`, `stencil`), which is expected — it is the wiring layer.
Note that `loomshed`'s own allowlist needs **no** change: the dependency runs `shedrecipe` → `loomshed`, never the reverse.

**Import-cycle check.** `loomshed` imports `shedengine`, `shedadapters`, `websterengine`, `loomengine`, `planparser`, `batcher`, `state`, `landingshed`.
None of those import `shedrecipe`, so `shedrecipe` importing `loomshed` is acyclic.

**Where the design doc is silent.** `manifest/designs/shed-recipe.md` covers geometry exclusion and the `ShedProducer` restriction but never addresses the injected seams (`Shuttle`, `BurlerRunner`, `WebsterRunner`, `RunDeps`, the clock, `landingshed.Deps`'s closures).
The `Env` decision above is this discussion's own resolution of that gap, not something the doc decided — and it is the single decision most worth re-examining if the loader task finds it awkward.

## Constraints

From `CONSTRAINTS.md`:

- **Shed Producer-Seam Invariant** — `internal/shedengine` production code imports only stdlib, `internal/state`, and `internal/lock`.
  Hard bar on putting the registry there;
  machine-enforced by `internal/shedengine/seam_enforcement_test.go`.
- **Told-Geometry Invariant** — an engine is handed the absolute paths it operates on and derives none of its own.
  `internal/shedrecipe` joins the machine-enforced list;
  the invariant's own enumeration of enforced packages must be updated in the same commit.
  The `Env` struct is the told bundle;
  no path is computed inside `shedrecipe`.
- **Config Strictness Invariant** — sets the repo's posture that config absence and config error are loud, not silently defaulted.
  Motivates the unknown-key rejection above.
  Note `shedrecipe` is *not* a member of this invariant's guard subject: it calls neither `configengine.Load` nor `LoadOrTemplate`, and reads no config file of its own.
- **Batcher Registry+Config Invariant** — the existing name-keyed-registry precedent; its lookup/error shape is reused.
- **Lyxdirs Single-Declarer Invariant** — `shedrecipe` must never write `_lyx` or `.lyx` in a path-construction context.
  Every path arrives via `Env`, so this is satisfied by construction, but it constrains any convenience helper anyone is tempted to add.
- **Documentation Lifecycle** — the module doc, `docs/overview.md`, and `CONSTRAINTS.md` all move in this commit.

From `CLAUDE.md`:

- A new cross-cutting invariant is recorded in `CONSTRAINTS.md` in the same commit.
- A task adding a module updates `manifest/designs/` and `docs/overview.md`'s module table in the same commit.
- `manifest/roadmap.md` moves on completing a planned item — this is one, so the item moves to Done.
- Markdown uses semantic line breaks, one sentence per line.

Discovered during exploration:

- `burlerengine.Profile` has an unexported field (`clusterLenses`), so it can only be built as a partial literal from outside the package — `internal/burlercli/run.go:53` shows this is already done and supported, with `Profile.validate` filling the rest.
- `shedadapters.NewBouncer` does I/O in the constructor (probing the rubric stencil, `bouncer.go:131`), so the `Bouncer` registry entry can fail on a missing stencil at construction time rather than at first call.
  Tests must supply a real stencils directory.

## Testing

TDD candidates, in order:

1. **Lookup and error shape** (`internal/shedrecipe`) — write first, it pins the public surface.
   Scenarios: a known name resolves to a non-nil constructor;
   an unknown name returns an error naming the offending string;
   an empty name is an error rather than a silent default (unlike `batcher.Select`, which defaults — there is no sensible default engine).
2. **Config accessors** (`internal/shedrecipe`) — write before any entry uses them.
   Scenarios per accessor: present and correctly typed;
   present but wrong type;
   absent-and-required;
   absent-and-optional falls back to the zero value.
   Plus the unknown-key rejector: a `Config` with one unrecognised key errors and names the key;
   a `Config` with only recognised keys passes.
3. **Per-entry construction**, one test per registry entry — twelve tests, each supplying a minimal valid `Config` plus a filled `Env` and asserting a non-nil `shedengine.ShedProducer` with no error.
   These are best driven as a table over the registry so a newly added entry without a test fails loudly.
   Cover the fallible entries' failure paths too: `Bouncer` with a missing rubric stencil, `Publish`/`Finalize` with a `landingshed.Deps` the underlying constructor rejects.
4. **`SingleLLM` spec composition** — the only entry with real logic of its own.
   Scenarios: the composed `Spec` carries the filled prompt, the resolved `OutputFiles`, and the `model`/`effort`/`version`/`interactive`/`role` values from `Config`;
   a stencil naming a token absent from `tokens` and absent from the geometry-derived set errors rather than filling empty;
   a missing stencil errors.
   Note the `SpecSource` is a closure — the test must call it, not merely construct the producer.
5. **`Bouncer` report-name pattern** — the pattern renders the expected filename for a given round;
   a pattern with no round placeholder is rejected at construction.
6. **Coverage guard** — every engine backing a row in `loomshed.New`'s current list has a registry entry.
7. **Import allowlist** (`internal/shedrecipe/seam_enforcement_test.go`) — mirrors `internal/loomshed/seam_enforcement_test.go`, asserting no production import outside the allowlist and specifically no `internal/lyxcwd`.

Scenarios that must not be missed:

- `loomshed`'s existing tests must still pass unchanged after the constructor renames — that is the proof the rename is behaviour-neutral.
  `internal/loomshed/sequence_test.go`, `loomshed_test.go`, and `resume_test.go` are the ones that exercise `New`'s assembled list.
- No test may write outside its own `t.TempDir()`, and the `Env` used in tests is built entirely from temp paths — a test accidentally passing a real repo path would mask a told-geometry violation.

Explicitly not tested here: anything about recipe file parsing, `ProducerDef` assembly, or routing-graph validity — none of that exists yet.

## Q&A log

- **Q:** Where does the registry live? **A:** [auto-pick] New `internal/shedrecipe` package. **Why:** `shedengine` is barred by the machine-enforced Shed Producer-Seam Invariant, and `shedadapters` cannot reach `loomshed`'s types without a cycle.
- **Q:** How do engines get into the registry — central table or `init()` self-registration? **A:** [auto-pick] Central `map[string]Constructor` table literal in one file. **Why:** entries span four packages, so `init()` registration would make registry membership depend on link-time blank imports;
  a table is greppable, deterministic, and has no mutable global.
- **Q:** What is the constructor signature? **A:** [auto-pick] `func(name string, cfg Config, env Env) (shedengine.ShedProducer, error)`. **Why:** the design doc requires "registry lookup + fixed-signature constructor call, no reflection";
  the `error` return is forced by the four already-fallible underlying constructors.
- **Q:** What type is `Config`? **A:** [auto-pick] A named type over `map[string]any`, decoded by the caller, validated per entry. **Why:** keeps this task out of the recipe file format (piece 2's scope) and avoids a union struct that grows a field set per engine.
- **Q:** Where do the live seams go — `Shuttle`, `WebsterRunner`, `modelspec.Registry`, the fabric-opener closures, the clock? **A:** [auto-pick] A told `Env` struct alongside geometry. **Why:** the design doc covers geometry but is silent on these;
  they are non-serializable so they cannot live in `Config` without destroying recipe portability.
- **Q:** Which engine names ship in this task? **A:** [auto-pick] All twelve, including `Stub`. **Why:** the roadmap omits `Stub` from its enumeration, but piece 4 converts loom's list including its five stub rows, which is impossible without a `Stub` engine.
- **Q:** Is there a coverage guard against loom's current list? **A:** [auto-pick] Yes, a test keyed by an explicit engine-name-per-row table. **Why:** without it, a row added before piece 4 lands silently has no engine and the gap surfaces only when piece 4 fails.
- **Q:** Does this task record a new invariant in `CONSTRAINTS.md`? **A:** [auto-pick] Yes — a Shed Recipe Registry Invariant, machine-enforced for the told-geometry half. **Why:** `CLAUDE.md` requires a new cross-cutting invariant in the same commit, and the `ShedProducer`-only restriction currently lives only in design-doc prose.
- **Q:** Does `Preflight` stop being pre-injected into `loomshed.Deps`? **A:** [auto-pick] No — register the engine, leave the field. **Why:** removing the field breaks `loomshed.New` with no loader yet to replace it;
  the removal belongs to piece 4.
- **Q:** How are unknown `Config` keys handled? **A:** [auto-pick] Strict — an unrecognised key is a construction error. **Why:** a typo in a hand-edited recipe must fail loud, matching the Config Strictness Invariant's posture and `stencil.Fill`'s own missing-token behaviour.
- **Q:** What happens to `manifest/designs/shed-recipe.md`'s "DRAFT — do not implement from this doc yet" banner? **A:** [auto-pick] Narrow it to pieces 2-4 and record piece 1 as built. **Why:** the banner as written forbids this task, so it cannot survive unchanged;
  removing it entirely would over-claim, since the recipe file format genuinely is still unsettled.
- **Q:** How does `loomshed` expose its five unexported constructors? **A:** [auto-pick] Export them in place, returning `shedengine.ShedProducer` rather than the unexported concrete type. **Why:** duplicating them in `shedrecipe` guarantees divergence, and moving the loom-specific producers out contradicts the design doc's point that a bespoke single-consumer engine is a valid registry entry precisely because it stays where it lives.
