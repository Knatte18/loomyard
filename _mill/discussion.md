# Discussion: Shed recipe: loader/builder

```yaml
task: 'Shed recipe: loader/builder'
slug: shed-recipe-loader-builder
status: discussing
parent: main
```

## Problem

`internal/loomshed.New()` builds loom's thirteen-row `[]shedengine.ProducerDef` as a hand-written Go literal (`internal/loomshed/loomshed.go`, the `producers := []shedengine.ProducerDef{...}` block).
Every row's name, engine, routing (`OnDone`/`OnStuck`), and bounce budget is compiled in, so changing loom's phase machine means editing Go and rebuilding.
`manifest/designs/shed-recipe.md` proposes replacing that literal with a declarative **recipe** — a data file naming, per row, `{Name, Engine, Config, OnDone, OnStuck, MaxBounces}` — loaded and assembled into the same `[]shedengine.ProducerDef` that `shedengine.Shed` already consumes, with no change to `shedengine` itself.

That design splits into four pieces.
Piece 1, the engine registry, shipped as `internal/shedrecipe` (commit `256a6d2b`): twelve engine names, each mapped to a fixed-signature `Constructor func(name string, cfg Config, env Env) (shedengine.ShedProducer, error)`, plus the `Config`/`Env` split that keeps geometry and live seams out of the portable half.
Piece 3, the validity checker, shipped as `internal/shedcheck` (commit `fbb93da4`).
This task is **piece 2**: the recipe file format, the loader that decodes it, and the builder that turns decoded rows plus a caller-supplied `shedrecipe.Env` into `[]shedengine.ProducerDef`.

**Why now:** the five `loom: real LLM producers` roadmap tasks are deliberately sequenced *after* the Shed-recipe group so that each of them authors its rows directly in recipe form rather than as a Go literal that then needs converting.
Piece 2 is the only piece still missing before piece 4 (`loom: convert to a Shed recipe`) can run, and piece 4 in turn gates all five of those.
Nothing existing breaks in the meantime — loom's hardcoded list keeps working — but every downstream task is blocked behind this one.

## Scope

**In:**

- A new package `internal/shedbuild`, sibling to `internal/shedrecipe` (registry) and `internal/shedcheck` (checker), importing `shedrecipe` and `shedengine` one-way.
- The recipe **file format**: a YAML document with `version`, `entry`, `terminals`, and `producers`, where each producer row carries `name`, `engine`, `config`, `on_done`, `on_stuck`, `segment`, `max_bounces`.
- A **loader**: `Parse(data []byte) (Recipe, error)` as the core, plus `Load(path string) (Recipe, error)` as a thin told-path wrapper.
- A **builder**: `Build(r Recipe, env shedrecipe.Env) ([]shedengine.ProducerDef, error)`, resolving each row's `engine` through `shedrecipe.Lookup` and calling the returned `Constructor` with the row's `name`, its `config` as a `shedrecipe.Config`, and the caller's `env`.
- A thin authoring-time convenience `Check(r Recipe, producers []shedengine.ProducerDef) []shedcheck.Finding`, forwarding the recipe's told `Entry`/`Terminals` to `shedcheck.Check`.
- Tests: table-driven parse/build coverage over `testdata/` fixtures, an equivalence test against `loomshed.New`'s live literal, and a told-geometry seam-enforcement test.
- Docs, same commit: `manifest/designs/shed-recipe.md` (header banner + piece-2 section + the `Segment` correction), `docs/overview.md` module table and shed narrative, `CONSTRAINTS.md` enforcement list, `manifest/roadmap.md` item moved to Done.

**Out:**

- **Converting `internal/loomshed`'s own list to a real recipe file** — that is piece 4, its own roadmap item (`loom: convert to a Shed recipe`), sequenced immediately after this one.
  This task ships no production recipe file at all; the only recipe documents it adds are `testdata/` fixtures.
- Any change to `internal/shedengine` — `ProducerDef`, `Shed`, `validate()`, and `Run` are untouched.
- Any change to `internal/shedrecipe` — the registry, `Config`, `Env`, and all twelve entries are consumed as-is, with no new exported surface added to that package.
- Any change to `internal/shedcheck`.
- Deciding **where recipe files live on disk** (embedded via `go:embed`, seeded like `internal/stencilstore`, or read from `_lyx/`): `Load` takes a told absolute path and derives nothing; piece 4 picks.
- Any new engine registered in `shedrecipe`'s registry.
- Any wiring change in `internal/loomcli` — `wire()` and `loomshed.Deps` stay as they are.
- Run-wide `Shed.MaxBounces` — stays a caller concern, not a recipe field.
- `shuttleengine.Spec` fields not already recipe-authorable in `shedrecipe`'s `SingleLLM` entry (`Timeout`, `ForkSubagents`, `KeepPane`, `Round`, `Parent`, `Display`).

