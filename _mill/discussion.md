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
- Twelve registry entries: the nine `ShedProducer` types reachable from loom's current list (`Webster`, `Preflight`, `Publish`, `Finalize`, `LoomPreflight`, `Batchifier`, `DiscussionValidate`, `PlanValidate`, `Stub`), the two shipped review adapters (`Bouncer`, `BurlerRound`), and `SingleLLM` — which is neither, having no production caller today, but is named as a registry entry by `manifest/roadmap.md` and is needed before any `*-Write` row can stop being a stub.
- Exporting `internal/loomshed`'s six currently-unexported producer constructors so the registry can reach them.
- A `SpecSource` builder for the `SingleLLM` entry — stencil read plus token fill — since no production caller of `shedadapters.NewSingleLLMProducer` exists yet.
- A coverage-guard test asserting every engine backing a row in `loomshed.New`'s current list has a registry entry.
- A `seam_enforcement_test.go` import allowlist for the new package, and a new **Shed Recipe Registry Invariant** section in `CONSTRAINTS.md`.
- `internal/shedrecipe/doc.go` — the package doc, same as every sibling in this family ships (`loomshed`, `landingshed`, `preflightshed`, `shedadapters`, `batcher`).
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

- Decision: one file in `internal/shedrecipe` declares an explicit unexported `map[string]Constructor` literal naming all twelve entries.
  The exported surface over it is two functions:
  `Lookup(name string) (Constructor, error)`, returning an error naming the unknown string (an empty name is an error too — there is no sensible default engine, unlike `batcher.Select`'s `identity`);
  and `Names() []string`, returning the registered names **sorted**, so the coverage guard and piece 2 both have a stable enumeration and neither reaches into the map directly.
- Rationale: the whole registry is greppable in one place, there is no mutable package global to corrupt across tests, and registration order is irrelevant.
  `internal/batcher`'s `init()` self-registration (`internal/batcher/registry.go`) is the in-repo precedent, but it works there because every batcher lives in the same package as the registry;
  here the entries would live in four packages, so `init()` registration would require blank imports at every consumer and would make "which engines exist" depend on which packages happened to be linked in.
- Rejected: `init()` self-registration per producer package (blank-import fragility, mutable global, non-deterministic membership);
  a `Register` function callable at runtime (same mutable-global problem, plus it makes the coverage-guard test order-dependent).

### `loomshed`'s six constructors become exported

- Decision: rename `newLoomPreflight`, `newBatchifier`, `newDiscussionValidate`, `newPlanValidate`, `newStub`, **and `newWebsterProducer`** to their exported forms in `internal/loomshed`, keeping their **parameters and behaviour** unchanged and widening only the declared return type to the seam interface.
  `newWebsterProducer` (`internal/loomshed/webster.go:41`) is the sixth: the `Webster` registry entry wraps loom's lazy wrapper, not `shedadapters.NewWebsterProducer` directly.
  The wrapper resolves `batcher.Active(anchorPath)` inside **every** `Call` and fills the result into a copy of `RunDeps` (`webster.go:59-70`), and it also maps a `batcher.Active` failure to `Stuck` rather than to a returned error, so the same fault ends the run the same way at the `Webster` row as at the `Batchifier` gate (`webster.go:48-53`).
  Calling `shedadapters.NewWebsterProducer` from the registry would drop both behaviours and require a `Batcher` value the registry has no way to resolve, so exporting the wrapper is the only workable option.
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
  each registry entry extracts and validates the keys it recognises, using small shared typed accessors that report a clear error on a missing required key or a wrong type.

  The accessors, and the **decoded Go representation each accepts** — this is the contract piece 2's decoder must satisfy, so it is pinned here rather than left to whichever format that task picks:

  | Accessor | Accepts |
  | --- | --- |
  | `configString` | `string` |
  | `configStringSlice` | `[]any` whose every element is a `string`, and `[]string` |
  | `configBool` | `bool` |
  | `configInt` | `int`, and `float64` **with an integral value** — a fractional `float64` is an error naming the key |
  | `configStringMap` | `map[string]any` whose every value is a `string` — backs `tokens` |
  | `configMap` | `map[string]any` — backs `profile` and its nested `target` / `fasit` |

  Nested maps are always `map[string]any`, never `map[string]string` or a typed struct.
- Rationale for the number rule: a YAML decoder into `any` yields `int`, while a JSON-shaped one yields `float64`, and the whole point of `map[string]any` is that piece 2 gets to choose.
  Accepting both while rejecting a fractional value keeps the registry format-agnostic without silently truncating `timeout_s: 1.5` to `1`.
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

### Required vs optional keys, pinned per entry

- Decision: every recognised key is explicitly either required or optional, and **an empty-string value counts as absent** — so a required key present but blank is the same error as one omitted.
  An absent optional key falls back to the zero value, which the underlying constructor then treats as its own documented default.

  | Entry | Required | Optional |
  | --- | --- | --- |
  | `SingleLLM` | `stencil`, `output_files` | `model`, `effort`, `version`, `interactive`, `role`, `tokens` |
  | `Bouncer` | `run_subdir`, `artifact_paths`, `rubric_stencil` | `model`, `effort`, `version` |
  | `BurlerRound` | `run_subdir`, `profile` | `model`, `effort`, `timeout_s` |
  | `Preflight`, `Publish`, `Finalize`, `LoomPreflight`, `Batchifier`, `DiscussionValidate`, `PlanValidate`, `Stub`, `Webster` | — (empty `Config`) | — |

- Rationale: strictness about *unknown* keys does nothing about *missing* ones, and the gap is not cosmetic.
  An absent `run_subdir` would resolve `RunDir` to a bare `Env.RunRoot`, which silently reinstates the exact cross-segment overwrite the `run_subdir` decision exists to prevent — no error, just two segments trampling each other's round files.
  Treating empty-as-absent matters for the same reason: `run_subdir: ""` joins to `RunRoot` unchanged.
  Within `profile`, the required-field checking is `burlerengine.Profile.validate`'s job, not the entry's — it already rejects an empty `Rubric` and a `Target`/`Fasit` with neither `Paths` nor `Instructions` (`internal/burlerengine/profile.go:73-97`) — so the entry requires only that `profile` itself is present.
- Rejected: leaving required-ness implicit (a missing key silently degrades to a zero value, which for `run_subdir` is actively harmful);
  treating empty-string as a legitimate value (indistinguishable from absent at the point where it does damage);
  re-checking `profile`'s inner required fields in the entry (duplicates `Profile.validate` and would drift from it).

### Live seams travel in a told `Env`, never in `Config`

- Decision: a single `Env` struct, filled once by whichever caller invokes the registry, carries both the resolved absolute paths and the injected non-serializable dependencies.
  Its shape:

  ```go
  type Env struct {
      // Geometry -- absolute paths and roots, resolved by the caller.
      Cwd                string // Preflight
      AnchorPath         string // Batchifier, PlanValidate, Webster, SingleLLM (anchor_path token)
      WorktreeRoot       string // PlanValidate, and the root every worktree-relative Config path resolves against
      StatusPath         string // LoomPreflight
      StatusLockPath     string // LoomPreflight
      StencilsDir        string // SingleLLM, Bouncer
      RunRoot            string // the root every Config run_subdir resolves against -- Bouncer, BurlerRound
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

  `Env` holds **roots and run-wide values only** — never a value that differs between two rows.
  Anything per-row is a relative path in `Config`, resolved against one of these roots by the entry (see the two path decisions below).

- Rationale: this is the gap `manifest/designs/shed-recipe.md` leaves open.
  The doc covers geometry ("never in a recipe, resolved once, centrally, by whichever caller invokes the recipe-builder") but says nothing about the *other* things the constructors need, which are neither paths nor static config:
  `shedadapters.Shuttle`, `shedadapters.BurlerRunner`, `shedadapters.WebsterRunner`, `websterengine.RunDeps`, `func() time.Time`, and `landingshed.Deps`'s three closures (`PushBranch`, `OpenFabric`, `OpenParentFabric`) plus its `modelspec.Registry`.
  None of those can be written in a recipe file at all.
  Putting them in the same told bundle as geometry extends the existing discipline rather than inventing a second one, and keeps the recipe portable exactly as the doc requires — the recipe still names no path and no mode.
- Rejected: extending `Config` to carry them (puts non-serializable values in the field the recipe file owns, destroying portability);
  package-level injection via a setter (mutable global, untestable in parallel);
  a variadic option list per entry (defeats the fixed signature).

### Every entry validates the `Env` fields it reads

- Decision: at construction, each entry checks exactly the `Env` fields it consumes — a path root must be non-empty and absolute, an injected seam must be non-nil — and returns an error naming the offending field otherwise.
  An entry never validates an `Env` field it does not use, so a caller filling only what its recipe needs is legal.
  `Env.Now` is the one exception: a nil clock is accepted and defaults to `time.Now`, matching what `NewSingleLLMProducer` and `NewBurlerProducer` already do with a nil `now`.

  **`Env.WebsterDeps` needs a third rule**, being neither a path root nor a single seam but a value struct (`websterengine.RunDeps`, `internal/websterengine/runlevel.go:104-131`).
  The `Webster` entry checks its four required inner seams — `Starter`, `Reed`, `Engine`, `RefMatcher` — for non-nil, and deliberately checks **none** of the other five nil-able fields, each for a stated reason:

  | `RunDeps` field | Checked? | Why |
  | --- | --- | --- |
  | `Starter`, `Reed`, `Engine`, `RefMatcher` | yes | required seams with no default; a zero value fails at every `Call` |
  | `Batcher` | **no** | the wrapper overwrites it on every `Call` (`internal/loomshed/webster.go:67-68`), and its own field doc says the caller leaves it nil |
  | `Clock` | no | nil selects the production `realClock` by design (`runlevel.go:118-122`) |
  | `OpenBisector` | no | nil is a legitimate mode meaning "no fabric in this mode", not a missing value (`runlevel.go:124-130`) |
  | `ShuttleCfg`, `Roles`, `Config`, `Geom` | no | value/map types whose validation belongs to `websterengine.Run`, not to a wiring layer |

  `Env.Landing` needs no equivalent rule: `landingshed.NewPublish` and `NewFinalize` already reject their own nil closures, so the entry inherits the check.
  `WebsterDeps` is the only unguarded value-struct field in `Env`, which is why it gets an explicit rule rather than falling under the general one.
- Rationale: `Config` strictness without `Env` strictness only moves the late failure.
  `NewSingleLLMProducer` validates nothing at all (`internal/shedadapters/singlellm.go:49-54`), so an empty or relative `Env.WorktreeRoot` produces a `SingleLLM` row that constructs cleanly and then fails on **every** `Call` — the exact failure mode the relative-path decision exists to prevent on the `Config` side.
  Some underlying constructors already check their own inputs (`NewBouncer` rejects a non-absolute `RunDir`, `NewBurlerProducer` rejects a nil runner and a non-absolute `runDir`), but the coverage is uneven, and an entry cannot rely on the one it happens to wrap.
  Validating in the entry makes the guarantee uniform across all twelve.
- Rejected: relying on each underlying constructor's own checks (uneven — `SingleLLM` and the four `loomshed` value-only constructors check nothing);
  validating all of `Env` once up front (would force every caller to fill fields its recipe never uses);
  validating nothing (turns a caller's wiring bug into a per-call runtime failure with no construction-time signal).

### Per-segment run directories: `Env.RunRoot` plus a `Config` `run_subdir`

- Decision: `Env` carries `RunRoot`, a single absolute base directory.
  `Bouncer` and `BurlerRound` each take a `run_subdir` config key — a relative, non-escaping path — and the entry passes `filepath.Join(env.RunRoot, cfg.run_subdir)` down as `RunDir`.
  Two rows forming one review segment (a `Bouncer` and the `BurlerRound` it gates) are authored with the **same** `run_subdir` value;
  two different segments must use different values.
- Rationale: the round-artifact filenames under `RunDir` are fixed and carry only a round number, no segment name — `round-<n>-bouncer-verdict.md`, `-ledger.md`, `-focus.md` (`internal/shedadapters/round.go:49-63`) and `round-<n>-review.md`, `round-<n>-fixer-report.md` (`internal/shedadapters/burler.go:106-113`).
  So a single run-wide `RunDir` would have `Discussion-Review` and `Plan-Review` overwriting each other's round-1 verdict, ledger, focus, review, and fixer report.
  Sharing within a segment is not incidental but required: `BouncerConfig.RunDir`'s own field doc defines it as "the directory the round producer this Bouncer gates writes its reports into, **and** this Bouncer writes its own verdict/ledger/focus files into", and `roundComplete` (`burler.go:119-127`) stats the Burler's own two files in that same directory.
  This is also what makes the `Env`-holds-only-roots rule above consistent with the `ReportName` decision's "it is per-row, not per-run" reasoning, which the flat `RunDir` field contradicted.
  **The `Bouncer` and `BurlerRound` entries create the joined directory themselves**, with `os.MkdirAll`, at construction.
  Nobody else can: the joined `RunRoot/<run_subdir>` path exists only inside the entry, so the caller filling `Env` does not know it.
  This is required, not defensive — `Bouncer.Call` reaches `ResolveRound` first (`internal/shedadapters/bouncer.go:154`), which `os.Stat`s `RunDir` and returns a **hard error** when it is absent (`internal/shedadapters/round.go:24-31`), and the `Bouncer` is the segment's entry point, so it runs before the `BurlerRound` that would otherwise have created the directory (`internal/shedadapters/burler.go:232`).
  Without this, every fresh segment fails on its first call.
  Constructor I/O is already the established shape here: `NewBouncer` deliberately probes its rubric stencil in the constructor for the same fail-early reason (`bouncer.go:124-133`).
  It does not contradict `NewBurlerProducer`'s "never stats, creates, or otherwise touches runDir — creating it is Call's job" contract either: that constructor still does not, `Call`'s own `MkdirAll` is idempotent and stays, and the directory simply exists sooner.
- Rejected: one run-wide `RunDir` in `Env` (silent cross-segment overwrite — the finding that forced this decision);
  leaving directory creation to `BurlerProducer.Call` (the `Bouncer` runs first and hard-errors before the Burler ever gets a turn);
  making creation a caller obligation (the caller cannot construct the path);
  an absolute `run_dir` in `Config` (barred outright — `manifest/designs/shed-recipe.md` says `Config` "never contains absolute paths", and it would break recipe portability);
  deriving the subdir from the row's `Name` (couples on-disk layout to a rename, and a `Bouncer`/`BurlerRound` pair has two different names but needs one directory).

### Relative `Config` paths resolve against a named `Env` root

- Decision: a general rule, applied by every entry that takes a path in `Config`:
  a `Config` path value is always relative, the entry joins it against a specific `Env` root, and the entry **rejects** both an absolute value and one escaping its root via `..`.
  The root per key:

  | `Config` key | Entry | Resolved against |
  | --- | --- | --- |
  | `artifact_paths` | `Bouncer` | `Env.WorktreeRoot` |
  | `output_files` | `SingleLLM` | `Env.WorktreeRoot` |
  | `run_subdir` | `Bouncer`, `BurlerRound` | `Env.RunRoot` |
  | `stencil`, `rubric_stencil` | `SingleLLM`, `Bouncer` | not a path — a `stencilstore` name, resolved by `stencilstore.Read(env.StencilsDir, name)` |
  | `profile.target.paths`, `profile.fasit.paths` | `BurlerRound` | **nothing — left relative deliberately.** See the exception below |

  **One exception, and it is the only one:** `BurlerRound`'s `profile.target.paths` and `profile.fasit.paths` are passed through **relative and unjoined**.
  `burlerengine.Profile.validate` already resolves them against its own told `worktreeRoot` and then stats every resolved entry for existence (`internal/burlerengine/profile.go:66-87`), so joining them in the registry entry would either double-resolve or hand `validate` an absolute path it did not resolve itself.
  The entry therefore does **not** apply the absolute-rejection check to these two keys either — an author who writes an absolute path there gets `validate`'s own behaviour, not the registry's.
  This exception is narrow and worth a comment at the entry, because it is the one place the general rule above does not hold.

- Rationale: without this rule the decided config shapes produce producers that cannot run.
  `NewBouncer` rejects an empty `ArtifactPaths` and any non-absolute entry (`internal/shedadapters/bouncer.go:91-103`), so a worktree-relative `artifact_paths` must be joined before it reaches the constructor.
  `SingleLLMProducer.Call` rejects any non-absolute `spec.OutputFiles` entry outright (`internal/shedadapters/singlellm.go:71-75`) — and deliberately so, per its own comment: `Spec.validate` would resolve relative entries against a worktree root "this adapter must not read".
  The earlier decision's "worktree-relative `output_files`" would therefore have produced a `SingleLLM` row that errors on every call.
  Joining in the registry entry is the right layer: it is the first place that holds both the recipe's relative value and the caller's told root.
  Rejecting absolute values is what mechanically enforces the design doc's portability rule rather than leaving it to recipe-author discipline.
- Rejected: passing relative paths through and letting each producer resolve them (two of the three constructors refuse to, by explicit design);
  putting the per-row absolute paths in `Env` (they are per-row — that is the first blocking finding again);
  allowing either absolute or relative and joining only when relative (makes a non-portable recipe silently work on the author's machine, which is the exact failure the doc's rule exists to prevent).

### `BurlerRound`'s `Config` mirrors `burlercli`'s profile key names

- Decision: `BurlerRound`'s config keys are `profile` (a nested map), `model`, `effort`, `timeout_s`, and `run_subdir`.
  The `profile` map's keys are exactly `internal/burlercli`'s existing kebab-case profile shape (`profileYAML`, `internal/burlercli/run.go:29-40`) — `target`, `fasit`, `rubric`, `fix-scope`, `tool-use`, `cluster-fan` — and **only those six**.
  `target` and `fasit` are themselves nested maps recognising exactly two keys each, `paths` and `instructions`, mirroring `fileSetYAML` (`internal/burlercli/run.go:23-26`).
  The strict unknown-key rule applies at that nested level too, so the rejector runs on the inner maps as well as the outer one.
  `model`, `effort`, and `timeout_s` fill `burlerengine.RunOpts`.
- Rationale: `NewBurlerProducer`'s own doc (`internal/shedadapters/burler.go:62-64`) states that the `profile` argument is a *template* whose `ReviewPath`, `FixerReportPath`, `PriorReviews`, `PriorFixerReports`, and `ClusterExclude` are overwritten per round, and that `opts` is a template whose `Round` is overwritten per attempt.
  Those five profile fields and `RunOpts.Round` are therefore not recipe-authorable — putting them in `Config` would invite an author to set a value the producer silently discards.
  That leaves exactly the six above, and reusing `burlercli`'s established key names means a human who has written a burler profile file can read a recipe row without a second vocabulary.
- Rejected: mirroring all ten `profileYAML` keys (four of them are per-round overwritten — see above);
  inventing a fresh key vocabulary (two names for one concept);
  importing `internal/burlercli` to share its decoder (it is a CLI package, and its `decodeProfile` takes YAML **bytes** with `KnownFields(true)` while `shedrecipe` receives an already-decoded `map[string]any` — the input types differ, so there is nothing to share without a refactor this task does not own).
  The plan must keep the two key-name sets identical by hand;
  that duplication is accepted deliberately and is worth a comment at both sites.

### `Publish` and `Finalize` ignore the row's `name`

- Decision: accept that the `name` argument is discarded for these two entries, and require the recipe row to be named exactly `Publish` / `Finalize`.
  The coverage-guard test asserts this pairing so a renamed row fails loudly rather than producing a producer whose on-disk identity disagrees with its row name.
- Rationale: `landingshed.NewPublish` / `NewFinalize` take only `Deps` (`publish.go:60`, `finalize.go:69`), and `landingshed.Deps` has no `Name` field — each producer's identity is a package constant (`publishName = "Publish"`, `publish.go:31`), which its log lines, error text, and stuck-reason filename all carry.
  So the fixed signature's "`name` is threaded straight into each producer's own name parameter" is true for ten of the twelve entries and false for these two.
  Adding a `Name` field to `landingshed.Deps` would change a package outside this task's scope and would let a recipe rename the stuck-reason filename that `Hardener`'s own list also depends on — `Publish` and `Finalize` are shared by reference with `landingshed`'s other consumer, so their names are not loom's to reassign.
- Rejected: adding `Deps.Name` (out of scope, and the name is shared with `Hardener`'s list);
  silently ignoring the mismatch (a row named `Ship` would log as `Publish`, which is exactly the kind of drift the guard test exists to catch).

### `landingshed.Deps` rides `Env` as a whole-struct passthrough

- Decision: `Env.Landing` is a `landingshed.Deps` value passed through unchanged to `landingshed.NewPublish` / `NewFinalize`.
  The `Publish` and `Finalize` registry entries therefore accept an empty `Config`.
- Rationale: `landingshed.Deps` already carries fourteen fields (`internal/landingshed/deps.go:31-91`), several of them closures or seams, and it is already told wholesale by `loomshed.Deps.Landing` today (`internal/loomshed/loomshed.go`).
  Flattening it into `Env` would duplicate that field set and invite divergence — the exact reasoning `loomshed.Deps`'s own field doc gives for keeping it a single passthrough.
- Rejected: flattening `landingshed.Deps`'s fields into `Env` (duplication, divergence);
  making the landing fields recipe `Config` (they include closures).

### The `SingleLLM` entry builds its `SpecSource` from a stencil

- Decision: the `SingleLLM` entry constructs the `shedadapters.SpecSource` closure itself.
  Its `Config` keys are `stencil` (a `stencilstore` name), `output_files` (a list of worktree-relative paths), `model`, `effort`, `version`, `interactive`, `role`, and `tokens` (a `map[string]string` of static token values).
  The closure reads the stencil via `stencilstore.Read(env.StencilsDir, cfg.stencil)`, fills it via `stencil.Fill`, and returns a `shuttleengine.Spec` whose `OutputFiles` are **already absolute**, each joined against `Env.WorktreeRoot` per the relative-path rule above.

  The `stencil.Fill` token map is the union of two closed sets.
  Four **reserved** geometry-derived tokens, supplied from `Env` and never authorable:

  | Token | Value |
  | --- | --- |
  | `worktree_root` | `Env.WorktreeRoot` |
  | `anchor_path` | `Env.AnchorPath` |
  | `stencils_dir` | `Env.StencilsDir` |
  | `output_files` | the resolved absolute `OutputFiles`, newline-joined |

  Everything else comes from `Config.tokens`.

  The remaining `shuttleengine.Spec` fields — `Timeout`, `ForkSubagents`, `KeepPane`, `Round`, `Parent`, and `Display` (`internal/shuttleengine/spec.go:41,53,55,58,71,74`) — are **deliberately not recipe-authorable in this task**, and their zero values defer to the shuttle engine's own config and defaults.
  Recording that here so piece 2 does not re-litigate it: the omission is a decision, not an oversight.
  `Round`, `Parent`, and `Display` in particular are per-*run* strand-display values, not per-row static config, so a recipe is the wrong place for them regardless.
  A `tokens` map naming any of the four reserved keys is **rejected** at construction rather than silently overriding or being overridden — a silent winner in either direction is a recipe that reads one way and behaves another.

  All four reserved tokens are supplied **unconditionally**, whether or not the stencil names them, and `SingleLLM` therefore validates `Env.WorktreeRoot`, `Env.AnchorPath`, and `Env.StencilsDir` unconditionally under the `Env`-validation rule above.
  This is safe because `stencil.Fill` errors only on a marker the template references whose value is **absent, empty, or whitespace-only** (`internal/stencil/stencil.go:29,39-46`, and `unfilledTopLevelMarkers` at `169-181`, which trims before testing) — never on an extra value the template ignores.
  Because empty counts as unfilled, an **empty or whitespace-only `Config.tokens` value is rejected at construction** rather than allowed through to fail at first `Call`, consistent with the empty-is-absent rule for required keys.
  A token that legitimately has no value must carry an explicit placeholder — `Bouncer` uses the literal `"(none)"` for precisely this (`bouncer.go:397`).
  The alternative, supplying and validating a token only when the stencil declares its marker, would make the entry's `Env` requirements depend on parsing template text, so the same recipe row would validate differently after a stencil edit that never touched the recipe.
  The entry **probes the stencil at construction** with `stencilstore.Read(env.StencilsDir, cfg.stencil)`, discarding the bytes, in addition to the read inside the closure.
  Without the probe a mistyped `stencil` name would construct cleanly and fail at first `Call`;
  `NewBouncer` probes its own rubric for exactly this reason and says so (`internal/shedadapters/bouncer.go:124-133`), and there is no reason for `SingleLLM` to be the asymmetric one.
  The closure still re-reads on each call rather than capturing the bytes, so a stencil edited mid-run takes effect without a restart — the probe buys fail-fast, not caching.

  The stencil here is the **template**, so `stencil.Fill` strips its stamp banner itself and no `stencil.StripLeadingComment` call is needed.
  That call is required only when stencil content is injected as a token *value* — `stencil.Fill` never strips a marker value, which is why `Bouncer` must strip its rubric before passing it in (`internal/shedadapters/bouncer.go:341-344`).
  Should a later `SingleLLM` row ever want a stencil as a token value, that row's entry pays the same strip.
  Absolute is not optional here: `SingleLLMProducer.Call` rejects a non-absolute `OutputFiles` entry outright (`internal/shedadapters/singlellm.go:71-75`) rather than resolving it, because resolution would require reading a worktree root that adapter is barred from touching.
- Rationale: `shedadapters.NewSingleLLMProducer` has **no production caller today** — only `internal/shedadapters/singlellm_test.go` — so someone has to build the spec-composition step, and the registry is where the roadmap places `SingleLLMProducer`.
  `shuttleengine.Spec`'s own doc states shuttle is dumb transport and "the caller composes" the prompt, so composition belongs on this side of the seam.
  `shedadapters.Bouncer` already does exactly this pair of calls (`internal/shedadapters/bouncer.go:330-346`, `413-433`), so the pattern is established rather than invented.
- Rejected: leaving `SingleLLM` out of the registry until `loom: Discussion-Write producer` needs it (the roadmap names it explicitly as a registry entry, and piece 4 cannot convert a `Discussion-Write` row without it);
  taking a pre-built `SpecSource` from `Env` (there is one `Env` per run but many `SingleLLM` rows, each needing a different spec — the difference is precisely what `Config` is for).

### `Bouncer`'s full `Config` key set

- Decision: `Bouncer` recognises exactly six keys — `run_subdir`, `artifact_paths`, `rubric_stencil`, `model`, `effort`, `version` — and takes everything else from `Env` or pins it.
  The full mapping onto `shedadapters.BouncerConfig`:

  | `BouncerConfig` field | Source |
  | --- | --- |
  | `Name` | the row's `name` argument |
  | `RunDir` | `Env.RunRoot` joined with `Config.run_subdir` |
  | `ArtifactPaths` | `Config.artifact_paths`, each joined against `Env.WorktreeRoot` |
  | `ReportName` | pinned to `round-%d-review.md` — not configurable, see below |
  | `StencilsDir` | `Env.StencilsDir` |
  | `RubricStencil` | `Config.rubric_stencil` |
  | `Model`, `Effort`, `Version` | `Config.model`, `Config.effort`, `Config.version` |
  | `Shuttle` | `Env.Shuttle` |
  | `Now` | `Env.Now` |

- Rationale: the strict unknown-key rule makes an entry's recognised key set its contract, so leaving `Bouncer`'s set implicit would leave the plan guessing.
  `Model`/`Effort`/`Version` in particular belong in `Config`, not `Env`: their own field doc calls them "an already-resolved triple threaded verbatim into `shuttleengine.Spec`, resolved at the caller's own config-load time" (`internal/shedadapters/bouncer.go:47-51`), and two review segments legitimately run at different model tiers, which a run-wide `Env` value could not express.
- Rejected: `Model`/`Effort`/`Version` in `Env` (forces every segment to the same tier);
  leaving the set implicit (unenumerable contract under a strict-unknown-key rule).

### `Bouncer`'s `ReportName` is pinned, not configurable

- Decision: the `Bouncer` entry builds `ReportName` as a fixed `round-%d-review.md` closure.
  There is **no** `report_name` config key.
- Rationale: the value is not a free choice — it is determined by the round producer the `Bouncer` gates, and today that is always a `BurlerRound`, which writes its report to a **hardcoded** `round-<n>-review.md` under `RunDir` (`internal/shedadapters/burler.go:106-108`).
  `ResolveRound` finds the current round by statting `reportName(n)` under `RunDir` and returning `n-1` at the first absence (`internal/shedadapters/round.go:24-46`), so a `report_name` that does not match what the Burler writes resolves the round to 0 forever: the `Bouncer` re-seeds every call until its bounce budget is spent, with no error anywhere.
  A configurable key with exactly one correct value is a silent-failure generator, not flexibility.
  This also retracts the round-1 rationale that "different segments legitimately name their reports differently" — segments are separated by `RunDir`, not by filename, and the filename is identical across all of them.
- Rejected: a free-form `report_name` pattern (one correct value, silent failure for every other — the finding that forced this change);
  a pattern key defaulting to the Burler name (same failure mode, merely less likely);
  putting the closure in `Env` (it is per-row, not per-run).
  If a non-`BurlerRound` round producer ever gates behind a `Bouncer`, the key returns then — with the pairing validated, not free-form.

### `Stub` is a registry entry, despite not being in the roadmap's list

- Decision: register `Stub` alongside the eleven engines `manifest/roadmap.md` names.
- Rationale: five of loom's thirteen rows are still stubs — `Discussion-Write`, `Discussion-Review`, `Plan-Write`, `Plan-Review`, and `Webster-Review`.
  The roadmap's piece 4 says it "converts the list as it stands (including the still-stubbed `*-Write`/`*-Review` rows)", which is impossible without a `Stub` engine name.
  The omission from the roadmap's enumeration reads as an oversight in the enumeration, not a deliberate exclusion.
- Rejected: shipping only the eleven named engines (leaves piece 4 blocked on a follow-up).

### A coverage-guard test pins the registry against loom's current list

- Decision: a test in `internal/shedrecipe` holds an explicit row-name → engine-name table and compares its **key set against the row names in `loomshed.New`'s assembled list**, failing on a mismatch in **either** direction — a row present in `New` but missing from the table, and a table entry naming a row `New` no longer has.
  It then asserts every engine name the table maps to is present in the registry.
- Rationale: the registry's whole reason to exist is that piece 4 can resolve every one of loom's rows.
  Without a guard, a row added to `loomshed` between now and piece 4 silently has no engine, and the gap surfaces only when piece 4 fails.
  Comparing against `New`'s actual output is what makes the guard catch that;
  a test that merely iterated its own table would pass forever no matter what `loomshed` grew, which is the failure mode the guard exists to prevent.
  `New`'s output does carry the row **names** even though it carries no engine name, so one side of the comparison is derivable and only the mapping has to be maintained by hand.
- Rejected: no guard (defers a known failure);
  a table-only test that never reads `New`'s output (passes forever regardless of what `loomshed` grows);
  deriving the **engine** side from `New` too (`ProducerDef` carries no engine name, so the row-name → engine-name mapping genuinely has to be written down — only the row-name set is derivable).

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
| `Webster` | `loomshed.newWebsterProducer(name, anchorPath, run, deps)` (`internal/loomshed/webster.go:41`) | **to be exported — the sixth rename.** Not `shedadapters.NewWebsterProducer`: the wrapper resolves `batcher.Active` on every `Call` and maps its failure to `Stuck`, and it leaves `RunDeps.Batcher` for itself to fill |
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
`stencil.Fill` errors on a missing token rather than substituting empty, which is why `bouncer.go:397` passes the literal `"(none)"` for an empty value.
The `SingleLLM` entry follows the same three steps.

**Import-allowlist tests.** The two producer-hosting packages already on the Told-Geometry Invariant's machine-enforced list carry one — `internal/loomshed` and `internal/landingshed`;
`internal/shedadapters` and `internal/preflightshed` carry none, and are not on that list.
`internal/loomshed/seam_enforcement_test.go` (`TestToldGeometryInvariant_AllowlistOnly`) is the closest model — it walks non-test `.go` files with `go/parser` in `parser.ImportsOnly` mode and fails any import outside an explicit allowlist map.
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
  The `Env` struct is the told bundle.
  The precise property `shedrecipe` holds — and the wording the new invariant's text must use verbatim, since a looser phrasing would ship false:
  **every root is told and none is derived;
  the package's only path construction is joining a told root with a recipe-relative value.**
  It resolves no cwd, consults no environment, and reads no config file to obtain a root.
  A flat "no path is computed inside `shedrecipe`" would be untrue the moment `run_subdir`, `artifact_paths`, or `output_files` is joined.
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

1. **`Lookup` and `Names`** (`internal/shedrecipe`) — write first, they pin the public surface.
   Scenarios: a known name resolves to a non-nil constructor;
   an unknown name returns an error naming the offending string;
   an empty name is an error rather than a silent default (unlike `batcher.Select`, which defaults — there is no sensible default engine);
   `Names()` returns all twelve, sorted, and a caller mutating the returned slice does not affect a later call.
2. **Config accessors** (`internal/shedrecipe`) — write before any entry uses them.
   Scenarios per accessor: present and correctly typed;
   present but wrong type;
   absent-and-required;
   absent-and-optional falls back to the zero value.
   Each accessor's accepted representations need their own cases: `configStringSlice` against both `[]any`-of-strings and `[]string`, and against an `[]any` with one non-string element;
   `configInt` against `int`, against an integral `float64`, and against a fractional `float64` (an error naming the key);
   `configStringMap` against a `map[string]any` with a non-string value.
   Plus the unknown-key rejector: a `Config` with one unrecognised key errors and names the key;
   a `Config` with only recognised keys passes.
3. **Per-entry construction**, one test per registry entry — twelve tests, each supplying a minimal valid `Config` plus a filled `Env` and asserting a non-nil `shedengine.ShedProducer` with no error.
   These are best driven as a table over the registry so a newly added entry without a test fails loudly.
   Cover the fallible entries' failure paths too: `Bouncer` with a missing rubric stencil, `Publish`/`Finalize` with a `landingshed.Deps` the underlying constructor rejects.
4. **`SingleLLM` spec composition** — the only entry with real logic of its own.
   Scenarios: the composed `Spec` carries the filled prompt, the resolved `OutputFiles`, and the `model`/`effort`/`version`/`interactive`/`role` values from `Config`;
   a stencil naming a token absent from `tokens` and absent from the geometry-derived set errors rather than filling empty;
   a mistyped `stencil` name errors **at construction**, via the probe, not at first `Call`;
   an empty or whitespace-only `tokens` value is rejected at construction.
   Note the `SpecSource` is a closure — the test must call it, not merely construct the producer.
   Also cover the four reserved tokens: each resolves to its documented `Env` value, and a `Config.tokens` map naming any reserved key is rejected at construction rather than winning or losing silently.
5. **`Bouncer` report-name pinning** — the built closure renders exactly `round-<n>-review.md`, byte-identical to what `BurlerRound` writes for the same round;
   a `report_name` key in `Config` is rejected as unknown.
   The byte-identity assertion is the one that matters: a drift here makes `ResolveRound` return 0 forever, which no other test would catch.
6. **Relative-path resolution**, shared across every entry taking a `Config` path.
   Scenarios: a relative value is joined against the documented `Env` root and arrives absolute at the underlying constructor;
   an **absolute** `Config` value is rejected with an error naming the key;
   a value escaping its root via `..` is rejected;
   an empty `artifact_paths` list is rejected before `NewBouncer` sees it.
   The `SingleLLM` and `Bouncer` cases matter most — both underlying constructors refuse non-absolute input, so a regression here yields a producer that fails at every call rather than at construction.
   The exception needs its own case: `BurlerRound`'s `profile.target.paths` / `profile.fasit.paths` reach `burlerengine.Profile` **still relative**, and an absolute value there is *not* rejected by the entry.
   Add the missing-required-key cases here too: for each entry, every required key omitted, and every required key present but empty-string, each failing at construction with an error naming the key.
7. **Per-segment run directories** — two `Bouncer` entries built with different `run_subdir` values resolve to different `RunDir`s;
   a `Bouncer` and a `BurlerRound` built with the *same* `run_subdir` resolve to the same `RunDir`, which is what makes `roundComplete` find the Burler's report.
   This is the regression guard for the cross-segment overwrite the flat `Env.RunDir` would have caused.
   Two more cases here: the directory **exists on disk after construction** and before any `Call` (a `Bouncer` constructed against a `RunRoot` with no subdir present must not leave `ResolveRound` to fail);
   and an omitted or empty `run_subdir` is rejected rather than resolving to bare `Env.RunRoot`.
8. **`BurlerRound` config** — the six recipe-authorable `profile` keys land in `burlerengine.Profile`;
   a `profile` map naming one of the five per-round-overwritten fields (`review-path`, `fixer-report-path`, `prior-reviews`, `prior-fixer-reports`, `cluster-exclude`) is rejected by the unknown-key rule rather than silently discarded.
9. **`Publish`/`Finalize` row naming** — the coverage guard fails when either row is named anything other than the package constant it will actually log as.
10. **Under-filled `Env`** — for each entry, an `Env` missing or mis-shaping a field that entry reads (empty root, relative root, nil seam) fails at **construction** with an error naming the field;
   an `Env` missing a field the entry does not read constructs fine;
   a nil `Env.Now` is accepted and defaults.
   The `SingleLLM` case is the load-bearing one, since its underlying constructor validates nothing at all.
   `Webster` needs its own cases against the table above: a `WebsterDeps` missing any of `Starter`, `Reed`, `Engine`, `RefMatcher` fails at construction;
   one with a nil `Batcher`, `Clock`, or `OpenBisector` constructs fine.
11. **Coverage guard** — the table's key set equals the row names in `loomshed.New`'s assembled list, and every engine it maps to is registered.
    Both mismatch directions must be covered: a row in `New` absent from the table, and a table entry naming a row `New` does not have.
    The first is the regression this guard exists for;
    the second keeps the table from accumulating dead entries.
12. **Import allowlist** (`internal/shedrecipe/seam_enforcement_test.go`) — mirrors `internal/loomshed/seam_enforcement_test.go`, asserting no production import outside the allowlist and specifically no `internal/lyxcwd`.

Scenarios that must not be missed:

- `loomshed`'s existing tests must still pass with their **assertions unchanged and their call sites renamed** — that, not literal file-untouched-ness, is the proof the rename is behaviour-neutral.
  Seven test files call the unexported constructors directly and therefore need mechanical rename edits: `stub_test.go`, `batchifier_test.go`, `planvalidate_test.go`, `loompreflight_test.go`, `discussionvalidate_test.go`, `webster_test.go`, and `resume_test.go`.
  The `New`-driven assertions in `sequence_test.go` and `loomshed_test.go` stay genuinely untouched, since they reach the producers through the assembled list rather than by constructor name — those two are the real behaviour-neutrality check.
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
- **Q:** [review r1] `Env.RunDir` is one field but two review segments each need their own — how is per-row geometry expressed? **A:** [auto-pick] `Env` carries `RunRoot`;
  `Config` carries a relative `run_subdir` the entry joins against it, shared within a `Bouncer`+`BurlerRound` segment and distinct between segments. **Why:** round-artifact filenames under `RunDir` carry only a round number (`round.go:49-63`, `burler.go:106-113`), so one shared dir would have `Discussion-Review` and `Plan-Review` overwriting each other's round state;
  the same dir must still be shared *within* a segment, since `roundComplete` stats the Burler's files where the Bouncer writes its own.
- **Q:** [review r1] `NewBouncer` demands absolute `ArtifactPaths` and `SingleLLMProducer.Call` rejects relative `OutputFiles`, but the design doc bars absolute paths from `Config` — what resolves them? **A:** [auto-pick] A general rule: `Config` path values are always relative, the entry joins each against a named `Env` root, and absolute or `..`-escaping values are rejected. **Why:** the registry entry is the first layer holding both the recipe's relative value and the caller's told root;
  rejecting absolute values enforces the doc's portability rule mechanically instead of by author discipline.
- **Q:** [review r1] What are `BurlerRound`'s `Config` keys, and is `burlercli`'s existing profile shape reused? **A:** [auto-pick] Six `profile` keys mirroring `burlercli`'s kebab-case names, plus `model`, `effort`, `timeout_s`, `run_subdir`;
  key names reused, decoder not. **Why:** `NewBurlerProducer`'s doc (`burler.go:62-64`) says five `Profile` fields and `RunOpts.Round` are overwritten per round, so they are not recipe-authorable;
  `burlercli`'s decoder takes YAML bytes while `shedrecipe` gets a decoded map, so there is nothing shareable without an out-of-scope refactor.
- **Q:** [review r1] Is `newWebsterProducer` exported too? **A:** [auto-pick] Yes — six renames, not five. **Why:** the wrapper resolves `batcher.Active` on every `Call` and maps its failure to `Stuck` (`webster.go:48-70`);
  calling `shedadapters.NewWebsterProducer` directly would drop both and need a `Batcher` the registry cannot resolve.
- **Q:** [review r1] `landingshed.Deps` has no `Name` field — what happens to a recipe row's `Name` for `Publish`/`Finalize`? **A:** [auto-pick] It is discarded;
  the row must be named to match the package constant, and the coverage guard asserts it. **Why:** both producers' identity is a package const (`publish.go:31`) carried by log lines and the stuck-reason filename, and both rows are shared by reference with `Hardener`'s list, so their names are not loom's to reassign.
- **Q:** [review r2] What is `Bouncer`'s complete recognised `Config` key set? **A:** [auto-pick] Seven keys — `run_subdir`, `artifact_paths`, `report_name`, `rubric_stencil`, `model`, `effort`, `version` — with a full field-by-field mapping onto `BouncerConfig`. **Superseded by review r3:** `report_name` was dropped, leaving six. **Why:** the strict unknown-key rule makes the key set the entry's contract, and `Model`/`Effort`/`Version` must be per-row rather than in `Env` because two review segments legitimately run at different model tiers.
- **Q:** [review r2] Which geometry-derived tokens does `SingleLLM` supply to `stencil.Fill`? **A:** [auto-pick] A closed reserved set of four — `worktree_root`, `anchor_path`, `stencils_dir`, `output_files` — with a `Config.tokens` collision rejected at construction. **Why:** `stencil.Fill` hard-errors on a missing token, so an unnamed set left the testing item asserting against nothing;
  rejecting collisions avoids a recipe that reads one way and behaves another.
- **Q:** [review r2] Do `BurlerRound`'s `profile.target.paths` / `profile.fasit.paths` follow the join-and-reject-absolute rule? **A:** [auto-pick] No — they are the single documented exception, passed through relative and unjoined. **Why:** `burlerengine.Profile.validate` already resolves them against its own told `worktreeRoot` and stats them for existence (`profile.go:66-87`), so joining first would double-resolve.
- **Q:** [review r2] The Told-Geometry bullet said "no path is computed inside `shedrecipe`", but entries join roots — which is it? **A:** [auto-pick] Reword to the property actually held: every root is told and none derived, joins onto told roots permitted;
  that wording is pinned as the invariant text. **Why:** the flat claim would ship false in `CONSTRAINTS.md`, since `run_subdir`, `artifact_paths`, and `output_files` are all joined.
- **Q:** [review r2] What exactly does the coverage guard compare? **A:** [auto-pick] Its table's key set against the row names in `loomshed.New`'s assembled list, failing on either direction of mismatch. **Why:** a table-only test would pass forever no matter what `loomshed` grew — precisely the failure the guard is supposed to catch.
- **Q:** [review r3] `report_name` is free-form, but does more than one value actually work? **A:** [auto-pick] No — pin `ReportName` to `round-%d-review.md` and drop the config key entirely. **Why:** `BurlerRound` writes that filename hardcoded (`burler.go:106-108`) and `ResolveRound` stats it (`round.go:24-46`), so any other value resolves the round to 0 forever and the Bouncer re-seeds until its budget is spent, silently.
- **Q:** [review r3] Does an entry validate the `Env` fields it reads? **A:** [auto-pick] Yes — non-empty and absolute for roots, non-nil for seams, checked at construction, only for fields that entry actually consumes;
  a nil `Env.Now` defaults. **Why:** `NewSingleLLMProducer` validates nothing (`singlellm.go:49-54`), so without this an under-filled `Env` yields a producer failing at every call — the same late failure the `Config` rules exist to prevent.
- **Q:** [review r3] What about `shuttleengine.Spec`'s `Timeout`, `ForkSubagents`, `KeepPane`, `Round`, `Parent`, `Display`? **A:** [auto-pick] Deliberately not recipe-authorable in this task;
  zero values defer to shuttle config. **Why:** recorded so piece 2 does not re-litigate it — and `Round`/`Parent`/`Display` are per-run strand values, wrong for a recipe at any point.
- **Q:** [review r3] What are `profile.target` / `profile.fasit`'s own recognised keys? **A:** [auto-pick] Exactly `paths` and `instructions`, mirroring `fileSetYAML` (`run.go:23-26`), with the strict unknown-key rule applying at the nested level too. **Why:** the rejector's recognised set has to be enumerable at every level it runs on.
- **Q:** [review r4] Does `SingleLLM` read `Env.AnchorPath` unconditionally, or only when its stencil names the marker? **A:** [auto-pick] Unconditionally — all four reserved tokens are always supplied, and `SingleLLM` validates all three roots it reads. **Why:** `stencil.Fill` errors only on a marker the template references but the map lacks (`stencil.go:29,39-46`), never on an unused extra value;
  making validation depend on template text would let a stencil edit change a recipe row's requirements.
- **Q:** [review r4] What decoded Go types do the `Config` accessors accept for nested maps and numbers? **A:** [auto-pick] Nested maps are always `map[string]any`;
  numbers accept `int` and integral `float64`, rejecting a fractional one;
  add `configStringMap` and `configMap` to the accessor set. **Why:** a YAML decoder into `any` yields `int` and a JSON-shaped one `float64`, and piece 2 picks the format — accepting both without truncating keeps the registry format-agnostic.
- **Q:** [review r4] What is the registry's exported lookup surface? **A:** [auto-pick] `Lookup(name) (Constructor, error)` and `Names() []string` (sorted), over an unexported map. **Why:** the coverage guard and piece 2 both need a stable enumeration, and neither should reach into the map directly.
- **Q:** [review r5] Who creates the joined `RunRoot/<run_subdir>` directory? **A:** [auto-pick] The `Bouncer` and `BurlerRound` entries, with `os.MkdirAll` at construction. **Why:** `Bouncer.Call` reaches `ResolveRound` first (`bouncer.go:154`), which hard-errors on a missing dir (`round.go:24-31`), and only `BurlerProducer.Call` creates it (`burler.go:232`) — but the Bouncer is the segment's entry point, so every fresh segment would fail on its first call;
  the caller cannot pre-create it because only the entry knows the joined path.
- **Q:** [review r5] Which `Config` keys are required and which optional? **A:** [auto-pick] Pinned per entry in a table, with empty-string treated as absent. **Why:** an absent `run_subdir` would silently resolve `RunDir` to bare `Env.RunRoot`, reinstating the cross-segment overwrite that key exists to prevent — unknown-key strictness says nothing about missing keys.
- **Q:** [review r5] Does `SingleLLM` probe its stencil at construction, like `NewBouncer` probes its rubric? **A:** [auto-pick] Yes — probe at construction, and still re-read inside the closure. **Why:** otherwise a mistyped `stencil` constructs cleanly and fails at first `Call`;
  re-reading keeps a mid-run stencil edit effective, so the probe buys fail-fast without caching.
- **Q:** [review r5] Is `stencil.Fill`'s error condition "absent" or "absent or empty"? **A:** [auto-pick] Absent **or** empty/whitespace-only, and an empty `Config.tokens` value is therefore rejected at construction. **Why:** `unfilledTopLevelMarkers` trims before testing (`stencil.go:169-181`), so an empty token value fails exactly like a missing one — a token with no natural value needs an explicit placeholder, as `Bouncer`'s `"(none)"` does.
- **Q:** [review r6] `Env.WebsterDeps` is a value struct, so neither the path-root nor the nil-seam rule reaches it — what does the `Webster` entry check? **A:** [auto-pick] Its four required inner seams (`Starter`, `Reed`, `Engine`, `RefMatcher`) non-nil, and explicitly none of `Batcher`, `Clock`, `OpenBisector`, or the value/map fields. **Why:** a zero `RunDeps` would otherwise construct cleanly and fail at every `Call`;
  the three unchecked nil-ables each have a documented nil meaning — `Batcher` is overwritten per `Call` by the wrapper, `Clock` nil selects the production clock, `OpenBisector` nil means "no fabric in this mode".
- **Q:** How does `loomshed` expose its six unexported constructors? **A:** [auto-pick] Export them in place, returning `shedengine.ShedProducer` rather than the unexported concrete type. **Why:** duplicating them in `shedrecipe` guarantees divergence, and moving the loom-specific producers out contradicts the design doc's point that a bespoke single-consumer engine is a valid registry entry precisely because it stays where it lives.