## Decisions

### New package `internal/shedbuild` rather than new files in `internal/shedrecipe`

- **Decision:** the loader/builder is its own package, `internal/shedbuild`, importing `internal/shedrecipe`, `internal/shedengine`, `internal/shedcheck`, `gopkg.in/yaml.v3`, and stdlib.
  `internal/shedrecipe` gains no new exported identifier and no new import.
- **Rationale:** `internal/shedrecipe/doc.go` states as a property of that package that `Config` is "the recipe row's own static, already-decoded configuration … which this package never learns the file format of," and `registry.go`'s `Names()` doc says it exists "so the coverage guard and the future recipe loader both have a stable enumeration and *neither reaches into registry directly*."
  Both sentences are written from the assumption that the loader is an outside caller.
  Putting a YAML decoder inside `shedrecipe` falsifies the first and makes the second pointless.
  A separate package also keeps `shedrecipe`'s existing seam-enforcement test meaningful without having to carve out a format-decoding exception.
- **Rejected:** new files inside `internal/shedrecipe` (falsifies two documented package properties, and forces `gopkg.in/yaml.v3` into the registry's import set);
  inside `internal/loomshed` (loomshed is one consumer of the mechanism, not its owner — a future sibling product's own list would have to import loomshed to get a loader).

### Package name `shedbuild`

- **Decision:** `internal/shedbuild`.
- **Rationale:** the repo's shed-stack naming is a `shed`/`*shed` prefix family — `shedengine`, `shedadapters`, `shedrecipe`, `shedcheck`, `loomshed`, `landingshed`, `preflightshed`.
  `shedbuild` reads as "builds a shed producer list" and sits naturally next to `shedcheck` ("checks one").
- **Rejected:** `shedload` (names only the loader half of a loader/builder);
  `shedrecipeload` (over-long, and no other package in the family compounds two prefixes).

### YAML as the recipe file format

- **Decision:** YAML, decoded with `gopkg.in/yaml.v3` — already an existing module dependency.
- **Rationale:** every configuration surface in this repo is YAML (`internal/configengine`, `internal/yamlengine`, and the `//go:embed template.yaml` pattern in `fabricengine`, `shuttleengine`, `websterengine`, `reedengine`, `burlerengine`, `landingshed`, `modelspec`, `batcher`, `boardengine`, `loomengine`).
  `manifest/designs/shed-recipe.md` names YAML as the likely format ("YAML, format TBD").
  Decisively: `shedrecipe.configInt`'s own doc comment says "a YAML decoder into `any` yields `int` while a JSON-shaped one yields `float64`, and **piece 2 picks the format**, so accepting both without truncating keeps this package format-agnostic" — piece 1 was written anticipating this choice, and `yaml.v3` decoding a mapping into `any` yields `map[string]any`, which is exactly `shedrecipe.Config`'s underlying type.
- **Rejected:** JSON (no comments, and every other config file in the repo is YAML);
  TOML (new dependency, no precedent).

### Byte-slice core with a told-path wrapper

- **Decision:** `Parse(data []byte) (Recipe, error)` is the real loader.
  `Load(path string) (Recipe, error)` reads the file and delegates, and is the only function in the package that touches the filesystem.
- **Rationale:** keeps every parse and build test filesystem-free, and keeps the package's told-geometry story trivial — `Load` reads exactly the absolute path it is handed and derives nothing.
  It also leaves piece 4 free to feed an embedded `[]byte` straight to `Parse` with no temp file.
- **Rejected:** path-only `Load` (forces a `t.TempDir()` into every table case);
  `io.Reader` (nothing in this repo streams config, and a `[]byte` is what `go:embed` produces).

### `Build` returns `[]shedengine.ProducerDef`, not a `*shedengine.Shed`

- **Decision:** `Build(r Recipe, env shedrecipe.Env) ([]shedengine.ProducerDef, error)`.
  The caller assembles the `shedengine.Shed` itself, filling `StatusPath`, `LockPath`, `StatusLockPath`, and `MaxBounces` from its own told geometry.
- **Rationale:** exactly what `manifest/designs/shed-recipe.md` and the roadmap item specify ("assembles the `[]shedengine.ProducerDef` list `shedengine.Shed` already consumes unchanged").
  Returning a `Shed` would require `shedbuild` to take the three Shed-level absolute paths, widening its geometry surface for no gain — `internal/loomcli/wiring.go` already has all three in hand.
- **Rejected:** returning a built `*shedengine.Shed` (pulls Shed-level geometry into this layer, and would make `shedbuild` a second place where a Shed is constructed).

### Recipe documents carry `entry` and `terminals`

- **Decision:** the recipe document has top-level `entry: <producer name>` and `terminals: [<producer name>, ...]`, surfaced on the `Recipe` value as `Entry string` and `Terminals []string`.
  They are metadata about the graph, never consumed by `Build`.
- **Rationale:** `internal/shedcheck`'s package doc is explicit that both endpoints are told, never inferred: "Check takes entry and terminals as explicit arguments rather than deriving them from the producer list.
  Shed has no entry field and no terminal field of its own — defaulting either to `Producers[0]` would re-introduce the positional routing meaning `internal/shedengine/doc.go` explicitly disclaims."
  Once loom's list lives in a file rather than in Go, the file is the only remaining place a human can write those two facts down.
- **Rejected:** omitting them (piece 4 would then have to hardcode loom's entry and terminals in a Go test, splitting one graph's description across two files);
  inferring `entry` from `producers[0]` (directly contradicts `shedengine/doc.go`'s "list order is display and enumeration order only").

### `segment` **is** a recipe row field — correcting `shed-recipe.md`

- **Decision:** a recipe row carries an optional `segment` string, mapped straight onto `shedengine.ProducerDef.Segment`.
  `manifest/designs/shed-recipe.md`'s "**Not in a recipe row: `Segment`**" paragraph is rewritten in this same commit to record the reversal and its reason.
- **Rationale:** the design doc's argument is that "leaving every row's `Segment` unset is already a no-op in `shedengine.validate()` today (the check is `segmentByName[p.OnStuck] != p.Segment`, which is always false when every producer's Segment is `""`)."
  That holds only while *every* row leaves it unset.
  Three already-planned roadmap items break that premise explicitly: `loom: Discussion-Review producer` specifies "Both rows share `Segment: "Discussion-Review"`", and `loom: Plan-Review producer` and `loom: Webster-Review producer` each specify the same shape for their own pair.
  Those five tasks are sequenced *after* this group precisely so they can author their rows in recipe form — which they cannot do if the format has no way to express a field their own roadmap entries require.
  Worse, `shedengine.validate()` enforces that a non-empty `OnStuck` names a target sharing the producer's `Segment`, so a mixed list (recipe rows at `""`, hand-wired rows at `"Discussion-Review"`) would fail validation at run time, not authoring time.
  The doc's other point — that `Segment`'s old cross-segment-wiring job is superseded by `shedcheck` — stays true and is not affected;
  what survives is `Segment` as a display/grouping label, which is exactly how the roadmap items use it.
- **Rejected:** keeping `segment` out and dropping it from the three roadmap items (throws away the segment grouping the whole `Bouncer`+`Burler` "perch" framing in `CLAUDE.md` and `manifest/designs/loom.md` is described in terms of);
  a document-level `segments:` grouping block that assigns rows to segments (a second way to say the same thing, and it separates a row's segment from the row, which is where `ProducerDef` keeps it).

### `Build` never calls `shedcheck.Check`; a separate `Check` helper exists

- **Decision:** `Build` performs no reachability, cycle, or blind-gate analysis.
  The package ships `Check(r Recipe, producers []shedengine.ProducerDef) []shedcheck.Finding`, a two-line forward to `shedcheck.Check(producers, r.Entry, r.Terminals)`, intended for a caller's own test suite.
- **Rationale:** `internal/shedcheck/doc.go` has a whole section headed "Nothing in production calls Check," ending "A caller assembling a producer list is expected to call Check in its own test suite, at authoring time, before that list ever reaches a Shed."
  Its stated reason applies verbatim here: reachability-from-entry is the wrong question for a resumed run that legitimately starts mid-graph.
  The helper exists only so the caller does not have to re-derive the argument order, and so `Recipe`'s `Entry`/`Terminals` have a documented consumer.
- **Rejected:** `Build` running `Check` and failing on findings (would make a legitimately resumable graph un-buildable, and contradicts a documented invariant of `shedcheck`);
  no helper at all (leaves `Entry`/`Terminals` on `Recipe` with no consumer in the package that defines them).

### Strict unknown-key rejection and a required `version`

- **Decision:** unknown keys are errors, at both the document level and the row level.
  The document requires `version: 1`;
  any other value is an error naming the value and the supported set.
- **Rationale:** mirrors `shedrecipe.configRejectUnknown`, which every one of the twelve registry entries already ends its key extraction with — the format's two halves should not disagree about strictness.
  A lenient loader silently swallows `on-done` for `on_done`, producing a row with an empty `OnDone` that `shedengine` reads as "end the whole run quietly."
  That failure is silent, and it is exactly the class of defect a declarative format introduces that a Go literal could not.
  The `version` gate costs one field and makes a future format change a loud error rather than a misparse.
- **Rejected:** strict keys with no `version` (a later format change has no way to fail loud on an old file);
  lenient (ignore unknown keys) — the silent-typo failure above.

### Validation split: shape here, routing elsewhere

- **Decision:** `Parse` validates document and row *shape* — `version` present and supported, `entry` non-empty, `terminals` non-empty, `producers` non-empty, and per row: `name` non-empty, `name` unique across the document, `engine` non-empty, `max_bounces` not negative, `config` a mapping if present.
  `Build` additionally resolves each `engine` through `shedrecipe.Lookup` and surfaces each `Constructor`'s own error.
  Neither validates that `on_done`/`on_stuck` name existing rows, that segments agree, that the graph is acyclic, or that every row is reachable.
- **Rationale:** `shedengine.validate()` already rejects dangling `OnDone`/`OnStuck`, self-referencing `OnDone`, duplicate names, negative `MaxBounces`, and cross-segment `OnStuck`, with its own distinct per-rule messages, before `Run` touches a lock or a producer.
  `shedcheck.Check` already reports unreachable rows, done cycles, blind gates, and unexpected terminals.
  A third copy in `shedbuild` is what drifts out of sync with those two.
  What `shedbuild` does own is everything neither of the other two can see: the file's own shape, and the mapping from an engine *name* to a constructor.
  Duplicate-`name` detection is the one deliberate overlap with `shedengine.validate()`, because a duplicate name in a recipe would otherwise silently shadow a row at parse time in a way the file's author cannot see.
- **Rejected:** duplicating dangling-target and segment checks in `Build` (three copies of one rule);
  validating nothing in `Parse` and deferring everything to `Run` (a malformed file would then fail at run time with a `shedengine:`-prefixed message that names no file and no line).

### First-error, not aggregated errors

- **Decision:** every function returns on its first error, wrapped with enough context to locate it: the row's list index and its `name` where a row is involved, and a `shedbuild: ` prefix throughout.
- **Rationale:** matches `shedrecipe`'s `config*` accessors and `shedengine.validate()`, both strictly first-error.
  `shedcheck` is the one report-everything surface in this stack, and it is deliberately a separate authoring-time tool rather than part of a constructor.
- **Rejected:** aggregating into a multi-error (inconsistent with both neighbours, and `errors.Join` output is hard to assert against in a table test).

### No document-level `max_bounces`

- **Decision:** `max_bounces` is a per-row field only.
  The recipe has no run-wide bounce-budget default;
  `shedengine.Shed.MaxBounces` stays entirely a caller concern.
- **Rationale:** YAGNI.
  `internal/loomcli/wiring.go` deliberately leaves `loomshed.Deps.MaxBounces` at zero, with a comment explaining that this selects `shedengine`'s own internal default of ten, and no caller wants to override it.
  Adding a recipe field for a value nobody sets invites a second place for the default to live.
- **Rejected:** an optional document-level `max_bounces` (speculative, and would need its own "0 means inherit" semantics documented a third time).

### `shedbuild` defines no on-disk location for recipes

- **Decision:** `Load` takes a told absolute path.
  The package declares no directory constant, no filename convention, and no embedded default.
- **Rationale:** the Told-Geometry Invariant — an engine is handed the absolute paths it operates on and derives none of its own.
  Choosing between `go:embed`, a `stencilstore`-style seed-and-reconcile, and a plain `_lyx/` file is piece 4's decision, taken with loom's real recipe in hand;
  pre-committing it here would be guessing.
- **Rejected:** defining a `_lyx/recipes/` convention now (derives geometry, and pre-empts piece 4).

## Technical context

### What already exists and is consumed as-is

**`internal/shedrecipe`** (`internal/shedrecipe/`) — piece 1, shipped.
Everything `shedbuild` needs from it is already exported:

- `type Constructor func(name string, cfg Config, env Env) (shedengine.ShedProducer, error)` (`recipe.go`).
- `type Config map[string]any` (`recipe.go`) — the row's decoded config map.
  A YAML mapping decoded into `any` by `yaml.v3` is a `map[string]any`, so the conversion in `Build` is `shedrecipe.Config(row.Config)` with no copying or key walking.
- `type Env struct` (`recipe.go`) — nine told absolute-path roots (`Cwd`, `AnchorPath`, `WorktreeRoot`, `StatusPath`, `StatusLockPath`, `StencilsDir`, `RunRoot`, `DecisionRecordPath`, `SupportLogPath`) plus six injected seams (`Shuttle`, `Burler`, `WebsterRun`, `WebsterDeps`, `Landing`, `Now`).
  `shedbuild` passes it through opaquely and never reads a field.
- `func Lookup(name string) (Constructor, error)` (`registry.go`) — errors on an empty name (deliberately no default engine) and on an unknown name.
  `Build` calls this and wraps its error with the row index and name.
- `func Names() []string` (`registry.go`) — sorted, freshly allocated per call.
  Useful for an error message listing the valid engines, and for a test asserting every registered engine is exercisable from a recipe.

The twelve registered engine names are `Preflight`, `Publish`, `Finalize`, `LoomPreflight`, `Batchifier`, `DiscussionValidate`, `PlanValidate`, `Stub`, `Webster`, `SingleLLM`, `Bouncer`, `BurlerRound`.
Nine of them (`entries_simple.go`) take an empty `Config` and call `configRejectUnknown(cfg)` with no known keys, so any `config:` block at all on those rows is an error from the constructor.
`SingleLLM` (`entries_singlellm.go`) takes `stencil`, `output_files`, `model`, `effort`, `version`, `role`, `interactive`, `tokens`.
`Bouncer` and `BurlerRound` have their own key sets in `entries_bouncer.go` / `entries_burler.go`, including nested maps that `configMap` feeds back through the same accessors.

**`internal/shedengine`** — `ProducerDef` (`producer.go`) has exactly the six fields a recipe row maps to: `Name`, `Producer`, `OnStuck`, `OnDone`, `Segment`, `MaxBounces`.
`validate()` (`validate.go`) is called by `Run` before it touches a lock, a file, or a producer, and rejects: empty `StatusPath`/`LockPath`/`StatusLockPath`, equal lock paths, negative `Shed.MaxBounces`, empty `Producers`, empty row `Name`, nil row `Producer`, duplicate row `Name`, negative row `MaxBounces`, `OnDone`/`OnStuck` naming no producer, `OnDone` naming itself, and `OnStuck` naming a producer in a different `Segment`.
Note the deliberate asymmetry documented there: `OnStuck: <self>` is legal (budgeted), `OnDone: <self>` is not (statically certain infinite loop).

**`internal/shedcheck`** — `Check(producers []shedengine.ProducerDef, entry string, terminals []string) []Finding`, and `Finding{Kind, Producer, Target, Message}` with eight `Kind` constants.
It never dereferences `Producer`, so a `ProducerDef` with a nil `Producer` is valid input — which means an authoring-time structural test can run against a recipe's routing without building any producer at all, if a future caller wants that.

**`internal/loomshed/loomshed.go`** — the thirteen-row literal this format must be able to express, and the equivalence test's fixture source.
Its thirteen name constants (`NamePreflight` … `NameFinalize`) are the on-disk `current_producer` identities;
its doc comment enumerates every row's backing, `OnStuck`, and `OnDone`.
No row sets `Segment` or `MaxBounces` today.

**`internal/shedrecipe/coverage_guard_test.go`** — carries `loomRowEngines`, a hand-written map from each of loom's thirteen row *names* to the engine name backing it (`"Discussion-Write": "Stub"`, `"Loom-Preflight": "LoomPreflight"`, and so on).
That map is the exact row-name-to-engine correspondence the equivalence-test fixture must reproduce, and reading it is the fastest way to author that fixture correctly.
Its fake-building helpers (`coverageGuardFakePreflight`, `coverageGuardFakeMergeShuttle`, `coverageGuardNilFabricOpener`, `coverageGuardLandingDeps`) are the pattern for constructing a `shedrecipe.Env` and a `loomshed.Deps` good enough for both sides of the equivalence test to build — note they live in package `shedrecipe`, so `shedbuild` needs its own copies rather than an import.

**`internal/loomcli/wiring.go`** — the future caller.
`wire()` is where every told absolute path is already in hand (`location.AnchorPath()`, `location.WorktreePath()`, `loomengine.LoomStatusFile(location)`, etc.) and where a `shedrecipe.Env` would be filled in piece 4.
This task does not touch it, but it is the shape the `Env`-filling story has to land in.

### Gotchas found while exploring

- **`yaml.v3` decoding into `any`.** A YAML mapping decodes to `map[string]any` (unlike `yaml.v2`, which produced `map[interface{}]interface{}`), and an integer scalar decodes to `int`.
  Both are what `shedrecipe`'s accessors expect.
  Decoding the `config:` block must go through `any`/`map[string]any`, *not* through a typed struct, or the whole point of `Config map[string]any` is lost.
- **`config:` must survive as an untouched sub-map.** The row struct's `Config` field needs to be `map[string]any`, and the loader must not normalise, lowercase, or walk its keys — each registry entry owns its own key validation, and `configRejectUnknown` is what reports a typo.
  A loader that pre-filtered keys would break that.
- **An absent `config:` is a nil map, and that is correct.** `configRejectUnknown` returns nil for a nil or empty `Config`, and every `configString`/`configInt`/etc. treats a missing key as absent.
  So a row with no `config:` block is legal input for the nine simple entries and an error for `SingleLLM`, which requires `stencil` and `output_files` — with the error coming from the entry, not the loader.
- **Producer identity is not comparable.** The equivalence test cannot compare `ProducerDef.Producer` values.
  Compare the five data fields (`Name`, `OnDone`, `OnStuck`, `Segment`, `MaxBounces`) literally, and the producer's concrete type via `reflect.TypeOf` or `%T`.
- **`Publish` and `Finalize` discard the row name.** Both `publishEntry` and `finalizeEntry` take `_ string` — `landingshed.Deps` carries no name field and their identities are package constants inside `landingshed`.
  So for those two rows, the recipe's `name` sets `ProducerDef.Name` but does *not* reach the producer.
  That is existing, documented behaviour (`entries_simple.go`), not something `shedbuild` should try to reconcile.
- **`shedrecipe.Lookup("")` is already an error**, with its own message explaining there is deliberately no default engine.
  `Parse` still rejects an empty `engine` at parse time so the error names the file position rather than surfacing from the registry.
- **`shedcheck.Check` returns `nil`, not an empty non-nil slice, on a clean graph** — assert `len(findings) == 0` or `findings == nil`, and do not build an expectation around an empty non-nil slice.

## Constraints

From `CONSTRAINTS.md`:

- **Told-Geometry Invariant.** `internal/shedbuild` must take every absolute path from its caller and have no direct production import of `internal/lyxcwd`.
  `Load(path)` reads the told path;
  nothing else in the package touches the filesystem or constructs a path.
  This is the invariant the new `seam_enforcement_test.go` machine-enforces, and `CONSTRAINTS.md`'s machine-enforced list gains `internal/shedbuild` in the same commit.
- **Shed Recipe Registry Invariant.** Every registry value constructs a `shedengine.ShedProducer` and nothing else, and the registry is reached only through `Lookup` and `Names`, never by `init()` self-registration and never by a runtime `Register` call.
  `shedbuild` must reach the registry only through `Lookup`/`Names`, and must not add any registration mechanism of its own — a recipe naming an unregistered engine is an error, never a reason to register one.
- **Shed Producer-Seam Invariant.** `internal/shedengine`'s import allowlist is stdlib + `internal/state` + `internal/lock`.
  `shedbuild` must not be imported by `shedengine`, and this task adds no import to `shedengine` at all.
- **Documentation Lifecycle** (`CLAUDE.md`'s "Task completion — docs land in the same commit").
  A task adding a module updates the module doc in `manifest/designs/`, `docs/overview.md` if the module table or execution stack changes, and `CONSTRAINTS.md` for any new cross-cutting invariant — all in the same commit.
  `manifest/roadmap.md` moves because this completes a planned item.
- **Markdown: semantic line breaks.** One sentence per line, plus breaks at internal independent-clause boundaries;
  never a fixed-column hard wrap.
  Applies to every `.md` file this task touches.
- **Cwd Resolution Invariant.** Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned outside `internal/lyxcwd` and `cmd/lyx/main.go`.
  `shedbuild` calls neither.

Discovered during discussion:

- **No new exported surface on `internal/shedrecipe`.** Everything `shedbuild` needs (`Constructor`, `Config`, `Env`, `Lookup`, `Names`) is already exported.
  If the implementation finds it needs something more, that is a signal the split is wrong, not a licence to widen `shedrecipe`.
- **`gopkg.in/yaml.v3` only** — no new module dependency.

## Testing

TDD candidates are marked **[TDD]**: write the test first, watch it fail, then implement.

### `internal/shedbuild` — `Parse` **[TDD]**

Table-driven over inline YAML byte-slice literals (no filesystem).
Each case asserts either the decoded `Recipe` value or a substring of the error message.
Scenarios that must be covered:

- A minimal well-formed document round-trips: `version`, `entry`, `terminals`, and one producer row with every field set, decoding to the expected `Recipe`.
- A row with no `config:` block yields a nil `Config`, not an empty non-nil map.
- A row's `config:` sub-map survives untouched, including a nested mapping (the `BurlerRound` `profile` shape) and a list value (`output_files`), with key names and value types unaltered.
- Row order in the file is preserved exactly in `Recipe.Producers`.
- Malformed YAML (a tab-indented line, an unclosed quote) errors with a `shedbuild: ` prefix.
- Missing `version`, and a `version` other than `1`, each error naming the offending value.
- An unknown document-level key errors naming that key.
- An unknown row-level key errors naming that key *and* the row.
- Empty `entry`, empty `terminals`, and empty `producers` each error distinctly.
- Per row: empty `name`, empty `engine`, negative `max_bounces`, and a `config:` that is a scalar or list rather than a mapping, each error naming the row's index and name.
- A duplicate `name` across two rows errors naming the name and both indices.
- Determinism: the same input parsed twice yields equal results, and an unknown-key error names the same key both times (guard against map-iteration-order nondeterminism in the rejection message — `shedrecipe.configRejectUnknown` sorts for exactly this reason).

### `internal/shedbuild` — `Load` **[TDD]**

Small: a `t.TempDir()` file parses identically to `Parse` on the same bytes;
a missing path errors;
a directory path errors.
Everything else is `Parse`'s job.

### `internal/shedbuild` — `Build` **[TDD]**

Table-driven over `Recipe` values built in Go (not parsed), so a `Build` failure is never a `Parse` failure in disguise.
Scenarios:

- A single `Stub` row builds to one `ProducerDef` with the row's `Name`, `OnDone`, `OnStuck`, `Segment`, and `MaxBounces` copied through and a non-nil `Producer`.
- An unknown `engine` errors, and the message names the row index, the row name, and the offending engine.
- A row whose constructor fails — a `Preflight` row with an empty `Env.Cwd`, and a `SingleLLM` row missing its required `stencil` key — surfaces the constructor's own error, wrapped with the row index and name.
- A `config:` block on one of the nine empty-config entries errors (proving the block is forwarded to the constructor rather than dropped).
- Every one of the twelve names in `shedrecipe.Names()` is buildable from a recipe given a sufficiently filled `Env` — driven off `Names()` rather than a local list, so a thirteenth registered engine fails this test until the fixture covers it.
- Build order matches recipe order.
- `Build` on a recipe with an empty `Producers` slice errors (defence in depth even though `Parse` rejects it, since `Build` accepts hand-built `Recipe` values).

### `internal/shedbuild` — loom-equivalence **[TDD]**, the key test

A `testdata/` recipe fixture hand-authoring loom's thirteen rows, built with a `shedrecipe.Env` and compared against `loomshed.New(deps)`'s live output:

- Row count matches.
- For each index: `Name`, `OnDone`, `OnStuck`, `Segment`, and `MaxBounces` are equal.
- For each index: the concrete type of `Producer` is equal (`reflect.TypeOf`), proving the right engine was selected — the `loomRowEngines` map in `internal/shedrecipe/coverage_guard_test.go` is the correspondence to author the fixture from.
- The fixture's `entry` and `terminals`, fed through the package's `Check` helper against the *built* list, produce zero findings.

This test is what makes the claim "the format can express loom's real list" checkable, and it fails loudly if a future `loomshed` change outgrows the format — which is the regression worth catching, since piece 4 depends on it.
Both sides need a `Preflight` fake, a `mergeresolve.Shuttle` fake, and a `landingshed.Deps` filled with told temp-dir paths and typed-nil fabric openers;
`internal/shedrecipe/coverage_guard_test.go` and `internal/loomshed/fixture_test.go` are the two patterns to copy.

### `internal/shedbuild` — `Check` helper

Thin: one clean-graph case returning no findings, and one deliberately broken graph (a dangling `on_done`) returning the expected `shedcheck.Kind`.
`shedcheck`'s own behaviour is already exhaustively tested in its own package and must not be re-tested here.

### `internal/shedbuild/seam_enforcement_test.go` — `TestToldGeometryInvariant_AllowlistOnly`

Copy the shape from `internal/shedrecipe/seam_enforcement_test.go`.
Allowlist: stdlib, `gopkg.in/yaml.v3`, `github.com/Knatte18/loomyard/internal/shedrecipe`, `github.com/Knatte18/loomyard/internal/shedengine`, `github.com/Knatte18/loomyard/internal/shedcheck`.
`internal/lyxcwd` must be absent from the production import set.

### Regression surface

`go build ./... && go test ./...` at the repo root.
No existing test should change: this task adds a package and touches no existing production file.
If an existing test *does* need editing, that is a signal the change leaked outside its scope.

## Q&A log

- **Q:** Where does the loader/builder live — a new package, inside `internal/shedrecipe`, or inside `internal/loomshed`? **A:** [auto-pick] New package `internal/shedbuild`. **Why:** `shedrecipe/doc.go` states as a package property that it "never learns the recipe file's format," and `registry.go`'s `Names()` doc says the future loader "neither reaches into registry directly" — both sentences assume the loader is an outside caller, and putting a YAML decoder inside `shedrecipe` falsifies them.
- **Q:** Recipe file format — YAML, JSON, or TOML? **A:** [auto-pick] YAML via `gopkg.in/yaml.v3`. **Why:** every config surface in the repo is YAML, and `shedrecipe.configInt`'s own doc comment anticipates this choice explicitly ("piece 2 picks the format"); `yaml.v3` decodes a mapping into `map[string]any`, exactly `shedrecipe.Config`'s underlying type.
- **Q:** Loader input surface — bytes plus a path wrapper, path-only, or `io.Reader`? **A:** [auto-pick] `Parse([]byte)` core with a thin `Load(path)` wrapper. **Why:** keeps every parse and build test filesystem-free, keeps the told-geometry story trivial, and lets piece 4 feed an embedded `[]byte` straight to `Parse`.
- **Q:** Does `Build` return `[]shedengine.ProducerDef` or a whole `*shedengine.Shed`? **A:** [auto-pick] `[]shedengine.ProducerDef`. **Why:** verbatim what `manifest/designs/shed-recipe.md` and the roadmap item specify; returning a `Shed` would drag the three Shed-level absolute paths into this layer for no gain.
- **Q:** Does this task also ship loom's own recipe file? **A:** [auto-pick] No — `testdata/` fixtures only. **Why:** converting `internal/loomshed`'s list is piece 4, its own roadmap item sequenced immediately after this one.
- **Q:** Top-level document shape — metadata plus rows, a bare list, or `producers:` only? **A:** [auto-pick] A document with `version`, `entry`, `terminals`, `producers`. **Why:** `shedcheck.Check` requires told `entry` and `terminals` and refuses to infer either, and once loom's list lives in a file the file is the only place a human can write those two facts down.
- **Q:** `Segment` — `shed-recipe.md` bans it from recipe rows, but three planned roadmap items require it. Include it or not? **A:** [auto-pick] Include an optional per-row `segment`, and correct `shed-recipe.md` in the same commit. **Why:** the doc's argument ("unset Segment is a no-op in `validate()`") holds only while every row leaves it unset, and `loom: Discussion-Review/Plan-Review/Webster-Review producer` each explicitly set it; without the field those three tasks cannot author their rows in recipe form, and a mixed list would fail `shedengine.validate()`'s cross-segment `OnStuck` rule at run time.
- **Q:** Does `Build` call `shedcheck.Check`? **A:** [auto-pick] No — a separate `Check(r, producers)` helper for a caller's own test suite. **Why:** `shedcheck/doc.go` has a section headed "Nothing in production calls Check," and its stated reason (a resumed run legitimately starts mid-graph, so reachability-from-entry is the wrong runtime question) applies here verbatim.
- **Q:** Unknown-key handling and versioning — strict with `version`, strict without, or lenient? **A:** [auto-pick] Strict at both document and row level, `version: 1` required. **Why:** mirrors `shedrecipe.configRejectUnknown`, which all twelve registry entries already end with; a lenient loader swallows `on-done` for `on_done`, yielding an empty `OnDone` that `shedengine` reads as "end the run quietly" — a silent failure a Go literal could not produce.
- **Q:** Which validations belong to `shedbuild` versus `shedengine.validate()` and `shedcheck`? **A:** [auto-pick] Shape and engine-name resolution only; routing-target, cycle, and reachability checks stay where they already are. **Why:** `shedengine.validate()` already rejects dangling targets, duplicates, and cross-segment `OnStuck` before `Run` touches anything, and `shedcheck` already reports the graph defects; a third copy is what drifts. Duplicate-`name` is the one deliberate overlap, so a shadowed row fails at parse time where its author can see it.
- **Q:** First error or aggregated errors? **A:** [auto-pick] First error, wrapped with row index and name. **Why:** matches `shedrecipe`'s accessors and `shedengine.validate()`, both strictly first-error; `shedcheck` is the one report-everything surface and it is deliberately a separate authoring-time tool.
- **Q:** Should the recipe document carry a run-wide `max_bounces` default? **A:** [auto-pick] No — per-row only. **Why:** YAGNI. `loomcli.wire` deliberately leaves it zero so `shedengine`'s own default applies, and no caller overrides it; a recipe field nobody sets would give the default a second home.
- **Q:** Verification approach — unit tests only, or unit tests plus a loom-equivalence fixture? **A:** [auto-pick] Both, with the equivalence test as the key one. **Why:** it is the actual proof the format can express loom's real thirteen-row list, and it earns that proof without doing piece 4's conversion; `Producer` identity is not comparable, but concrete type is.
- **Q:** Told-geometry enforcement for the new package — machine-enforced or review obligation? **A:** [auto-pick] Machine-enforced, with `internal/shedbuild` added to `CONSTRAINTS.md`'s list in the same commit. **Why:** every sibling in this stack is machine-enforced, and a new package that resolves a path is exactly where the invariant rots silently.
- **Q:** Does `shedbuild` define where recipe files live on disk? **A:** [auto-pick] No — `Load` takes a told absolute path. **Why:** Told-Geometry Invariant, and choosing between `go:embed`, a `stencilstore`-style seed, and a plain `_lyx/` file is piece 4's decision to take with loom's real recipe in hand.
- **Q:** Which docs land in the same commit? **A:** [auto-pick] `manifest/designs/shed-recipe.md`, `docs/overview.md`, `CONSTRAINTS.md`, and `manifest/roadmap.md`. **Why:** `CLAUDE.md`'s task-completion rule — a task adding a module updates the module doc, the overview, and `CONSTRAINTS.md` for a new invariant, in the same commit; the roadmap moves because this completes a planned item.
